package controllers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"

	"github.com/casosorg/casos/object"
)

const (
	mcpSandboxDefaultTTL  = 60
	mcpSandboxMaxTTL      = 7 * 24 * 60
	mcpSandboxDefaultWait = 180 * time.Second

	mcpExecDefaultTimeout = 120
	mcpExecMaxTimeout     = 600
	mcpExecMaxOutput      = 32 * 1024

	mcpReadDefaultBytes = 64 * 1024
	mcpReadMaxBytes     = 512 * 1024
)

// The command runs in the working folder, under bash when the image has it,
// after the same profile a job reads. timeout(1) stops it inside the
// container: cancelling the exec stream alone would leave it running there.
const sandboxExecScript = `cd -- "$1" || exit 126
sh=/bin/sh; [ -x /bin/bash ] && sh=/bin/bash
if command -v timeout >/dev/null 2>&1; then exec timeout "$2" "$sh" -c "$3"; fi
exec "$sh" -c "$3"`

const sandboxReadScript = `if [ ! -f "$1" ]; then
  if [ -d "$1" ]; then echo "$1 is a directory; list it with exec_in_sandbox" >&2; else echo "no such file: $1" >&2; fi
  exit 2
fi
wc -c < "$1"
head -c "$2" -- "$1"`

const sandboxWriteScript = `mkdir -p -- "$(dirname -- "$1")" || exit 1
if [ "$2" = append ]; then cat >> "$1"; else cat > "$1"; fi`

var (
	mcpSandboxNameProp = mcpProp("string", "Sandbox name, as create_sandbox returned it. Any DevBox listed by list_sandboxes works too.")
	mcpSandboxPathProp = mcpProp("string", "File path. A relative path is taken from the sandbox's working folder; ~ is /home/coder.")
	mcpSandboxTTLProp  = mcpProp("integer", "Minutes from now until the sandbox and its disk are deleted, at most 10080 (7 days).")
)

