package audit

import (
	"testing"

	"github.com/tolvi-labs/canary/internal/manifest"
)

func TestCompute_FindsUntestedRange(t *testing.T) {
	m := manifest.GlobalManifest{
		Tests: []string{"TestAdd"},
		Coverage: map[string][]manifest.CoveredRange{
			"mathutil/mathutil.go": {
				{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}},
				{StartLine: 5, EndLine: 5, Tests: nil},
			},
		},
	}
	got := Compute(m)
	if len(got.UntestedPaths) != 1 || got.UntestedPaths[0].File != "mathutil/mathutil.go" || got.UntestedPaths[0].StartLine != 5 {
		t.Fatalf("unexpected untested paths: %v", got.UntestedPaths)
	}
}

func TestCompute_FindsOrphanTest(t *testing.T) {
	m := manifest.GlobalManifest{
		Tests: []string{"TestAdd", "TestOrphan"},
		Coverage: map[string][]manifest.CoveredRange{
			"mathutil/mathutil.go": {
				{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}},
			},
		},
	}
	got := Compute(m)
	if len(got.OrphanTests) != 1 || got.OrphanTests[0] != "TestOrphan" {
		t.Fatalf("unexpected orphan tests: %v", got.OrphanTests)
	}
}

func TestCompute_NoFalsePositivesOnFullyCoveredManifest(t *testing.T) {
	m := manifest.GlobalManifest{
		Tests: []string{"TestAdd"},
		Coverage: map[string][]manifest.CoveredRange{
			"mathutil/mathutil.go": {
				{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}},
			},
		},
	}
	got := Compute(m)
	if len(got.OrphanTests) != 0 || len(got.UntestedPaths) != 0 {
		t.Fatalf("expected no findings, got: %+v", got)
	}
}
