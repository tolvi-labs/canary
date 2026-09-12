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
	result := Check("pr", m, false, []string{"mathutil/mathutil.go"}, changedRanges, nil, nil, []string{".go"})
	if result.ManifestStatus != "fresh" {
		t.Fatalf("expected fresh, got %s", result.ManifestStatus)
	}
	if len(result.SelectedTests) != 1 || result.SelectedTests[0].Test != "TestAdd" || result.SelectedTests[0].Reason != "coverage" {
		t.Fatalf("unexpected selected tests: %+v", result.SelectedTests)
	}
}

func TestCheck_PRGate_StaleManifestFallsBackToFullSuite(t *testing.T) {
	m := testManifest()
	result := Check("pr", m, true, nil, nil, nil, nil, []string{".go"})
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
	result := Check("merge", m, false, []string{"mathutil/mathutil.go"}, changedRanges, nil, nil, []string{".go"})
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
	result := Check("pr", m, false, []string{"mathutil/mathutil.go"}, changedRanges, decisions, nil, []string{".go"})
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
	result := Check("pr", m, false, []string{"newpkg/newfile.go"}, changedRanges, nil, nil, []string{".go"})
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
	result := Check("pr", m, false, []string{"broken/other.go"}, changedRanges, nil, nil, []string{".go"})
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

func TestCheck_DegradedPackageWithNoCoverageHistoryFallsBackToFullSuite(t *testing.T) {
	m := testManifest()
	m.DegradedPackages = []string{"broken"}
	// Unlike TestCheck_DegradedPackageForcesAllItsTests, no Coverage entry
	// exists anywhere under "broken/" — this package has never
	// successfully compiled for coverage, so degradedPackageTests has
	// nothing to force-include for it.
	changedRanges := map[string][]gitutil.LineRange{
		"broken/new.go": {{Start: 1, End: 1}},
	}
	result := Check("pr", m, false, []string{"broken/new.go"}, changedRanges, nil, nil, []string{".go"})
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

// TestCheck_DegradedPackageDoesNotCoverAnUnrelatedNestedSubPackage is the
// regression test for N1: a Go package can contain a nested subdirectory
// that is itself a *separate, unrelated* package (e.g. "broken" degraded,
// "broken/sub" a distinct, healthy package with its own tests). The
// degraded-scope check must not treat every file nested under "broken/"
// as covered by "broken"'s own degradation — only "broken" itself. A
// brand-new, untested file in the unrelated sibling package must still
// trip the unmapped-code fallback.
func TestCheck_DegradedPackageDoesNotCoverAnUnrelatedNestedSubPackage(t *testing.T) {
	m := testManifest()
	m.DegradedPackages = []string{"broken"}
	m.Coverage["broken/broken.go"] = []manifest.CoveredRange{
		{StartLine: 3, EndLine: 3, Tests: []string{"TestOldBroken"}},
	}
	m.Tests = append(m.Tests, "TestOldBroken")
	// "broken/sub" is a distinct package nested under "broken"'s directory
	// — not the degraded unit itself — receiving a brand-new untested file.
	changedRanges := map[string][]gitutil.LineRange{
		"broken/sub/newfile.go": {{Start: 1, End: 3}},
	}
	result := Check("pr", m, false, []string{"broken/sub/newfile.go"}, changedRanges, nil, nil, []string{".go"})
	if result.ManifestStatus != "partial-fallback" {
		t.Fatalf("expected partial-fallback (the unrelated sub-package must not be treated as covered by broken's degradation), got %s with selected: %+v", result.ManifestStatus, result.SelectedTests)
	}
	if len(result.SelectedTests) != 3 {
		t.Fatalf("expected the full suite (3 tests), got: %+v", result.SelectedTests)
	}
	for _, s := range result.SelectedTests {
		if s.Reason != "unmapped-code-fallback" {
			t.Fatalf("expected reason unmapped-code-fallback, got: %+v", s)
		}
	}
}

// nodeManifest is a manifest as the Node backend produces one: units are
// test *files*, and node:test excludes the test file itself from its
// coverage report, so the only covered paths are plain source files.
func nodeManifest() manifest.GlobalManifest {
	return manifest.GlobalManifest{
		Tests: []string{"add works", "sub works"},
		Coverage: map[string][]manifest.CoveredRange{
			"lib.js": {
				{StartLine: 1, EndLine: 1, Tests: []string{"add works"}},
				{StartLine: 2, EndLine: 2, Tests: []string{"sub works"}},
			},
		},
		TestsByUnit: map[string][]string{"lib.test.js": {"add works", "sub works"}},
	}
}

// TestCheck_PRGate_UnmappedNonGoFileFallsBackToFullSuite is the
// regression test for the gate's safety net having been hardcoded to
// ".go": a brand-new, untested .js file in a Node repo has no coverage
// data at all, so it must force the full-suite fallback exactly the way
// a new .go file does in a Go repo. Before the fix this file was skipped
// by the extension guard entirely and the gate reported "fresh" with
// zero tests selected — a green gate over completely unmapped code.
func TestCheck_PRGate_UnmappedNonGoFileFallsBackToFullSuite(t *testing.T) {
	m := nodeManifest()
	changedRanges := map[string][]gitutil.LineRange{
		"brandnew.js": {{Start: 1, End: 3}},
	}
	result := Check("pr", m, false, []string{"brandnew.js"}, changedRanges, nil, nil, []string{".js", ".mjs", ".cjs"})
	if result.ManifestStatus != "partial-fallback" {
		t.Fatalf("expected partial-fallback for an unmapped .js file, got %s (selected: %+v)", result.ManifestStatus, result.SelectedTests)
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

// TestCheck_PRGate_ExtensionsScopeTheSafetyNet is the other half: a
// changed file the repo's backend produces no coverage for at all (a
// .md, or a .go file in a Node repo) must not trip the fallback, or
// every documentation-only PR would run the whole suite.
func TestCheck_PRGate_ExtensionsScopeTheSafetyNet(t *testing.T) {
	m := nodeManifest()
	changedRanges := map[string][]gitutil.LineRange{
		"README.md":          {{Start: 1, End: 3}},
		"lib.js":             {{Start: 1, End: 1}},
		"tools/helper.go":    {{Start: 1, End: 1}},
		"docs/guide.mdx.txt": {{Start: 1, End: 1}},
	}
	result := Check("pr", m, false, []string{"README.md", "lib.js", "tools/helper.go"}, changedRanges, nil, nil, []string{".js", ".mjs", ".cjs"})
	if result.ManifestStatus != "fresh" {
		t.Fatalf("expected fresh, got %s", result.ManifestStatus)
	}
	if len(result.SelectedTests) != 1 || result.SelectedTests[0].Test != "add works" {
		t.Fatalf("expected just the coverage-derived test, got: %+v", result.SelectedTests)
	}
}

// TestCheck_DegradedFileShapedUnitForcesAllItsTests is the regression
// test for the degraded-unit fallback having been unreachable for
// Python/Node. Their degraded entries are test-file *paths*, and the
// gate used to derive a changed file's parent directory and look that
// up — a directory never equals a file path, so the conservative
// force-include could never fire. Here the degraded unit is a Python
// test file that still has coverage history; touching it must
// force-include everything that file is known to cover.
func TestCheck_DegradedFileShapedUnitForcesAllItsTests(t *testing.T) {
	m := manifest.GlobalManifest{
		Tests: []string{"test_add", "test_legacy"},
		Coverage: map[string][]manifest.CoveredRange{
			"mathutil/__init__.py":         {{StartLine: 2, EndLine: 2, Tests: []string{"test_add"}}},
			"mathutil/test_mathutil.py":    {{StartLine: 4, EndLine: 4, Tests: []string{"test_add"}}},
			"mathutil/test_legacy.py":      {{StartLine: 4, EndLine: 4, Tests: []string{"test_legacy"}}},
			"mathutil/legacy_internals.py": {{StartLine: 2, EndLine: 2, Tests: []string{"test_legacy"}}},
		},
		DegradedPackages: []string{"mathutil/test_legacy.py"},
	}
	changedRanges := map[string][]gitutil.LineRange{
		// A line of the degraded test file the manifest has no range for,
		// so coverage-based selection alone would pick nothing here.
		"mathutil/test_legacy.py": {{Start: 9, End: 9}},
	}
	result := Check("pr", m, false, []string{"mathutil/test_legacy.py"}, changedRanges, nil, nil, []string{".py"})
	if result.ManifestStatus != "fresh" {
		t.Fatalf("expected the degraded unit's own history to cover this, got %s", result.ManifestStatus)
	}
	var found bool
	for _, s := range result.SelectedTests {
		if s.Test == "test_legacy" && s.Reason == "degraded-package-fallback" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected test_legacy forced by the degraded file-shaped unit, got: %+v", result.SelectedTests)
	}
}

func TestCheck_ResultCarriesDegradedPackages(t *testing.T) {
	m := testManifest()
	m.DegradedPackages = []string{"broken"}
	result := Check("pr", m, false, nil, nil, nil, nil, []string{".go"})
	if len(result.DegradedPackages) != 1 || result.DegradedPackages[0] != "broken" {
		t.Fatalf("expected DegradedPackages to carry through from the manifest, got: %+v", result.DegradedPackages)
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
	result := Check("pr", m, false, []string{"mathutil/mathutil.go"}, changedRanges, nil, provReport, []string{".go"})
	if len(result.Impact) != 1 || result.Impact[0].Path != "mathutil/mathutil.go" {
		t.Fatalf("unexpected impact: %+v", result.Impact)
	}
	if len(result.Impact[0].ProvenanceRisk) != 1 || result.Impact[0].ProvenanceRisk[0] != "prov-dec-1" {
		t.Fatalf("expected provenance risk annotation, got: %+v", result.Impact[0])
	}
}
