package cmdcheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestRun_SelectsCoverageDerivedTests(t *testing.T) {
	dir := setupRepo(t)

	m, err := manifest.Build(dir)
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

func TestRun_RequiresBaseFlag(t *testing.T) {
	dir := setupRepo(t)
	code := Run([]string{"--repo", dir})
	if code != 2 {
		t.Fatalf("expected exit 2 without --base, got %d", code)
	}
}
