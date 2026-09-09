package bindings

import (
	"testing"

	"github.com/tolvi-labs/canary/internal/vault"
)

func testDecision() vault.Decision {
	return vault.Decision{
		Slug:   "2026-07-04-stale-cache",
		Status: "active",
		XCanaryBindings: []vault.Binding{
			{Paths: []string{"internal/cache/**"}, Tests: []string{"TestCacheInvalidation"}},
		},
	}
}

func TestEvaluate_MatchedBindingForcesKnownTest(t *testing.T) {
	decisions := []vault.Decision{testDecision()}
	known := map[string]bool{"TestCacheInvalidation": true}
	forced, problems := Evaluate(decisions, []string{"internal/cache/store.go"}, known)
	if len(problems) != 0 {
		t.Fatalf("expected no problems, got: %v", problems)
	}
	if len(forced) != 1 || forced[0].Test != "TestCacheInvalidation" || forced[0].Decision != "2026-07-04-stale-cache" {
		t.Fatalf("unexpected forced tests: %v", forced)
	}
}

func TestEvaluate_UnknownTestBecomesProblemNotForced(t *testing.T) {
	decisions := []vault.Decision{testDecision()}
	known := map[string]bool{} // TestCacheInvalidation doesn't exist
	forced, problems := Evaluate(decisions, []string{"internal/cache/store.go"}, known)
	if len(forced) != 0 {
		t.Fatalf("expected no forced tests, got: %v", forced)
	}
	if len(problems) != 1 || problems[0].Decision != "2026-07-04-stale-cache" {
		t.Fatalf("unexpected problems: %v", problems)
	}
}

func TestEvaluate_NonMatchingPathIgnored(t *testing.T) {
	decisions := []vault.Decision{testDecision()}
	known := map[string]bool{"TestCacheInvalidation": true}
	forced, problems := Evaluate(decisions, []string{"internal/other/file.go"}, known)
	if len(forced) != 0 || len(problems) != 0 {
		t.Fatalf("expected no forced tests or problems, got forced=%v problems=%v", forced, problems)
	}
}

func TestEvaluate_InactiveDecisionIgnored(t *testing.T) {
	d := testDecision()
	d.Status = "superseded"
	known := map[string]bool{"TestCacheInvalidation": true}
	forced, _ := Evaluate([]vault.Decision{d}, []string{"internal/cache/store.go"}, known)
	if len(forced) != 0 {
		t.Fatalf("expected no forced tests for a non-active decision, got: %v", forced)
	}
}
