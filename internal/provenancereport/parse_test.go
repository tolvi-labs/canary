package provenancereport

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleReport = `{
  "schema": "tolvi-provenance-report-v1",
  "range": {"base": "abc", "head": "def"},
  "status": "pass",
  "governed_files": [
    {"path": "src/billing/webhook.go", "implicated_decisions": ["dec-1"], "addressed_by": ["dec-1"]}
  ],
  "captures": [],
  "new_decisions": [],
  "blocked_on": [
    {"decision": "dec-2", "files": ["src/other.go"]}
  ]
}`

func TestLoad_ParsesRealShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(sampleReport), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if r.Schema != "tolvi-provenance-report-v1" {
		t.Fatalf("unexpected schema: %s", r.Schema)
	}
	if len(r.GovernedFiles) != 1 || r.GovernedFiles[0].Path != "src/billing/webhook.go" {
		t.Fatalf("unexpected governed files: %v", r.GovernedFiles)
	}
	if len(r.BlockedOn) != 1 || r.BlockedOn[0].Decision != "dec-2" {
		t.Fatalf("unexpected blocked_on: %v", r.BlockedOn)
	}
}

func TestLoad_EmptyPathReturnsNil(t *testing.T) {
	r, err := Load("")
	if err != nil {
		t.Fatalf("expected no error for an empty path, got: %v", err)
	}
	if r != nil {
		t.Fatalf("expected nil report for an empty path, got: %v", r)
	}
}

func TestLoad_MissingFileReturnsError(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
