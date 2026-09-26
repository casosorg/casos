package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/beego/beego/logs"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/deploy"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/server"
)

// Deploying from a Git repository.
//
// A build is a Job in the app's namespace. It clones the repository, writes a
// Dockerfile when the project has none and casos recognises its stack, and has
// rootless BuildKit build the image and push it to a registry casos runs in the
// cluster. A watcher then points the app at the new image. Nodes pull that
// image from 127.0.0.1 on the registry's NodePort: containerd talks plain HTTP
// to a loopback registry without being told to, and kube-proxy forwards the
// port to wherever the registry runs, so no node needs configuring.

const (
	buildNamespace        = "casos-system"
	buildRegistryName     = "casos-registry"
	buildRegistryImage    = "registry:2.8.3"
	buildRegistryPort     = 5000
	buildRegistryNodePort = 30500
	buildRegistryDisk     = "20Gi"
	buildKitImage         = "moby/buildkit:v0.23.2-rootless"

	gitBuildLabel           = "casos.io/git-build"
	gitRepoAnnotation       = "casos.io/git-repo"
	gitBranchAnnotation     = "casos.io/git-branch"
	gitPathAnnotation       = "casos.io/git-path"
	gitPortAnnotation       = "casos.io/git-port"
	gitCommitAnnotation     = "casos.io/git-commit"
	gitDomainAnnotation     = "casos.io/git-domain"
	gitEnvAnnotation        = "casos.io/git-env"
	gitFirstBuildAnnotation = "casos.io/git-first-build"
	gitBuildStateAnnotation = "casos.io/git-build-state"
	gitBuildNoteAnnotation  = "casos.io/git-build-note"

	gitBuildContainer     = "build"
	gitBuildTTLSeconds    = int32(7 * 24 * 60 * 60)
	gitBuildWatchInterval = 5 * time.Second
	gitBuildTimeFormat    = "2006-01-02 15:04:05"

	gitTokenKey = "token"

	gitBuildDeployed     = "deployed"
	gitBuildDeployFailed = "deploy-failed"
	gitBuildFailed       = "failed"
)

