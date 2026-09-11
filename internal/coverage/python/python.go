package python

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tolvi-labs/canary/internal/coverage"
)

// Backend implements coverage.Backend for Python, via pytest and
// coverage.py's dynamic-context feature. Unlike the Go backend, it does
// not re-invoke per test — a single `pytest --cov-context=test` run per
// file, then a query for per-line test attribution, avoids the
// per-process-launch cost a literal per-test port of the Go model would
// pay against a suite the size of birdie-os-sls's (2,100+ tests).
type Backend struct{}

func (Backend) ModulePath(repoDir string) (string, error) {
	if raw, err := os.ReadFile(filepath.Join(repoDir, "pyproject.toml")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					return strings.Trim(strings.TrimSpace(parts[1]), "\"'"), nil
				}
			}
		}
	}
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		return "", err
	}
	return filepath.Base(abs), nil
}

// ListUnits returns every pytest-collectible test file under repoDir, as
// repo-relative paths — one unit per file, not per individual test:
// coverage.py's context feature attributes to individual tests within
// one file in a single run, so per-file is the right granularity for
// the "how many processes do we launch" question.
func (Backend) ListUnits(repoDir string) ([]string, error) {
	cmd := exec.Command("python3", "-m", "pytest", "--collect-only", "-q")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			// The command never ran at all (e.g. python3/pytest missing
			// from PATH) — Python coverage is non-functional for the
			// whole repo, not just degraded for one file. Surface it
			// rather than silently reporting zero units.
			return nil, fmt.Errorf("python3 -m pytest --collect-only: %w", err)
		}
		// A non-zero exit with a collection error in one file shouldn't
		// abort listing every other file; UnitTests reports per-file
		// failures individually. Fall through and parse whatever was
		// collected.
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "::") {
			continue
		}
		file := strings.SplitN(line, "::", 2)[0]
		if strings.HasSuffix(file, ".py") {
			seen[file] = true
		}
	}
	var units []string
	for f := range seen {
		units = append(units, f)
	}
	sort.Strings(units)
	return units, nil
}

type coverageJSON struct {
	Files map[string]struct {
		ExecutedLines []int               `json:"executed_lines"`
		MissingLines  []int               `json:"missing_lines"`
		Contexts      map[string][]string `json:"contexts"`
	} `json:"files"`
}