var mcpSandboxTools = []*mcpTool{
	{
		Name:  "create_sandbox",
		Title: "Create sandbox",
		Description: "Create a sandbox: an isolated Linux workspace on the casos cluster to clone code into, install dependencies, and run commands, tests and GPU jobs. " +
			"The home directory /home/coder is kept on a disk, and the working folder is the cloned repository, or /home/coder without one. " +
			"The sandbox is a DevBox, so the user can open it in VS Code in the browser with the url and password in the result. " +
			"It is deleted with its disk when its lease runs out; extend_sandbox moves the lease. Waits until the sandbox is ready.",
		InputSchema: mcpSchema(nil, map[string]interface{}{
			"name":         mcpProp("string", "Sandbox name: lowercase letters, digits and '-', at most 63 characters. Generated when omitted."),
			"namespace":    mcpNamespaceProp,
			"image":        mcpProp("string", "Environment image: any glibc-based Linux image, such as python:3.12, node:22 or nvidia/cuda:12.6.3-cudnn-devel-ubuntu24.04. Defaults to code-server's Debian image, which has git."),
			"repo":         mcpProp("string", "Public Git repository cloned on first start, such as https://github.com/owner/project.git. It becomes the working folder."),
			"branch":       mcpProp("string", "Branch or tag to check out. Defaults to the repository's default branch."),
			"setup":        mcpProp("string", "Shell script run once in the working folder before the sandbox is ready, such as pip install -r requirements.txt. Its output is in ~/.casos/setup.log."),
			"ttl_minutes":  mcpProp("integer", "Minutes until the sandbox and its disk are deleted, at most 10080 (7 days). Defaults to 60."),
			"cpu":          mcpProp("string", "CPU limit, e.g. 2."),
			"memory":       mcpProp("string", "Memory limit, e.g. 4Gi."),
			"disk":         mcpProp("string", "Size of the home disk. Defaults to 5Gi."),
			"wait_seconds": mcpProp("integer", "Seconds to wait for the sandbox to be ready, at most 600. Defaults to 180; 0 returns at once."),
		}),
		Annotations: mcpToolAnnotations{OpenWorldHint: true},
		handler:     mcpCreateSandbox,
	},
	{
		Name:        "list_sandboxes",
		Title:       "List sandboxes",
		Description: "List the sandboxes and DevBoxes on casos with their status, working folder, and when each one's lease runs out. A box without a lease was made or kept by a person and is never deleted automatically.",
		InputSchema: mcpSchema(nil, map[string]interface{}{
			"namespace": mcpProp("string", "Only list sandboxes in this namespace. Lists every namespace when omitted."),
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpListSandboxes,
	},
	{
		Name:  "exec_in_sandbox",
		Title: "Run command in sandbox",
		Description: "Run a shell command in a sandbox and return its exit code, stdout and stderr. It runs under bash when the image has it, as the user coder, in the working folder unless workdir says otherwise. " +
			"Each call is a fresh shell: cd and exported variables do not carry over. Output beyond 32 KB per stream keeps only its end. " +
			"For anything longer than the timeout, or that needs the GPU, use start_sandbox_job.",
		InputSchema: mcpSchema([]string{"name", "command"}, map[string]interface{}{
			"name":            mcpSandboxNameProp,
			"namespace":       mcpNamespaceProp,
			"command":         mcpProp("string", "Shell command or script to run."),
			"workdir":         mcpProp("string", "Directory to run in. A relative path is taken from the working folder."),
			"stdin":           mcpProp("string", "Text passed to the command's standard input."),
			"timeout_seconds": mcpProp("integer", "Seconds before the command is stopped, at most 600. Defaults to 120."),
		}),
		Annotations: mcpToolAnnotations{DestructiveHint: true, OpenWorldHint: true},
		handler:     mcpExecInSandbox,
	},
	{
		Name:        "read_sandbox_file",
		Title:       "Read sandbox file",
		Description: "Read a file from a sandbox. Text comes back as it is; a binary file comes back base64-encoded.",
		InputSchema: mcpSchema([]string{"name", "path"}, map[string]interface{}{
			"name":      mcpSandboxNameProp,
			"namespace": mcpNamespaceProp,
			"path":      mcpSandboxPathProp,
			"max_bytes": mcpProp("integer", "Bytes to read from the start of the file, at most 524288. Defaults to 65536."),
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpReadSandboxFile,
	},
	{
		Name:        "write_sandbox_file",
		Title:       "Write sandbox file",
		Description: "Write a file in a sandbox, creating the directories on its path. Replaces the file unless append is set.",
		InputSchema: mcpSchema([]string{"name", "path", "content"}, map[string]interface{}{
			"name":      mcpSandboxNameProp,
			"namespace": mcpNamespaceProp,
			"path":      mcpSandboxPathProp,
			"content":   mcpProp("string", "The file's content."),
			"encoding":  map[string]interface{}{"type": "string", "enum": []string{"text", "base64"}, "description": "How content is encoded. Defaults to text; use base64 for a binary file."},
			"append":    mcpProp("boolean", "Add to the end of the file instead of replacing it."),
		}),
		Annotations: mcpToolAnnotations{DestructiveHint: true},
		handler:     mcpWriteSandboxFile,
	},
	{
		Name:  "start_sandbox_job",
		Title: "Start sandbox job",
		Description: "Run a command from a sandbox as a background job: same image, same home disk, same working folder, optionally on the cluster's NVIDIA GPU. " +
			"Use it for training, long builds and test suites. Returns at once; follow the job with get_sandbox_job.",
		InputSchema: mcpSchema([]string{"name", "command"}, map[string]interface{}{
			"name":      mcpSandboxNameProp,
			"namespace": mcpNamespaceProp,
			"command":   mcpProp("string", "Shell command or script to run."),
			"gpu":       mcpProp("boolean", "Run on the NVIDIA GPU."),
			"cpu":       mcpProp("string", "CPU limit for the job, e.g. 4. Defaults to the sandbox's."),
			"memory":    mcpProp("string", "Memory limit for the job, e.g. 16Gi. Defaults to the sandbox's."),
		}),
		Annotations: mcpToolAnnotations{OpenWorldHint: true},
		handler:     mcpStartSandboxJob,
	},
	{
		Name:        "get_sandbox_job",
		Title:       "Get sandbox job",
		Description: "Show a sandbox job's status (queued, running, succeeded or failed) and the end of its output.",
		InputSchema: mcpSchema([]string{"job"}, map[string]interface{}{
			"job":        mcpProp("string", "Job name, as start_sandbox_job returned it."),
			"namespace":  mcpNamespaceProp,
			"tail_lines": mcpProp("integer", "Lines to read from the end of the job's output, at most 1000. Defaults to 100."),
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpGetSandboxJob,
	},
	{
		Name:        "extend_sandbox",
		Title:       "Extend sandbox",
		Description: "Move a sandbox's lease, so it is deleted ttl_minutes from now instead.",
		InputSchema: mcpSchema([]string{"name", "ttl_minutes"}, map[string]interface{}{
			"name":        mcpSandboxNameProp,
			"namespace":   mcpNamespaceProp,
			"ttl_minutes": mcpSandboxTTLProp,
		}),
		Annotations: mcpToolAnnotations{IdempotentHint: true},
		handler:     mcpExtendSandbox,
	},
	{
		Name:        "delete_sandbox",
		Title:       "Delete sandbox",
		Description: "Delete a sandbox an agent created, with its disk and jobs. A box a person made or chose to keep can only be deleted by them.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":      mcpSandboxNameProp,
			"namespace": mcpNamespaceProp,
		}),
		Annotations: mcpToolAnnotations{DestructiveHint: true, IdempotentHint: true},
		handler:     mcpDeleteSandbox,
	},
}

func init() {
	mcpTools = append(mcpTools, mcpSandboxTools...)
}

var mcpPrepareSteps = map[string]string{
	devboxEditorInit: "pulling the images and installing the editor",
	devboxUserInit:   "adding the user",
	"ssh-tools":      "installing SSH",
	devboxCloneInit:  "cloning the repository",
	devboxSetupInit:  "running the setup script",
}

type mcpSandbox struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Image      string   `json:"image"`
	Status     string   `json:"status"`
	Preparing  string   `json:"preparing,omitempty"`
	Folder     string   `json:"folder"`
	Repo       string   `json:"repo,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	Commit     string   `json:"commit,omitempty"`
	Url        string   `json:"url,omitempty"`
	ExpiresAt  string   `json:"expiresAt,omitempty"`
	Agent      string   `json:"agent,omitempty"`
	ActiveJobs int      `json:"activeJobs,omitempty"`
	Problems   []string `json:"problems,omitempty"`
}

func mcpSandboxOf(box devboxSummary) mcpSandbox {
	sandbox := mcpSandbox{
		Name:       box.Name,
		Namespace:  box.Namespace,
		Image:      box.Image,
		Status:     box.Status,
		Preparing:  mcpPrepareSteps[box.PrepareStep],
		Folder:     box.Folder,
		Repo:       box.Repo,
		Branch:     box.Branch,
		Commit:     box.Commit,
		Url:        box.Url,
		ExpiresAt:  box.ExpiresAt,
		Agent:      box.Agent,
		ActiveJobs: box.ActiveRuns,
	}
	if box.CloneError != "" {
		sandbox.Problems = append(sandbox.Problems, "cloning the repository failed: "+box.CloneError)
	}
	if box.SetupFailed {
		sandbox.Problems = append(sandbox.Problems, "the setup script failed; read ~/.casos/setup.log")
	}
	return sandbox
}

func mcpTTL(minutes *int, fallback int) (time.Time, error) {
	ttl := fallback
	if minutes != nil {
		ttl = *minutes
	}
	if ttl < 1 || ttl > mcpSandboxMaxTTL {
		return time.Time{}, fmt.Errorf("ttl_minutes must be between 1 and %d", mcpSandboxMaxTTL)
	}
	return time.Now().Add(time.Duration(ttl) * time.Minute), nil
}

type mcpCreateSandboxResult struct {
	Sandbox  mcpSandbox `json:"sandbox"`
	Password string     `json:"password"`
	Rollout  mcpRollout `json:"rollout"`
	Next     string     `json:"next,omitempty"`
}

func mcpCreateSandbox(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		Image       string  `json:"image"`
		Repo        string  `json:"repo"`
		Branch      string  `json:"branch"`
		Setup       string  `json:"setup"`
		TTLMinutes  *int    `json:"ttl_minutes"`
		CPU         *string `json:"cpu"`
		Memory      *string `json:"memory"`
		Disk        string  `json:"disk"`
		WaitSeconds *int    `json:"wait_seconds"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Name) == "" {
		args.Name = "sandbox-" + randomPassword()[:6]
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}
	expiresAt, err := mcpTTL(args.TTLMinutes, mcpSandboxDefaultTTL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Disk) == "0" {
		return nil, fmt.Errorf("a sandbox needs a home disk; leave disk out for the default size")
	}

	if _, err := object.GetDeployment(cfg, args.Namespace, args.Name); err == nil {
		return nil, fmt.Errorf("%s already exists in namespace %s; pick another name, or leave name out to get a new one", args.Name, args.Namespace)
	} else if !errors.IsNotFound(err) {
		return nil, err
	}
	if err := mcpEnsureNamespace(cfg, args.Namespace); err != nil {
		return nil, err
	}

	created, err := deployDevbox(cfg, deployDevboxRequest{
		Namespace:   args.Namespace,
		Name:        args.Name,
		Image:       args.Image,
		Repo:        args.Repo,
		Branch:      args.Branch,
		Setup:       args.Setup,
		DiskSize:    args.Disk,
		CpuLimit:    args.CPU,
		MemoryLimit: args.Memory,
		expiresAt:   expiresAt,
		agent:       mcpCallerOf(ctx).token,
	})
	if err != nil {
		return nil, err
	}

	wait := mcpSandboxDefaultWait
	if args.WaitSeconds != nil {
		wait = mcpWaitDuration(args.WaitSeconds)
	}
	result := mcpCreateSandboxResult{
		Sandbox:  mcpSandboxOf(created.devboxSummary),
		Password: created.Password,
		Rollout:  waitForMCPRollout(ctx, cfg, args.Namespace, args.Name, wait),
	}
	if box, err := mcpSandboxSummary(cfg, args.Namespace, args.Name); err == nil {
		result.Sandbox = mcpSandboxOf(*box)
	}

	switch result.Rollout.State {
	case mcpRolloutLive:
		result.Next = "The sandbox is ready. Run commands with exec_in_sandbox. Give the user its url and password if they want to watch or take over in VS Code."
	case mcpRolloutFailed:
		result.Next = "The sandbox did not start. Read get_app_logs with this name to see why, then delete_sandbox and try again, for example with another image."
	default:
		result.Next = "The sandbox is still starting; a large image takes a while to pull the first time. Call list_sandboxes to follow it."
	}
	return result, nil
}

func mcpSandboxSummary(cfg *rest.Config, namespace, name string) (*devboxSummary, error) {
	boxes, err := listDevboxes(cfg, namespace)
	if err != nil {
		return nil, err
	}
	for _, box := range boxes {
		if box.Name == name {
			return &box, nil
		}
	}
	return nil, fmt.Errorf("no sandbox named %s in namespace %s; list_sandboxes shows the ones there are", name, namespace)
}

func mcpListSandboxes(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		Namespace string `json:"namespace"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	boxes, err := listDevboxes(cfg, strings.TrimSpace(args.Namespace))
	if err != nil {
		return nil, err
	}
	if len(boxes) == 0 {
		return "There are no sandboxes yet. Use create_sandbox to create one.", nil
	}
	result := make([]mcpSandbox, 0, len(boxes))
	for _, box := range boxes {
		result = append(result, mcpSandboxOf(box))
	}
	return result, nil
}

func mcpSandboxDeployment(cfg *rest.Config, ref *mcpAppRef) (*appsv1.Deployment, error) {
	if err := ref.normalize(); err != nil {
		return nil, err
	}
	depl, err := object.GetDeployment(cfg, ref.Namespace, ref.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no sandbox named %s in namespace %s; list_sandboxes shows the ones there are", ref.Name, ref.Namespace)
		}
		return nil, err
	}
	if depl.Labels[devboxLabel] != "true" {
		return nil, fmt.Errorf("%s is an app, not a sandbox", ref.Name)
	}
	return depl, nil
}

// sandboxPod is the running workspace pod, where commands and file access go.
func sandboxPod(cfg *rest.Config, depl *appsv1.Deployment) (*corev1.Pod, error) {
	if depl.Spec.Replicas != nil && *depl.Spec.Replicas == 0 {
		return nil, fmt.Errorf("sandbox %s is stopped; the user can start it again from the DevBoxes page", depl.Name)
	}
	pods, err := mcpDeploymentPods(cfg, depl)
	if err != nil {
		return nil, err
	}
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || len(pod.Spec.Containers) == 0 {
			continue
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == pod.Spec.Containers[0].Name && status.State.Running != nil {
				return pod, nil
			}
		}
	}
	return nil, fmt.Errorf("sandbox %s is not running yet; list_sandboxes shows its status", depl.Name)
}

// tailBuffer keeps the last limit bytes written to it, where the error that
// ended a long build usually is.
type tailBuffer struct {
	limit   int
	buf     []byte
	dropped int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.limit; over > 0 {
		b.dropped += over
		b.buf = append([]byte(nil), b.buf[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	text := strings.ToValidUTF8(string(b.buf), "")
	if b.dropped > 0 {
		return fmt.Sprintf("(%d earlier bytes truncated)\n%s", b.dropped, text)
	}
	return text
}

// execInSandbox runs command in the pod's first container. A command that exits
// non-zero is a result, not an error.
func execInSandbox(ctx context.Context, cfg *rest.Config, pod *corev1.Pod, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return execInContainer(ctx, cfg, pod, pod.Spec.Containers[0].Name, command, stdin, stdout, stderr)
}

func execInContainer(ctx context.Context, cfg *rest.Config, pod *corev1.Pod, container string, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return 0, err
	}
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(pod.Name).Namespace(pod.Namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return 0, err
	}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: stderr})
	var exitErr utilexec.ExitError
	if stderrors.As(err, &exitErr) {
		return exitErr.ExitStatus(), nil
	}
	return 0, err
}

func sandboxPath(folder, name string) string {
	name = strings.TrimSpace(name)
	switch {
	case name == "" || name == ".":
		return folder
	case name == "~":
		return devboxHomeMount
	case strings.HasPrefix(name, "~/"):
		return path.Join(devboxHomeMount, name[2:])
	case path.IsAbs(name):
		return path.Clean(name)
	}
	return path.Join(folder, name)
}

func mcpExecInSandbox(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		Command        string  `json:"command"`
		Workdir        string  `json:"workdir"`
		Stdin          *string `json:"stdin"`
		TimeoutSeconds *int    `json:"timeout_seconds"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	depl, err := mcpSandboxDeployment(cfg, &args.mcpAppRef)
	if err != nil {
		return nil, err
	}
	pod, err := sandboxPod(cfg, depl)
	if err != nil {
		return nil, err
	}

	timeout := mcpExecDefaultTimeout
	if args.TimeoutSeconds != nil {
		timeout = *args.TimeoutSeconds
	}
	if timeout < 1 || timeout > mcpExecMaxTimeout {
		return nil, fmt.Errorf("timeout_seconds must be between 1 and %d; use start_sandbox_job for longer work", mcpExecMaxTimeout)
	}
	// Past timeout(1)'s own deadline, for an image that does not have it.
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout+15)*time.Second)
	defer cancel()

	var stdin io.Reader
	if args.Stdin != nil {
		stdin = strings.NewReader(*args.Stdin)
	}
	stdout := &tailBuffer{limit: mcpExecMaxOutput}
	stderr := &tailBuffer{limit: mcpExecMaxOutput}
	command := []string{"/bin/sh", "-c", sandboxExecScript, "sh", sandboxPath(devboxFolder(*depl), args.Workdir), strconv.Itoa(timeout), devboxRunPrelude + args.Command}
	code, err := execInSandbox(execCtx, cfg, pod, command, stdin, stdout, stderr)
	timedOut := code == 124 || execCtx.Err() == context.DeadlineExceeded
	if err != nil && !timedOut {
		return nil, err
	}

	var out strings.Builder
	switch {
	case timedOut:
		fmt.Fprintf(&out, "timed out after %d seconds; use start_sandbox_job for longer work\n", timeout)
	default:
		fmt.Fprintf(&out, "exit code %d\n", code)
	}
	if text := stdout.String(); text != "" {
		out.WriteString("\n[stdout]\n" + strings.TrimRight(text, "\n") + "\n")
	}
	if text := stderr.String(); text != "" {
		out.WriteString("\n[stderr]\n" + strings.TrimRight(text, "\n") + "\n")
	}
	return out.String(), nil
}