// The build container's script. Clone and build failures are written to the
// termination message as well as the log, so the list can say why in a line.
const gitBuildScript = `set -u
fail() { printf 'error=%s\n' "$1" > /dev/termination-log; echo "error: $1" >&2; exit 1; }
mkdir -p "$HOME/.config/buildkit"
printf '%s' "$BUILDKITD_TOML" > "$HOME/.config/buildkit/buildkitd.toml"

if [ -n "${GIT_TOKEN:-}" ]; then
  export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.helper \
    GIT_CONFIG_VALUE_0='!f() { test "$1" = get && printf "username=oauth2\npassword=%s\n" "$GIT_TOKEN"; }; f'
fi
echo "==> Cloning $REPO${BRANCH:+ at $BRANCH}"
set --
if [ -n "$BRANCH" ]; then set -- --branch "$BRANCH"; fi
# A full clone is the fallback, for a server that cannot serve a shallow one
# and for a flaky connection.
if ! git clone --depth 1 "$@" -- "$REPO" /workspace/src; then
  rm -rf /workspace/src
  echo "==> Retrying with a full clone"
  git clone "$@" -- "$REPO" /workspace/src || fail "could not clone $REPO"
fi
commit=$(git -C /workspace/src rev-parse --short=12 HEAD)
dir="/workspace/src${SUBDIR:+/$SUBDIR}"
[ -d "$dir" ] || fail "the repository has no folder $SUBDIR"
cd "$dir"

file=Dockerfile
port=""
if [ -f Dockerfile ]; then
  kind=dockerfile
  port=$(awk 'toupper($1) == "EXPOSE" { split($2, p, "/"); print p[1]; exit }' Dockerfile)
elif [ -f package.json ]; then
  kind=node; port=3000; file=Dockerfile.casos
  cat > "$file" <<'EOF'
FROM node:22-slim
WORKDIR /app
COPY . .
RUN if [ -f pnpm-lock.yaml ]; then corepack enable && pnpm install --frozen-lockfile; \
    elif [ -f yarn.lock ]; then corepack enable && yarn install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then npm ci; \
    else npm install; fi
RUN npm run build --if-present
ENV NODE_ENV=production PORT=3000 HOST=0.0.0.0
EXPOSE 3000
CMD ["npm", "start"]
EOF
elif [ -f requirements.txt ] || [ -f pyproject.toml ]; then
  kind=python; port=8000; file=Dockerfile.casos
  if [ -f Procfile ] && grep -q '^web:' Procfile; then start=$(sed -n 's/^web:[[:space:]]*//p' Procfile | head -n 1)
  elif [ -f manage.py ]; then start='python manage.py runserver 0.0.0.0:$PORT'
  elif [ -f main.py ]; then start='python main.py'
  elif [ -f app.py ]; then start='python app.py'
  else fail "casos cannot tell how to start this Python project: add a Procfile with a web: line, or a Dockerfile"
  fi
  printf '%s\n' "$start" > .casos-start
  cat > "$file" <<'EOF'
FROM python:3.12-slim
WORKDIR /app
COPY . .
RUN if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; else pip install --no-cache-dir .; fi
ENV PORT=8000 PYTHONUNBUFFERED=1
EXPOSE 8000
CMD ["sh", "/app/.casos-start"]
EOF
elif [ -f go.mod ]; then
  kind=go; port=8080; file=Dockerfile.casos
  cat > "$file" <<'EOF'
FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/app .
FROM alpine:3.21
COPY --from=build /out/app /app
ENV PORT=8080
EXPOSE 8080
CMD ["/app"]
EOF
elif [ -f index.html ]; then
  kind=static; port=80; file=Dockerfile.casos
  cat > "$file" <<'EOF'
FROM nginx:1.27-alpine
COPY . /usr/share/nginx/html
EXPOSE 80
EOF
else
  fail "there is no Dockerfile, and casos does not recognise the project: add a Dockerfile"
fi

echo "==> Building a $kind project at commit $commit"
buildctl-daemonless.sh build --progress plain \
  --frontend dockerfile.v0 --local context=. --local dockerfile=. --opt filename="$file" \
  --output "type=image,name=$IMAGE:$commit,push=true,registry.insecure=true" \
  --export-cache "type=registry,ref=$IMAGE:buildcache,mode=max,registry.insecure=true" \
  --import-cache "type=registry,ref=$IMAGE:buildcache,registry.insecure=true" \
  || fail "the build failed; the log says why"
printf 'commit=%s\nkind=%s\nport=%s\n' "$commit" "$kind" "$port" > /dev/termination-log
echo "==> Pushed $IMAGE:$commit"
`

func buildRegistryHost() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", buildRegistryName, buildNamespace, buildRegistryPort)
}

func gitAppImage(namespace, app, commit string) string {
	return fmt.Sprintf("127.0.0.1:%d/%s/%s:%s", buildRegistryNodePort, namespace, app, commit)
}

// BuildKit keeps to the mirrors the nodes use, and falls back to the
// canonical registry when a mirror fails.
func buildkitdConfig() string {
	var b strings.Builder
	cfg := getServerConfig()
	if cfg == nil || cfg.RegistryMirrorMode != server.RegistryMirrorModeNever {
		fmt.Fprintf(&b, "[registry.\"docker.io\"]\n  mirrors = [%q]\n", deploy.DockerHubMirror)
		fmt.Fprintf(&b, "[registry.\"registry.k8s.io\"]\n  mirrors = [%q]\n", deploy.K8sRegistryMirror)
		fmt.Fprintf(&b, "[registry.\"ghcr.io\"]\n  mirrors = [%q]\n", deploy.GhcrMirror)
	}
	fmt.Fprintf(&b, "[registry.%q]\n  http = true\n  insecure = true\n", buildRegistryHost())
	return b.String()
}

