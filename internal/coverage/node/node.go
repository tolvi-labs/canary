package node

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tolvi-labs/canary/internal/coverage"
)

// Backend implements coverage.Backend for Node's built-in `node:test`
// runner. Unlike the Python backend, this invokes once per test (same
// model as Go) — node:test's coverage has no per-test-context feature
// analogous to coverage.py's, and per-test relaunching is acceptable at
// the scale of a suite like birdie-os-extension's. Revisit if a future
// Node target grows the way birdie-os-sls did, the same way the Go
// engine's own per-test-invocation cost was accepted at its scale
// rather than solved preemptively.
type Backend struct{}

func (Backend) ModulePath(repoDir string) (string, error) {
	if raw, err := os.ReadFile(filepath.Join(repoDir, "package.json")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "\"name\"") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					v := strings.TrimSpace(parts[1])
					v = strings.TrimSuffix(v, ",")
					return strings.Trim(v, "\""), nil
				}
			}
		}
	}
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		return "", err
	}
	return filepath.Base(abs), nil // birdie-os-extension has no package.json at all — the directory name is the fallback
}

// SourceExtensions returns the JavaScript extensions node:test runs —
// the same suffixes TouchedUnits already filters changed files by.
func (Backend) SourceExtensions() []string { return []string{".js", ".mjs", ".cjs"} }

