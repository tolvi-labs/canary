package cmdinit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tolvi-labs/canary/internal/langconfig"
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
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "mathutil.go"), []byte("package mathutil\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Sub(a, b int) int { return a - b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "mathutil_test.go"), []byte("package mathutil\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"bad add\")\n\t}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestRun_BuildsAndSavesManifest(t *testing.T) {
	dir := setupRepo(t)
	code := Run([]string{"--repo", dir})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".canary", "global-manifest.json")); err != nil {
		t.Fatalf("expected manifest file to exist: %v", err)
	}
}

func TestRun_AuditFlagPrintsUntestedPath(t *testing.T) {
	dir := setupRepo(t)
	jsonOut := filepath.Join(t.TempDir(), "audit.json")
	code := Run([]string{"--repo", dir, "--audit", "--json-out", jsonOut})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(jsonOut)
	if err != nil {
		t.Fatalf("reading json-out: %v", err)
	}
	if !strings.Contains(string(raw), "mathutil/mathutil.go") {
		t.Fatalf("expected Sub's untested path in audit output, got: %s", raw)
	}
}

func TestRun_ScaffoldsCanaryYmlFromDetection(t *testing.T) {
	dir := setupRepo(t)
	code := Run([]string{"--repo", dir})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "canary.yml"))
	if err != nil {
		t.Fatalf("expected canary.yml to be scaffolded: %v", err)
	}
	if !strings.Contains(string(raw), "language: go") {
		t.Fatalf("expected canary.yml to detect go, got: %s", raw)
	}
}

func TestRun_ReusesExistingCanaryYml(t *testing.T) {
	dir := setupRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "canary.yml"), []byte("language: go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"--repo", dir})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Unchanged — Run must not have rewritten it.
	raw, err := os.ReadFile(filepath.Join(dir, "canary.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "language: go\n" {
		t.Fatalf("expected canary.yml untouched, got: %s", raw)
	}
}

func setupPythonRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"fixture\"\n\n[tool.pytest.ini_options]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mathutil"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "__init__.py"), []byte("def add(a, b):\n    return a + b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "test_mathutil.py"), []byte("from mathutil import add\n\n\ndef test_add():\n    assert add(2, 3) == 5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestRun_PythonRepoEndToEnd(t *testing.T) {
	dir := setupPythonRepo(t)
	code := Run([]string{"--repo", dir, "--lang", "python"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".canary", "global-manifest.json"))
	if err != nil {
		t.Fatalf("expected manifest file to exist: %v", err)
	}
	if !strings.Contains(string(raw), "test_add") {
		t.Fatalf("expected test_add in manifest, got: %s", raw)
	}
}

func setupNodeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "lib.js"), []byte("function add(a, b) { return a + b; }\nmodule.exports = { add };\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.test.js"), []byte("const { test } = require('node:test');\nconst assert = require('node:assert');\nconst { add } = require('./lib.js');\n\ntest('add works', () => {\n  assert.strictEqual(add(2, 3), 5);\n});\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestRun_NodeRepoEndToEnd(t *testing.T) {
	dir := setupNodeRepo(t)
	code := Run([]string{"--repo", dir, "--lang", "node"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".canary", "global-manifest.json"))
	if err != nil {
		t.Fatalf("expected manifest file to exist: %v", err)
	}
	if !strings.Contains(string(raw), "add works") {
		t.Fatalf("expected \"add works\" in manifest, got: %s", raw)
	}
}

func TestRun_LangFlagOverridesDetection(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	// A real Python repo, but one Detect cannot classify: no go.mod, and
	// a pyproject.toml with no [tool.pytest]/[tool.coverage] table, which
	// is exactly what Detect keys off. --lang must skip Detect entirely
	// — while still being a language this repo genuinely is.
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"fixture\"\nversion = \"0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")

	if _, err := langconfig.Detect(dir); err == nil {
		t.Fatal("fixture no longer exercises the override: Detect can classify it on its own")
	}

	code := Run([]string{"--repo", dir, "--lang", "python"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "canary.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "language: python") {
		t.Fatalf("expected canary.yml to say python, got: %s", raw)
	}
}

