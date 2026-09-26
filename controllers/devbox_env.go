package controllers

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Init containers bring code-server into any glibc image, give uid 1000 a name,
// clone the repository and run the setup script. Everything runs as uid 1000,
// so what the setup installs lands on the home disk, the only thing kept.

const (
	devboxRepoAnnotation   = "casos.io/devbox-repo"
	devboxFolderAnnotation = "casos.io/devbox-folder"

	devboxUid         = int64(1000)
	devboxHomeVolume  = "home"
	devboxToolsVolume = "casos-tools"
	devboxToolsMount  = "/opt/casos"

	devboxEditorInit = "editor"
	devboxUserInit   = "user"
	devboxCloneInit  = "clone"
	devboxSetupInit  = "setup"
)

// One line without quotes, so it survives the launchpad edit form's command
// field. The image's own code-server wins, so a lab image keeps its version.
const devboxEditorScript = `export PATH=$HOME/.local/bin:$PATH; exec $(command -v code-server || echo /opt/casos/code-server/bin/code-server) --bind-addr 0.0.0.0:8080 --auth password --disable-telemetry $CASOS_FOLDER`

const devboxEditorCopyScript = `cp -a /usr/lib/code-server /opt/casos/`

// Most images have no user 1000, and sshd and many tools refuse a uid without a name.
const devboxUserScript = `set -e
e=/opt/casos/etc
mkdir -p "$e"
shell=/bin/sh; [ -x /bin/bash ] && shell=/bin/bash
{ grep -Ev '^coder:|^[^:]*:[^:]*:1000:' /etc/passwd || true; echo "coder:x:1000:1000:coder:/home/coder:$shell"; } > "$e/passwd"
{ grep -Ev '^coder:|^[^:]*:[^:]*:1000:' /etc/group || true; echo "coder:x:1000:"; } > "$e/group"
`

// Clone and setup exit 0 on failure: a bad URL or a broken requirements.txt
// should leave an editor to fix it in, not a pod stuck in Init. The outcome is
// in the termination message, which the list reads back.
const devboxCloneScript = `dir="$CASOS_FOLDER"
mkdir -p "$dir"
if [ ! -d "$dir/.git" ]; then
  if [ -n "$(ls -A "$dir")" ]; then
    printf 'error=%s is not empty and is not a git checkout, so nothing was cloned into it\n' "$dir" > /dev/termination-log
    exit 0
  fi
  set --
  if [ -n "$CASOS_BRANCH" ]; then set -- --branch "$CASOS_BRANCH"; fi
  { git clone --progress "$@" -- "$CASOS_REPO" "$dir"; echo $? > /tmp/clone.status; } 2>&1 | tee /tmp/clone.log
  if [ "$(cat /tmp/clone.status)" != 0 ]; then
    printf 'error=%s\n' "$(tr '\r' '\n' < /tmp/clone.log | grep -v '^Cloning into' | grep . | tail -n 1)" > /dev/termination-log
    exit 0
  fi
fi
printf 'branch=%s\ncommit=%s\n' "$(git -C "$dir" rev-parse --abbrev-ref HEAD)" "$(git -C "$dir" rev-parse --short HEAD)" > /dev/termination-log
`

const devboxSetupScript = `s="$HOME/.casos"
if [ -f "$s/setup-done" ]; then
  echo "Already set up. Delete ~/.casos/setup-done to run the setup script again on the next start."
  exit 0
fi
mkdir -p "$s"
cd "$CASOS_FOLDER" 2>/dev/null || cd "$HOME"
export PATH="$HOME/.local/bin:$PATH"
shell=sh; command -v bash >/dev/null && shell=bash
{ "$shell" -ec "$CASOS_SETUP"; echo $? > "$s/setup.status"; } 2>&1 | tee "$s/setup.log"
if [ "$(cat "$s/setup.status")" = 0 ]; then touch "$s/setup-done"; else echo failed > /dev/termination-log; fi
exit 0
`

type devboxEnvironment struct {
	image  string
	repo   string
	branch string
	setup  string
	folder string
}

var devboxFolderPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Credentials in the URL are refused, as they would sit in the Deployment in plain text.
func devboxRepoFolder(repo string) (string, error) {
	u, err := url.Parse(repo)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "git") {
		return "", fmt.Errorf("the repository must be an http(s) or git:// address, such as https://github.com/owner/project.git")
	}
	if u.User != nil {
		return "", fmt.Errorf("leave credentials out of the repository address")
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(u.Path, "/")), ".git")
	if !devboxFolderPattern.MatchString(name) || name == "." || name == ".." {
		return "", fmt.Errorf("cannot tell a folder name from the repository address %q", repo)
	}
	return name, nil
}

func devboxEnvVars(env devboxEnvironment, password, sshKeys string) []envVarRequest {
	vars := []envVarRequest{
		{Name: "PASSWORD", Value: password},
		{Name: "HOME", Value: devboxHomeMount},
		{Name: "CASOS_FOLDER", Value: env.folder},
		// The image's conda belongs to root; environments go to the home disk instead.
		{Name: "CONDA_ENVS_PATH", Value: devboxHomeMount + "/.conda/envs"},
		{Name: "CONDA_PKGS_DIRS", Value: devboxHomeMount + "/.conda/pkgs"},
	}
	if sshKeys != "" {
		vars = append(vars, envVarRequest{Name: devboxSshKeyEnv, Value: sshKeys})
	}
	return vars
}

