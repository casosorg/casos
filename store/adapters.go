package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmChartAdapter struct {
	valuesPatches map[string]interface{}
	// chartValuesPatchFn builds patches that depend on the chart's own layout —
	// where this particular fork keeps a value — and on what the user already
	// wrote. Its result is deterministic, so unlike generatedValuesPatchFn the
	// install dialog previews it.
	chartValuesPatchFn func(ch *chart.Chart, explicitValues map[string]interface{}) map[string]interface{}
	// warningFn explains an adapter default the user should weigh before
	// installing. It reads the rendered values, so a user who already changed
	// the value back is not warned about it.
	warningFn func(values map[string]interface{}) string
	// generatedValuesPatchFn mints per-install values such as a random secret.
	// The values preview skips it so the install dialog stays reproducible.
	generatedValuesPatchFn func(explicitValues map[string]interface{}) (map[string]interface{}, error)
	// clusterValuesPatchFn builds patches out of cluster state the values
	// preview cannot know — a node address, a free node port. Like
	// generatedValuesPatchFn the preview skips it, so the dialog stays
	// reproducible; unlike it, this one reaches the API server.
	clusterValuesPatchFn func(cluster clusterContext) (map[string]interface{}, error)
	// clusterHostBindings publish the cluster addresses into a chart's host
	// allowlist. The values preview skips them: they depend on cluster state.
	clusterHostBindings []clusterHostBinding
	// preservedValuePaths are carried over from the installed release on upgrade.
	preservedValuePaths [][]string
}

// clusterHostBinding names where a chart keeps the hostnames its app answers
// to. CasOS publishes app store installs as NodePort services, so users reach
// them through a cluster node address the chart cannot know; apps that check
// the Host header reject those requests outright (Nextcloud's trusted_domains
// with a 400, Django's ALLOWED_HOSTS, Grafana's server.domain with broken
// links). A binding is data, not code: supporting another app means naming a
// values path and a format.
type clusterHostBinding struct {
	// path is where the assembled host list is written.
	path []string
	// sourcePaths hold hosts the app must keep answering to, in priority
	// order — typically its configured domain, then the destination's current
	// value, then route hostnames. Each is read from the install values and
	// from the chart defaults, and may hold a string or a list of strings.
	sourcePaths [][]string
	// render turns the host list into the shape the chart expects.
	render func(hosts []string) interface{}
}

// clusterContext carries install-time facts the values preview cannot know.
// nodeIPs is lazy so charts without a binding never trigger the node list call.
type clusterContext struct {
	nodeIPs func() []string
	// usedNodePorts reports every node port already bound anywhere in the
	// cluster, so an adapter that must pin one does not collide.
	usedNodePorts func() map[int32]bool
	// releaseNodePorts maps a Service port of this release to the node port it
	// already holds, so an upgrade keeps the address users bookmarked.
	releaseNodePorts func() map[int32]int32
	releaseName      string
	namespace        string
}

// Kubernetes' default --service-node-port-range. Ports outside it are rejected
// by the apiserver, so an adapter that pins one has to stay inside.
const (
	clusterNodePortRangeStart = 30000
	clusterNodePortRangeEnd   = 32767
)

// firstNodeIP returns an address a user can reach the cluster on, or "".
func (cluster clusterContext) firstNodeIP() string {
	if cluster.nodeIPs == nil {
		return ""
	}
	for _, ip := range cluster.nodeIPs() {
		if ip != "" {
			return ip
		}
	}
	return ""
}

