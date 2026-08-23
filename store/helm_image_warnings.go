package store

import (
	"context"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

// ImageVulnerabilityReporter describes what is already known about a release's
// images. It is supplied by the caller, because the scan results live in the
// database layer and store stays independent of it. A nil reporter reports
// nothing.
//
// It reports; it does not decide. Installing an app is the operator's call, and
// a scan finding is information they can act on — pick another chart version,
// update the image, or accept it — not a reason for the platform to refuse the
// install on their behalf. The findings stay listed on the Trivy scan results
// page for whoever wants to look.
var ImageVulnerabilityReporter func(images []string) []string

// helmInstallImageWarnings renders the chart client-side and asks the reporter
// what is known about the images the release would run. A render that fails
// yields no warnings: the real install renders again and reports the problem
// with far better context than this check could.
func helmInstallImageWarnings(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) []string {
	report := ImageVulnerabilityReporter
	if report == nil || chartToInstall == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Client-side only: this runs after the compatibility dry-run has already
	// checked the chart against the cluster, so it needs the manifest, not
	// another round of server-side validation.
	dryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace, false)
	rendered, err := dryRun.RunWithContext(ctx, chartToInstall, values)
	if err != nil || rendered == nil {
		return nil
	}
	images := uniqueManifestImages(manifestImages([]byte(rendered.Manifest)))
	if len(images) == 0 {
		return nil
	}
	return report(images)
}

// uniqueManifestImages keeps the first occurrence of each image. One image
// commonly appears several times in a chart — an init container and the app
// container often share it — and the report names what it found, so a repeated
// image would be named repeatedly in the same sentence.
func uniqueManifestImages(images []string) []string {
	seen := make(map[string]bool, len(images))
	unique := make([]string, 0, len(images))
	for _, image := range images {
		if image == "" || seen[image] {
			continue
		}
		seen[image] = true
		unique = append(unique, image)
	}
	return unique
}
