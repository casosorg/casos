package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/beego/beego/logs"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
)

// Freeze and queue: a workspace is where the work is written, but while it sits
// there idle it holds a GPU nobody is using. Freezing scales the editor to zero
// and submits the same container as a Job — same image, same home disk, same
// files — so the training run starts from exactly what the user was looking at,
// on the hardware the workspace just let go of. When the run ends the workspace
// can come back on its own.
//
// The order matters: the editor is stopped before the Job is created, or on a
// one-GPU node the Job would queue forever behind the workspace that is waiting
// for it to finish.

const (
	devboxJobLabel = "casos.io/devbox-job"
	// Whether the workspace comes back by itself when the run ends, and the mark
	// saying that has already happened — a finished Job is reconciled once.
	devboxThawAnnotation      = "casos.io/thaw-on-finish"
	devboxThawedAnnotation    = "casos.io/thawed"
	devboxRunCommandAnnotaton = "casos.io/run-command"

	devboxDefaultGpuResource = "nvidia.com/gpu"
	// A finished run is kept for a day so the list still answers "what happened
	// last night", then Kubernetes collects it.
	devboxRunTTLSeconds = int32(24 * 60 * 60)
	devboxQueueInterval = 15 * time.Second
)

type freezeDevboxRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Command is a shell line, run in the workspace's folder — its checkout
	// when it has one, home otherwise.
	Command string `json:"command"`
	// Gpu is how many accelerators the run asks for, in GpuResource units
	// (nvidia.com/gpu unless the cluster counts them under another name).
	Gpu         int64   `json:"gpu"`
	GpuResource string  `json:"gpuResource"`
	CpuLimit    *string `json:"cpuLimit"`
	MemoryLimit *string `json:"memoryLimit"`
	// ThawOnFinish brings the workspace back when the run ends, whether it
	// succeeded or failed — the user asked for an editor, not a lottery.
	ThawOnFinish bool `json:"thawOnFinish"`
}

type devboxRunSummary struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Devbox       string `json:"devbox"`
	Command      string `json:"command"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Gpu          int64  `json:"gpu"`
	CreatedAt    string `json:"createdAt"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt"`
	ThawOnFinish bool   `json:"thawOnFinish"`
	PodName      string `json:"podName"`
}