// reserveNodePort returns the node port to publish servicePort on: the one this
// release already holds if it has one, otherwise the lowest free port in the
// range. Zero means the cluster could not be read, and the caller should leave
// the port to Kubernetes rather than guess.
func (cluster clusterContext) reserveNodePort(servicePort int32) int32 {
	if cluster.releaseNodePorts != nil {
		if assigned := cluster.releaseNodePorts()[servicePort]; assigned != 0 {
			return assigned
		}
	}
	if cluster.usedNodePorts == nil {
		return 0
	}
	used := cluster.usedNodePorts()
	if used == nil {
		return 0
	}
	for port := int32(clusterNodePortRangeStart); port <= clusterNodePortRangeEnd; port++ {
		if !used[port] {
			return port
		}
	}
	return 0
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"argo-cd":               {valuesPatches: argoCDValuesPatches()},
	"cert-manager":          {valuesPatches: certManagerValuesPatches()},
	"grafana":               {valuesPatches: nodePortServiceValuesPatch()},
	"ingress-nginx":         {valuesPatches: ingressNginxValuesPatches()},
	"kube-prometheus-stack": {valuesPatches: kubePrometheusStackValuesPatches()},
	"kubernetes-dashboard":  {valuesPatches: kubernetesDashboardValuesPatches()},
	"loki":                  {valuesPatches: lokiValuesPatches()},
	"metrics-server":        {valuesPatches: metricsServerValuesPatches()},
	"pgadmin4":              {valuesPatches: nodePortServiceValuesPatch()},
	"postgresql":            {valuesPatches: postgresqlValuesPatches()},
	"prometheus":            {valuesPatches: prometheusValuesPatches()},
	"redis":                 {valuesPatches: redisValuesPatches()},
	"traefik":               {valuesPatches: traefikValuesPatches()},
	"vault": {
		valuesPatches: vaultValuesPatches(),
		warningFn:     vaultDevModeWarning,
	},
	"harbor": {
		valuesPatches:        harborValuesPatches(),
		clusterValuesPatchFn: harborClusterValuesPatch,
	},
	"keycloak": {valuesPatches: nodePortServiceValuesPatch()},
	"jenkins":  {valuesPatches: jenkinsValuesPatches()},
	"longhorn": {valuesPatches: longhornValuesPatches()},
	"rabbitmq": {valuesPatches: nodePortServiceValuesPatch()},
	"gitlab": {
		valuesPatches: gitLabValuesPatches(),
		warningFn:     gitLabExternalDependencyWarning,
	},
	"n8n": {
		valuesPatches:      nodePortServiceValuesPatch(),
		chartValuesPatchFn: n8nSecureCookiePatch,
		warningFn:          n8nSecureCookieWarning,
	},
	"superset": {
		valuesPatches:          supersetValuesPatches(),
		generatedValuesPatchFn: supersetSecretKeyPatch,
		preservedValuePaths:    supersetPreservedValuePaths,
	},
	"nextcloud": {
		valuesPatches:       nodePortServiceValuesPatch(),
		clusterHostBindings: nextcloudTrustedDomainsBinding,
	},
}

// nextcloudTrustedDomainsBinding keeps Nextcloud reachable through the node
// address the NodePort patch above publishes: any Host outside trusted_domains
// gets a 400 "Access through untrusted domain". The chart templates this path
// into NEXTCLOUD_TRUSTED_DOMAINS, and doing so drops its own default of
// nextcloud.host — the Host header its probes send to /status.php — so that
// name heads the source list.
var nextcloudTrustedDomainsBinding = []clusterHostBinding{{
	path: []string{"nextcloud", "trustedDomains"},
	sourcePaths: [][]string{
		{"nextcloud", "host"},
		{"nextcloud", "trustedDomains"},
		{"httpRoute", "hostnames"},
	},
	render: helmHostStringList,
}}

// nodePortServiceValuesPatch returns a fresh patch each call so the registry
// never shares mutable state.
func nodePortServiceValuesPatch() map[string]interface{} {
	return map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
}

func argoCDValuesPatches() map[string]interface{} {
	return helmValuePatch([]string{"server", "service"}, map[string]interface{}{"type": "NodePort"})
}

func certManagerValuesPatches() map[string]interface{} {
	return helmValuePatch([]string{"crds"}, map[string]interface{}{"enabled": true})
}

// metricsServerValuesPatches lets metrics-server scrape the kubelet that CasOS
// provisions. That kubelet's serving certificate is intentionally local and
// does not carry the WSL node IP as a SAN, so the upstream default can start
// but never becomes ready (there are no metrics to serve).
func metricsServerValuesPatches() map[string]interface{} {
	return map[string]interface{}{
		"args": []interface{}{"--kubelet-insecure-tls"},
	}
}

// vaultValuesPatches also starts Vault in dev mode. A standalone Vault comes up
// sealed and uninitialised, so its readiness probe never passes: the pod sits at
// 0/1 for good, CasOS reports the app as broken, and the UI only answers at all
// because the chart's Service publishes not-ready addresses. Dev mode unseals
// itself, which is what makes a one-click install usable — at the cost this
// adapter's warning spells out.
func vaultValuesPatches() map[string]interface{} {
	patches := helmValuePatch([]string{"server", "service"}, map[string]interface{}{"type": "NodePort"})
	server, ok := patches["server"].(map[string]interface{})
	if !ok {
		return patches
	}
	server["dev"] = map[string]interface{}{"enabled": true}
	return patches
}

