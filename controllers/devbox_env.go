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
// The image is not required to contain code-server or an SSH server. Both are
// copied out of images that have them into a scratch volume and run from
// there, inside the workspace container, so any glibc-based image — python,
// pytorch, a lab's own — becomes a workspace as it is; whether it is opened in
// the browser, from a desktop VS Code, or frozen into a run, the work happens
// in that one image.

const (
	devboxRepoAnnotation   = "casos.io/devbox-repo"
	devboxFolderAnnotation = "casos.io/devbox-folder"

	devboxGitImage = "alpine/git:latest"
	// sshd, its helpers and the musl libraries they need are taken from here;
	// the image itself never runs. Pinned, because the copy script names files
	// inside it that a later build is free to move.
	devboxSshImage = "lscr.io/linuxserver/openssh-server:10.3_p1-r1-ls237"

	// The scripts below name this path literally.
	devboxToolsVolume = "casos-tools"
	devboxToolsMount  = "/opt/casos"
	devboxHomeVolume  = "home"

	devboxEditorInit = "editor"
	devboxSshInit    = "ssh"
	devboxUserInit   = "user"
	devboxCloneInit  = "clone"
	devboxSetupInit  = "setup"

	devboxBundledCodeServer  = "/usr/lib/code-server/bin/code-server"
	devboxInjectedCodeServer = devboxToolsMount + "/code-server/bin/code-server"

	// Written in the home disk once the setup script has succeeded, so a restart
	// does not reinstall everything; deleting it runs the script again.
	devboxSetupMarker = devboxHomeMount + "/.casos/setup-done"
)

// The Alpine sshd is dynamically linked against musl, which a glibc image does
// not have, so it is run through the musl loader it came with. sshd starts its
// per-connection helpers by path, which is why those are small wrappers that
// do the same.
const devboxSshCopyScript = `d=/opt/casos/ssh
mkdir -p "$d/lib"
cp /lib/ld-musl-*.so.1 "$d/ld-musl"
cp /usr/sbin/sshd.pam "$d/sshd"
cp /usr/lib/ssh/sshd-session.pam "$d/sshd-session.bin"
cp /usr/lib/ssh/sshd-auth.pam "$d/sshd-auth.bin"
cp /usr/lib/ssh/sftp-server "$d/sftp-server.bin"
cp /usr/bin/ssh-keygen "$d/ssh-keygen"
for bin in "$d"/sshd "$d"/*.bin "$d"/ssh-keygen; do
  ldd "$bin" | awk '$2 == "=>" && $3 !~ /ld-musl/ {print $3}'
done | sort -u | while read -r lib; do cp -L "$lib" "$d/lib/"; done
for name in sshd-session sshd-auth sftp-server; do
  printf '#!/bin/sh\nexec %s/ld-musl --library-path %s/lib %s/%s.bin "$@"\n' "$d" "$d" "$d" "$name" > "$d/$name"
  chmod 755 "$d/$name"
done
`

// Most images have no user 1000, and sshd refuses a login it cannot look up —
// a shell prompt says "I have no name!" for the same reason. The image's own
// accounts are kept and coder added as 1000, along with the unprivileged user
// sshd expects to exist; the result is mounted over /etc/passwd and /etc/group.
const devboxUserScript = `e=/opt/casos/etc
mkdir -p "$e"
awk -F: '$3 != 1000 && $1 != "coder" && $1 != "sshd"' /etc/passwd > "$e/passwd"
shell=/bin/sh; [ -x /bin/bash ] && shell=/bin/bash
echo "coder:x:1000:1000:coder:/home/coder:$shell" >> "$e/passwd"
echo "sshd:x:22:22:sshd privsep:/var/empty:/sbin/nologin" >> "$e/passwd"
awk -F: '$3 != 1000 && $1 != "coder" && $1 != "sshd"' /etc/group > "$e/group"
echo "coder:x:1000:" >> "$e/group"
echo "sshd:x:22:" >> "$e/group"
`

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