// TestRun_RejectsLangWithNoProjectMarker replaces what this fixture used
// to assert. A repo with no Python in it at all used to accept `--lang
// python`, build a manifest of "0 packages, 0 tests", exit 0, and
// green-gate every later `canary check` with zero tests selected. The
// design calls that a hard error, and this is it.
func TestRun_RejectsLangWithNoProjectMarker(t *testing.T) {
	dir := setupRepo(t) // a pure-Go repo: go.mod, no Python anywhere

	code := Run([]string{"--repo", dir, "--lang", "python"})
	if code == 0 {
		t.Fatal("expected a non-zero exit for --lang python in a repo with no Python marker")
	}
	if _, err := os.Stat(filepath.Join(dir, "canary.yml")); err == nil {
		t.Fatal("a rejected --lang value must not be written to canary.yml")
	}
	if _, err := os.Stat(filepath.Join(dir, ".canary", "global-manifest.json")); err == nil {
		t.Fatal("a misconfigured repo must not produce a manifest at all")
	}
}

// TestRun_SurfacesMalformedCanaryYml is the recovery path for a bad
// canary.yml already on disk — from a typo'd --lang on an older build,
// a half-written file, or merge-conflict markers in a checked-in config.
// Every later --lang used to be ignored in favour of a bare "canary.yml
// already exists", which never hinted that the file was the problem.
func TestRun_SurfacesMalformedCanaryYml(t *testing.T) {
	dir := setupRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "canary.yml"), []byte("language: [go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"--repo", dir, "--lang", "go"})
	if code == 0 {
		t.Fatal("expected a non-zero exit for a malformed canary.yml")
	}
}

func setupBrokenPythonRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"fixture\"\n\n[tool.pytest.ini_options]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mathutil"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "__init__.py"), []byte("def add(a, b):\n    return a + b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "test_a_good.py"), []byte("from mathutil import add\n\n\ndef test_add():\n    assert add(2, 3) == 5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "test_m_broken.py"), []byte("import a_module_that_does_not_exist\n\n\ndef test_never_collected():\n    assert True\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mathutil", "test_z_after.py"), []byte("def test_after_the_broken_file():\n    assert True\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// TestRun_PythonCollectionFailureIsDegradedNotSilent is the CLI-level
// proof of the Python collection fix. `canary init` on this repo used to
// report "1 packages, 1 tests" with an empty DegradedPackages and exit
// 0: the broken file emitted no collectible test line so it vanished
// from the unit list, and pytest's default abort-on-first-error took the
// files after it down with it. Both surviving files and the broken one
// must now show up, the broken one as a degraded unit.
func TestRun_PythonCollectionFailureIsDegradedNotSilent(t *testing.T) {
	dir := setupBrokenPythonRepo(t)

	code := Run([]string{"--repo", dir, "--lang", "python"})
	if code != 0 {
		t.Fatalf("expected exit 0 (one broken file must not abort the build), got %d", code)
	}
	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}
	if len(m.DegradedPackages) != 1 || m.DegradedPackages[0] != "mathutil/test_m_broken.py" {
		t.Fatalf("expected the broken file in DegradedPackages, got: %v", m.DegradedPackages)
	}
	if len(m.BuildWarnings) != 1 || !strings.Contains(m.BuildWarnings[0], "a_module_that_does_not_exist") {
		t.Fatalf("expected a warning naming the import error, got: %v", m.BuildWarnings)
	}
	// The file after the broken one, in collection order, must survive —
	// this is what pytest's default collection abort silently truncated.
	var foundGood, foundAfter bool
	for _, tn := range m.Tests {
		switch tn {
		case "test_add":
			foundGood = true
		case "test_after_the_broken_file":
			foundAfter = true
		}
	}
	if !foundGood || !foundAfter {
		t.Fatalf("expected both healthy files' tests in the registry, got: %v", m.Tests)
	}
}