// vaultDevModeWarning states what dev mode costs, so the install dialog shows
// why the value is there and when to turn it off.
func vaultDevModeWarning(values map[string]interface{}) string {
	if !helmValueIsTrue(helmValueAtPath(values, []string{"server", "dev", "enabled"})) {
		return ""
	}
	return "CasOS starts Vault in dev mode so it comes up unsealed and the web UI is usable right away; dev mode keeps everything in memory and every restart loses all secrets, so turn it off and initialise Vault yourself before storing anything real"
}

// helmValueIsTrue reports whether a values leaf holds true in either the boolean
// or the string form Helm accepts for it.
func helmValueIsTrue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	}
	return false
}

func harborValuesPatches() map[string]interface{} {
	return harborExposePatches(0, "")
}

// harborExposePatches publishes Harbor on a node port. A zero httpNodePort
// leaves the port for Kubernetes to allocate, which is what the values preview
// shows; the install pins one so externalURL can name it.
func harborExposePatches(httpNodePort int32, externalURL string) map[string]interface{} {
	var httpPort interface{}
	if httpNodePort > 0 {
		httpPort = httpNodePort
	}
	patches := map[string]interface{}{
		"expose": map[string]interface{}{
			"type": "nodePort",
			"tls":  map[string]interface{}{"enabled": false},
			"nodePort": map[string]interface{}{
				"ports": map[string]interface{}{
					"http":  map[string]interface{}{"nodePort": httpPort},
					"https": map[string]interface{}{"nodePort": nil},
				},
			},
		},
	}
	if externalURL != "" {
		patches["externalURL"] = externalURL
	}
	return patches
}

// harborClusterValuesPatch points Harbor at the address users actually reach it
// on. Harbor is a registry, and externalURL is what it prints in every docker
// login and docker push command and uses for its own redirects — left at the
// chart default it advertises https://core.harbor.domain, a name nothing
// resolves, so the web UI works while the registry cannot be used at all.
//
// Naming the port means pinning it, because the URL has to be known before the
// Service exists. An upgrade reuses the port Harbor already has, so the URL a
// user bookmarked keeps working.
func harborClusterValuesPatch(cluster clusterContext) (map[string]interface{}, error) {
	host := cluster.firstNodeIP()
	if host == "" {
		return nil, nil
	}
	port := cluster.reserveNodePort(harborHTTPServicePort)
	if port == 0 {
		return nil, nil
	}
	return harborExposePatches(port, fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(int(port))))), nil
}

// harborHTTPServicePort is the Service port Harbor's chart publishes over plain
// HTTP; the node port bound to it is the one externalURL has to name.
const harborHTTPServicePort = 80

func jenkinsValuesPatches() map[string]interface{} {
	return helmValuePatch([]string{"controller"}, map[string]interface{}{
		"serviceType": "NodePort",
		"nodePort":    nil,
	})
}

// longhornValuesPatches also confirms deletion up front. Longhorn's uninstall
// Job refuses to run unless the deleting-confirmation-flag setting is true, and
// that setting only exists once longhorn-manager has come up — so an operator
// whose Longhorn never started cannot remove it at all. The Job then fails with
// BackoffLimitExceeded, and the release wedges in "uninstalling", a state the
// App Store offers no way to clear.
func longhornValuesPatches() map[string]interface{} {
	patches := helmValuePatch([]string{"service", "ui"}, map[string]interface{}{
		"type":     "NodePort",
		"nodePort": nil,
	})
	patches["defaultSettings"] = map[string]interface{}{"deletingConfirmationFlag": true}
	return patches
}

// GitLab 19 enables its Gateway API cert-manager integration by default, but
// the chart deliberately leaves the ACME account email empty. CasOS does not
// own an email address it can safely submit to a certificate authority, so the
// local App Store install disables that integration instead of failing Helm's
// render preflight before creating any resources.
func gitLabValuesPatches() map[string]interface{} {
	return map[string]interface{}{
		"installCertmanager": false,
		"global": map[string]interface{}{
			"gatewayApi": map[string]interface{}{"configureCertmanager": false},
			"ingress":    map[string]interface{}{"configureCertmanager": false},
		},
	}
}