// The launcher starts sshd, when there is a key, beside code-server. sshd gives
// every session a fresh environment, which would lose the image's own — PATH
// with conda on it, the CUDA variables — so the container's is written into
// its config as SetEnv (one line; sshd keeps only the first), and into a file
// ~/.profile reads, since login shells such as tmux's reset PATH afterwards.
// The shell stays in the foreground rather than exec'ing the editor, so that
// sshd's session processes have a parent that reaps them.
const devboxLaunchScript = `export PATH="$HOME/.local/bin:$PATH"
if [ -n "$CASOS_SSH_PUBLIC_KEY" ]; then
  d=/opt/casos/ssh; s="$HOME/.casos/ssh"
  mkdir -p "$s" && chmod 700 "$s"
  [ -f "$s/host_ed25519" ] || "$d/ld-musl" --library-path "$d/lib" "$d/ssh-keygen" -q -t ed25519 -N "" -f "$s/host_ed25519"
  printf '%s\n' "$CASOS_SSH_PUBLIC_KEY" > "$s/authorized_keys"
  (unset PASSWORD CASOS_SSH_PUBLIC_KEY PWD OLDPWD SHLVL HOSTNAME _; export -p) > "$HOME/.casos/env.sh"
  touch "$HOME/.profile"
  grep -q '.casos/env.sh' "$HOME/.profile" || printf '\n# Added by CasOS: the workspace image environment, which login shells otherwise reset.\n[ -f "$HOME/.casos/env.sh" ] && . "$HOME/.casos/env.sh"\n' >> "$HOME/.profile"
  cat > "$s/sshd_config" <<CFG
Port 2222
HostKey $s/host_ed25519
PidFile none
AuthorizedKeysFile $s/authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
StrictModes no
AllowTcpForwarding yes
X11Forwarding no
SshdSessionPath $d/sshd-session
SshdAuthPath $d/sshd-auth
Subsystem sftp $d/sftp-server
CFG
  awk 'BEGIN {
    printf "SetEnv"
    for (name in ENVIRON) {
      value = ENVIRON[name]
      if (name !~ /^[A-Za-z_][A-Za-z0-9_]*$/ || value ~ /\n/) continue
      if (name ~ /^(PASSWORD|CASOS_SSH_PUBLIC_KEY|HOSTNAME|PWD|OLDPWD|SHLVL|_)$/) continue
      gsub(/\\/, "\\\\", value); gsub(/"/, "\\\"", value)
      printf " \"%s=%s\"", name, value
    }
    print ""
  }' >> "$s/sshd_config"
  "$d/ld-musl" --library-path "$d/lib" "$d/sshd" -D -e -f "$s/sshd_config" &
fi
"$CASOS_CODE_SERVER" --bind-addr 0.0.0.0:8080 --auth password --disable-telemetry "$CASOS_FOLDER" &
editor=$!
trap 'kill -TERM $editor 2>/dev/null' TERM INT
wait $editor
`

