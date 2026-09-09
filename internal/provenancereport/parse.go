package provenancereport

import (
	"encoding/json"
	"fmt"
	"os"
)

// GovernedFile mirrors one entry of Provenance's
// tolvi-provenance-report-v1 governed_files array.
type GovernedFile struct {
	Path                string   `json:"path"`
	ImplicatedDecisions []string `json:"implicated_decisions"`
	AddressedBy         []string `json:"addressed_by"`
}

// BlockedEntry mirrors one entry of Provenance's blocked_on array.
type BlockedEntry struct {
	Decision string   `json:"decision"`
	Files    []string `json:"files"`
}

// Report is the subset of Provenance's tolvi-provenance-report-v1 schema
// Canary consumes: declared provenance + governance, never a
// reachability-derived impact report.
type Report struct {
	Schema        string         `json:"schema"`
	Status        string         `json:"status"`
	GovernedFiles []GovernedFile `json:"governed_files"`
	BlockedOn     []BlockedEntry `json:"blocked_on"`
}

// Load reads a Provenance JSON report from path. An empty path is a
// valid "no report supplied" case and returns (nil, nil) — the input is
// optional.
func Load(path string) (*Report, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var r Report
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &r, nil
}
