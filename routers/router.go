package routers

import (
	"github.com/beego/beego"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/controllers"
)

func InitAPI() {
	beego.Router("/api/signin", &controllers.ApiController{}, "POST:Signin")
	beego.Router("/api/signout", &controllers.ApiController{}, "POST:Signout")
	beego.Router("/api/get-account", &controllers.ApiController{}, "GET:GetAccount")
	if conf.GetConfigBool("e2eTestMode") {
		beego.Router("/api/e2e/signin", &controllers.ApiController{}, "POST:E2ESignin")
	}

	beego.Router("/api/get-pods", &controllers.ApiController{}, "GET:GetPods")
	beego.Router("/api/get-pod", &controllers.ApiController{}, "GET:GetPod")
	beego.Router("/api/get-pod-events", &controllers.ApiController{}, "GET:GetPodEvents")
	beego.Router("/api/get-pod-logs", &controllers.ApiController{}, "GET:GetPodLogs")
	beego.Router("/api/add-pod", &controllers.ApiController{}, "POST:AddPod")
	beego.Router("/api/update-pod", &controllers.ApiController{}, "POST:UpdatePod")
	beego.Router("/api/delete-pod", &controllers.ApiController{}, "POST:DeletePod")

	beego.Router("/api/get-nodes", &controllers.ApiController{}, "GET:GetNodes")
	beego.Router("/api/get-node", &controllers.ApiController{}, "GET:GetNode")
	beego.Router("/api/update-node", &controllers.ApiController{}, "POST:UpdateNode")
	beego.Router("/api/delete-node", &controllers.ApiController{}, "POST:DeleteNode")
	beego.Router("/api/get-worker-kubeconfig", &controllers.ApiController{}, "GET:GetWorkerKubeconfig")

	beego.Router("/api/get-namespaces", &controllers.ApiController{}, "GET:GetNamespaces")
	beego.Router("/api/get-namespace", &controllers.ApiController{}, "GET:GetNamespace")
	beego.Router("/api/add-namespace", &controllers.ApiController{}, "POST:AddNamespace")
	beego.Router("/api/update-namespace", &controllers.ApiController{}, "POST:UpdateNamespace")
	beego.Router("/api/delete-namespace", &controllers.ApiController{}, "POST:DeleteNamespace")
	beego.Router("/api/force-delete-namespace", &controllers.ApiController{}, "POST:ForceDeleteNamespace")

	beego.Router("/api/get-serviceaccounts", &controllers.ApiController{}, "GET:GetServiceAccounts")
	beego.Router("/api/get-serviceaccount", &controllers.ApiController{}, "GET:GetServiceAccount")
	beego.Router("/api/add-serviceaccount", &controllers.ApiController{}, "POST:AddServiceAccount")
	beego.Router("/api/update-serviceaccount", &controllers.ApiController{}, "POST:UpdateServiceAccount")
	beego.Router("/api/delete-serviceaccount", &controllers.ApiController{}, "POST:DeleteServiceAccount")

	beego.Router("/api/search-docker-hub-images", &controllers.ApiController{}, "GET:SearchDockerHubImages")
	beego.Router("/api/get-docker-hub-image-tags", &controllers.ApiController{}, "GET:GetDockerHubImageTags")

	beego.Router("/api/get-ingresses", &controllers.ApiController{}, "GET:GetIngresses")
	beego.Router("/api/get-ingress", &controllers.ApiController{}, "GET:GetIngress")
	beego.Router("/api/add-ingress", &controllers.ApiController{}, "POST:AddIngress")
	beego.Router("/api/update-ingress", &controllers.ApiController{}, "POST:UpdateIngress")
	beego.Router("/api/delete-ingress", &controllers.ApiController{}, "POST:DeleteIngress")

	beego.Router("/api/request-le-cert", &controllers.ApiController{}, "POST:RequestLECert")
	beego.Router("/api/upload-cert", &controllers.ApiController{}, "POST:UploadCert")
	beego.Router("/api/get-cert-status", &controllers.ApiController{}, "GET:GetCertStatus")

	// Public — no auth, must be reachable by Let's Encrypt verification servers.
	beego.Router("/.well-known/acme-challenge/:token", &controllers.ApiController{}, "GET:ServeACMEChallenge")

	beego.Router("/api/get-services", &controllers.ApiController{}, "GET:GetServices")
	beego.Router("/api/get-service", &controllers.ApiController{}, "GET:GetService")
	beego.Router("/api/add-service", &controllers.ApiController{}, "POST:AddService")
	beego.Router("/api/update-service", &controllers.ApiController{}, "POST:UpdateService")
	beego.Router("/api/delete-service", &controllers.ApiController{}, "POST:DeleteService")

	beego.Router("/api/get-rolebindings", &controllers.ApiController{}, "GET:GetRoleBindings")
	beego.Router("/api/get-rolebinding", &controllers.ApiController{}, "GET:GetRoleBinding")
	beego.Router("/api/add-rolebinding", &controllers.ApiController{}, "POST:AddRoleBinding")
	beego.Router("/api/update-rolebinding", &controllers.ApiController{}, "POST:UpdateRoleBinding")
	beego.Router("/api/delete-rolebinding", &controllers.ApiController{}, "POST:DeleteRoleBinding")

	beego.Router("/api/get-clusterrolebindings", &controllers.ApiController{}, "GET:GetClusterRoleBindings")
	beego.Router("/api/get-clusterrolebinding", &controllers.ApiController{}, "GET:GetClusterRoleBinding")
	beego.Router("/api/add-clusterrolebinding", &controllers.ApiController{}, "POST:AddClusterRoleBinding")
	beego.Router("/api/update-clusterrolebinding", &controllers.ApiController{}, "POST:UpdateClusterRoleBinding")
	beego.Router("/api/delete-clusterrolebinding", &controllers.ApiController{}, "POST:DeleteClusterRoleBinding")

	beego.Router("/api/get-dashboard", &controllers.ApiController{}, "GET:GetDashboard")
	beego.Router("/api/get-metrics", &controllers.ApiController{}, "GET:GetMetrics")

	beego.Router("/api/get-global-sites", &controllers.ApiController{}, "GET:GetGlobalSites")
	beego.Router("/api/get-sites", &controllers.ApiController{}, "GET:GetSites")
	beego.Router("/api/get-site", &controllers.ApiController{}, "GET:GetSite")
	beego.Router("/api/get-built-in-site", &controllers.ApiController{}, "GET:GetBuiltInSite")
	beego.Router("/api/add-site", &controllers.ApiController{}, "POST:AddSite")
	beego.Router("/api/update-site", &controllers.ApiController{}, "POST:UpdateSite")
	beego.Router("/api/delete-site", &controllers.ApiController{}, "POST:DeleteSite")

	beego.Router("/api/get-global-machines", &controllers.ApiController{}, "GET:GetGlobalMachines")
	beego.Router("/api/get-machine", &controllers.ApiController{}, "GET:GetMachine")
	beego.Router("/api/add-machine", &controllers.ApiController{}, "POST:AddMachine")
	beego.Router("/api/update-machine", &controllers.ApiController{}, "POST:UpdateMachine")
	beego.Router("/api/delete-machine", &controllers.ApiController{}, "POST:DeleteMachine")

	beego.Router("/api/get-configmaps", &controllers.ApiController{}, "GET:GetConfigMaps")
	beego.Router("/api/get-configmap", &controllers.ApiController{}, "GET:GetConfigMap")
	beego.Router("/api/add-configmap", &controllers.ApiController{}, "POST:AddConfigMap")
	beego.Router("/api/update-configmap", &controllers.ApiController{}, "POST:UpdateConfigMap")
	beego.Router("/api/delete-configmap", &controllers.ApiController{}, "POST:DeleteConfigMap")

	beego.Router("/api/get-secrets", &controllers.ApiController{}, "GET:GetSecrets")
	beego.Router("/api/get-secret", &controllers.ApiController{}, "GET:GetSecret")
	beego.Router("/api/add-secret", &controllers.ApiController{}, "POST:AddSecret")
	beego.Router("/api/update-secret", &controllers.ApiController{}, "POST:UpdateSecret")
	beego.Router("/api/delete-secret", &controllers.ApiController{}, "POST:DeleteSecret")

	beego.Router("/api/get-pvcs", &controllers.ApiController{}, "GET:GetPersistentVolumeClaims")
	beego.Router("/api/add-pvc", &controllers.ApiController{}, "POST:AddPersistentVolumeClaim")
	beego.Router("/api/delete-pvc", &controllers.ApiController{}, "POST:DeletePersistentVolumeClaim")

	beego.Router("/api/get-statefulsets", &controllers.ApiController{}, "GET:GetStatefulSets")
	beego.Router("/api/get-statefulset", &controllers.ApiController{}, "GET:GetStatefulSet")
	beego.Router("/api/add-statefulset", &controllers.ApiController{}, "POST:AddStatefulSet")
	beego.Router("/api/update-statefulset", &controllers.ApiController{}, "POST:UpdateStatefulSet")
	beego.Router("/api/delete-statefulset", &controllers.ApiController{}, "POST:DeleteStatefulSet")

	beego.Router("/api/get-deployments", &controllers.ApiController{}, "GET:GetDeployments")
	beego.Router("/api/get-deployment", &controllers.ApiController{}, "GET:GetDeployment")
	beego.Router("/api/add-deployment", &controllers.ApiController{}, "POST:AddDeployment")
	beego.Router("/api/update-deployment", &controllers.ApiController{}, "POST:UpdateDeployment")
	beego.Router("/api/delete-deployment", &controllers.ApiController{}, "POST:DeleteDeployment")
	beego.Router("/api/restart-deployment", &controllers.ApiController{}, "POST:RestartDeployment")

	beego.Router("/api/deploy-app", &controllers.ApiController{}, "POST:DeployApp")
	beego.Router("/api/get-app-templates", &controllers.ApiController{}, "GET:GetAppTemplates")

	beego.Router("/api/get-networkpolicies", &controllers.ApiController{}, "GET:GetNetworkPolicies")
	beego.Router("/api/get-networkpolicy", &controllers.ApiController{}, "GET:GetNetworkPolicy")
	beego.Router("/api/add-networkpolicy", &controllers.ApiController{}, "POST:AddNetworkPolicy")
	beego.Router("/api/update-networkpolicy", &controllers.ApiController{}, "POST:UpdateNetworkPolicy")
	beego.Router("/api/delete-networkpolicy", &controllers.ApiController{}, "POST:DeleteNetworkPolicy")

	beego.Router("/api/get-cronjobs", &controllers.ApiController{}, "GET:GetCronJobs")
	beego.Router("/api/get-cronjob", &controllers.ApiController{}, "GET:GetCronJob")
	beego.Router("/api/add-cronjob", &controllers.ApiController{}, "POST:AddCronJob")
	beego.Router("/api/update-cronjob", &controllers.ApiController{}, "POST:UpdateCronJob")
	beego.Router("/api/delete-cronjob", &controllers.ApiController{}, "POST:DeleteCronJob")
	beego.Router("/api/get-cronjob-jobs", &controllers.ApiController{}, "GET:GetCronJobJobs")
	beego.Router("/api/trigger-cronjob", &controllers.ApiController{}, "POST:TriggerCronJob")

	beego.Router("/api/get-hpas", &controllers.ApiController{}, "GET:GetHPAs")
	beego.Router("/api/get-hpa", &controllers.ApiController{}, "GET:GetHPA")
	beego.Router("/api/add-hpa", &controllers.ApiController{}, "POST:AddHPA")
	beego.Router("/api/update-hpa", &controllers.ApiController{}, "POST:UpdateHPA")
	beego.Router("/api/delete-hpa", &controllers.ApiController{}, "POST:DeleteHPA")

	beego.Router("/api/get-resourcequotas", &controllers.ApiController{}, "GET:GetResourceQuotas")
	beego.Router("/api/get-resourcequota", &controllers.ApiController{}, "GET:GetResourceQuota")
	beego.Router("/api/add-resourcequota", &controllers.ApiController{}, "POST:AddResourceQuota")
	beego.Router("/api/update-resourcequota", &controllers.ApiController{}, "POST:UpdateResourceQuota")
	beego.Router("/api/delete-resourcequota", &controllers.ApiController{}, "POST:DeleteResourceQuota")

	beego.Router("/api/get-aggregated-logs", &controllers.ApiController{}, "GET:GetAggregatedLogs")

	beego.Router("/api/pod-terminal", &controllers.ApiController{}, "GET:PodTerminal")
	beego.Router("/api/pod-file-list", &controllers.ApiController{}, "GET:ListPodFiles")
	beego.Router("/api/pod-file-download", &controllers.ApiController{}, "GET:DownloadPodFile")
	beego.Router("/api/pod-file-upload", &controllers.ApiController{}, "POST:UploadPodFile")

	beego.Router("/api/get-trivy-scan-results", &controllers.ApiController{}, "GET:GetTrivyScanResults")
	beego.Router("/api/trigger-trivy-scan", &controllers.ApiController{}, "POST:TriggerTrivyScan")
	beego.Router("/api/delete-trivy-scan-result", &controllers.ApiController{}, "POST:DeleteTrivyScanResult")

	beego.Router("/api/get-casbin-rules", &controllers.ApiController{}, "GET:GetCasbinRules")
	beego.Router("/api/add-casbin-rule", &controllers.ApiController{}, "POST:AddCasbinRule")
	beego.Router("/api/delete-casbin-rule", &controllers.ApiController{}, "POST:DeleteCasbinRule")
	beego.Router("/api/reload-casbin-enforcer", &controllers.ApiController{}, "POST:ReloadCasbinEnforcer")
}