// ListUnits returns every *.test.js/.mjs/.cjs file under repoDir, as
// repo-relative paths. There is no `go list`/`pytest --collect-only`
// equivalent; this is a plain file walk, matching how node:test itself
// discovers files by naming convention.
func (Backend) ListUnits(repoDir string) ([]string, error) {
	var units []string
	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".test.js") || strings.HasSuffix(name, ".test.mjs") || strings.HasSuffix(name, ".test.cjs") {
			rel, err := filepath.Rel(repoDir, path)
			if err != nil {
				return err
			}
			units = append(units, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(units)
	return units, nil
}

// UnitTests lists the individual tests in unit via a dry pass, then runs
// each one in isolation with --experimental-test-coverage and an lcov
// reporter, parsing the resulting DA: (line, hit-count) records.
func (Backend) UnitTests(repoDir, modulePath, unit, workDir string) (map[string][]coverage.Block, error) {
	names, err := listTestNames(repoDir, unit)
	if err != nil {
		return nil, fmt.Errorf("listing tests in %s: %w", unit, err)
	}
	if len(names) == 0 {
		return map[string][]coverage.Block{}, nil
	}

	result := map[string][]coverage.Block{}
	for _, name := range names {
		lcovPath := filepath.Join(workDir, sanitize(unit+"_"+name)+".lcov")
		if err := os.MkdirAll(filepath.Dir(lcovPath), 0755); err != nil {
			return nil, fmt.Errorf("creating lcov dir: %w", err)
		}
		cmd := exec.Command("node", "--test",
			"--experimental-test-coverage",
			"--test-reporter=lcov", "--test-reporter-destination="+lcovPath,
			// Anchored with ^...$: --test-name-pattern's regex otherwise
			// does an unanchored substring match, so an unanchored
			// "sends" would also match a distinct test named "sends
			// twice" and run both together, merging their coverage into
			// this one result entry.
			"--test-name-pattern=^"+regexEscapeExact(name)+"$",
			unit)
		cmd.Dir = repoDir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		_ = cmd.Run() // a failing test still ran and still produced coverage data, same as the Go backend

		blocks, err := parseLCOV(lcovPath, repoDir)
		if err != nil {
			return nil, fmt.Errorf("parsing lcov for %s/%s: %w", unit, name, err)
		}
		result[name] = blocks
	}
	return result, nil
}

// TouchedUnits maps a touched test file directly to itself. A touched
// non-test .js/.mjs/.cjs file maps to every Node unit — same deliberate
// v1 simplification as the Python backend's TouchedUnits, for the same
// reason (no co-located source/test convention to reverse-map through),
// and even less costly here given birdie-os-extension's current scale.
func (b Backend) TouchedUnits(repoDir string, changedFiles []string) []string {
	var touchedTestFiles, touchedOther []string
	for _, f := range changedFiles {
		isJS := strings.HasSuffix(f, ".js") || strings.HasSuffix(f, ".mjs") || strings.HasSuffix(f, ".cjs")
		if !isJS {
			continue
		}
		isTest := strings.HasSuffix(f, ".test.js") || strings.HasSuffix(f, ".test.mjs") || strings.HasSuffix(f, ".test.cjs")
		if isTest {
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

// listTestNames runs unit under node:test with the TAP reporter (no
// coverage) purely to enumerate its test names — "ok N - <name>" lines.
//
// Two TAP points must NOT be treated as test names:
//
//   - A `describe` suite emits its own summary point alongside the
//     points for the `it`/`test` cases inside it, carrying `type:
//     'suite'` in its YAML diagnostic block where a real test carries
//     `type: 'test'`. Counting it would cost an extra per-test coverage
//     invocation and write a suite-named entry into the manifest that
//     no CI invocation can ever run as a test.
//
//   - A file that fails to load at all (syntax error, missing require)
//     is reported by node:test as a single failing top-level point named
//     after the file itself — `not ok 1 - broken.test.js`. Treating that
//     as a test name would fabricate a phantom test in the durable
//     manifest while the file's real tests, which never ran, vanish
//     silently. It is returned as an error instead, so UnitTests fails
//     and the manifest records the unit in DegradedPackages.
func listTestNames(repoDir, unit string) ([]string, error) {
	cmd := exec.Command("node", "--test", "--test-reporter=tap", unit)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			// The command never ran at all (e.g. node missing from PATH)
			// — Node coverage is non-functional for the whole repo, not
			// just "this file has zero tests." Surface it rather than
			// letting UnitTests's len(names)==0 path report a clean,
			// silent empty result indistinguishable from a legitimately
			// empty test file.
			return nil, fmt.Errorf("node --test --test-reporter=tap %s: %w", unit, err)
		}
		// A non-zero exit just means a test failed (node:test exits
		// non-zero on failure); TAP output was still produced on stdout
		// and still lists correctly. Fall through and parse it.
	}

	type tapPoint struct {
		name    string
		indent  int
		failed  bool
		isSuite bool
	}
	var names []string
	var diagnostics []string
	var loadFailed bool
	var cur *tapPoint
	sawPoint := false

	finish := func(p *tapPoint) {
		if p == nil || p.isSuite || p.name == "" {
			return
		}
		if p.failed && p.indent == 0 && p.name == unit {
			// node:test reported the unit file itself as the failing
			// test point: the file never loaded.
			loadFailed = true
			return
		}
		names = append(names, p.name)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		indent := len(raw) - len(strings.TrimLeft(raw, " "))

		if strings.HasPrefix(trimmed, "ok ") || strings.HasPrefix(trimmed, "not ok ") {
			finish(cur)
			sawPoint = true
			name := ""
			if idx := strings.Index(trimmed, "- "); idx >= 0 {
				name = trimmed[idx+2:]
			}
			cur = &tapPoint{name: name, indent: indent, failed: strings.HasPrefix(trimmed, "not ok ")}
			continue
		}
		// A point's YAML diagnostic block is indented relative to the
		// point line itself; nested points are emitted before their
		// parent's summary point, so "more indented than the point we
		// are currently holding" only ever means "part of its block".
		if cur != nil && indent > cur.indent && trimmed == "type: 'suite'" {
			cur.isSuite = true
			continue
		}
		// node prints an unhandled load error as TAP comments before any
		// test point; those lines are the only useful diagnosis of a
		// file-level failure, so keep a bounded number of them.
		if !sawPoint && strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "# Subtest:") && len(diagnostics) < 8 {
			diagnostics = append(diagnostics, strings.TrimPrefix(trimmed, "# "))
		}
	}
	if err := scanner.Err(); err != nil {
		// A truncated scan would silently return a partial test list,
		// which is the same class of quiet data loss this parse exists to
		// prevent — fail instead and let the unit be recorded as degraded.
		return nil, fmt.Errorf("reading node:test TAP output for %s: %w", unit, err)
	}
	finish(cur)

	if loadFailed {
		return nil, fmt.Errorf("%s failed to load under node:test (reported as a failing test point named after the file itself)\n%s",
			unit, strings.Join(diagnostics, "\n"))
	}
	return names, nil
}

// parseLCOV reads a single-file lcov report (one SF:/end_of_record
// block, since UnitTests always runs exactly one test file) and returns
// its DA: (line, hit-count) records as Blocks.
func parseLCOV(path, repoDir string) ([]coverage.Block, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file string
	var blocks []coverage.Block
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SF:") {
			file = strings.TrimPrefix(line, "SF:")
			continue
		}
		if strings.HasPrefix(line, "DA:") {
			parts := strings.SplitN(strings.TrimPrefix(line, "DA:"), ",", 2)
			if len(parts) != 2 {
				continue
			}
			lineNo, err1 := strconv.Atoi(parts[0])
			count, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				continue
			}
			relFile := file
			if abs, err := filepath.Abs(repoDir); err == nil {
				if r, err := filepath.Rel(abs, filepath.Join(repoDir, file)); err == nil {
					relFile = r
				}
			}
			blocks = append(blocks, coverage.Block{
				File: relFile, StartLine: lineNo, EndLine: lineNo, Count: count,
			})
		}
	}
	return blocks, nil
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "_", ".", "_", " ", "_").Replace(s)
}

// regexEscapeExact escapes name's regex metacharacters so it can be
// embedded in a --test-name-pattern value (the flag takes a regex; test
// names themselves are plain text here, but escaping keeps this correct
// if a fixture or real test name ever contains a regex metacharacter).
// It does not anchor the result — the caller must wrap it in ^...$ to
// get an exact match, since --test-name-pattern otherwise does an
// unanchored substring match.
func regexEscapeExact(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`.*+?()[]{}|^$\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
