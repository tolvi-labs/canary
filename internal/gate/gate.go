package gate

import (
	"sort"
	"strings"

	"github.com/tolvi-labs/canary/internal/audit"
	"github.com/tolvi-labs/canary/internal/bindings"
	"github.com/tolvi-labs/canary/internal/gitutil"
	"github.com/tolvi-labs/canary/internal/manifest"
	"github.com/tolvi-labs/canary/internal/provenancereport"
	"github.com/tolvi-labs/canary/internal/vault"
)

// SelectedTest is one test chosen to run, and why.
type SelectedTest struct {
	Test            string
	Reason          string // "coverage" | "never-skip-binding" | "full-suite-gate" | "stale-manifest-fallback"
	BindingDecision string // set only when Reason == "never-skip-binding"
}

// ImpactEntry summarizes, per changed file, which tests it selected and
// what Provenance's declared governance says about the risk there.
type ImpactEntry struct {
	Path           string
	Tests          []string
	ProvenanceRisk []string
}

// Result is the outcome of one canary check/init/audit run.
type Result struct {
	Gate           string // "pr" | "merge" | "release" | "audit"
	ManifestStatus string // "fresh" | "stale-fallback"
	SelectedTests  []SelectedTest
	Impact         []ImpactEntry
	Audit          *audit.Result
	Problems       []bindings.Problem
}

// Check runs the PR/merge/release gate. For "merge" and "release", the
// diff is ignored entirely and every test in the manifest's registry is
// selected — per-PR passes do not compose into a release guarantee. For
// "pr", the selection is coverage-derived from changedRanges, unioned
// with any vault binding whose glob matches a changed file. A stale
// manifest falls back to the full suite — the safe direction, never a
// block.
func Check(
	gateMode string,
	m manifest.GlobalManifest,
	manifestStale bool,
	changedFiles []string,
	changedRanges map[string][]gitutil.LineRange,
	decisions []vault.Decision,
	provReport *provenancereport.Report,
) Result {
	result := Result{Gate: gateMode, ManifestStatus: "fresh"}

	if gateMode != "pr" {
		result.SelectedTests = fullSuite(m, "full-suite-gate")
		return result
	}

	if manifestStale {
		result.ManifestStatus = "stale-fallback"
		result.SelectedTests = fullSuite(m, "stale-manifest-fallback")
		return result
	}

	coverageTests := manifest.SelectByCoverage(m, changedRanges)
	knownTests := make(map[string]bool, len(m.Tests))
	for _, t := range m.Tests {
		knownTests[t] = true
	}
	forced, problems := bindings.Evaluate(decisions, changedFiles, knownTests)
	result.Problems = problems

	selected := map[string]SelectedTest{}
	for _, t := range coverageTests {
		selected[t] = SelectedTest{Test: t, Reason: "coverage"}
	}
	for _, t := range degradedPackageTests(m, changedFiles) {
		selected[t] = SelectedTest{Test: t, Reason: "degraded-package-fallback"}
	}
	for _, f := range forced {
		selected[f.Test] = SelectedTest{Test: f.Test, Reason: "never-skip-binding", BindingDecision: f.Decision}
	}

	var out []SelectedTest
	for _, s := range selected {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Test < out[j].Test })
	result.SelectedTests = out

	result.Impact = buildImpact(m, changedFiles, out, provReport)
	return result
}

// degradedPackageTests returns every test known for any package under
// m.DegradedPackages that a changed file falls into — the safe-failure
// default for a package whose coverage couldn't be rebuilt (a compile
// failure): force-include everything known about it rather than trust a
// coverage-based selection that might no longer align with the code.
func degradedPackageTests(m manifest.GlobalManifest, changedFiles []string) []string {
	if len(m.DegradedPackages) == 0 {
		return nil
	}
	degraded := make(map[string]bool, len(m.DegradedPackages))
	for _, d := range m.DegradedPackages {
		degraded[d] = true
	}
	touched := map[string]bool{}
	for _, f := range changedFiles {
		dir := ""
		if idx := strings.LastIndex(f, "/"); idx >= 0 {
			dir = f[:idx]
		}
		if degraded[dir] {
			touched[dir] = true
		}
	}
	var out []string
	for dir := range touched {
		out = append(out, manifest.AllTestsUnderPackage(m, dir)...)
	}
	return dedupeSortedStrings(out)
}

func fullSuite(m manifest.GlobalManifest, reason string) []SelectedTest {
	out := make([]SelectedTest, 0, len(m.Tests))
	for _, t := range m.Tests {
		out = append(out, SelectedTest{Test: t, Reason: reason})
	}
	return out
}

func buildImpact(m manifest.GlobalManifest, changedFiles []string, selected []SelectedTest, provReport *provenancereport.Report) []ImpactEntry {
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s.Test] = true
	}

	risk := map[string][]string{}
	if provReport != nil {
		for _, gf := range provReport.GovernedFiles {
			risk[gf.Path] = gf.ImplicatedDecisions
		}
	}

	var impact []ImpactEntry
	for _, f := range changedFiles {
		var testsForFile []string
		for _, r := range m.Coverage[f] {
			for _, t := range r.Tests {
				if selectedSet[t] {
					testsForFile = append(testsForFile, t)
				}
			}
		}
		testsForFile = dedupeSortedStrings(testsForFile)
		if len(testsForFile) == 0 && len(risk[f]) == 0 {
			continue
		}
		impact = append(impact, ImpactEntry{Path: f, Tests: testsForFile, ProvenanceRisk: risk[f]})
	}
	sort.Slice(impact, func(i, j int) bool { return impact[i].Path < impact[j].Path })
	return impact
}

func dedupeSortedStrings(in []string) []string {
	set := map[string]bool{}
	for _, s := range in {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