// gitLabExternalDependencyWarning says what the chart needs before the user
// spends a click on it. Since chart v10 GitLab dropped its bundled PostgreSQL
// and Redis and requires external ones, plus object storage for the registry,
// so a default install cannot succeed — it fails Helm's render with a wall of
// chart NOTES that reads like an internal error rather than a prerequisite.
// CasOS has nothing to point those settings at, so this states the requirement
// rather than inventing a value.
func gitLabExternalDependencyWarning(values map[string]interface{}) string {
	if helmValueTrimmedString(helmValueAtPath(values, []string{"global", "psql", "host"})) != "" {
		return ""
	}
	return "GitLab's chart no longer bundles PostgreSQL or Redis: set global.psql.host, global.psql.password.secret and global.redis.host to servers you already run, and configure object storage for the registry, or this install will fail before it creates anything"
}

// helmValueTrimmedString reads a values leaf that is expected to hold a string.
func helmValueTrimmedString(value interface{}) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func prometheusExporterCompatibilityPatch() map[string]interface{} {
	return map[string]interface{}{
		"prometheus-node-exporter": map[string]interface{}{
			"hostRootFsMount": map[string]interface{}{"enabled": false},
			"hostNetwork":     false,
			"hostPID":         false,
		},
	}
}

func prometheusValuesPatches() map[string]interface{} {
	patches := prometheusExporterCompatibilityPatch()
	patches["server"] = map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
	return patches
}

func kubePrometheusStackValuesPatches() map[string]interface{} {
	patches := prometheusExporterCompatibilityPatch()
	patches["grafana"] = map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
	patches["prometheus"] = map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
	return patches
}

func kubernetesDashboardValuesPatches() map[string]interface{} {
	return helmValuePatch([]string{"kong", "proxy"}, map[string]interface{}{"type": "NodePort"})
}

func ingressNginxValuesPatches() map[string]interface{} {
	return helmValuePatch([]string{"controller", "service"}, map[string]interface{}{"type": "NodePort"})
}

func redisValuesPatches() map[string]interface{} {
	return map[string]interface{}{
		"architecture": "standalone",
		"master": map[string]interface{}{
			"service": map[string]interface{}{"type": "NodePort"},
		},
	}
}

func postgresqlValuesPatches() map[string]interface{} {
	return helmValuePatch([]string{"primary", "service"}, map[string]interface{}{"type": "NodePort"})
}

func traefikValuesPatches() map[string]interface{} {
	return map[string]interface{}{
		"ingressClass": map[string]interface{}{
			"enabled":        true,
			"isDefaultClass": false,
			"name":           "traefik-app",
		},
		"providers": map[string]interface{}{
			"kubernetesCRD":     map[string]interface{}{"ingressClass": "traefik-app"},
			"kubernetesIngress": map[string]interface{}{"ingressClass": "traefik-app"},
		},
		"service": map[string]interface{}{
			"spec": map[string]interface{}{"type": "NodePort"},
		},
	}
}

func lokiValuesPatches() map[string]interface{} {
	return map[string]interface{}{
		"deploymentMode": "SingleBinary",
		"loki": map[string]interface{}{
			"commonConfig": map[string]interface{}{"replication_factor": 1},
			"schemaConfig": map[string]interface{}{
				"configs": []interface{}{
					map[string]interface{}{
						"from":         "2024-04-01",
						"store":        "tsdb",
						"object_store": "filesystem",
						"schema":       "v13",
						"index": map[string]interface{}{
							"prefix": "loki_index_",
							"period": "24h",
						},
					},
				},
			},
			"storage": map[string]interface{}{"type": "filesystem"},
		},
		"singleBinary": map[string]interface{}{"replicas": 1},
		"backend":      map[string]interface{}{"replicas": 0},
		"read":         map[string]interface{}{"replicas": 0},
		"write":        map[string]interface{}{"replicas": 0},
		"gateway": map[string]interface{}{
			"service": map[string]interface{}{"type": "NodePort"},
		},
	}
}

const n8nSecureCookieEnvVar = "N8N_SECURE_COOKIE"