// FreezeDevbox stops a workspace and queues its work as a training Job.
// @router /api/freeze-devbox [post]
func (c *ApiController) FreezeDevbox() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req freezeDevboxRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if strings.TrimSpace(req.Name) == "" {
		c.ResponseError("a DevBox name is required")
		return
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		c.ResponseError("a command is required — this is what the run will execute in the workspace")
		return
	}

	depl, err := object.GetDeployment(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if depl.Labels[devboxLabel] != "true" {
		c.ResponseError(fmt.Sprintf("%s is not a DevBox", req.Name))
		return
	}

	// Two runs would mount the same home disk and write over each other; the
	// workspace is one desk, and the queue is what keeps them taking turns.
	for _, run := range devboxRunsOf(cfg, req.Namespace, req.Name) {
		if run.Status == "queued" || run.Status == "running" {
			c.ResponseError(fmt.Sprintf("%s is already running — cancel it first, or wait for it to finish", run.Name))
			return
		}
	}

	job, err := buildDevboxRunJob(*depl, req, command)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	// Freeze first: the run is queued for the resources the workspace is holding,
	// so it cannot get them until the workspace has let go.
	if err := scaleAppDeployment(cfg, depl, false); err != nil {
		c.ResponseError("the workspace could not be frozen: " + err.Error())
		return
	}
	created, err := object.AddJob(cfg, job)
	if err != nil {
		// Nothing was queued, so leave the user with the workspace they had.
		if thawErr := thawDevboxDeployment(cfg, req.Namespace, req.Name); thawErr != nil {
			logs.Warning("devbox %s/%s: queueing failed and the workspace could not be restarted: %v", req.Namespace, req.Name, thawErr)
		}
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(devboxRunOf(*created, ""))
}

// GetDevboxRuns lists the runs queued from a workspace, newest first.
// @router /api/get-devbox-runs [get]
func (c *ApiController) GetDevboxRuns() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	devbox := c.GetString("devbox")

	jobs, err := object.GetJobs(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	pods, _ := object.GetPods(cfg, namespace)

	result := []devboxRunSummary{}
	for _, job := range jobs {
		owner := job.Labels[devboxJobLabel]
		if owner == "" || (devbox != "" && owner != devbox) {
			continue
		}
		result = append(result, devboxRunOf(job, runPodName(pods, job.Name)))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	c.ResponseOk(result)
}

// devboxRunsOf is every run queued from one workspace.
func devboxRunsOf(cfg *rest.Config, namespace, devbox string) []devboxRunSummary {
	result := []devboxRunSummary{}
	jobs, err := object.GetJobs(cfg, namespace)
	if err != nil {
		return result
	}
	for _, job := range jobs {
		if job.Labels[devboxJobLabel] == devbox {
			result = append(result, devboxRunOf(job, ""))
		}
	}
	return result
}

// latestDevboxRuns is the newest run of each workspace, which is the only one a
// list has room to say anything about.
func latestDevboxRuns(cfg *rest.Config, namespace string) map[string]devboxRunSummary {
	result := map[string]devboxRunSummary{}
	jobs, err := object.GetJobs(cfg, namespace)
	if err != nil {
		return result
	}
	for _, job := range jobs {
		devbox := job.Labels[devboxJobLabel]
		if devbox == "" {
			continue
		}
		run := devboxRunOf(job, "")
		if previous, ok := result[devbox]; ok && previous.CreatedAt >= run.CreatedAt {
			continue
		}
		result[devbox] = run
	}
	return result
}

// CancelDevboxRun stops a run that is queued or in flight, and brings the
// workspace back if that is what the run was told to do when it ended.
// @router /api/cancel-devbox-run [post]
func (c *ApiController) CancelDevboxRun() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	job, err := object.GetJob(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	devbox := job.Labels[devboxJobLabel]
	if devbox == "" {
		c.ResponseError(fmt.Sprintf("%s is not a DevBox run", req.Name))
		return
	}
	if err := object.DeleteJob(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if job.Annotations[devboxThawAnnotation] == "true" {
		if err := thawDevboxDeployment(cfg, req.Namespace, devbox); err != nil {
			c.ResponseError("the run was cancelled but the workspace could not be restarted: " + err.Error())
			return
		}
	}
	c.ResponseOk()
}

func thawDevboxDeployment(cfg *rest.Config, namespace, name string) error {
	depl, err := object.GetDeployment(cfg, namespace, name)
	if err != nil {
		return err
	}
	return scaleAppDeployment(cfg, depl, true)
}

// buildDevboxRunJob turns the workspace into a batch of one: the same image and
// the same home disk, so the run sees the files as they were left, and none of
// the editor's own plumbing, which a training run has no use for.
func buildDevboxRunJob(depl appsv1.Deployment, req freezeDevboxRequest, command string) (*batchv1.Job, error) {
	editor := devboxEditorContainer(depl)
	if editor == nil {
		return nil, fmt.Errorf("the workspace has no container to run from")
	}

	container := corev1.Container{
		Name:  "run",
		Image: editor.Image,
		// The same PATH the editor's terminal has, so a tool pip installed for
		// the user runs here as it did there.
		Command:      []string{"/bin/sh", "-lc", `export PATH="$HOME/.local/bin:$PATH"` + "\n" + command},
		WorkingDir:   devboxFolder(depl),
		VolumeMounts: runVolumeMounts(editor.VolumeMounts),
		Env:          runEnv(editor.Env),
		// The run writes into the same home as the editor, so it has to be the
		// same user, or the next time the workspace opens its outputs are read-only.
		SecurityContext: editor.SecurityContext,
	}
	if err := applyResources(&container, resourceRequest{CpuLimit: req.CpuLimit, MemoryLimit: req.MemoryLimit}); err != nil {
		return nil, err
	}
	if req.Gpu > 0 {
		gpuResource := corev1.ResourceName(strings.TrimSpace(req.GpuResource))
		if gpuResource == "" {
			gpuResource = devboxDefaultGpuResource
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		container.Resources.Limits[gpuResource] = *resource.NewQuantity(req.Gpu, resource.DecimalSI)
	}

	backoff := int32(0)
	ttl := devboxRunTTLSeconds
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devboxRunName(depl.Name),
			Namespace: depl.Namespace,
			Labels:    map[string]string{devboxJobLabel: depl.Name},
			Annotations: map[string]string{
				devboxRunCommandAnnotaton: command,
				devboxThawAnnotation:      fmt.Sprintf("%t", req.ThawOnFinish),
			},
		},
		Spec: batchv1.JobSpec{
			// A training run that died is a result, not something to quietly try
			// again with the same inputs and the same GPU.
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{devboxJobLabel: depl.Name}},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{container},
					Volumes:       depl.Spec.Template.Spec.Volumes,
					NodeSelector:  depl.Spec.Template.Spec.NodeSelector,
					Tolerations:   depl.Spec.Template.Spec.Tolerations,
				},
			},
		},
	}
	return job, nil
}

