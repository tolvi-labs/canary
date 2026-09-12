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
	Reason          string // "coverage" | "never-skip-binding" | "full-suite-gate" | "stale-manifest-fallback" | "unmapped-code-fallback"
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
	Gate             string // "pr" | "merge" | "release" | "audit"
	ManifestStatus   string // "fresh" | "stale-fallback" | "partial-fallback"
	SelectedTests    []SelectedTest
	Impact           []ImpactEntry
	Audit            *audit.Result
	Problems         []bindings.Problem
	DegradedPackages []string
}

// Check runs the PR/merge/release gate. For "merge" and "release", the
// diff is ignored entirely and every test in the manifest's registry is
// selected — per-PR passes do not compose into a release guarantee. For
// "pr", the selection is coverage-derived from changedRanges, unioned
// with any vault binding whose glob matches a changed file. A stale
// manifest falls back to the full suite — the safe direction, never a
// block.
//
// sourceExtensions comes from the repo's own coverage backend (via
// canary.yml), and scopes the unmapped-code safety net below to the
// files that backend's coverage is supposed to account for. It must
// never be hardcoded: the net is what makes Canary a gate rather than a
// suggestion, and a hardcoded language silently disables it everywhere
// else. An empty list applies the net to every changed file.
func Check(
	gateMode string,
	m manifest.GlobalManifest,
	manifestStale bool,
	changedFiles []string,
	changedRanges map[string][]gitutil.LineRange,
	decisions []vault.Decision,
	provReport *provenancereport.Report,
	sourceExtensions []string,
) Result {
	result := Result{Gate: gateMode, ManifestStatus: "fresh"}
	result.DegradedPackages = m.DegradedPackages

	if gateMode != "pr" {
		result.SelectedTests = fullSuite(m, "full-suite-gate")
		return result
	}

	if manifestStale {
		result.ManifestStatus = "stale-fallback"
		result.SelectedTests = fullSuite(m, "stale-manifest-fallback")
		return result
	}

	for file := range changedRanges {
		if !isSourceFile(file, sourceExtensions) {
			continue
		}
		if _, ok := m.Coverage[file]; ok {
			continue
		}
		// A file inside a known degraded unit isn't silently skipped —
		// degradedPackageTests below already force-includes everything
		// known about that unit, but only if it actually has coverage
		// history to pull from. A unit that has never successfully built
		// for coverage (e.g. a compile error from day one) has nothing in
		// m.Coverage, so AllTestsUnderPackage would return nothing for it
		// either. Only a degraded unit with real coverage history is
		// excluded here; a degraded unit with zero history falls through
		// to the same full-suite fallback as a file the manifest has
		// never heard of at all.
		//
		// This check deliberately requires an exact match on the file's
		// own unit (manifest.FileOwnScope), not FileInScope's broader
		// ancestor-descendant match that degradedPackageTests uses below.
		// A Go package's directory can contain an unrelated, healthy
		// sub-package nested inside it; FileInScope would treat every
		// file under the degraded parent's directory as "covered" by that
		// degradation, silently exempting a genuinely unmapped sibling
		// package's new file from this safety net (found by a
		// whole-branch review's own scoped re-review of its fix for the
		// symmetrical Python/Node file-shaped-unit gap this same net
		// closes below).
		own := manifest.FileOwnScope(file)
		var handledByDegraded bool
		for _, scope := range m.DegradedPackages {
			if scope != file && scope != own {
				continue
			}
			if len(manifest.AllTestsUnderPackage(m, scope)) > 0 {
				handledByDegraded = true
				break
			}
		}
		if handledByDegraded {
			continue
		}
		result.ManifestStatus = "partial-fallback"
		result.SelectedTests = fullSuite(m, "unmapped-code-fallback")
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

// degradedPackageTests returns every test known for any degraded unit a
// changed file falls within — the safe-failure default for a unit whose
// coverage couldn't be rebuilt (a Go compile failure, a pytest
// collection error, a Node test file that won't load): force-include
// everything known about it rather than trust a coverage-based selection
// that might no longer align with the code.
func degradedPackageTests(m manifest.GlobalManifest, changedFiles []string) []string {
	if len(m.DegradedPackages) == 0 {
		return nil
	}
	touched := map[string]bool{}
	for _, f := range changedFiles {
		for _, scope := range degradedScopesFor(m.DegradedPackages, f) {
			touched[scope] = true
		}
	}
	var out []string
	for scope := range touched {
		out = append(out, manifest.AllTestsUnderPackage(m, scope)...)
	}
	return dedupeSortedStrings(out)
}

// degradedScopesFor returns every degraded unit scope a changed file
// falls within. Scopes are not all directories: a Go unit's scope is its
// package directory, but a Python or Node unit's scope is a single test
// file, which is why membership is tested with manifest.FileInScope
// rather than by deriving the file's parent directory and looking that
// up — a parent directory never equals a file path, so the directory
// form silently matched nothing at all for the file-shaped backends.
func degradedScopesFor(degraded []string, file string) []string {
	var out []string
	for _, scope := range degraded {
		if manifest.FileInScope(file, scope) {
			out = append(out, scope)
		}
	}
	return out
}

// isSourceFile reports whether a changed file is one the repo's backend
// produces coverage for. An empty extension list means "every file" —
// the conservative reading, so a caller that failed to supply the repo's
// real extensions over-triggers the fallback rather than disabling it.
func isSourceFile(file string, extensions []string) bool {
	if len(extensions) == 0 {
		return true
	}
	for _, ext := range extensions {
		if strings.HasSuffix(file, ext) {
			return true
		}
	}
	return false
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
