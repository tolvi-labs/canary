package cmdcheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tolvi-labs/canary/internal/cmdinit"
	"github.com/tolvi-labs/canary/internal/coverage/golang"
	"github.com/tolvi-labs/canary/internal/manifest"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitRevParse(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	s := string(out)
	return s[:len(s)-1]
}

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.26\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mathutil"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "mathutil.go"), []byte("package mathutil\n\nfunc Add(a, b int) int { return a + b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "mathutil_test.go"), []byte("package mathutil\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"bad add\")\n\t}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "canary.yml"), []byte("language: go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestRun_SelectsCoverageDerivedTests(t *testing.T) {
	dir := setupRepo(t)

	m, err := manifest.Build(dir, golang.Backend{})
	if err != nil {
		t.Fatalf("manifest.Build failed: %v", err)
	}
	if err := manifest.Save(dir, m); err != nil {
		t.Fatalf("manifest.Save failed: %v", err)
	}
	base := m.BuiltAtSHA

	if err := os.WriteFile(filepath.Join(dir, "mathutil", "mathutil.go"), []byte("package mathutil\n\nfunc Add(a, b int) int { return a + b + 0 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "commit", "-q", "-am", "tweak Add")

	jsonOut := filepath.Join(t.TempDir(), "report.json")
	code := Run([]string{"--repo", dir, "--base", base, "--head", "HEAD", "--json-out", jsonOut})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	raw, err := os.ReadFile(jsonOut)
	if err != nil {
		t.Fatalf("reading json-out: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["manifest_status"] != "fresh" {
		t.Fatalf("expected fresh manifest, got: %v", parsed["manifest_status"])
	}
	selected, ok := parsed["selected_tests"].([]interface{})
	if !ok || len(selected) != 1 {
		t.Fatalf("expected exactly 1 selected test, got: %v", parsed["selected_tests"])
	}
	first := selected[0].(map[string]interface{})
	if first["test"] != "TestAdd" {
		t.Fatalf("expected TestAdd selected, got: %v", first)
	}
}

func TestRun_MissingManifestFallsBackToFullSuite(t *testing.T) {
	dir := setupRepo(t)
	head := gitRevParse(t, dir)

	jsonOut := filepath.Join(t.TempDir(), "report.json")
	code := Run([]string{"--repo", dir, "--base", head, "--head", "HEAD", "--json-out", jsonOut})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(jsonOut)
	if err != nil {
		t.Fatalf("reading json-out: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["manifest_status"] != "stale-fallback" {
		t.Fatalf("expected stale-fallback, got: %v", parsed["manifest_status"])
	}
}

func TestRun_MergeGateSucceedsWithAllZerosBase(t *testing.T) {
	// GitHub sets `before` to all-zeros on a branch's first push, which
	// is not a resolvable git ref. The merge gate never reads the diff,
	// so this must succeed rather than failing on ChangedFiles/ChangedRanges.
	dir := setupRepo(t)
	allZeros := "0000000000000000000000000000000000000000"
	code := Run([]string{"--repo", dir, "--base", allZeros, "--head", "HEAD", "--gate", "merge"})
	if code != 0 {
		t.Fatalf("expected exit 0 for merge gate with an all-zeros base, got %d", code)
	}
}

func TestRun_RequiresBaseFlag(t *testing.T) {
	dir := setupRepo(t)
	code := Run([]string{"--repo", dir})
	if code != 2 {
		t.Fatalf("expected exit 2 without --base, got %d", code)
	}
}

func TestRun_RequiresCanaryYml(t *testing.T) {
	dir := setupRepo(t)
	if err := os.Remove(filepath.Join(dir, "canary.yml")); err != nil {
		t.Fatal(err)
	}
	head := gitRevParse(t, dir)
	// `check` has to know which backend the repo uses to scope its
	// unmapped-code safety net, so — like `refresh` — it can't proceed
	// without canary.yml and must not quietly assume a language.
	code := Run([]string{"--repo", dir, "--base", head, "--head", "HEAD"})
	if code != 2 {
		t.Fatalf("expected exit 2 with no canary.yml, got %d", code)
	}
}

func TestRun_RejectsLanguageWithNoProjectMarker(t *testing.T) {
	dir := setupRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "canary.yml"), []byte("language: python\n"), 0644); err != nil {
		t.Fatal(err)
	}
	head := gitRevParse(t, dir)
	// A pure-Go repo declaring python used to check out green with zero
	// tests selected; a misconfigured repo must fail loudly instead.
	code := Run([]string{"--repo", dir, "--base", head, "--head", "HEAD"})
	if code != 2 {
		t.Fatalf("expected exit 2 for a language with no matching project marker, got %d", code)
	}
}

// readReport runs the gate and parses its JSON output.
func readReport(t *testing.T, dir, base string) map[string]interface{} {
	t.Helper()
	jsonOut := filepath.Join(t.TempDir(), "report.json")
	if code := Run([]string{"--repo", dir, "--base", base, "--head", "HEAD", "--json-out", jsonOut}); code != 0 {
		t.Fatalf("expected exit 0 from canary check, got %d", code)
	}
	raw, err := os.ReadFile(jsonOut)
	if err != nil {
		t.Fatalf("reading json-out: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return parsed
}

func selectedTestNames(t *testing.T, report map[string]interface{}) []string {
	t.Helper()
	raw, _ := report["selected_tests"].([]interface{})
	var out []string
	for _, r := range raw {
		out = append(out, r.(map[string]interface{})["test"].(string))
	}
	return out
}

// TestEndToEnd_PythonInitThenCheck is one of the two end-to-end tests the
// design called for and the branch shipped without: `canary init`
// followed by a real two-commit diff through `canary check`, for a
// non-Go repo. Its absence is why the gate's safety net could be
// hardcoded to ".go" and go unnoticed — no test ever ran the gate
// against a Python or Node repo at all.
func TestEndToEnd_PythonInitThenCheck(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"fixture\"\nversion = \"0\"\n\n[tool.pytest.ini_options]\n")
	writeFile(t, dir, "mathutil/__init__.py", "def add(a, b):\n    return a + b\n")
	writeFile(t, dir, "mathutil/test_mathutil.py", "from mathutil import add\n\n\ndef test_add():\n    assert add(2, 3) == 5\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")

	if code := cmdinit.Run([]string{"--repo", dir, "--lang", "python"}); code != 0 {
		t.Fatalf("canary init failed with exit %d", code)
	}
	base := gitRevParse(t, dir)

	// Commit 2: edit a covered line. The gate should narrow to test_add.
	writeFile(t, dir, "mathutil/__init__.py", "def add(a, b):\n    return b + a\n")
	runGit(t, dir, "commit", "-q", "-am", "tweak add")

	report := readReport(t, dir, base)
	if report["manifest_status"] != "fresh" {
		t.Fatalf("expected fresh, got %v", report["manifest_status"])
	}
	selected := selectedTestNames(t, report)
	if len(selected) != 1 || selected[0] != "test_add" {
		t.Fatalf("expected exactly [test_add], got %v", selected)
	}

	// Commit 3: add a brand-new, entirely untested Python file. The
	// manifest has no coverage for it at all, so the gate must fall back
	// to the full suite rather than select nothing.
	writeFile(t, dir, "mathutil/untested.py", "def brand_new():\n    return 1\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "add an untested module")

	report = readReport(t, dir, base)
	if report["manifest_status"] != "partial-fallback" {
		t.Fatalf("expected partial-fallback for an unmapped .py file, got %v (selected %v)", report["manifest_status"], selectedTestNames(t, report))
	}
	if got := selectedTestNames(t, report); len(got) != 1 || got[0] != "test_add" {
		t.Fatalf("expected the full suite, got %v", got)
	}
}

// TestEndToEnd_NodeInitThenCheck is the Node half of the same gap.
func TestEndToEnd_NodeInitThenCheck(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "lib.js", "function add(a, b) { return a + b; }\nmodule.exports = { add };\n")
	writeFile(t, dir, "lib.test.js", "const { test } = require('node:test');\nconst assert = require('node:assert');\nconst { add } = require('./lib.js');\n\ntest('add works', () => {\n  assert.strictEqual(add(2, 3), 5);\n});\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")

	if code := cmdinit.Run([]string{"--repo", dir, "--lang", "node"}); code != 0 {
		t.Fatalf("canary init failed with exit %d", code)
	}
	base := gitRevParse(t, dir)

	writeFile(t, dir, "lib.js", "function add(a, b) { return b + a; }\nmodule.exports = { add };\n")
	runGit(t, dir, "commit", "-q", "-am", "tweak add")

	report := readReport(t, dir, base)
	if report["manifest_status"] != "fresh" {
		t.Fatalf("expected fresh, got %v", report["manifest_status"])
	}
	if got := selectedTestNames(t, report); len(got) != 1 || got[0] != "add works" {
		t.Fatalf("expected exactly [add works], got %v", got)
	}

	writeFile(t, dir, "untested.js", "function brandNew() { return 1; }\nmodule.exports = { brandNew };\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "add an untested module")

	report = readReport(t, dir, base)
	if report["manifest_status"] != "partial-fallback" {
		t.Fatalf("expected partial-fallback for an unmapped .js file, got %v (selected %v)", report["manifest_status"], selectedTestNames(t, report))
	}
	if got := selectedTestNames(t, report); len(got) != 1 || got[0] != "add works" {
		t.Fatalf("expected the full suite, got %v", got)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
