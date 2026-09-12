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
	DegradedPackages []string                  // repo-relative unit scopes (a package dir for Go, a test-file path for Python/Node) that failed to build for coverage on the last build/refresh attempt
	BuildWarnings    []string                  // human-readable detail per degraded unit, for canary init/refresh output
	// TestsByUnit records, per unit (the backend's own unit string, as
	// handed to coverage.Backend.UnitTests), the test names that unit's
	// last successful coverage build produced. Refresh needs it to retire
	// a unit's stale test names: for a package-shaped unit (Go) the
	// coverage entries to invalidate can be identified by directory
	// prefix, but for a file-shaped unit (Python, Node) a test file's
	// coverage lands on arbitrary source files that other units also
	// cover, so the only sound way to invalidate "what this unit
	// previously claimed" is by test name. Absent (a manifest built
	// before this field existed), a refresh falls back to the older
	// prefix-only invalidation and a full `canary init` repopulates it.
	TestsByUnit map[string][]string
}

// Build runs the full per-test coverage instrumentation across every
// package in repoDir and produces a fresh GlobalManifest.
func Build(repoDir string, backend coverage.Backend) (GlobalManifest, error) {
	modulePath, err := backend.ModulePath(repoDir)
	if err != nil {
		return GlobalManifest{}, err
	}
	packages, err := backend.ListUnits(repoDir)
	if err != nil {
		return GlobalManifest{}, err
	}
	return buildFromPackages(repoDir, modulePath, packages, GlobalManifest{
		Coverage: map[string][]CoveredRange{},
	}, backend)
}

// testsByUnitComplete reports whether base.TestsByUnit accounts for
// every unit base already knows about — the only condition under which
// its ownership index can be trusted to retire a stale test name. A
// manifest missing even one unit's entry cannot prove that unit doesn't
// still produce a name buildFromPackages is about to consider retiring.
func testsByUnitComplete(base GlobalManifest) bool {
	for _, pkg := range base.Packages {
		if _, ok := base.TestsByUnit[pkg]; !ok {
			return false
		}
	}
	return true
}

// Refresh re-runs coverage instrumentation only for touchedPackages and
// merges the result into existing, replacing any prior coverage data for
// those units — both the entries under each unit's own scope and every
// coverage attribution to a test name that unit used to produce and no
// longer does (see TestsByUnit). A test name that survives nowhere after
// that is dropped from the registry, so a renamed or deleted test can't
// linger and be handed to a CI runner that no longer has it. A unit that
// is never touched again is still not revisited — a full `canary init`
// rebuild remains the way to clear staleness for a unit that disappeared
// from the repo entirely.
func Refresh(existing GlobalManifest, repoDir string, touchedPackages []string, backend coverage.Backend) (GlobalManifest, error) {
	modulePath, err := backend.ModulePath(repoDir)
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
		TestsByUnit:      existing.TestsByUnit,      // ditto: a unit not rebuilt this round keeps whatever test names it last produced
	}
	return buildFromPackages(repoDir, modulePath, touchedPackages, merged, backend)
}