// n8nSecureCookiePatch turns off n8n's secure-cookie check. n8n marks its
// session cookie Secure by default and then refuses to serve the editor to a
// browser that arrived over plain HTTP — which is exactly how the App Store
// publishes it, on the NodePort URL of a node address. Without this the
// install succeeds and the web UI never opens.
//
// Both n8n charts on ArtifactHub are one search away and they disagree on
// where plain environment variables go, so the shape is read from the chart
// instead of assumed: community-charts keeps a name/value map in
// main.extraEnvVars and reserves the main.extraEnv list for valueFrom entries,
// while the 8gears chart has no extraEnvVars and keys main.extraEnv by
// variable name. A chart that declares neither is left alone — community-charts
// pins additionalProperties:false, where an invented path fails the install.
func n8nSecureCookiePatch(ch *chart.Chart, explicitValues map[string]interface{}) map[string]interface{} {
	main := helmValueMapAtPath(ch.Values, "main")
	if _, ok := main["extraEnvVars"]; ok {
		return helmValuePatch(n8nEnvVarsPath(explicitValues), map[string]interface{}{n8nSecureCookieEnvVar: "false"})
	}
	if _, ok := main["extraEnv"]; ok {
		return helmValuePatch([]string{"main", "extraEnv", n8nSecureCookieEnvVar}, map[string]interface{}{"value": "false"})
	}
	return nil
}

// n8nEnvVarsPath chooses between the chart's two name/value maps. The template
// reads main.extraEnvVars in preference to the deprecated top-level one, so
// writing ours into main would silently drop everything a user put in the
// top-level map: in that one case ours joins theirs instead.
func n8nEnvVarsPath(explicitValues map[string]interface{}) []string {
	if len(helmValueMapAtPath(explicitValues, "main", "extraEnvVars")) == 0 &&
		len(helmValueMapAtPath(explicitValues, "extraEnvVars")) != 0 {
		return []string{"extraEnvVars"}
	}
	return []string{"main", "extraEnvVars"}
}

// n8nSecureCookieValuePaths are the paths n8nSecureCookiePatch writes to, read
// back to report whether the patch is still in force.
var n8nSecureCookieValuePaths = [][]string{
	{"main", "extraEnvVars", n8nSecureCookieEnvVar},
	{"extraEnvVars", n8nSecureCookieEnvVar},
	{"main", "extraEnv", n8nSecureCookieEnvVar, "value"},
}

// n8nSecureCookieWarning states the trade-off the patch makes, so the install
// dialog shows why the value is there and what it costs.
func n8nSecureCookieWarning(values map[string]interface{}) string {
	for _, path := range n8nSecureCookieValuePaths {
		if helmValueIsFalse(helmValueAtPath(values, path)) {
			return fmt.Sprintf(
				"CasOS sets %s=false so the n8n web UI opens over the plain-HTTP node address; the session cookie is then not marked Secure and travels in the clear, so set it back to true once n8n is fronted by HTTPS",
				n8nSecureCookieEnvVar,
			)
		}
	}
	return ""
}

// helmValueIsFalse reports whether a values leaf holds false in either the
// boolean or the string form Helm accepts for it.
func helmValueIsFalse(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return !typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "false")
	}
	return false
}

// supersetValuesPatches also clears the chart's node port default: it ships
// `http: nil`, which YAML reads as the string "nil" and renders as an invalid
// `nodePort: nil`. A real null lets Kubernetes allocate a free port, where a
// fixed number would collide with the next install.
func supersetValuesPatches() map[string]interface{} {
	patches := nodePortServiceValuesPatch()
	service := patches["service"].(map[string]interface{})
	service["nodePort"] = map[string]interface{}{"http": nil}
	patches["bootstrapScript"] = supersetBootstrapScript
	return patches
}

const (
	supersetDriverTarget    = "/tmp/pgdrivers"
	supersetPsycopg2Version = "2.9.10"
	supersetSecretKeyEnvVar = "SUPERSET_SECRET_KEY"
)

