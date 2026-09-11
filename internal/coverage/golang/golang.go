package golang

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/tolvi-labs/canary/internal/coverage"
)

// Backend implements coverage.Backend for Go, via `go list`/`go test
// -c -cover`/`go tool covdata` — the original, unchanged v1 engine.
type Backend struct{}

func (Backend) ModulePath(repoDir string) (string, error) {
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ListUnits returns every package under repoDir, as repo-relative
// "./dir" patterns (or "." for the module root) — usable directly both as
// `go test` package arguments and, after stripping the leading "./", as
// repo-relative directories.
func (Backend) ListUnits(repoDir string) ([]string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", "./...")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list ./...: %w", err)
	}
	absRepoDir, err := filepath.Abs(repoDir)
	if err != nil {
		return nil, err
	}
	var units []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(absRepoDir, line)
		if err != nil {
			return nil, fmt.Errorf("relativizing package dir %q: %w", line, err)
		}
		if rel == "." {
			units = append(units, ".")
		} else {
			units = append(units, "./"+filepath.ToSlash(rel))
		}
	}
	sort.Strings(units)
	return units, nil
}

// UnitTests compiles pkg's test binary once (into workDir) and, for
// every test it contains, runs it in isolation with its own coverage
// directory, converts the result to a text profile via `go tool covdata`,
// and parses it with golang.org/x/tools/cover. The expensive
// N-invocation cost is paid once per package here, never at check time.
// A package with no test files is not an error — `go test -c` exits 0
// but writes no binary — and returns an empty map.
func (Backend) UnitTests(repoDir, modulePath, pkg, workDir string) (map[string][]coverage.Block, error) {
	sanitized := strings.NewReplacer("/", "_", ".", "_").Replace(pkg)
	binPath := filepath.Join(workDir, sanitized+".test")

	buildCmd := exec.Command("go", "test", "-c", "-cover", "-o", binPath, pkg)
	buildCmd.Dir = repoDir
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go test -c %s: %w\n%s", pkg, err, out)
	}
	if _, statErr := os.Stat(binPath); statErr != nil {
		return map[string][]coverage.Block{}, nil // no test files in this package
	}

	listOut, err := exec.Command(binPath, "-test.list", ".*").Output()
	if err != nil {
		return nil, fmt.Errorf("listing tests in %s: %w", pkg, err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}

	result := map[string][]coverage.Block{}
	for _, name := range names {
		covDir := filepath.Join(workDir, "cov", sanitized, name)
		if err := os.MkdirAll(covDir, 0755); err != nil {
			return nil, fmt.Errorf("creating coverage dir for %s: %w", name, err)
		}
		runCmd := exec.Command(binPath, "-test.run", "^"+name+"$", "-test.gocoverdir="+covDir)
		// A failing test still executed code up to the point it failed —
		// its exit status doesn't affect what it covered.
		_ = runCmd.Run()

		profilePath := filepath.Join(workDir, "profiles", sanitized, name+".txt")
		if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
			return nil, fmt.Errorf("creating profile dir for %s: %w", name, err)
		}
		textCmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+covDir, "-o="+profilePath)
		if err := textCmd.Run(); err != nil {
			// No coverage data emitted (e.g. the test crashed before it
			// could flush) — treat as zero blocks for this test.
			result[name] = nil
			continue
		}

		profiles, err := cover.ParseProfiles(profilePath)
		if err != nil {
			return nil, fmt.Errorf("parsing coverage profile for %s: %w", name, err)
		}
		var blocks []coverage.Block
		for _, p := range profiles {
			file := strings.TrimPrefix(p.FileName, modulePath+"/")
			for _, b := range p.Blocks {
				blocks = append(blocks, coverage.Block{
					File:      file,
					StartLine: b.StartLine,
					EndLine:   b.EndLine,
					Count:     b.Count,
				})
			}
		}
		result[name] = blocks
	}
	return result, nil
}

// TouchedUnits maps changed .go files to their owning package
// directories, in the "./dir" form ListUnits itself uses. A directory
// that's no longer a buildable package (e.g. it was deleted) is
// silently skipped — nothing to refresh there. Moved from
// internal/cmdrefresh/cmdrefresh.go unchanged.
func (Backend) TouchedUnits(repoDir string, changedFiles []string) []string {
	dirs := map[string]bool{}
	for _, f := range changedFiles {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		dir := filepath.Dir(f)
		if dir == "." {
			dirs["."] = true
		} else {
			dirs["./"+dir] = true
		}
	}
	var out []string
	for d := range dirs {
		cmd := exec.Command("go", "list", d)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}