func applyDevboxEnvironment(env devboxEnvironment, ssh bool) func(*appsv1.Deployment) error {
	return func(depl *appsv1.Deployment) error {
		spec := &depl.Spec.Template.Spec
		if len(spec.Containers) == 0 {
			return fmt.Errorf("the workspace has no container to set up")
		}
		editor := &spec.Containers[0]
		uid := devboxUid
		onRootMismatch := corev1.FSGroupChangeOnRootMismatch
		spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser:           &uid,
			RunAsGroup:          &uid,
			FSGroup:             &uid,
			FSGroupChangePolicy: &onRootMismatch,
		}
		// The pause container becomes PID 1 and reaps what SSH and terminal sessions leave behind.
		shareProcesses := true
		spec.ShareProcessNamespace = &shareProcesses

		home, ok := devboxHomeMountOf(*editor)
		if !ok {
			spec.Volumes = append(spec.Volumes, emptyDirVolume(devboxHomeVolume))
			home = corev1.VolumeMount{Name: devboxHomeVolume, MountPath: devboxHomeMount}
			editor.VolumeMounts = append(editor.VolumeMounts, home)
		}

		var accounts []corev1.VolumeMount
		if env.image != devboxDefaultImage {
			tools := corev1.VolumeMount{Name: devboxToolsVolume, MountPath: devboxToolsMount}
			spec.Volumes = append(spec.Volumes, emptyDirVolume(devboxToolsVolume))
			spec.InitContainers = append(spec.InitContainers,
				devboxInitContainer(devboxEditorInit, devboxDefaultImage, devboxEditorCopyScript, nil, editor.Resources, tools),
				devboxInitContainer(devboxUserInit, env.image, devboxUserScript, nil, editor.Resources, tools),
			)
			accounts = []corev1.VolumeMount{
				{Name: devboxToolsVolume, MountPath: "/etc/passwd", SubPath: "etc/passwd", ReadOnly: true},
				{Name: devboxToolsVolume, MountPath: "/etc/group", SubPath: "etc/group", ReadOnly: true},
			}
			tools.ReadOnly = true
			editor.VolumeMounts = append(append(editor.VolumeMounts, tools), accounts...)
		}

		if ssh {
			if err := addDevboxSshServer(depl); err != nil {
				return err
			}
		}

		if env.repo != "" {
			spec.InitContainers = append(spec.InitContainers, devboxInitContainer(devboxCloneInit, devboxDefaultImage, devboxCloneScript, []corev1.EnvVar{
				{Name: "CASOS_REPO", Value: env.repo},
				{Name: "CASOS_BRANCH", Value: env.branch},
				{Name: "CASOS_FOLDER", Value: env.folder},
				{Name: "HOME", Value: "/tmp"},
				{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
			}, editor.Resources, home))
		}

		if env.setup != "" {
			setupEnv := append(devboxRunEnv(editor.Env, false), corev1.EnvVar{Name: "CASOS_SETUP", Value: env.setup})
			mounts := append([]corev1.VolumeMount{home}, accounts...)
			spec.InitContainers = append(spec.InitContainers,
				devboxInitContainer(devboxSetupInit, env.image, devboxSetupScript, setupEnv, editor.Resources, mounts...))
		}

		applyAnnotation(&depl.ObjectMeta, devboxFolderAnnotation, env.folder)
		if env.repo != "" {
			applyAnnotation(&depl.ObjectMeta, devboxRepoAnnotation, env.repo)
		}
		return nil
	}
}

// Init containers take the editor's resources, so a quota that requires limits still admits the pod.
func devboxInitContainer(name, image, script string, env []corev1.EnvVar, resources corev1.ResourceRequirements, mounts ...corev1.VolumeMount) corev1.Container {
	return corev1.Container{
		Name:         name,
		Image:        image,
		Command:      []string{"/bin/sh", "-c", script},
		Env:          env,
		VolumeMounts: mounts,
		Resources:    resources,
	}
}

func emptyDirVolume(name string) corev1.Volume {
	return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
}

func devboxHomeMountOf(c corev1.Container) (corev1.VolumeMount, bool) {
	for _, mount := range c.VolumeMounts {
		if mount.MountPath == devboxHomeMount {
			return mount, true
		}
	}
	return corev1.VolumeMount{}, false
}

func devboxFolder(depl appsv1.Deployment) string {
	if folder := depl.Annotations[devboxFolderAnnotation]; folder != "" {
		return folder
	}
	return devboxHomeMount
}

type devboxPodState struct {
	podName       string
	prepareStep   string
	branch        string
	commit        string
	cloneError    string
	setupFailed   bool
	logContainers []string
}

func devboxPodStateOf(d appsv1.Deployment, pods []corev1.Pod) devboxPodState {
	state := devboxPodState{}
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return state
	}
	var newest *corev1.Pod
	for i := range pods {
		pod := &pods[i]
		if pod.Namespace != d.Namespace || pod.DeletionTimestamp != nil || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		if newest == nil || pod.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = pod
		}
	}
	if newest == nil {
		return state
	}

	state.podName = newest.Name
	for _, init := range newest.Spec.InitContainers {
		if init.Name == devboxCloneInit || init.Name == devboxSetupInit {
			state.logContainers = append(state.logContainers, init.Name)
		}
	}
	for _, status := range newest.Status.InitContainerStatuses {
		terminated := status.State.Terminated
		if terminated == nil {
			if state.prepareStep == "" {
				state.prepareStep = status.Name
			}
			continue
		}
		switch status.Name {
		case devboxCloneInit:
			for _, line := range strings.Split(terminated.Message, "\n") {
				key, value, _ := strings.Cut(line, "=")
				switch key {
				case "branch":
					// A tag leaves HEAD detached; the commit says enough.
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
	}
	return state
}