// supersetBootstrapScript makes the PostgreSQL driver importable at pod start:
// the apache/superset image does not ship psycopg2 and the app venv has no pip,
// so the system python installs it into a writable target dir. The import probe
// keeps images that already carry it, and every restart on an offline cluster,
// from reaching for PyPI at all; the short timeout fails fast when it must.
// ${PYTHONPATH:+...} drops the separator when PYTHONPATH is unset, where a
// trailing colon would put the working directory on sys.path.
var supersetBootstrapScript = strings.Join([]string{
	"#!/bin/bash",
	"if ! python -c 'import psycopg2' >/dev/null 2>&1; then",
	fmt.Sprintf(
		"  /usr/local/bin/python3 -m pip install --no-cache-dir --disable-pip-version-check --timeout 15 --retries 1 --target %s 'psycopg2-binary==%s' || true",
		supersetDriverTarget, supersetPsycopg2Version,
	),
	"fi",
	fmt.Sprintf("export PYTHONPATH=%s${PYTHONPATH:+:${PYTHONPATH}}", supersetDriverTarget),
}, "\n")

// supersetPreservedValuePaths must survive an upgrade: Superset encrypts stored
// database credentials with SECRET_KEY, so a new key orphans them. The second
// path is where releases installed before the switch keep their key.
var supersetPreservedValuePaths = [][]string{
	{"extraSecretEnv", supersetSecretKeyEnvVar},
	{"configOverrides", "secret"},
}

// supersetSecretKeyPatch generates a random SECRET_KEY per install, because
// Superset refuses to start with the packaged default. extraSecretEnv reaches
// superset/config.py through the env Secret, avoiding configOverrides, whose
// merge order the chart documents as unspecified.
func supersetSecretKeyPatch(explicitValues map[string]interface{}) (map[string]interface{}, error) {
	if supersetSecretKeyAlreadySet(explicitValues) {
		return nil, nil
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate Superset SECRET_KEY: %w", err)
	}
	return map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{supersetSecretKeyEnvVar: hex.EncodeToString(buf)},
	}, nil
}

// supersetSecretKeyAlreadySet reports whether configOverrides already defines a
// SECRET_KEY. superset_config.py is read after the environment, so it wins and
// a generated key would only look like a rotation on upgrade.
func supersetSecretKeyAlreadySet(values map[string]interface{}) bool {
	overrides, ok := values["configOverrides"].(map[string]interface{})
	if !ok {
		return false
	}
	secret, ok := overrides["secret"].(string)
	return ok && strings.TrimSpace(secret) != ""
}