func mcpReadSandboxFile(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		Path     string `json:"path"`
		MaxBytes *int   `json:"max_bytes"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	limit := mcpReadDefaultBytes
	if args.MaxBytes != nil {
		limit = *args.MaxBytes
	}
	if limit < 1 || limit > mcpReadMaxBytes {
		return nil, fmt.Errorf("max_bytes must be between 1 and %d", mcpReadMaxBytes)
	}
	depl, err := mcpSandboxDeployment(cfg, &args.mcpAppRef)
	if err != nil {
		return nil, err
	}
	pod, err := sandboxPod(cfg, depl)
	if err != nil {
		return nil, err
	}

	file := sandboxPath(devboxFolder(*depl), args.Path)
	var stdout, stderr bytes.Buffer
	code, err := execInSandbox(ctx, cfg, pod, []string{"/bin/sh", "-c", sandboxReadScript, "sh", file, strconv.Itoa(limit)}, nil, &stdout, &stderr)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	sizeLine, content, _ := bytes.Cut(stdout.Bytes(), []byte("\n"))
	size, _ := strconv.Atoi(strings.TrimSpace(string(sizeLine)))
	var note string
	if size > len(content) {
		note = fmt.Sprintf("\n(truncated: the first %d of %d bytes; pass a larger max_bytes, at most %d)", len(content), size, mcpReadMaxBytes)
	}
	if utf8.Valid(content) {
		return string(content) + note, nil
	}
	return fmt.Sprintf("%s is a binary file of %d bytes; base64:\n%s%s", file, size, base64.StdEncoding.EncodeToString(content), note), nil
}

func mcpWriteSandboxFile(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Append   bool   `json:"append"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	content := []byte(args.Content)
	switch args.Encoding {
	case "", "text":
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(args.Content)
		if err != nil {
			return nil, fmt.Errorf("content is not valid base64: %v", err)
		}
		content = decoded
	default:
		return nil, fmt.Errorf("encoding must be text or base64")
	}
	depl, err := mcpSandboxDeployment(cfg, &args.mcpAppRef)
	if err != nil {
		return nil, err
	}
	pod, err := sandboxPod(cfg, depl)
	if err != nil {
		return nil, err
	}

	file := sandboxPath(devboxFolder(*depl), args.Path)
	mode := "replace"
	if args.Append {
		mode = "append"
	}
	var stderr bytes.Buffer
	code, err := execInSandbox(ctx, cfg, pod, []string{"/bin/sh", "-c", sandboxWriteScript, "sh", file, mode}, bytes.NewReader(content), io.Discard, &stderr)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("could not write %s: %s", file, strings.TrimSpace(stderr.String()))
	}
	if args.Append {
		return fmt.Sprintf("appended %d bytes to %s", len(content), file), nil
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), file), nil
}

