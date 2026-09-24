package controllers

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// A DevBox's environment is three things layered on the editor: an image to
// work in, a repository to work on, and a script that turns the two into a
// working setup. Each is optional, and each is done by an init container, so
// the pod's own status says which step it is on and its log says what went
// wrong.
//
// The image is not required to contain code-server. The editor is copied out of
// the code-server image into a scratch volume and run from there, so any
// glibc-based image — python, pytorch, a lab's own — becomes a workspace as it
// is, and a run frozen from it trains in that same image.

const (
	devboxRepoAnnotation   = "casos.io/devbox-repo"
	devboxFolderAnnotation = "casos.io/devbox-folder"

	devboxGitImage     = "alpine/git:latest"
	devboxEditorVolume = "casos-editor"
	devboxEditorMount  = "/opt/casos"
	devboxHomeVolume   = "home"

	devboxEditorInit = "editor"
	devboxCloneInit  = "clone"
	devboxSetupInit  = "setup"

	// Written in the home disk once the setup script has succeeded, so a restart
	// does not reinstall everything; deleting it runs the script again.
	devboxSetupMarker = devboxHomeMount + "/.casos/setup-done"
)

// The clone never overwrites: a folder that already holds a checkout is where
// the user's uncommitted work lives. It reports the branch and commit it left
// the folder on, or why it could not, through the termination message, and
// always exits 0 — a bad URL should leave an editor to fix it in, not a pod
// stuck in Init:CrashLoopBackOff.
const devboxCloneScript = `dir="$CASOS_FOLDER"
mkdir -p "$dir"
if [ ! -d "$dir/.git" ]; then
  if [ -n "$(ls -A "$dir")" ]; then
    printf 'error=%s is not empty and is not a git checkout, so nothing was cloned into it\n' "$dir" > /dev/termination-log
    exit 0
  fi
  set --
  if [ -n "$CASOS_BRANCH" ]; then set -- --branch "$CASOS_BRANCH"; fi
  git clone --progress "$@" -- "$CASOS_REPO" "$dir" 2>/tmp/clone.log
  status=$?
  cat /tmp/clone.log >&2
  if [ $status -ne 0 ]; then
    printf 'error=%s\n' "$(grep -v '^Cloning into' /tmp/clone.log | tail -n 1)" > /dev/termination-log
    exit 0
  fi
fi
printf 'branch=%s\ncommit=%s\n' "$(git -C "$dir" rev-parse --abbrev-ref HEAD)" "$(git -C "$dir" rev-parse --short HEAD)" > /dev/termination-log
`

const devboxSetupScript = `if [ -f "$CASOS_SETUP_MARKER" ]; then
  echo "Already set up; delete $CASOS_SETUP_MARKER to run the setup script again."
  exit 0
fi
mkdir -p "$(dirname "$CASOS_SETUP_MARKER")"
cd "$CASOS_FOLDER" 2>/dev/null || cd "$HOME"
if sh -ec "$CASOS_SETUP"; then
  touch "$CASOS_SETUP_MARKER"
else
  echo "failed" > /dev/termination-log
fi
`

const devboxEditorLaunch = `export PATH="$HOME/.local/bin:$PATH"
exec ` + devboxEditorMount + `/code-server/bin/code-server --bind-addr 0.0.0.0:8080 --auth password --disable-telemetry "$CASOS_FOLDER"
`

type devboxEnvironment struct {
	image  string
	repo   string
	branch string
	setup  string
	folder string
}

var devboxFolderPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// devboxRepoFolder checks a repository address and names the folder it is
// cloned into. Credentials in the URL are refused rather than stored: they
// would sit in the Deployment for anyone who can read it, and in the checkout's
// remote for anyone who can read the disk.
func devboxRepoFolder(repo string) (string, error) {
	u, err := url.Parse(repo)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("the repository must be an http(s) or git:// address, such as https://github.com/owner/project.git")
	}
	switch u.Scheme {
	case "https", "http", "git":
	default:
		return "", fmt.Errorf("the repository must be an http(s) or git:// address, such as https://github.com/owner/project.git")
	}
	if u.User != nil {
		return "", fmt.Errorf("leave credentials out of the repository address — private repositories are not supported yet")
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(u.Path, "/")), ".git")
	if !devboxFolderPattern.MatchString(name) || name == "." || name == ".." {
		return "", fmt.Errorf("cannot tell a folder name from the repository address %q", repo)
	}
	return name, nil
}

func devboxRunsAsUser() *corev1.SecurityContext {
	uid := int64(1000)
	return &corev1.SecurityContext{RunAsUser: &uid, RunAsGroup: &uid}
}

