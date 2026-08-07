package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmChartAdapter struct {
	valuesPatches map[string]interface{}
	valuesPatchFn func(nodeIPs []string) (map[string]interface{}, error)
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"grafana":  {valuesPatches: nodePortServiceValuesPatch},
	"pgadmin4": {valuesPatches: nodePortServiceValuesPatch},
	"n8n":      {valuesPatches: nodePortServiceValuesPatch},
	"superset": {
		valuesPatches: map[string]interface{}{
			"service":         map[string]interface{}{"type": "NodePort"},
			"bootstrapScript": supersetBootstrapScript,
		},
		valuesPatchFn: supersetConfigOverridesPatch,
	},
	"nextcloud": {
		valuesPatches: map[string]interface{}{
			"service": map[string]interface{}{"type": "NodePort"},
		},
		valuesPatchFn: nextcloudTrustedDomainsPatch,
	},
}

var nodePortServiceValuesPatch = map[string]interface{}{
	"service": map[string]interface{}{"type": "NodePort"},
}

// nextcloudTrustedDomainsPatch configures trusted_domains via the chart's
// post-install occ job: the probe host (the chart probes /status.php with
// Host nextcloud.kube.home, so omitting it makes liveness fail and the pod
// restart-loop) plus the cluster node IPs the user reaches the app through.
func nextcloudTrustedDomainsPatch(nodeIPs []string) (map[string]interface{}, error) {
	occ := []interface{}{
		nextcloudOccCommand("config:system:set", []string{"trusted_domains", "0", "--value=nextcloud.kube.home"}),
	}
	for index, ip := range nodeIPs {
		occ = append(occ, nextcloudOccCommand("config:system:set", []string{"trusted_domains", strconv.Itoa(index + 1), "--value=" + ip}))
	}
	return map[string]interface{}{
		"nextcloud": map[string]interface{}{"occ": occ},
	}, nil
}

func nextcloudOccCommand(command string, args []string) map[string]interface{} {
	argValues := make([]interface{}, 0, len(args))
	for _, arg := range args {
		argValues = append(argValues, arg)
	}
	return map[string]interface{}{"command": command, "args": argValues}
}

// supersetBootstrapScript installs the psycopg2 driver into a writable target
// dir at pod start: the apache/superset image has no psycopg2 and its venv has
// no pip, so the system pip installs to /tmp and PYTHONPATH picks it up.
const supersetBootstrapScript = "python3 -m pip install --no-cache-dir --target /tmp/pgdrivers psycopg2-binary && export PYTHONPATH=/tmp/pgdrivers:$PYTHONPATH"

// supersetConfigOverridesPatch generates a random SECRET_KEY: the default
// value makes Superset refuse to start.
func supersetConfigOverridesPatch(_ []string) (map[string]interface{}, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate Superset SECRET_KEY: %w", err)
	}
	return map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": hex.EncodeToString(buf)},
	}, nil
}

// applyHelmChartAdapter merges chart-specific compatibility values into the
// install values; user-set top-level keys are left untouched.
func applyHelmChartAdapter(ch *chart.Chart, values, explicitValues map[string]interface{}, nodeIPs []string) error {
	if ch == nil {
		return nil
	}
	adapter, ok := helmChartAdapterRegistry[strings.ToLower(strings.TrimSpace(ch.Name()))]
	if !ok {
		return nil
	}
	for topKey, patch := range adapter.valuesPatches {
		if err := mergeAdapterValuesPatch(values, explicitValues, topKey, patch); err != nil {
			return err
		}
	}
	if adapter.valuesPatchFn != nil {
		fnPatches, err := adapter.valuesPatchFn(nodeIPs)
		if err != nil {
			return err
		}
		for topKey, patch := range fnPatches {
			if err := mergeAdapterValuesPatch(values, explicitValues, topKey, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeAdapterValuesPatch(values, explicitValues map[string]interface{}, topKey string, patch interface{}) error {
	if _, explicitlySet := explicitValues[topKey]; explicitlySet {
		return nil
	}
	return mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil)
}