func mcpStartSandboxJob(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		Command string  `json:"command"`
		GPU     bool    `json:"gpu"`
		CPU     *string `json:"cpu"`
		Memory  *string `json:"memory"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if _, err := mcpSandboxDeployment(cfg, &args.mcpAppRef); err != nil {
		return nil, err
	}
	run, err := startDevboxRun(cfg, runDevboxRequest{
		Namespace:   args.Namespace,
		Devbox:      args.Name,
		Command:     args.Command,
		Gpu:         args.GPU,
		CpuLimit:    args.CPU,
		MemoryLimit: args.Memory,
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"job":  run,
		"next": fmt.Sprintf("Call get_sandbox_job with job %q to follow it.", run.Name),
	}, nil
}

func mcpGetSandboxJob(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		Job       string `json:"job"`
		Namespace string `json:"namespace"`
		TailLines *int64 `json:"tail_lines"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	namespace := strings.TrimSpace(args.Namespace)
	if namespace == "" {
		namespace = "default"
	}
	tail := int64(100)
	if args.TailLines != nil && *args.TailLines > 0 {
		tail = *args.TailLines
	}
	if tail > 1000 {
		tail = 1000
	}

	job, err := object.GetJob(cfg, namespace, strings.TrimSpace(args.Job))
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no job named %s in namespace %s; a finished job is removed after 7 days", args.Job, namespace)
		}
		return nil, err
	}
	if job.Labels[devboxRunLabel] == "" {
		return nil, fmt.Errorf("%s is not a sandbox job", args.Job)
	}
	pods, _ := object.GetPods(cfg, namespace)
	run := devboxRunOf(*job, pods)

	var out strings.Builder
	fmt.Fprintf(&out, "job %s in sandbox %s: %s\n", run.Name, run.Devbox, run.Status)
	if run.Message != "" {
		fmt.Fprintf(&out, "%s\n", run.Message)
	}
	fmt.Fprintf(&out, "command: %s\ngpu: %t\n", run.Command, run.Gpu)
	if run.StartedAt != "" {
		fmt.Fprintf(&out, "started: %s UTC\n", run.StartedAt)
	}
	if run.FinishedAt != "" {
		fmt.Fprintf(&out, "finished: %s UTC\n", run.FinishedAt)
	}
	if run.PodName == "" || run.StartedAt == "" {
		return out.String(), nil
	}
	logs, err := object.GetPodLogsWithOptions(cfg, namespace, run.PodName, corev1.PodLogOptions{Container: devboxRunContainer, TailLines: &tail})
	if err != nil {
		fmt.Fprintf(&out, "\n(output unavailable: %v)\n", err)
		return out.String(), nil
	}
	if len(logs) > mcpMaxLogBytes {
		logs = "(earlier output truncated)\n" + logs[len(logs)-mcpMaxLogBytes:]
	}
	if strings.TrimSpace(logs) == "" {
		logs = "(no output yet)"
	}
	fmt.Fprintf(&out, "\n[output]\n%s\n", strings.TrimRight(logs, "\n"))
	return out.String(), nil
}

