package manifest

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tolvi-labs/canary/internal/coverage"
	"github.com/tolvi-labs/canary/internal/gitutil"
)

const Schema = "tolvi-canary-global-manifest-v1"

// CoveredRange is one instrumented source-code block and the tests that
// actually executed it (Count > 0 in at least one test's profile). An
// empty Tests list means the block exists but no test currently covers
// it — an "impacted-but-untested" gap for audit mode.
type CoveredRange struct {
	StartLine, EndLine int
	Tests              []string
}

// GlobalManifest is the full, durable test registry + coverage map for a
// repository. It is the only artifact that can drift; canary check's
// staleness check guards against a stale BuiltAtSHA.
type GlobalManifest struct {
	Schema           string
	BuiltAtSHA       string
	Packages         []string
	Tests            []string
	Coverage         map[string][]CoveredRange // key: repo-relative file path
	DegradedPackages []string                  // repo-relative package dirs that failed to compile for coverage on the last build/refresh attempt
	BuildWarnings    []string                  // human-readable detail per degraded package, for canary init/refresh output
}

// Build runs the full per-test coverage instrumentation across every
// package in repoDir and produces a fresh GlobalManifest.
func Build(repoDir string) (GlobalManifest, error) {
	modulePath, err := coverage.ModulePath(repoDir)
	if err != nil {
		return GlobalManifest{}, err
	}
	packages, err := coverage.ListPackages(repoDir)
	if err != nil {
		return GlobalManifest{}, err
	}
	return buildFromPackages(repoDir, modulePath, packages, GlobalManifest{
		Coverage: map[string][]CoveredRange{},
	})
}

// Refresh re-runs coverage instrumentation only for touchedPackages and
// merges the result into existing, replacing any prior coverage data for
// files under those packages. A test that no longer exists anywhere is
// not pruned from the registry in v1 — a full `canary init` rebuild
// clears that staleness; this is a deliberate v1 simplification, not an
// oversight.
func Refresh(existing GlobalManifest, repoDir string, touchedPackages []string) (GlobalManifest, error) {
	modulePath, err := coverage.ModulePath(repoDir)
	if err != nil {
		return GlobalManifest{}, err
	}

	// Coverage for touched packages is seeded wholesale from existing, not
	// pre-dropped: buildFromPackages only replaces a package's entries
	// once it has confirmed a fresh rebuild succeeded, so a package that
	// fails to compile this round keeps whatever it had before instead of
	// losing its data to an aborted rebuild.
	merged := GlobalManifest{
		Coverage:         existing.Coverage,
		Tests:            append([]string{}, existing.Tests...),
		Packages:         unionSorted(existing.Packages, touchedPackages),
		DegradedPackages: existing.DegradedPackages, // buildFromPackages carries this forward for anything not re-evaluated this round
	}
	return buildFromPackages(repoDir, modulePath, touchedPackages, merged)
}

// buildFromPackages runs coverage.PackageTests for each of packages and
// folds the result into base, then finalizes (sorts, sets BuiltAtSHA). A
// package that fails to compile for coverage does not abort the whole
// build — it is recorded in DegradedPackages/BuildWarnings and its prior
// coverage entries (if any) are left untouched, so gate.Check can force-
// include everything known about it rather than trust a selection that
// might no longer align with the code (see Task 11).
func buildFromPackages(repoDir, modulePath string, packages []string, base GlobalManifest) (GlobalManifest, error) {
	workDir, err := os.MkdirTemp("", "canary-coverage-*")
	if err != nil {
		return GlobalManifest{}, err
	}
	defer os.RemoveAll(workDir)

	testSet := map[string]bool{}
	for _, t := range base.Tests {
		testSet[t] = true
	}

	type rangeKey struct {
		file               string
		startLine, endLine int
	}
	seen := map[rangeKey][]string{}
	for file, ranges := range base.Coverage {
		for _, r := range ranges {
			seen[rangeKey{file, r.StartLine, r.EndLine}] = append([]string{}, r.Tests...)
		}
	}

	sortedPackages := append([]string{}, packages...)
	sort.Strings(sortedPackages)

	touchedThisRound := map[string]bool{}
	for _, pkg := range sortedPackages {
		touchedThisRound[packageDir(pkg)] = true
	}
	degraded := map[string]bool{}
	for _, d := range base.DegradedPackages {
		if !touchedThisRound[d] {
			degraded[d] = true // not re-evaluated this round, still unresolved
		}
	}

	var warnings []string
	for _, pkg := range sortedPackages {
		testBlocks, err := coverage.PackageTests(repoDir, modulePath, pkg, workDir)
		if err != nil {
			degraded[packageDir(pkg)] = true
			warnings = append(warnings, fmt.Sprintf("%s: %v", pkg, err))
			continue // leave this package's prior entries (if any) exactly as they were
		}
		// Rebuilt successfully — drop any stale entries for this package
		// before merging in the fresh ones.
		prefix := packageDir(pkg)
		for k := range seen {
			if underAny(k.file, []string{prefix}) {
				delete(seen, k)
			}
		}
		for testName, blocks := range testBlocks {
			testSet[testName] = true
			for _, b := range blocks {
				k := rangeKey{b.File, b.StartLine, b.EndLine}
				if b.Count > 0 {
					seen[k] = append(seen[k], testName)
				} else if _, exists := seen[k]; !exists {
					seen[k] = []string{}
				}
			}
		}
	}

	coverageByFile := map[string][]CoveredRange{}
	for k, tests := range seen {
		coverageByFile[k.file] = append(coverageByFile[k.file], CoveredRange{
			StartLine: k.startLine,
			EndLine:   k.endLine,
			Tests:     dedupeSorted(tests),
		})
	}
	for file := range coverageByFile {
		sort.Slice(coverageByFile[file], func(i, j int) bool {
			return coverageByFile[file][i].StartLine < coverageByFile[file][j].StartLine
		})
	}

	headSHA, err := gitutil.HeadSHA(repoDir)
	if err != nil {
		return GlobalManifest{}, err
	}

	tests := make([]string, 0, len(testSet))
	for t := range testSet {
		tests = append(tests, t)
	}
	sort.Strings(tests)

	degradedList := make([]string, 0, len(degraded))
	for d := range degraded {
		degradedList = append(degradedList, d)
	}
	sort.Strings(degradedList)

	return GlobalManifest{
		Schema:           Schema,
		BuiltAtSHA:       headSHA,
		Packages:         unionSorted(base.Packages, sortedPackages),
		Tests:            tests,
		Coverage:         coverageByFile,
		DegradedPackages: degradedList,
		BuildWarnings:    warnings,
	}, nil
}

func dedupeSorted(in []string) []string {
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

func unionSorted(a, b []string) []string {
	set := map[string]bool{}
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// packageDir strips the leading "./" from a package pattern like
// "./internal/gate" (the form coverage.ListPackages returns), or returns
// "" for the module root (".").
func packageDir(pkg string) string {
	if pkg == "." {
		return ""
	}
	return strings.TrimPrefix(pkg, "./")
}

// underAny reports whether file falls under any of the given
// repo-relative package directories (an empty prefix means the module
// root: a file with no "/" in its path).
func underAny(file string, prefixes []string) bool {
	for _, p := range prefixes {
		if p == "" {
			if !strings.Contains(file, "/") {
				return true
			}
			continue
		}
		if file == p || strings.HasPrefix(file, p+"/") {
			return true
		}
	}
	return false
}
