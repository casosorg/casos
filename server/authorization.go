package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterAuthorizationHandler mounts the Authorization Webhook endpoint on mux.
func RegisterAuthorizationHandler(mux *http.ServeMux) {
	mux.HandleFunc("/authorization/authorize", authorizationHandler)
}

func authorizationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var review authzv1.SubjectAccessReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		http.Error(w, "decode error: "+err.Error(), http.StatusBadRequest)
		return
	}

	spec := review.Spec

	// Pass non-resource requests and system users through without Casbin enforcement.
	// Node and RBAC authorizers have already had their say; we only supplement policy
	// for regular users on resource requests.
	if spec.NonResourceAttributes != nil || isSystemUser(spec.User, spec.Groups) {
		review.Status = authzv1.SubjectAccessReviewStatus{Allowed: true}
		writeAuthzResponse(w, review)
		return
	}

	ra := spec.ResourceAttributes
	namespace := ra.Namespace
	if namespace == "" {
		namespace = "*"
	}

	allowed, err := object.EnforceAuthorizationPolicy(spec.User, namespace, ra.Resource, ra.Verb)
	if err != nil {
		logs.Error("authz enforce: %v", err)
		// Return no-opinion on error so the default deny takes effect.
		review.Status = authzv1.SubjectAccessReviewStatus{Allowed: false, Denied: false}
	} else {
		review.Status = authzv1.SubjectAccessReviewStatus{
			Allowed: allowed,
			Denied:  !allowed,
		}
		if !allowed {
			review.Status.Reason = "denied by Casbin admission policy"
		}
	}

	writeAuthzResponse(w, review)
}

func isSystemUser(user string, groups []string) bool {
	if strings.HasPrefix(user, "system:") {
		return true
	}
	for _, g := range groups {
		if strings.HasPrefix(g, "system:") {
			return true
		}
	}
	return false
}

func writeAuthzResponse(w http.ResponseWriter, review authzv1.SubjectAccessReview) {
	review.TypeMeta = metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SubjectAccessReview"}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(review); err != nil {
		logs.Error("authz response encode: %v", err)
	}
}