// applyDevboxEnvironment lays the environment onto a built workspace: the home
// disk every step shares, then the steps themselves in the order they depend on
// each other — editor, repository, setup.
func applyDevboxEnvironment(env devboxEnvironment) func(*appsv1.Deployment) error {
	return func(depl *appsv1.Deployment) error {
		spec := &depl.Spec.Template.Spec
		if len(spec.Containers) == 0 {
			return fmt.Errorf("the workspace has no container to set up")
		}
		editor := &spec.Containers[0]

		home, ok := devboxHomeMountOf(*editor)
		if !ok {
			// A stateless box still needs one home that the clone, the setup
			// script, the editor and SSH all see; it just ends with the pod.
			spec.Volumes = append(spec.Volumes, corev1.Volume{
				Name:         devboxHomeVolume,
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})
			home = corev1.VolumeMount{Name: devboxHomeVolume, MountPath: devboxHomeMount}
			editor.VolumeMounts = append(editor.VolumeMounts, home)
		}

		folderEnv := corev1.EnvVar{Name: "CASOS_FOLDER", Value: env.folder}
		homeEnv := corev1.EnvVar{Name: "HOME", Value: devboxHomeMount}
		editor.Env = append(editor.Env, folderEnv)
		editor.SecurityContext = devboxRunsAsUser()

		if env.image == devboxDefaultImage {
			// The image's own entrypoint opens ".", so the folder is where it starts.
			editor.WorkingDir = env.folder
		} else {
			spec.Volumes = append(spec.Volumes, corev1.Volume{
				Name:         devboxEditorVolume,
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})
			editorMount := corev1.VolumeMount{Name: devboxEditorVolume, MountPath: devboxEditorMount}
			spec.InitContainers = append(spec.InitContainers, corev1.Container{
				Name:            devboxEditorInit,
				Image:           devboxDefaultImage,
				Command:         []string{"/bin/sh", "-c", "cp -a /usr/lib/code-server " + devboxEditorMount + "/"},
				VolumeMounts:    []corev1.VolumeMount{editorMount},
				SecurityContext: devboxRunsAsUser(),
			})
			editor.Command = []string{"/bin/sh", "-c", devboxEditorLaunch}
			editor.Args = nil
			editor.VolumeMounts = append(editor.VolumeMounts, editorMount)
			// Most images have no user 1000, so nothing else would say where home
			// is; conda is told to keep environments there too, since the one it
			// ships with belongs to root.
			editor.Env = append(editor.Env,
				homeEnv,
				corev1.EnvVar{Name: "CONDA_ENVS_PATH", Value: devboxHomeMount + "/.conda/envs"},
				corev1.EnvVar{Name: "CONDA_PKGS_DIRS", Value: devboxHomeMount + "/.conda/pkgs"},
			)
		}

		if env.repo != "" {
			spec.InitContainers = append(spec.InitContainers, corev1.Container{
				Name:    devboxCloneInit,
				Image:   devboxGitImage,
				Command: []string{"/bin/sh", "-c", devboxCloneScript},
				Env: []corev1.EnvVar{
					{Name: "CASOS_REPO", Value: env.repo},
					{Name: "CASOS_BRANCH", Value: env.branch},
					folderEnv,
					{Name: "HOME", Value: "/tmp"},
					// Without this, a repository that wants a password waits forever
					// on a prompt nobody can see.
					{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
				},
				VolumeMounts:    []corev1.VolumeMount{home},
				SecurityContext: devboxRunsAsUser(),
			})
		}

		if env.setup != "" {
			// The script runs in the workspace's own image, so what it installs
			// matches what the editor runs — but only what lands in the home disk
			// outlives it, which is why pip and conda are pointed there.
			setupEnv := append(withoutEnv(editor.Env, "PASSWORD", "HOME"),
				homeEnv,
				corev1.EnvVar{Name: "CASOS_SETUP", Value: env.setup},
				corev1.EnvVar{Name: "CASOS_SETUP_MARKER", Value: devboxSetupMarker},
			)
			spec.InitContainers = append(spec.InitContainers, corev1.Container{
				Name:            devboxSetupInit,
				Image:           env.image,
				Command:         []string{"/bin/sh", "-c", devboxSetupScript},
				Env:             setupEnv,
				VolumeMounts:    []corev1.VolumeMount{home},
				Resources:       editor.Resources,
				SecurityContext: devboxRunsAsUser(),
			})
		}

		applyAnnotation(&depl.ObjectMeta, devboxFolderAnnotation, env.folder)
		if env.repo != "" {
			applyAnnotation(&depl.ObjectMeta, devboxRepoAnnotation, env.repo)
		}
		return nil
	}
}

func devboxHomeMountOf(c corev1.Container) (corev1.VolumeMount, bool) {
	for _, mount := range c.VolumeMounts {
		if mount.MountPath == devboxHomeMount {
			return mount, true
		}
	}
	return corev1.VolumeMount{}, false
}

func withoutEnv(env []corev1.EnvVar, names ...string) []corev1.EnvVar {
	result := []corev1.EnvVar{}
	for _, item := range env {
		if !slices.Contains(names, item.Name) {
			result = append(result, item)
		}
	}
	return result
}

// devboxFolder is where a workspace opens and where its runs start: the
// checkout when it has one, home otherwise, and home for boxes made before
// either was recorded.
func devboxFolder(depl appsv1.Deployment) string {
	if folder := depl.Annotations[devboxFolderAnnotation]; folder != "" {
		return folder
	}
	return devboxHomeMount
}

type devboxPodState struct {
	podName     string
	prepareStep string
	branch      string
	commit      string
	cloneError  string
	setupFailed bool
}

// devboxPodStateOf reads the newest pod's init containers: which step is still
// going, and what the finished ones left in their termination messages.
func devboxPodStateOf(pods []corev1.Pod) devboxPodState {
	var newest *corev1.Pod
	for i := range pods {
		if pods[i].DeletionTimestamp != nil {
			continue
		}
		if newest == nil || pods[i].CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = &pods[i]
		}
	}
	state := devboxPodState{}
	if newest == nil {
		return state
	}
	state.podName = newest.Name
	for _, status := range newest.Status.InitContainerStatuses {
		if terminated := status.State.Terminated; terminated != nil {
			switch status.Name {
			case devboxCloneInit:
				for _, line := range strings.Split(terminated.Message, "\n") {
					key, value, _ := strings.Cut(line, "=")
					switch key {
					case "branch":
						// A tag or a bare commit leaves HEAD detached; the commit says it all.
						if value != "HEAD" {
							state.branch = value
						}
					case "commit":
						state.commit = value
					case "error":
						state.cloneError = value
					}
				}
			case devboxSetupInit:
				state.setupFailed = strings.TrimSpace(terminated.Message) == "failed"
			}
			continue
		}
		if state.prepareStep == "" {
			state.prepareStep = status.Name
		}
	}
	return state
}
