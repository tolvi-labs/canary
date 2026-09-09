package cmdinit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
