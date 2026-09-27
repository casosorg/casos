package controllers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// A DevBox can start from a Git repository on the machine running casos. The
// pod cannot see that disk, and may not be able to reach this machine over the
// network either, so the repository travels the one way that always works: its
// clone init container waits, and casos bundles the repository and streams it
// in through the API server's exec, the same channel the pod terminal uses.
//
// Nothing about the transfer is stored. Whenever a clone container is waiting
// the repository is bundled afresh, so a stateless box that restarts, or a
// casos that restarted mid-way, simply sends it again.

const (
	devboxRepoWanted = "/tmp/casos-repo.wanted"
	devboxRepoBundle = "/tmp/casos-repo.bundle"
	devboxRepoError  = "/tmp/casos-repo.error"

	devboxUploadInterval = 3 * time.Second
	devboxUploadTimeout  = 10 * time.Minute
)

type devboxLocalRepo struct {
	root   string
	folder string
	branch string
	origin string
}

var scpLikeRemote = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:`)

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func inspectLocalRepo(dir string) (devboxLocalRepo, error) {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if dir == "" {
		return devboxLocalRepo{}, fmt.Errorf("the folder of the local repository is required")
	}
	if !filepath.IsAbs(dir) {
		return devboxLocalRepo{}, fmt.Errorf("give the full path of the folder, not %q", dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return devboxLocalRepo{}, fmt.Errorf("there is no folder %s on the machine running casos", dir)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return devboxLocalRepo{}, fmt.Errorf("git is not installed on the machine running casos, so it cannot read a local repository")
	}
	root, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return devboxLocalRepo{}, fmt.Errorf("%s is not a Git repository: run git init and commit in it first, or start from scratch instead", dir)
	}
	root = filepath.Clean(filepath.FromSlash(root))
	if _, err := runGit(root, "rev-parse", "-q", "--verify", "HEAD"); err != nil {
		return devboxLocalRepo{}, fmt.Errorf("%s has no commits yet: only committed work is copied, so commit something first", root)
	}

	repo := devboxLocalRepo{root: root, folder: devboxFolderName(filepath.Base(root))}
	repo.branch, _ = runGit(root, "symbolic-ref", "-q", "--short", "HEAD")
	if origin, err := runGit(root, "remote", "get-url", "origin"); err == nil {
		repo.origin = shareableRemote(origin)
	}
	return repo, nil
}

// Only a remote the box itself could fetch from is kept, and never with the
// credentials some remotes carry: it ends up in the Deployment in plain text.
func shareableRemote(remote string) string {
	if scpLikeRemote.MatchString(remote) {
		return remote
	}
	u, err := url.Parse(remote)
	if err != nil || u.Host == "" || u.Scheme == "file" {
		return ""
	}
	u.User = nil
	return u.String()
}

var devboxFolderUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func devboxFolderName(name string) string {
	name = strings.Trim(devboxFolderUnsafe.ReplaceAllString(name, "-"), "-.")
	if name == "" {
		return "project"
	}
	return name
}

func bundleLocalRepo(root string) (string, error) {
	file, err := os.CreateTemp("", "casos-devbox-*.bundle")
	if err != nil {
		return "", err
	}
	path := file.Name()
	file.Close()
	if _, err := runGit(root, "bundle", "create", path, "--all"); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("could not bundle %s: %v", root, err)
	}
	return path, nil
}

// Pod UIDs with a transfer in flight or done, so one tick does not start a
// second copy of what the last is still sending.
var devboxUploads sync.Map

func StartDevboxRepoUploader(ctx context.Context) {
	ticker := time.NewTicker(devboxUploadInterval)
	defer ticker.Stop()
	for {
		sendWaitingLocalRepos(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func sendWaitingLocalRepos(ctx context.Context) {
	cfg := getAdminRestConfig()
	if cfg == nil {
		return
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return
	}
	pods, err := clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: devboxLabel + "=true"})
	if err != nil {
		return
	}
	seen := map[types.UID]bool{}
	for i := range pods.Items {
		pod := pods.Items[i]
		seen[pod.UID] = true
		root := localRepoWaitingIn(pod)
		if root == "" {
			continue
		}
		if _, busy := devboxUploads.LoadOrStore(pod.UID, true); busy {
			continue
		}
		go func() {
			sendCtx, cancel := context.WithTimeout(ctx, devboxUploadTimeout)
			defer cancel()
			if retry := sendLocalRepo(sendCtx, cfg, &pod, root); retry {
				devboxUploads.Delete(pod.UID)
			}
		}()
	}
	devboxUploads.Range(func(key, _ interface{}) bool {
		if !seen[key.(types.UID)] {
			devboxUploads.Delete(key)
		}
		return true
	})
}

func localRepoWaitingIn(pod corev1.Pod) string {
	running := false
	for _, status := range pod.Status.InitContainerStatuses {
		if status.Name == devboxCloneInit {
			running = status.State.Running != nil
		}
	}
	if !running {
		return ""
	}
	for _, init := range pod.Spec.InitContainers {
		if init.Name != devboxCloneInit {
			continue
		}
		source, root := "", ""
		for _, env := range init.Env {
			switch env.Name {
			case "CASOS_SOURCE":
				source = env.Value
			case "CASOS_LOCAL_REPO":
				root = env.Value
			}
		}
		if source == devboxSourceLocal {
			return root
		}
	}
	return ""
}

// sendLocalRepo reports whether it should be tried again on a later tick: the
// container may not have asked yet, or may already be gone.
func sendLocalRepo(ctx context.Context, cfg *rest.Config, pod *corev1.Pod, root string) bool {
	code, err := execInContainer(ctx, cfg, pod, devboxCloneInit, []string{"test", "-f", devboxRepoWanted}, nil, io.Discard, io.Discard)
	if err != nil || code != 0 {
		return true
	}

	bundle, err := bundleLocalRepo(root)
	if err != nil {
		reportLocalRepoError(ctx, cfg, pod, err.Error())
		return false
	}
	defer os.Remove(bundle)
	file, err := os.Open(bundle)
	if err != nil {
		reportLocalRepoError(ctx, cfg, pod, err.Error())
		return false
	}
	defer file.Close()

	var stderr bytes.Buffer
	script := `cat > "$0.part" && mv "$0.part" "$0"`
	code, err = execInContainer(ctx, cfg, pod, devboxCloneInit, []string{"sh", "-c", script, devboxRepoBundle}, file, io.Discard, &stderr)
	if err != nil || code != 0 {
		reason := strings.TrimSpace(stderr.String())
		if err != nil {
			reason = err.Error()
		}
		logs.Warning("devbox %s/%s: send %s: %s", pod.Namespace, pod.Name, root, reason)
		reportLocalRepoError(ctx, cfg, pod, "could not copy the repository into the DevBox: "+reason)
	}
	return false
}

func reportLocalRepoError(ctx context.Context, cfg *rest.Config, pod *corev1.Pod, message string) {
	script := `printf '%s' "$1" > "$0"`
	_, _ = execInContainer(ctx, cfg, pod, devboxCloneInit, []string{"sh", "-c", script, devboxRepoError, message}, nil, io.Discard, io.Discard)
}