// devboxEditorContainer is the code-server container: the one the workspace was
// built from, and the one whose image and disks the run inherits. Anything
// beside it — the SSH server, for one — belongs to the editor, not to the work.
func devboxEditorContainer(depl appsv1.Deployment) *corev1.Container {
	if len(depl.Spec.Template.Spec.Containers) == 0 {
		return nil
	}
	return &depl.Spec.Template.Spec.Containers[0]
}

// runEnv keeps the workspace's environment minus what only the editor and its
// SSH server use: the run serves nothing, so handing it those buys nothing.
func runEnv(env []corev1.EnvVar) []corev1.EnvVar {
	return withoutEnv(env, "PASSWORD", "CASOS_SSH_PUBLIC_KEY")
}

// runVolumeMounts leaves out the editor's tools volume. The run has no init
// containers to fill it, and an /etc/passwd mounted from a path that does not
// exist would be created as a directory and stop the container starting.
func runVolumeMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	result := []corev1.VolumeMount{}
	for _, mount := range mounts {
		if mount.Name != devboxToolsVolume {
			result = append(result, mount)
		}
	}
	return result
}

func devboxRunName(devbox string) string {
	return fmt.Sprintf("%s-run-%s", devbox, time.Now().UTC().Format("20060102-150405"))
}

func runPodName(pods []corev1.Pod, job string) string {
	for _, pod := range pods {
		if pod.Labels["job-name"] == job {
			return pod.Name
		}
	}
	return ""
}

func devboxRunOf(job batchv1.Job, podName string) devboxRunSummary {
	run := devboxRunSummary{
		Name:         job.Name,
		Namespace:    job.Namespace,
		Devbox:       job.Labels[devboxJobLabel],
		Command:      job.Annotations[devboxRunCommandAnnotaton],
		Status:       "queued",
		ThawOnFinish: job.Annotations[devboxThawAnnotation] == "true",
		CreatedAt:    job.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		PodName:      podName,
	}
	if job.Status.StartTime != nil {
		run.StartedAt = job.Status.StartTime.UTC().Format("2006-01-02 15:04:05")
	}
	if job.Status.Active > 0 {
		run.Status = "running"
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			run.Status = "succeeded"
		case batchv1.JobFailed:
			run.Status = "failed"
			run.Message = condition.Message
		}
	}
	if job.Status.CompletionTime != nil {
		run.FinishedAt = job.Status.CompletionTime.UTC().Format("2006-01-02 15:04:05")
	}
	for _, container := range job.Spec.Template.Spec.Containers {
		for name, quantity := range container.Resources.Limits {
			if strings.Contains(strings.ToLower(string(name)), "gpu") {
				run.Gpu += quantity.Value()
			}
		}
	}
	return run
}

func devboxRunFinished(job batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		if condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed {
			return true
		}
	}
	return false
}

// StartDevboxQueue watches for runs that have ended and gives their workspace
// back. It is a loop rather than anything the cluster does on its own: nothing
// in Kubernetes connects a Job finishing to a Deployment starting, and the
// point of freezing is that the user does not have to come back and undo it.
func StartDevboxQueue(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(devboxQueueInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcileDevboxQueue()
			}
		}
	}()
}

func reconcileDevboxQueue() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		return
	}
	jobs, err := object.GetJobs(cfg, "")
	if err != nil {
		return
	}
	for i := range jobs {
		job := jobs[i]
		devbox := job.Labels[devboxJobLabel]
		if devbox == "" || job.Annotations[devboxThawAnnotation] != "true" {
			continue
		}
		if job.Annotations[devboxThawedAnnotation] == "true" || !devboxRunFinished(job) {
			continue
		}
		if err := thawDevboxDeployment(cfg, job.Namespace, devbox); err != nil {
			logs.Warning("devbox %s/%s: run %s ended but the workspace could not be restarted: %v", job.Namespace, devbox, job.Name, err)
			continue
		}
		// Marked on the Job rather than remembered in this process, so a restart
		// of CasOS does not thaw every workspace whose run ended while it was down.
		applyAnnotation(&job.ObjectMeta, devboxThawedAnnotation, "true")
		if _, err := object.UpdateJob(cfg, &job); err != nil {
			logs.Warning("devbox run %s/%s: could not record that its workspace was restarted: %v", job.Namespace, job.Name, err)
		}
	}
}
