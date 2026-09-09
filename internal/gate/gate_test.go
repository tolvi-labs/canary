package gate

import (
	"testing"

	"github.com/tolvi-labs/canary/internal/gitutil"
	"github.com/tolvi-labs/canary/internal/manifest"
	"github.com/tolvi-labs/canary/internal/provenancereport"
	"github.com/tolvi-labs/canary/internal/vault"
)

func testManifest() manifest.GlobalManifest {
	return manifest.GlobalManifest{
		Tests: []string{"TestAdd", "TestHello"},
		Coverage: map[string][]manifest.CoveredRange{
			"mathutil/mathutil.go": {{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}}},
			"greet/greet.go":       {{StartLine: 3, EndLine: 3, Tests: []string{"TestHello"}}},
		},
	}
}

func TestCheck_PRGate_SelectsByCoverage(t *testing.T) {
	m := testManifest()
	changedRanges := map[string][]gitutil.LineRange{
		"mathutil/mathutil.go": {{Start: 3, End: 3}},
	}
	result := Check("pr", m, false, []string{"mathutil/mathutil.go"}, changedRanges, nil, nil)
	if result.ManifestStatus != "fresh" {
		t.Fatalf("expected fresh, got %s", result.ManifestStatus)
	}
	if len(result.SelectedTests) != 1 || result.SelectedTests[0].Test != "TestAdd" || result.SelectedTests[0].Reason != "coverage" {
		t.Fatalf("unexpected selected tests: %+v", result.SelectedTests)
	}
}

func TestCheck_PRGate_StaleManifestFallsBackToFullSuite(t *testing.T) {
	m := testManifest()
	result := Check("pr", m, true, nil, nil, nil, nil)
	if result.ManifestStatus != "stale-fallback" {
		t.Fatalf("expected stale-fallback, got %s", result.ManifestStatus)
	}
	if len(result.SelectedTests) != 2 {
		t.Fatalf("expected the full suite (2 tests), got: %+v", result.SelectedTests)
	}
}

func TestCheck_MergeGate_IgnoresDiffSelectsFullSuite(t *testing.T) {
	m := testManifest()
	changedRanges := map[string][]gitutil.LineRange{
		"mathutil/mathutil.go": {{Start: 3, End: 3}},
	}
	result := Check("merge", m, false, []string{"mathutil/mathutil.go"}, changedRanges, nil, nil)
	if len(result.SelectedTests) != 2 {
		t.Fatalf("expected the full suite (2 tests) for a merge gate, got: %+v", result.SelectedTests)
	}
	for _, s := range result.SelectedTests {
		if s.Reason != "full-suite-gate" {
			t.Fatalf("expected reason full-suite-gate, got: %+v", s)
		}
	}
}

func TestCheck_PRGate_BindingForcesUnrelatedTest(t *testing.T) {
	m := testManifest()
	decisions := []vault.Decision{
		{
			Slug:   "dec-1",
			Status: "active",
			XCanaryBindings: []vault.Binding{
				{Paths: []string{"mathutil/**"}, Tests: []string{"TestHello"}},
			},
		},
	}
	changedRanges := map[string][]gitutil.LineRange{
		"mathutil/mathutil.go": {{Start: 3, End: 3}},
	}
	result := Check("pr", m, false, []string{"mathutil/mathutil.go"}, changedRanges, decisions, nil)
	var foundForced bool
	for _, s := range result.SelectedTests {
		if s.Test == "TestHello" && s.Reason == "never-skip-binding" && s.BindingDecision == "dec-1" {
			foundForced = true
		}
	}
	if !foundForced {
		t.Fatalf("expected TestHello forced by the binding, got: %+v", result.SelectedTests)
	}
}

func TestCheck_PRGate_UnmappedNewFileFallsBackToFullSuite(t *testing.T) {
	m := testManifest()
	changedRanges := map[string][]gitutil.LineRange{
		"newpkg/newfile.go": {{Start: 1, End: 3}},
	}
	result := Check("pr", m, false, []string{"newpkg/newfile.go"}, changedRanges, nil, nil)
	if result.ManifestStatus != "partial-fallback" {
		t.Fatalf("expected partial-fallback, got %s", result.ManifestStatus)
	}
	if len(result.SelectedTests) != 2 {
		t.Fatalf("expected the full suite (2 tests), got: %+v", result.SelectedTests)
	}
	for _, s := range result.SelectedTests {
		if s.Reason != "unmapped-code-fallback" {
			t.Fatalf("expected reason unmapped-code-fallback, got: %+v", s)
		}
	}
}

func TestCheck_DegradedPackageForcesAllItsTests(t *testing.T) {
	m := testManifest()
	m.DegradedPackages = []string{"broken"}
	m.Coverage["broken/broken.go"] = []manifest.CoveredRange{
		{StartLine: 3, EndLine: 3, Tests: []string{"TestOldBroken"}},
	}
	m.Tests = append(m.Tests, "TestOldBroken")
	// The diff touches a *different* file in the same degraded package —
	// coverage-based selection alone wouldn't catch this file at all.
	changedRanges := map[string][]gitutil.LineRange{
		"broken/other.go": {{Start: 1, End: 1}},
	}
	result := Check("pr", m, false, []string{"broken/other.go"}, changedRanges, nil, nil)
	var found bool
	for _, s := range result.SelectedTests {
		if s.Test == "TestOldBroken" && s.Reason == "degraded-package-fallback" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TestOldBroken forced by the degraded package, got: %+v", result.SelectedTests)
	}
}

func TestCheck_ImpactIncludesProvenanceRisk(t *testing.T) {
	m := testManifest()
	changedRanges := map[string][]gitutil.LineRange{
		"mathutil/mathutil.go": {{Start: 3, End: 3}},
	}
	provReport := &provenancereport.Report{
		GovernedFiles: []provenancereport.GovernedFile{
			{Path: "mathutil/mathutil.go", ImplicatedDecisions: []string{"prov-dec-1"}},
		},
	}
	result := Check("pr", m, false, []string{"mathutil/mathutil.go"}, changedRanges, nil, provReport)
	if len(result.Impact) != 1 || result.Impact[0].Path != "mathutil/mathutil.go" {
		t.Fatalf("unexpected impact: %+v", result.Impact)
	}
	if len(result.Impact[0].ProvenanceRisk) != 1 || result.Impact[0].ProvenanceRisk[0] != "prov-dec-1" {
		t.Fatalf("expected provenance risk annotation, got: %+v", result.Impact[0])
	}
}
