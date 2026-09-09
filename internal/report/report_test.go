package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tolvi-labs/canary/internal/audit"
	"github.com/tolvi-labs/canary/internal/gate"
)

func TestToJSON_Shape(t *testing.T) {
	result := gate.Result{
		Gate:           "pr",
		ManifestStatus: "fresh",
		SelectedTests:  []gate.SelectedTest{{Test: "TestAdd", Reason: "coverage"}},
		Impact:         []gate.ImpactEntry{{Path: "mathutil/mathutil.go", Tests: []string{"TestAdd"}}},
	}
	raw, err := ToJSON(result, "abc", "def")
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["schema"] != "tolvi-canary-report-v1" {
		t.Fatalf("unexpected schema field: %v", parsed["schema"])
	}
	if parsed["gate"] != "pr" {
		t.Fatalf("unexpected gate field: %v", parsed["gate"])
	}
	rng, ok := parsed["range"].(map[string]interface{})
	if !ok || rng["base"] != "abc" || rng["head"] != "def" {
		t.Fatalf("unexpected range field: %v", parsed["range"])
	}
	selected, ok := parsed["selected_tests"].([]interface{})
	if !ok || len(selected) != 1 {
		t.Fatalf("unexpected selected_tests: %v", parsed["selected_tests"])
	}
}

func TestToJSON_IncludesAuditWhenPresent(t *testing.T) {
	a := audit.Result{OrphanTests: []string{"TestOrphan"}}
	result := gate.Result{Gate: "audit", ManifestStatus: "fresh", Audit: &a}
	raw, err := ToJSON(result, "", "")
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if !strings.Contains(string(raw), "TestOrphan") {
		t.Fatalf("expected orphan test in JSON output, got: %s", raw)
	}
}

func TestToHuman_ListsSelectedTests(t *testing.T) {
	result := gate.Result{
		Gate:           "pr",
		ManifestStatus: "fresh",
		SelectedTests:  []gate.SelectedTest{{Test: "TestAdd", Reason: "coverage"}},
	}
	out := ToHuman(result, "abc", "def")
	if !strings.Contains(out, "TestAdd") {
		t.Fatalf("expected TestAdd in output, got: %s", out)
	}
}

func TestToJSON_IncludesDegradedPackages(t *testing.T) {
	result := gate.Result{Gate: "pr", ManifestStatus: "fresh", DegradedPackages: []string{"broken"}}
	raw, err := ToJSON(result, "", "")
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	degraded, ok := parsed["degraded_packages"].([]interface{})
	if !ok || len(degraded) != 1 || degraded[0] != "broken" {
		t.Fatalf("unexpected degraded_packages: %v", parsed["degraded_packages"])
	}
}

func TestToHuman_MentionsDegradedPackagesWhenPresent(t *testing.T) {
	result := gate.Result{Gate: "pr", ManifestStatus: "fresh", DegradedPackages: []string{"broken"}}
	out := ToHuman(result, "abc", "def")
	if !strings.Contains(out, "Degraded packages") || !strings.Contains(out, "broken") {
		t.Fatalf("expected degraded packages mentioned, got: %s", out)
	}
}

func TestToHuman_MentionsNeverSkipBinding(t *testing.T) {
	result := gate.Result{
		Gate:           "pr",
		ManifestStatus: "fresh",
		SelectedTests:  []gate.SelectedTest{{Test: "TestHello", Reason: "never-skip-binding", BindingDecision: "dec-1"}},
	}
	out := ToHuman(result, "abc", "def")
	if !strings.Contains(out, "never-skip") || !strings.Contains(out, "dec-1") {
		t.Fatalf("expected never-skip binding mentioned, got: %s", out)
	}
}