// ensureBuildRegistry installs the registry builds push to, the first time a
// build needs it.
func ensureBuildRegistry(ctx context.Context, cfg *rest.Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	ignoreExists := func(err error) error {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	labels := map[string]string{"app": buildRegistryName, "app.kubernetes.io/managed-by": "casos"}

	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: buildNamespace}}, metav1.CreateOptions{})
	if err := ignoreExists(err); err != nil {
		return err
	}
	_, err = client.CoreV1().PersistentVolumeClaims(buildNamespace).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: buildRegistryName, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(buildRegistryDisk)},
			},
		},
	}, metav1.CreateOptions{})
	if err := ignoreExists(err); err != nil {
		return err
	}

	replicas := int32(1)
	_, err = client.AppsV1().Deployments(buildNamespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: buildRegistryName, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": buildRegistryName}},
			// The disk attaches to one pod at a time.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "registry",
						Image: buildRegistryImage,
						Env:   []corev1.EnvVar{{Name: "REGISTRY_STORAGE_DELETE_ENABLED", Value: "true"}},
						Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: buildRegistryPort}},
						ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{Path: "/v2/", Port: intstr.FromInt32(buildRegistryPort)},
						}},
						VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/var/lib/registry"}},
					}},
					Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: buildRegistryName},
					}}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err := ignoreExists(err); err != nil {
		return err
	}

	// Looked up first: a create that repeats a taken node port is refused as
	// invalid before the apiserver notices the Service already exists.
	if _, err := client.CoreV1().Services(buildNamespace).Get(ctx, buildRegistryName, metav1.GetOptions{}); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}
	_, err = client.CoreV1().Services(buildNamespace).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: buildRegistryName, Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"app": buildRegistryName},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       buildRegistryPort,
				TargetPort: intstr.FromInt32(buildRegistryPort),
				// Fixed, because every built image is named after it.
				NodePort: buildRegistryNodePort,
			}},
		},
	}, metav1.CreateOptions{})
	if err := ignoreExists(err); err != nil {
		return fmt.Errorf("the image registry needs node port %d: %w", buildRegistryNodePort, err)
	}
	return nil
}

type gitSource struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

func (s *gitSource) normalize() error {
	s.Repo = strings.TrimSpace(s.Repo)
	s.Branch = strings.TrimSpace(s.Branch)
	s.Path = strings.Trim(strings.TrimSpace(s.Path), "/")
	if s.Repo == "" {
		return fmt.Errorf("a repository is required")
	}
	if _, err := devboxRepoFolder(s.Repo); err != nil {
		return err
	}
	for _, part := range strings.Split(s.Path, "/") {
		if part == ".." {
			return fmt.Errorf("the folder must stay inside the repository")
		}
	}
	return nil
}

func gitSourceOf(meta metav1.ObjectMeta) (gitSource, bool) {
	source := gitSource{
		Repo:   meta.Annotations[gitRepoAnnotation],
		Branch: meta.Annotations[gitBranchAnnotation],
		Path:   meta.Annotations[gitPathAnnotation],
	}
	return source, source.Repo != ""
}

type gitBuildRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	gitSource
	// Port overrides the one the build finds; 0 keeps what it finds.
	Port    int32           `json:"port"`
	EnvVars []envVarRequest `json:"envVars"`
	Token   string          `json:"token"`
	// The domain the app gets its address under, from the address casos is
	// reached by.
	domain string
}