type devboxEnvironment struct {
	image        string
	repo         string
	branch       string
	setup        string
	folder       string
	sshPublicKey string
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
// each other — tools, accounts, repository, setup.
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
			// script and the editor all see; it just ends with the pod.
			spec.Volumes = append(spec.Volumes, emptyDirVolume(devboxHomeVolume))
			home = corev1.VolumeMount{Name: devboxHomeVolume, MountPath: devboxHomeMount}
			editor.VolumeMounts = append(editor.VolumeMounts, home)
		}
		spec.Volumes = append(spec.Volumes, emptyDirVolume(devboxToolsVolume))
		tools := corev1.VolumeMount{Name: devboxToolsVolume, MountPath: devboxToolsMount}
		accounts := []corev1.VolumeMount{
			{Name: devboxToolsVolume, MountPath: "/etc/passwd", SubPath: "etc/passwd", ReadOnly: true},
			{Name: devboxToolsVolume, MountPath: "/etc/group", SubPath: "etc/group", ReadOnly: true},
		}

		codeServer := devboxBundledCodeServer
		if env.image != devboxDefaultImage {
			codeServer = devboxInjectedCodeServer
			spec.InitContainers = append(spec.InitContainers, devboxInitContainer(devboxEditorInit, devboxDefaultImage,
				"cp -a /usr/lib/code-server "+devboxToolsMount+"/", nil, tools))
		}
		if env.sshPublicKey != "" {
			spec.InitContainers = append(spec.InitContainers, devboxInitContainer(devboxSshInit, devboxSshImage,
				devboxSshCopyScript, nil, tools))
		}
		spec.InitContainers = append(spec.InitContainers, devboxInitContainer(devboxUserInit, env.image,
			devboxUserScript, nil, tools))

		folderEnv := corev1.EnvVar{Name: "CASOS_FOLDER", Value: env.folder}
		homeEnv := corev1.EnvVar{Name: "HOME", Value: devboxHomeMount}
		editor.Command = []string{"/bin/sh", "-c", devboxLaunchScript}
		editor.Args = nil
		editor.SecurityContext = devboxRunsAsUser()
		editor.VolumeMounts = append(append(editor.VolumeMounts, tools), accounts...)
		// conda is told to keep environments in home, since the one an image
		// ships with belongs to root and only home outlives a restart anyway.
		editor.Env = append(editor.Env,
			folderEnv,
			homeEnv,
			corev1.EnvVar{Name: "CASOS_CODE_SERVER", Value: codeServer},
			corev1.EnvVar{Name: "CONDA_ENVS_PATH", Value: devboxHomeMount + "/.conda/envs"},
			corev1.EnvVar{Name: "CONDA_PKGS_DIRS", Value: devboxHomeMount + "/.conda/pkgs"},
		)
		if env.sshPublicKey != "" {
			editor.Env = append(editor.Env, corev1.EnvVar{Name: "CASOS_SSH_PUBLIC_KEY", Value: env.sshPublicKey})
		}

		if env.repo != "" {
			spec.InitContainers = append(spec.InitContainers, devboxInitContainer(devboxCloneInit, devboxGitImage,
				devboxCloneScript, []corev1.EnvVar{
					{Name: "CASOS_REPO", Value: env.repo},
					{Name: "CASOS_BRANCH", Value: env.branch},
					folderEnv,
					{Name: "HOME", Value: "/tmp"},
					// Without this, a repository that wants a password waits forever
					// on a prompt nobody can see.
					{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
				}, home))
		}

		if env.setup != "" {
			// The script runs in the workspace's own image, so what it installs
			// matches what the editor runs — but only what lands in the home disk
			// outlives it, which is why pip and conda are pointed there.
			setupEnv := append(withoutEnv(editor.Env, "PASSWORD", "CASOS_SSH_PUBLIC_KEY"),
				corev1.EnvVar{Name: "CASOS_SETUP", Value: env.setup},
				corev1.EnvVar{Name: "CASOS_SETUP_MARKER", Value: devboxSetupMarker},
			)
			setup := devboxInitContainer(devboxSetupInit, env.image, devboxSetupScript, setupEnv, home)
			setup.VolumeMounts = append(setup.VolumeMounts, accounts...)
			setup.Resources = editor.Resources
			spec.InitContainers = append(spec.InitContainers, setup)
		}

		applyAnnotation(&depl.ObjectMeta, devboxFolderAnnotation, env.folder)
		if env.repo != "" {
			applyAnnotation(&depl.ObjectMeta, devboxRepoAnnotation, env.repo)
		}
		return nil
	}
}

func devboxInitContainer(name, image, script string, env []corev1.EnvVar, mounts ...corev1.VolumeMount) corev1.Container {
	return corev1.Container{
		Name:            name,
		Image:           image,
		Command:         []string{"/bin/sh", "-c", script},
		Env:             env,
		VolumeMounts:    mounts,
		SecurityContext: devboxRunsAsUser(),
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