// buildFromPackages runs coverage.UnitTests for each of packages and
// folds the result into base, then finalizes (sorts, sets BuiltAtSHA). A
// package that fails to compile for coverage does not abort the whole
// build — it is recorded in DegradedPackages/BuildWarnings and its prior
// coverage entries (if any) are left untouched, so gate.Check can force-
// include everything known about it rather than trust a selection that
// might no longer align with the code (see Task 11).
func buildFromPackages(repoDir, modulePath string, packages []string, base GlobalManifest, backend coverage.Backend) (GlobalManifest, error) {
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

	// testsByUnit carries every unit's previously-recorded test names
	// forward; owners inverts it so a name can be retired the moment no
	// unit still produces it. A unit that fails to rebuild keeps both, so
	// its tests are never retired on the strength of a build that didn't
	// happen.
	//
	// Retirement is only trustworthy when base.TestsByUnit accounts for
	// EVERY known unit. A partially-populated index (an older manifest
	// upgraded mid-stream, or one built before this field existed) can
	// only prove "this rebuilt unit no longer claims this name" — it
	// cannot prove no *other*, unaccounted-for unit still claims it too.
	// Treating a name as retired on that incomplete evidence would delete
	// a live test another unit genuinely still produces the moment two
	// units happen to share a name (found by a whole-branch review's own
	// scoped re-review, reproduced with two Node test files sharing a
	// test name and only one of them refreshed). So an incomplete index
	// is handled exactly like an absent one: no retirement this round —
	// each rebuilt unit's own entry still gets recorded fresh below,
	// which incrementally restores completeness, and a full `canary init`
	// restores it in one shot regardless.
	complete := testsByUnitComplete(base)
	testsByUnit := map[string][]string{}
	owners := map[string]map[string]bool{}
	for unit, names := range base.TestsByUnit {
		testsByUnit[unit] = append([]string{}, names...)
		if !complete {
			continue // no ownership index while incomplete — see above
		}
		for _, n := range names {
			if owners[n] == nil {
				owners[n] = map[string]bool{}
			}
			owners[n][unit] = true
		}
	}
	retired := map[string]bool{}

	var warnings []string
	for _, pkg := range sortedPackages {
		testBlocks, err := backend.UnitTests(repoDir, modulePath, pkg, workDir)
		if err != nil {
			degraded[packageDir(pkg)] = true
			warnings = append(warnings, fmt.Sprintf("%s: %v", pkg, err))
			continue // leave this package's prior entries (if any) exactly as they were
		}
		// Rebuilt successfully — drop any stale entries for this package
		// before merging in the fresh ones.
		prefix := packageDir(pkg)
		for k := range seen {
			if FileInScope(k.file, prefix) {
				delete(seen, k)
			}
		}
		// ...and release this unit's claim on the test names it used to
		// produce. Anything it no longer produces, and no other unit
		// does either, is retired below: for a file-shaped unit the
		// prefix sweep above only reaches the test file's own entries,
		// so a renamed test's attributions on the source files it
		// covered would otherwise accumulate forever.
		for _, old := range testsByUnit[pkg] {
			if owners[old] == nil {
				continue
			}
			delete(owners[old], pkg)
			if len(owners[old]) == 0 {
				retired[old] = true
			}
		}
		fresh := make([]string, 0, len(testBlocks))
		for testName, blocks := range testBlocks {
			fresh = append(fresh, testName)
			testSet[testName] = true
			if owners[testName] == nil {
				owners[testName] = map[string]bool{}
			}
			owners[testName][pkg] = true
			for _, b := range blocks {
				k := rangeKey{b.File, b.StartLine, b.EndLine}
				if b.Count > 0 {
					seen[k] = append(seen[k], testName)
				} else if _, exists := seen[k]; !exists {
					seen[k] = []string{}
				}
			}
		}
		sort.Strings(fresh)
		testsByUnit[pkg] = fresh
	}

	// A name is only really retired if nothing re-claimed it later in the
	// loop (two units can legitimately share a test name).
	for name := range retired {
		if len(owners[name]) > 0 {
			delete(retired, name)
			continue
		}
		delete(testSet, name)
		delete(owners, name)
	}
	if len(retired) > 0 {
		for k, tests := range seen {
			kept := make([]string, 0, len(tests))
			for _, t := range tests {
				if !retired[t] {
					kept = append(kept, t)
				}
			}
			seen[k] = kept
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
		TestsByUnit:      testsByUnit,
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

// packageDir turns a backend's unit string into the repo-relative scope
// it owns: a Go package pattern like "./internal/gate" becomes the
// directory "internal/gate" (and "." becomes "", the module root), while
// a file-shaped unit from the Python or Node backend ("lib/test_x.py",
// "lib.test.js") is already its own scope and passes through unchanged.
func packageDir(pkg string) string {
	if pkg == "." {
		return ""
	}
	return strings.TrimPrefix(pkg, "./")
}

// FileInScope reports whether a repo-relative file path falls within a
// unit scope, as produced by packageDir. The same predicate covers both
// unit shapes: a package-shaped scope is a directory, so every file
// under it matches; a file-shaped scope matches only itself. An empty
// scope is the module root — a file with no "/" in its path.
//
// The gate needs this rather than "derive the file's parent directory
// and look it up": a parent directory can never equal a file-shaped
// scope, so a directory-keyed lookup silently never matches a degraded
// Python or Node unit.
func FileInScope(file, scope string) bool {
	if scope == "" {
		return !strings.Contains(file, "/")
	}
	return file == scope || strings.HasPrefix(file, scope+"/")
}

// FileOwnScope returns the ONE scope a file's own unit occupies — its
// immediate containing directory (matching a Go package's own scope), or
// "" for a module-root file with no "/". Unlike FileInScope, this never
// matches an ancestor several levels up: "internal/gate/gate.go"'s own
// scope is "internal/gate", not "internal" — a directory further up the
// tree may itself be degraded without that degradation extending to an
// unrelated sub-package nested inside it. Use this where the question is
// "which single unit does this file itself belong to," and FileInScope
// where the question is "does this degraded unit's coverage cover this
// file" (deliberately broader — its own descendants are its own).
func FileOwnScope(file string) string {
	i := strings.LastIndex(file, "/")
	if i < 0 {
		return ""
	}
	return file[:i]
}
