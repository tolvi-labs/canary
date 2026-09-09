package manifest

import (
	"testing"

	"github.com/tolvi-labs/canary/internal/gitutil"
)

func TestSelectByCoverage_OverlappingRangeSelectsTest(t *testing.T) {
	m := GlobalManifest{
		Coverage: map[string][]CoveredRange{
			"mathutil/mathutil.go": {
				{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}},
				{StartLine: 5, EndLine: 5, Tests: nil},
			},
		},
	}
	changed := map[string][]gitutil.LineRange{
		"mathutil/mathutil.go": {{Start: 3, End: 3}},
	}
	got := SelectByCoverage(m, changed)
	if len(got) != 1 || got[0] != "TestAdd" {
		t.Fatalf("expected [TestAdd], got: %v", got)
	}
}

func TestSelectByCoverage_NonOverlappingRangeSelectsNothing(t *testing.T) {
	m := GlobalManifest{
		Coverage: map[string][]CoveredRange{
			"mathutil/mathutil.go": {
				{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}},
			},
		},
	}
	changed := map[string][]gitutil.LineRange{
		"mathutil/mathutil.go": {{Start: 10, End: 12}},
	}
	got := SelectByCoverage(m, changed)
	if len(got) != 0 {
		t.Fatalf("expected no tests selected, got: %v", got)
	}
}

func TestSelectByCoverage_UnknownFileIgnored(t *testing.T) {
	m := GlobalManifest{Coverage: map[string][]CoveredRange{}}
	changed := map[string][]gitutil.LineRange{
		"unknown.go": {{Start: 1, End: 1}},
	}
	got := SelectByCoverage(m, changed)
	if len(got) != 0 {
		t.Fatalf("expected no tests selected for an unknown file, got: %v", got)
	}
}

func TestAllTestsUnderPackage_ReturnsEveryTestInThatDir(t *testing.T) {
	m := GlobalManifest{
		Coverage: map[string][]CoveredRange{
			"broken/broken.go": {{StartLine: 3, EndLine: 3, Tests: []string{"TestOldBroken"}}},
			"broken/other.go":  {{StartLine: 1, EndLine: 1, Tests: []string{"TestOtherOldBroken"}}},
			"good/good.go":     {{StartLine: 3, EndLine: 3, Tests: []string{"TestOk"}}},
		},
	}
	got := AllTestsUnderPackage(m, "broken")
	if len(got) != 2 || got[0] != "TestOldBroken" || got[1] != "TestOtherOldBroken" {
		t.Fatalf("unexpected result: %v", got)
	}
}