type gitBuild struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	App        string `json:"app"`
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Commit     string `json:"commit"`
	Kind       string `json:"kind"`
	PodName    string `json:"podName"`
	CreatedAt  string `json:"createdAt"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

func activeGitBuild(cfg *rest.Config, namespace, app string) (string, error) {
	jobs, err := object.GetJobs(cfg, namespace)
	if err != nil {
		return "", err
	}
	for _, job := range jobs {
		if job.Labels[gitBuildLabel] == app && job.Annotations[gitBuildStateAnnotation] == "" {
			return job.Name, nil
		}
	}
	return "", nil
}

// startGitBuild builds a new app from a repository, or rebuilds one that was
// deployed from one.
func startGitBuild(ctx context.Context, cfg *rest.Config, req gitBuildRequest) (*gitBuild, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if problems := validation.IsDNS1123Label(req.Name); len(problems) > 0 {
		return nil, fmt.Errorf("the name %q is invalid: %s", req.Name, strings.Join(problems, "; "))
	}

	first := false
	existing, err := object.GetDeployment(cfg, req.Namespace, req.Name)
	switch {
	case errors.IsNotFound(err):
		first = true
		if err := req.gitSource.normalize(); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		source, ok := gitSourceOf(existing.ObjectMeta)
		if !ok {
			return nil, fmt.Errorf("%s already exists and was not deployed from a repository; pick another name", req.Name)
		}
		if req.Repo != "" && req.Repo != source.Repo {
			return nil, fmt.Errorf("%s is built from %s; pick another name to deploy a different repository", req.Name, source.Repo)
		}
		req.gitSource = source
	}
	if running, err := activeGitBuild(cfg, req.Namespace, req.Name); err != nil {
		return nil, err
	} else if running != "" {
		return nil, fmt.Errorf("%s is already building (%s); wait for it to finish", req.Name, running)
	}

	if err := mcpEnsureNamespace(cfg, req.Namespace); err != nil {
		return nil, err
	}
	if err := ensureBuildRegistry(ctx, cfg); err != nil {
		return nil, err
	}
	if token := strings.TrimSpace(req.Token); token != "" {
		if err := saveGitToken(cfg, req.Namespace, req.Name, token); err != nil {
			return nil, err
		}
	}

	annotations := map[string]string{
		gitRepoAnnotation:   req.Repo,
		gitBranchAnnotation: req.Branch,
		gitPathAnnotation:   req.Path,
	}
	if first {
		annotations[gitFirstBuildAnnotation] = "true"
		annotations[gitDomainAnnotation] = req.domain
		if req.Port > 0 {
			annotations[gitPortAnnotation] = strconv.Itoa(int(req.Port))
		}
		if len(req.EnvVars) > 0 {
			encoded, _ := json.Marshal(req.EnvVars)
			annotations[gitEnvAnnotation] = string(encoded)
		}
	}

	backoff := int32(0)
	ttl := gitBuildTTLSeconds
	optional := true
	tokenEnv := secretEnv("GIT_TOKEN", gitTokenSecretName(req.Name), gitTokenKey)
	tokenEnv.ValueFrom.SecretKeyRef.Optional = &optional
	uid := int64(1000)
	podLabels := map[string]string{gitBuildLabel: req.Name}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: req.Name + "-build-",
			Namespace:    req.Namespace,
			Labels:       podLabels,
			Annotations:  annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    gitBuildContainer,
						Image:   buildKitImage,
						Command: []string{"/bin/sh", "-c", gitBuildScript},
						Env: []corev1.EnvVar{
							{Name: "REPO", Value: req.Repo},
							{Name: "BRANCH", Value: req.Branch},
							{Name: "SUBDIR", Value: req.Path},
							{Name: "IMAGE", Value: fmt.Sprintf("%s/%s/%s", buildRegistryHost(), req.Namespace, req.Name)},
							{Name: "BUILDKITD_TOML", Value: buildkitdConfig()},
							{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
							{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
							tokenEnv,
						},
						// Rootless BuildKit creates user namespaces, which the
						// default seccomp and AppArmor profiles forbid.
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:       &uid,
							RunAsGroup:      &uid,
							SeccompProfile:  &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
							AppArmorProfile: &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeUnconfined},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "workspace", MountPath: "/workspace"},
							{Name: "buildkit", MountPath: "/home/user/.local/share/buildkit"},
						},
					}},
					Volumes: []corev1.Volume{emptyDirVolume("workspace"), emptyDirVolume("buildkit")},
				},
			},
		},
	}
	created, err := object.AddJob(cfg, job)
	if err != nil {
		return nil, err
	}
	build := gitBuildOf(*created, nil)
	return &build, nil
}

func gitTokenSecretName(app string) string { return app + "-git" }

// saveGitToken keeps the token beside the app, so rebuilds reuse it and
// deleting the app removes it.
func saveGitToken(cfg *rest.Config, namespace, app, token string) error {
	name := gitTokenSecretName(app)
	data := map[string][]byte{gitTokenKey: []byte(token)}
	existing, err := object.GetSecret(cfg, namespace, name)
	switch {
	case errors.IsNotFound(err):
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: appOwnershipLabels(app, "")},
			Data:       data,
		}
		_, err = object.AddSecret(cfg, secret)
		return err
	case err != nil:
		return err
	case !ownedByApp(existing.ObjectMeta, app):
		return fmt.Errorf("the secret %s already exists and does not belong to %s; pick another name", name, app)
	}
	existing.Data = data
	_, err = object.UpdateSecret(cfg, existing)
	return err
}

func forgetGitToken(cfg *rest.Config, namespace, app string) {
	name := gitTokenSecretName(app)
	if secret, err := object.GetSecret(cfg, namespace, name); err == nil && ownedByApp(secret.ObjectMeta, app) {
		_ = object.DeleteSecret(cfg, namespace, name)
	}
}

func gitBuildOf(job batchv1.Job, pods []corev1.Pod) gitBuild {
	build := gitBuild{
		Name:      job.Name,
		Namespace: job.Namespace,
		App:       job.Labels[gitBuildLabel],
		Repo:      job.Annotations[gitRepoAnnotation],
		Branch:    job.Annotations[gitBranchAnnotation],
		Path:      job.Annotations[gitPathAnnotation],
		Status:    "queued",
		Message:   job.Annotations[gitBuildNoteAnnotation],
		CreatedAt: job.CreationTimestamp.UTC().Format(gitBuildTimeFormat),
	}
	var pod *corev1.Pod
	for i := range pods {
		if pods[i].Namespace == job.Namespace && pods[i].Labels["job-name"] == job.Name {
			pod = &pods[i]
		}
	}
	var state corev1.ContainerState
	if pod != nil {
		build.PodName = pod.Name
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == gitBuildContainer {
				state = status.State
			}
		}
	}
	if state.Running != nil {
		build.Status = "building"
		build.StartedAt = state.Running.StartedAt.UTC().Format(gitBuildTimeFormat)
	}
	if terminated := state.Terminated; terminated != nil {
		build.StartedAt = terminated.StartedAt.UTC().Format(gitBuildTimeFormat)
		build.FinishedAt = terminated.FinishedAt.UTC().Format(gitBuildTimeFormat)
		result := parseBuildResult(terminated.Message)
		build.Commit = result["commit"]
		build.Kind = result["kind"]
		if build.Message == "" {
			build.Message = result["error"]
		}
	}

	switch job.Annotations[gitBuildStateAnnotation] {
	case gitBuildDeployed:
		build.Status = "deployed"
	case gitBuildFailed, gitBuildDeployFailed:
		build.Status = "failed"
	default:
		if condition := devboxRunFinished(job); condition != nil {
			if condition.Type == batchv1.JobComplete {
				build.Status = "deploying"
			} else {
				build.Status = "failed"
				if build.Message == "" {
					build.Message = condition.Message
				}
			}
		} else if pod != nil && state.Waiting != nil && state.Waiting.Reason != "ContainerCreating" {
			build.Message = strings.TrimSpace(state.Waiting.Reason + " " + state.Waiting.Message)
		}
	}
	return build
}

func parseBuildResult(message string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(message, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			result[key] = strings.TrimSpace(value)
		}
	}
	return result
}

func listGitBuilds(cfg *rest.Config, namespace, app string) ([]gitBuild, error) {
	jobs, err := object.GetJobs(cfg, namespace)
	if err != nil {
		return nil, err
	}
	pods, _ := object.GetPods(cfg, namespace)
	builds := []gitBuild{}
	for _, job := range jobs {
		if owner := job.Labels[gitBuildLabel]; owner != "" && (app == "" || owner == app) {
			builds = append(builds, gitBuildOf(job, pods))
		}
	}
	sort.Slice(builds, func(i, j int) bool { return builds[i].CreatedAt > builds[j].CreatedAt })
	return builds, nil
}

// StartGitBuildWatcher deploys each build once it has pushed its image, until
// ctx ends. State lives on the Job, so a build that finished while casos was
// down is deployed when it comes back.
func StartGitBuildWatcher(ctx context.Context) {
	ticker := time.NewTicker(gitBuildWatchInterval)
	defer ticker.Stop()
	for {
		deployFinishedGitBuilds()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func deployFinishedGitBuilds() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		return
	}
	jobs, err := object.GetJobs(cfg, "")
	if err != nil {
		return
	}
	for _, job := range jobs {
		if job.Labels[gitBuildLabel] == "" || job.Annotations[gitBuildStateAnnotation] != "" {
			continue
		}
		condition := devboxRunFinished(job)
		if condition == nil {
			continue
		}
		if condition.Type != batchv1.JobComplete {
			// No app exists yet to delete the token along with.
			if job.Annotations[gitFirstBuildAnnotation] == "true" {
				forgetGitToken(cfg, job.Namespace, job.Labels[gitBuildLabel])
			}
			markGitBuild(cfg, job, gitBuildFailed, "")
			continue
		}
		if err := deployGitBuild(cfg, job); err != nil {
			logs.Warning("git build %s/%s: deploy: %v", job.Namespace, job.Name, err)
			markGitBuild(cfg, job, gitBuildDeployFailed, "the image was built, but deploying it failed: "+err.Error())
			continue
		}
		markGitBuild(cfg, job, gitBuildDeployed, "")
	}
}

func markGitBuild(cfg *rest.Config, job batchv1.Job, state, note string) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return
	}
	annotations := map[string]string{gitBuildStateAnnotation: state}
	if note != "" {
		annotations[gitBuildNoteAnnotation] = note
	}
	patch, _ := json.Marshal(map[string]interface{}{"metadata": map[string]interface{}{"annotations": annotations}})
	if _, err := client.BatchV1().Jobs(job.Namespace).Patch(context.Background(), job.Name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		logs.Warning("git build %s/%s: record %s: %v", job.Namespace, job.Name, state, err)
	}
}

func buildResultOf(cfg *rest.Config, job batchv1.Job) (map[string]string, error) {
	pods, err := object.GetPods(cfg, job.Namespace)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		if pod.Labels["job-name"] != job.Name {
			continue
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == gitBuildContainer && status.State.Terminated != nil {
				return parseBuildResult(status.State.Terminated.Message), nil
			}
		}
	}
	return nil, fmt.Errorf("the build's pod is gone, so its result cannot be read")
}

func deployGitBuild(cfg *rest.Config, job batchv1.Job) error {
	app := job.Labels[gitBuildLabel]
	result, err := buildResultOf(cfg, job)
	if err != nil {
		return err
	}
	commit := result["commit"]
	if commit == "" {
		return fmt.Errorf("the build did not report which commit it built")
	}
	image := gitAppImage(job.Namespace, app, commit)

	existing, err := object.GetDeployment(cfg, job.Namespace, app)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err == nil {
		if len(existing.Spec.Template.Spec.Containers) == 0 {
			return fmt.Errorf("%s has no container to update", app)
		}
		existing.Spec.Template.Spec.Containers[0].Image = image
		applyAnnotation(&existing.ObjectMeta, appImageAnnotation, image)
		applyAnnotation(&existing.ObjectMeta, gitCommitAnnotation, commit)
		_, err := object.UpdateDeployment(cfg, existing)
		return err
	}
	if job.Annotations[gitFirstBuildAnnotation] != "true" {
		return fmt.Errorf("%s was deleted while it was building", app)
	}

	port := int32(0)
	for _, candidate := range []string{job.Annotations[gitPortAnnotation], result["port"]} {
		if value, err := strconv.Atoi(candidate); err == nil && value > 0 && value < 65536 {
			port = int32(value)
			break
		}
	}
	req := deployAppRequest{Namespace: job.Namespace, Name: app, Image: image}
	if env := job.Annotations[gitEnvAnnotation]; env != "" {
		_ = json.Unmarshal([]byte(env), &req.EnvVars)
	}
	if port > 0 {
		req.Ports = []appPortRequest{{Name: "http", ContainerPort: port, Protocol: "TCP"}}
		if domain := job.Annotations[gitDomainAnnotation]; domain != "" {
			req.Domains = &[]appDomain{{Host: app + "." + domain, Port: port}}
		}
	}
	source := map[string]string{
		gitRepoAnnotation:   job.Annotations[gitRepoAnnotation],
		gitBranchAnnotation: job.Annotations[gitBranchAnnotation],
		gitPathAnnotation:   job.Annotations[gitPathAnnotation],
		gitCommitAnnotation: commit,
	}
	_, err = deployAppWorkload(cfg, req, workloadOptions{mutate: func(depl *appsv1.Deployment) error {
		for key, value := range source {
			applyAnnotation(&depl.ObjectMeta, key, value)
		}
		return nil
	}})
	return err
}

// DeployGitApp builds an app from a Git repository and deploys it.
// @router /api/deploy-git-app [post]
func (c *ApiController) DeployGitApp() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req gitBuildRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	req.domain = defaultCloudDomain(c.Ctx.Request.Host)
	build, err := startGitBuild(c.Ctx.Request.Context(), cfg, req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(build)
}

// GetGitBuilds lists an app's builds, newest first.
// @router /api/get-git-builds [get]
func (c *ApiController) GetGitBuilds() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	builds, err := listGitBuilds(cfg, c.GetString("namespace"), c.GetString("name"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(builds)
}