// applyHelmChartAdapter merges chart-specific compatibility values into the
// install values; user-set top-level keys are left untouched.
func applyHelmChartAdapter(ch *chart.Chart, values, explicitValues map[string]interface{}, includeDynamic bool, cluster clusterContext) error {
	if ch == nil {
		return nil
	}
	adapter, ok := helmChartAdapterRegistry[helmChartAdapterKey(ch)]
	if !ok {
		return nil
	}
	patches := adapter.valuesPatches
	if adapter.chartValuesPatchFn != nil {
		patches = mergedHelmAdapterPatches(patches, adapter.chartValuesPatchFn(ch, explicitValues))
	}
	if includeDynamic && adapter.generatedValuesPatchFn != nil {
		generated, err := adapter.generatedValuesPatchFn(explicitValues)
		if err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
		patches = mergedHelmAdapterPatches(patches, generated)
	}
	if includeDynamic && adapter.clusterValuesPatchFn != nil {
		fromCluster, err := adapter.clusterValuesPatchFn(cluster)
		if err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
		patches = mergedHelmAdapterPatches(patches, fromCluster)
	}
	for topKey, patch := range patches {
		if adapterPatchExplicitlyOverridden(explicitValues, topKey, patch) {
			continue
		}
		if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	if includeDynamic {
		if err := applyClusterHostBindings(ch, values, adapter.clusterHostBindings, cluster); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	return nil
}

// helmChartAdapterWarning returns what the chart's adapter wants the user to
// know about the values it produced, or "" when it has nothing to say.
func helmChartAdapterWarning(ch *chart.Chart, values map[string]interface{}) string {
	if ch == nil {
		return ""
	}
	adapter, ok := helmChartAdapterRegistry[helmChartAdapterKey(ch)]
	if !ok || adapter.warningFn == nil {
		return ""
	}
	return adapter.warningFn(values)
}

// applyClusterHostBindings writes the reachable host list into each bound
// values path. Unlike the patches above this never yields to explicit values:
// the list is additive, so the user's own hosts head it and CasOS only appends
// the addresses the app would otherwise turn away.
func applyClusterHostBindings(ch *chart.Chart, values map[string]interface{}, bindings []clusterHostBinding, cluster clusterContext) error {
	for _, binding := range bindings {
		if len(binding.path) == 0 || binding.render == nil {
			continue
		}
		hosts := clusterReachableHosts(ch, values, binding.sourcePaths, cluster)
		if len(hosts) == 0 {
			continue
		}
		if err := mergeHelmValueOverrides(values, helmValuePatch(binding.path, binding.render(hosts)), nil); err != nil {
			return err
		}
	}
	return nil
}

// clusterReachableHosts assembles every address the app is reached through:
// the binding's own sources first, then loopback, the in-cluster service names
// and the node addresses. Order is stable and duplicates are dropped, so a
// binding that renders only the first host still gets the app's own domain.
func clusterReachableHosts(ch *chart.Chart, values map[string]interface{}, sourcePaths [][]string, cluster clusterContext) []string {
	seen := map[string]bool{}
	var hosts []string
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" || seen[host] {
			return
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	for _, path := range sourcePaths {
		for _, host := range helmValueHosts(values, path) {
			add(host)
		}
		if ch != nil {
			for _, host := range helmValueHosts(ch.Values, path) {
				add(host)
			}
		}
	}
	add("localhost")
	for _, host := range cluster.inClusterHosts(ch, values) {
		add(host)
	}
	if cluster.nodeIPs != nil {
		for _, ip := range cluster.nodeIPs() {
			add(ip)
		}
	}
	return hosts
}

// inClusterHosts returns the service DNS names other pods reach the release
// through; sidecars such as the Nextcloud metrics exporter scrape the app that
// way, and a host allowlist that omits them turns those callers away.
func (cluster clusterContext) inClusterHosts(ch *chart.Chart, values map[string]interface{}) []string {
	if cluster.releaseName == "" || cluster.namespace == "" {
		return nil
	}
	names := cluster.releaseServiceNames(ch, values)
	hosts := make([]string, 0, len(names))
	for _, name := range names {
		hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", name, cluster.namespace))
	}
	return hosts
}

// releaseServiceNames lists the names a release's service may carry. It follows
// the fullname template nearly every chart copies from `helm create`:
// fullnameOverride decides outright, otherwise the release name is used when it
// already carries the chart name (or its nameOverride) and is prefixed to it
// when it does not. The bare release name heads the list either way, for the
// charts that name a service after the release alone — a host allowlist
// tolerates a spare entry, not a missing one.
func (cluster clusterContext) releaseServiceNames(ch *chart.Chart, values map[string]interface{}) []string {
	names := []string{cluster.releaseName}
	if fullname := helmNameOverride(ch, values, "fullnameOverride"); fullname != "" {
		names = append(names, fullname)
	} else {
		name := helmNameOverride(ch, values, "nameOverride")
		if name == "" && ch != nil {
			name = helmChartAdapterKey(ch)
		}
		if name != "" && !strings.Contains(cluster.releaseName, name) {
			names = append(names, cluster.releaseName+"-"+name)
		}
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = helmDNSName(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

// helmNameOverride reads a top-level naming override the way Helm coalesces it:
// the install values win over the chart's own default.
func helmNameOverride(ch *chart.Chart, values map[string]interface{}, key string) string {
	sources := []map[string]interface{}{values}
	if ch != nil {
		sources = append(sources, ch.Values)
	}
	for _, source := range sources {
		if override, ok := helmValueAtPath(source, []string{key}).(string); ok {
			if override = strings.TrimSpace(override); override != "" {
				return override
			}
		}
	}
	return ""
}

// helmDNSName applies the `trunc 63 | trimSuffix "-"` every fullname template
// ends with, so a long release name yields the name Kubernetes actually holds.
func helmDNSName(name string) string {
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.TrimSuffix(name, "-")
}

// helmHostStringList renders the host list as a YAML list of strings.
func helmHostStringList(hosts []string) interface{} {
	items := make([]interface{}, 0, len(hosts))
	for _, host := range hosts {
		items = append(items, host)
	}
	return items
}

// helmValueHosts reads a values path holding one host or a list of hosts,
// ignoring entries that are not strings.
func helmValueHosts(values map[string]interface{}, path []string) []string {
	switch typed := helmValueAtPath(values, path).(type) {
	case string:
		return []string{typed}
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		hosts := make([]string, 0, len(typed))
		for _, item := range typed {
			if host, ok := item.(string); ok {
				hosts = append(hosts, host)
			}
		}
		return hosts
	}
	return nil
}

// helmValueAtPath returns the leaf a path points at, or nil if any parent is
// missing or is not a map.
func helmValueAtPath(values map[string]interface{}, path []string) interface{} {
	for index, key := range path {
		if values == nil {
			return nil
		}
		if index == len(path)-1 {
			return values[key]
		}
		next, ok := values[key].(map[string]interface{})
		if !ok {
			return nil
		}
		values = next
	}
	return nil
}

// helmValueMapAtPath returns the map a path points at, or nil when the path is
// missing or holds anything else.
func helmValueMapAtPath(values map[string]interface{}, path ...string) map[string]interface{} {
	nested, _ := helmValueAtPath(values, path).(map[string]interface{})
	return nested
}

// helmValuePatch wraps a leaf value in the nested maps its path describes.
func helmValuePatch(path []string, value interface{}) map[string]interface{} {
	patch := value
	for index := len(path) - 1; index > 0; index-- {
		patch = map[string]interface{}{path[index]: patch}
	}
	return map[string]interface{}{path[0]: patch}
}

// preserveHelmChartAdapterValues copies the installed release's adapter-owned
// secrets into the upgrade values, where the adapter then reads them as
// explicit input and leaves them alone. Caller-set values win.
func preserveHelmChartAdapterValues(actionConfig *action.Configuration, ch *chart.Chart, releaseName string, values map[string]interface{}) {
	if ch == nil || actionConfig == nil || actionConfig.Releases == nil {
		return
	}
	adapter, ok := helmChartAdapterRegistry[helmChartAdapterKey(ch)]
	if !ok || len(adapter.preservedValuePaths) == 0 {
		return
	}
	installedRelease, err := actionConfig.Releases.Last(releaseName)
	if err != nil || installedRelease == nil || installedRelease.Chart == nil {
		return
	}
	installed, err := chartutil.CoalesceValues(installedRelease.Chart, cloneHelmValues(installedRelease.Config))
	if err != nil {
		return
	}
	for _, path := range adapter.preservedValuePaths {
		copyHelmValueIfUnset(installed, values, path)
	}
}

// copyHelmValueIfUnset copies one non-empty string leaf between value trees,
// creating parent maps on the way and never replacing an existing leaf.
func copyHelmValueIfUnset(source, target map[string]interface{}, path []string) {
	if len(path) == 0 {
		return
	}
	parents, leaf := path[:len(path)-1], path[len(path)-1]
	for _, key := range parents {
		next, ok := source[key].(map[string]interface{})
		if !ok {
			return
		}
		source = next
	}
	value, ok := source[leaf].(string)
	if !ok || value == "" {
		return
	}
	for _, key := range parents {
		switch existing := target[key].(type) {
		case map[string]interface{}:
			target = existing
		case nil:
			next := map[string]interface{}{}
			target[key] = next
			target = next
		default:
			return
		}
	}
	if _, exists := target[leaf]; exists {
		return
	}
	target[leaf] = value
}

// mergedHelmAdapterPatches overlays generated patches on the static ones
// without mutating the registry entry.
func mergedHelmAdapterPatches(static, generated map[string]interface{}) map[string]interface{} {
	if len(generated) == 0 {
		return static
	}
	merged := make(map[string]interface{}, len(static)+len(generated))
	for topKey, patch := range static {
		merged[topKey] = patch
	}
	for topKey, patch := range generated {
		merged[topKey] = patch
	}
	return merged
}

func helmChartAdapterKey(ch *chart.Chart) string {
	return strings.ToLower(strings.TrimSpace(ch.Name()))
}

// adapterPatchExplicitlyOverridden reports whether the user explicitly set
// any leaf of the patch within explicitValues. Leaf-level checking (not the
// top-level key) keeps the adapter active when the user only touched a
// sibling key, e.g. service.port while the patch targets service.type.
func adapterPatchExplicitlyOverridden(explicitValues map[string]interface{}, topKey string, patch interface{}) bool {
	explicit, exists := explicitValues[topKey]
	if !exists {
		return false
	}
	patchMap, patchIsMap := patch.(map[string]interface{})
	explicitMap, explicitIsMap := explicit.(map[string]interface{})
	if !patchIsMap || !explicitIsMap {
		return true
	}
	for key, subPatch := range patchMap {
		if adapterPatchExplicitlyOverridden(explicitMap, key, subPatch) {
			return true
		}
	}
	return false
}