// UnitTests runs unit (one test file) once with coverage.py dynamic
// contexts enabled, then queries coverage.py's own JSON report for
// per-line test attribution — never by embedding Python from Go.
// COVERAGE_CORE=ctrace is required: coverage.py's newer sysmon core
// (default on Python 3.12+) warns "Dynamic contexts aren't supported
// with core=sysmon; context data may be incomplete" and cannot be
// trusted here.
func (Backend) UnitTests(repoDir, modulePath, unit, workDir string) (map[string][]coverage.Block, error) {
	dataFile := filepath.Join(workDir, strings.NewReplacer("/", "_").Replace(unit)+".coverage")

	runCmd := exec.Command("python3", "-m", "pytest", unit,
		"--cov="+repoDir, "--cov-context=test", "--cov-report=")
	runCmd.Dir = repoDir
	runCmd.Env = append(os.Environ(), "COVERAGE_CORE=ctrace", "COVERAGE_FILE="+dataFile)
	var stderr bytes.Buffer
	runCmd.Stderr = &stderr
	if err := runCmd.Run(); err != nil {
		if _, statErr := os.Stat(dataFile); statErr != nil {
			// No coverage data at all — treat as "collection failed",
			// same class of degraded-unit outcome the Go backend's
			// compile-failure path represents.
			return nil, fmt.Errorf("pytest %s: %w\n%s", unit, err, stderr.String())
		}
		// pytest exits non-zero on a failing test, which still ran and
		// still produced valid coverage data — not an error here (same
		// reasoning as the Go backend not treating a failing test's
		// exit status as fatal).
	}

	if _, err := os.Stat(dataFile); err != nil {
		return map[string][]coverage.Block{}, nil // a file with no tests produced no coverage data at all
	}

	jsonCmd := exec.Command("python3", "-m", "coverage", "json", "--show-contexts", "-o", "-")
	jsonCmd.Dir = repoDir
	jsonCmd.Env = append(os.Environ(), "COVERAGE_FILE="+dataFile)
	var jsonStderr bytes.Buffer
	jsonCmd.Stderr = &jsonStderr
	jsonOut, err := jsonCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("coverage json for %s: %w\n%s", unit, err, jsonStderr.String())
	}

	var report coverageJSON
	if err := json.Unmarshal(jsonOut, &report); err != nil {
		return nil, fmt.Errorf("parsing coverage json for %s: %w", unit, err)
	}

	// relFile is computed once per raw file path (not once per pass) —
	// filepath.Rel is called from two loops below and always returns the
	// same answer for the same file.
	relFileCache := map[string]string{}
	relFileFor := func(file string) string {
		if rf, ok := relFileCache[file]; ok {
			return rf
		}
		rf, relErr := filepath.Rel(repoDir, file)
		if relErr != nil {
			rf = file
		}
		relFileCache[file] = rf
		return rf
	}

	result := map[string][]coverage.Block{}
	testNames := map[string]bool{}
	// touchedFiles is the set of files this unit's test session actually
	// attributed to a real test somewhere — NOT every file coverage.py
	// happened to report on. --cov=repoDir measures the whole repo (so
	// that pytest-cov can find the module under test without our own
	// code knowing where it lives), so report.Files also includes files
	// this unit's tests never imported at all, each 0%-covered with
	// every line in MissingLines. Without this filter, pass 2 below
	// would attribute every one of THOSE unrelated files' lines as
	// Count:0 blocks to every test in this unit too.
	touchedFiles := map[string]bool{}
	// One pass to discover every test name mentioned anywhere, and to
	// record each test's directly-covered lines.
	for file, data := range report.Files {
		relFile := relFileFor(file)
		for lineStr, contexts := range data.Contexts {
			line := atoiOrZero(lineStr)
			for _, ctx := range contexts {
				if ctx == "" {
					continue // import/collection-time execution, not attributable to a specific test
				}
				testName := testNameFromContext(ctx)
				testNames[testName] = true
				touchedFiles[file] = true
				result[testName] = append(result[testName], coverage.Block{
					File: relFile, StartLine: line, EndLine: line, Count: 1,
				})
			}
		}
	}
	// Second pass: every test's block list also carries every genuinely
	// never-executed line (Count: 0), but only in files this unit's own
	// tests actually touched (per touchedFiles above) — mirroring the Go
	// backend's own per-test profile, which includes a *package's*
	// zero-count blocks, not the whole module's. Without this pass, an
	// uncovered line with zero tests attributed would never reach
	// manifest.buildFromPackages's `seen[k]` initialization at all.
	for testName := range testNames {
		for file, data := range report.Files {
			if !touchedFiles[file] {
				continue
			}
			relFile := relFileFor(file)
			for _, line := range data.MissingLines {
				result[testName] = append(result[testName], coverage.Block{
					File: relFile, StartLine: line, EndLine: line, Count: 0,
				})
			}
		}
	}
	return result, nil
}

// TouchedUnits maps a touched test file directly to itself. A touched
// non-test .py file maps to every Python unit — a full rebuild of the
// Python portion, not a precise reverse-lookup from source file to the
// test file(s) that cover it. Deliberate v1 simplification: unlike Go,
// where a package's test and source files are co-located so touching
// either one identifies the same package to rebuild, pytest test files
// commonly live in a separate directory from the source they exercise,
// so a path-only reverse mapping isn't available without consulting the
// existing coverage manifest — the same category of v1 limitation
// internal/manifest/global.go's own Refresh already documents for
// pruning removed tests (resolved by a full `canary init`, not solved
// incrementally).
func (b Backend) TouchedUnits(repoDir string, changedFiles []string) []string {
	var touchedTestFiles, touchedOther []string
	for _, f := range changedFiles {
		if !strings.HasSuffix(f, ".py") {
			continue
		}
		base := filepath.Base(f)
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			touchedTestFiles = append(touchedTestFiles, f)
		} else {
			touchedOther = append(touchedOther, f)
		}
	}
	if len(touchedOther) == 0 {
		sort.Strings(touchedTestFiles)
		return touchedTestFiles
	}
	all, err := b.ListUnits(repoDir)
	if err != nil {
		return touchedTestFiles
	}
	return all
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// testNameFromContext parses pytest-cov's context label, e.g.
// "mathutil/test_mathutil.py::test_add|run", into just "test_add".
func testNameFromContext(ctx string) string {
	ctx = strings.TrimSuffix(ctx, "|run")
	if i := strings.LastIndex(ctx, "::"); i >= 0 {
		return ctx[i+2:]
	}
	return ctx
}
