package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterAdmissionHandler mounts the ValidatingAdmissionWebhook endpoint on mux.
func RegisterAdmissionHandler(mux *http.ServeMux) {
	mux.HandleFunc("/admission/validate", admissionValidateHandler)
}

// enforceAdmission is the policy check the handler runs; a seam so the tests
// can assert which requests reach the enforcer at all.
var enforceAdmission = object.EnforceAdmissionPolicy

// Kubernetes attaches system:authenticated (or system:unauthenticated) to every
// request that reaches the API server, so those two say nothing about who the
// caller is and cannot exempt anyone.
var pseudoGroups = map[string]bool{
	"system:authenticated":   true,
	"system:unauthenticated": true,
}

// isReservedIdentity reports whether a request comes from the cluster itself —
// a kubelet, a control-plane component, a controller's service account — rather
// than from a subject an operator would write admission rules for. Kubernetes
// reserves the "system:" prefix for exactly these, on both usernames and
// groups. Note that the image scan below still applies to them: the pods a
// controller creates are the ones worth scanning, and a chart's workload
// reaches the apiserver through a controller's service account.
func isReservedIdentity(username string, groups []string) bool {
	if strings.HasPrefix(username, "system:") {
		return true
	}
	for _, g := range groups {
		if strings.HasPrefix(g, "system:") && !pseudoGroups[g] {
			return true
		}
	}
	return false
}

func admissionValidateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		http.Error(w, "decode error: "+err.Error(), http.StatusBadRequest)
		return
	}

	req := review.Request
	namespace := req.Namespace
	if namespace == "" {
		namespace = "*"
	}

	// The Casbin admission policy is written by an operator for the people and
	// workloads they administer. The cluster's own components are not in it:
	// nobody writes an allow rule for system:node:<name>, so enforcing the
	// policy against them rejects the cluster's own bookkeeping — a kubelet
	// that cannot CREATE its Node never registers, and the node it runs on
	// shows up NotReady with the reason living only in the kubelet log.
	// The authorization webhook already skips these subjects for the same
	// reason; admission has to agree with it, or narrowing the policy for one
	// user quietly breaks node registration.
	allowed := true
	var err error
	if !isReservedIdentity(req.UserInfo.Username, req.UserInfo.Groups) {
		allowed, err = enforceAdmission(
			req.UserInfo.Username,
			namespace,
			req.Resource.Resource,
			string(req.Operation),
		)
	}

	resp := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Response: &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: allowed && err == nil,
		},
	}

	if !allowed || err != nil {
		// "denied by Casbin policy" told an operator nothing: not who was
		// denied, not what for, and not which policy did it. Spelling out the
		// four request fields makes the message a description of the missing
		// rule, so it can be pasted straight back into the policy editor.
		msg := fmt.Sprintf(
			"denied by Casbin admission policy: no allow rule matches sub=%q ns=%q resource=%q action=%q",
			req.UserInfo.Username, namespace, req.Resource.Resource, string(req.Operation),
		)
		if err != nil {
			msg = "Casbin admission policy could not be evaluated: " + err.Error()
		}
		RecordAdmissionDenial(req.UserInfo.Username, namespace, req.Resource.Resource, string(req.Operation), msg)
		resp.Response.Result = &metav1.Status{Message: msg}
		writeAdmissionResponse(w, resp)
		return
	}

	// Keep the scan cache current from what the cluster actually runs. This
	// never denies the pod — see recordPodImages.
	if req.Resource.Resource == "pods" && (req.Operation == admissionv1.Create || req.Operation == admissionv1.Update) {
		recordPodImages(req.Object.Raw)
	}

	writeAdmissionResponse(w, resp)
}

// recordPodImages keeps the scan cache filled from what actually runs on the
// cluster, and reports images already known to carry CRITICAL findings.
//
// It never denies the pod, and neither does the install path
// (store.ImageVulnerabilityReporter): a scan finding is information for the
// operator, who decides whether to run the image. Both places record what is
// known and let the Trivy scan results page show it.
func recordPodImages(raw []byte) {
	var pod corev1.Pod
	if err := json.Unmarshal(raw, &pod); err != nil {
		return
	}

	var images []string
	for _, c := range pod.Spec.InitContainers {
		images = append(images, c.Image)
	}
	for _, c := range pod.Spec.Containers {
		images = append(images, c.Image)
	}

	for _, image := range images {
		result, err := object.GetTrivyScanResultByImage(image)
		if err != nil {
			logs.Error("trivy cache lookup %s: %v", image, err)
			continue
		}
		if result == nil {
			object.TriggerScan(image)
			continue
		}
		if result.Status == "done" && result.Critical > 0 {
			logs.Warning("image %s has %d CRITICAL vulnerabilities; it is listed under Trivy scan results", image, result.Critical)
		}
	}
}

func writeAdmissionResponse(w http.ResponseWriter, resp *admissionv1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logs.Error("admission response encode: %v", err)
	}
}