// leasedSandbox refuses a box a person made or chose to keep: an agent may
// work in one, but not decide when it goes away.
func leasedSandbox(cfg *rest.Config, ref *mcpAppRef) (*appsv1.Deployment, error) {
	depl, err := mcpSandboxDeployment(cfg, ref)
	if err != nil {
		return nil, err
	}
	if _, ok := devboxExpiry(depl.ObjectMeta); !ok || depl.Annotations[devboxAgentAnnotation] == "" {
		return nil, fmt.Errorf("%s was made or kept by a person, so only they can change or delete it, from the DevBoxes page", ref.Name)
	}
	return depl, nil
}

func mcpExtendSandbox(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		TTLMinutes *int `json:"ttl_minutes"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if args.TTLMinutes == nil {
		return nil, fmt.Errorf("ttl_minutes is required")
	}
	expiresAt, err := mcpTTL(args.TTLMinutes, 0)
	if err != nil {
		return nil, err
	}
	if _, err := leasedSandbox(cfg, &args.mcpAppRef); err != nil {
		return nil, err
	}
	if err := setDevboxExpiry(cfg, args.Namespace, args.Name, expiresAt); err != nil {
		return nil, err
	}
	return fmt.Sprintf("sandbox %s is now deleted at %s", args.Name, expiresAt.UTC().Format(time.RFC3339)), nil
}

func mcpDeleteSandbox(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args mcpAppRef
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if _, err := leasedSandbox(cfg, &args); err != nil {
		return nil, err
	}
	if err := uninstallImageApp(cfg, args.Namespace, args.Name, true); err != nil {
		return nil, err
	}
	return fmt.Sprintf("deleted sandbox %s with its disk", args.Name), nil
}
