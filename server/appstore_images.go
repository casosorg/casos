package server

import (
	"fmt"

	"github.com/beego/beego/logs"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/store"
)

// InstallImageVulnerabilityWarnings reports what the scan cache already knows
// about the images a chart is about to run.
//
// It runs before Helm creates anything so the operator sees the finding while
// they are still choosing, but it never stops the install: the platform's job
// here is to tell them, not to decide for them. Images with no scan yet are
// queued and produce no warning — the report says what is known, and never
// guesses. Full details stay on the Trivy scan results page.
func InstallImageVulnerabilityWarnings(images []string) []string {
	var warnings []string
	for _, image := range images {
		if image == "" {
			continue
		}
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
			warnings = append(warnings, fmt.Sprintf(
				"image %s has %d known CRITICAL vulnerabilities — see the Trivy scan results page for details",
				image, result.Critical,
			))
		}
	}
	return warnings
}

// RegisterInstallImageVulnerabilityReporter hands the reporter to the store
// package, which cannot reach the scan results itself.
func RegisterInstallImageVulnerabilityReporter() {
	store.ImageVulnerabilityReporter = InstallImageVulnerabilityWarnings
}
