package server

import (
	"encoding/json"
	"net/http"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterAdmissionHandler mounts the ValidatingAdmissionWebhook endpoint on mux.
func RegisterAdmissionHandler(mux *http.ServeMux) {
	mux.HandleFunc("/admission/validate", admissionValidateHandler)
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
	allowed, err := object.EnforceAdmissionPolicy(
		req.UserInfo.Username,
		req.Namespace,
		req.Resource.Resource,
		string(req.Operation),
	)

	resp := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Response: &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: allowed && err == nil,
		},
	}
	if !allowed || err != nil {
		msg := "denied by Casbin policy"
		if err != nil {
			msg = err.Error()
		}
		resp.Response.Result = &metav1.Status{Message: msg}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logs.Error("admission response encode: %v", err)
	}
}
