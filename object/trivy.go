package object

import (
	"encoding/json"
	"os/exec"
	"time"

	"github.com/beego/beego/logs"
)

type TrivyVulnerability struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Severity         string `json:"Severity"`
	Title            string `json:"Title"`
}

// trivyReport mirrors the JSON structure emitted by `trivy image --format json`.
type trivyReport struct {
	Results []struct {
		Vulnerabilities []TrivyVulnerability `json:"Vulnerabilities"`
	} `json:"Results"`
}

type TrivyScanResult struct {
	Id              int64                `xorm:"pk autoincr" json:"id"`
	Image           string               `xorm:"varchar(512) notnull index" json:"image"`
	Status          string               `xorm:"varchar(32) notnull" json:"status"` // pending | done | failed | ignored
	Critical        int                  `json:"critical"`
	High            int                  `json:"high"`
	Medium          int                  `json:"medium"`
	Low             int                  `json:"low"`
	VulnsJSON       string               `xorm:"text 'vulns_json'" json:"-"`
	Vulnerabilities []TrivyVulnerability `xorm:"-" json:"vulnerabilities"`
	ErrorMsg        string               `xorm:"text" json:"errorMsg"`
	ScannedAt       time.Time            `json:"scannedAt"`
}

func GetTrivyScanResults() ([]*TrivyScanResult, error) {
	var results []*TrivyScanResult
	if err := ormer.Engine.Where("status <> ?", "ignored").OrderBy("id desc").Find(&results); err != nil {
		return nil, err
	}
	for _, r := range results {
		_ = json.Unmarshal([]byte(r.VulnsJSON), &r.Vulnerabilities)
	}
	return results, nil
}

func GetTrivyScanResultByImage(image string) (*TrivyScanResult, error) {
	result := &TrivyScanResult{}
	found, err := ormer.Engine.Where("image = ?", image).OrderBy("id desc").Get(result)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	_ = json.Unmarshal([]byte(result.VulnsJSON), &result.Vulnerabilities)
	return result, nil
}

func TriggerScan(image string) {
	existing := &TrivyScanResult{}
	found, _ := ormer.Engine.Where("image = ? AND status IN (?, ?)", image, "pending", "ignored").Get(existing)
	if found {
		return
	}

	row := &TrivyScanResult{
		Image:     image,
		Status:    "pending",
		ScannedAt: time.Now(),
	}
	if _, err := ormer.Engine.Insert(row); err != nil {
		logs.Error("trivy: insert pending record: %v", err)
		return
	}

	go runScan(row.Id, image)
}

func RunScanSync(image string) (*TrivyScanResult, error) {
	row := &TrivyScanResult{
		Image:     image,
		Status:    "pending",
		ScannedAt: time.Now(),
	}
	if _, err := ormer.Engine.Insert(row); err != nil {
		return nil, err
	}
	runScan(row.Id, image)

	result := &TrivyScanResult{}
	_, err := ormer.Engine.ID(row.Id).Get(result)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(result.VulnsJSON), &result.Vulnerabilities)
	return result, nil
}

func runScan(id int64, image string) {
	out, err := exec.Command("trivy", "image", "--format", "json", "--quiet", image).Output()

	update := &TrivyScanResult{ScannedAt: time.Now()}

	if err != nil {
		update.Status = "failed"
		update.ErrorMsg = err.Error()
		if len(out) > 0 {
			update.ErrorMsg += ": " + string(out)
		}
		_, _ = ormer.Engine.ID(id).Cols("status", "error_msg", "scanned_at").Update(update)
		return
	}

	var report trivyReport
	if err := json.Unmarshal(out, &report); err != nil {
		update.Status = "failed"
		update.ErrorMsg = "parse error: " + err.Error()
		_, _ = ormer.Engine.ID(id).Cols("status", "error_msg", "scanned_at").Update(update)
		return
	}

	var all []TrivyVulnerability
	for _, r := range report.Results {
		all = append(all, r.Vulnerabilities...)
	}
	for _, v := range all {
		switch v.Severity {
		case "CRITICAL":
			update.Critical++
		case "HIGH":
			update.High++
		case "MEDIUM":
			update.Medium++
		case "LOW":
			update.Low++
		}
	}

	vulnsBytes, _ := json.Marshal(all)
	update.VulnsJSON = string(vulnsBytes)
	update.Status = "done"
	_, _ = ormer.Engine.ID(id).
		Cols("status", "critical", "high", "medium", "low", "vulns_json", "scanned_at").
		Update(update)
}

func DeleteTrivyScanResult(id int64) error {
	// Deleting a finding is the operator's documented override. Keeping a
	// hidden tombstone makes that choice durable; physically deleting the row
	// caused admission to enqueue the same image immediately.
	override := &TrivyScanResult{Status: "ignored", VulnsJSON: "", ErrorMsg: ""}
	_, err := ormer.Engine.ID(id).
		Cols("status", "critical", "high", "medium", "low", "vulns_json", "error_msg").
		Update(override)
	return err
}
