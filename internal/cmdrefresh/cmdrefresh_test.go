package cmdrefresh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.26\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "canary.yml"), []byte("language: go\n"), 0644); err != nil {
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

func TestRun_NoManifestFails(t *testing.T) {
	dir := setupRepo(t)
	code := Run([]string{"--repo", dir})
	if code != 1 {
		t.Fatalf("expected exit 1 without a manifest, got %d", code)
	}
}

func TestRun_RefreshesAfterNewCommit(t *testing.T) {
	dir := setupRepo(t)
	m, err := manifest.Build(dir, golang.Backend{})
	if err != nil {
		t.Fatalf("manifest.Build failed: %v", err)
	}
	if err := manifest.Save(dir, m); err != nil {
		t.Fatalf("manifest.Save failed: %v", err)
	}
	oldSHA := m.BuiltAtSHA

	if err := os.MkdirAll(filepath.Join(dir, "greet"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greet", "greet.go"), []byte("package greet\n\nfunc Hello() string { return \"hello\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greet", "greet_test.go"), []byte("package greet\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif Hello() != \"hello\" {\n\t\tt.Fatal(\"bad hello\")\n\t}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "add greet")

	code := Run([]string{"--repo", dir})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	updated, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("manifest.Load failed: %v", err)
	}
	if updated.BuiltAtSHA == oldSHA {
		t.Fatal("expected BuiltAtSHA to change after refresh")
	}
	found := false
	for _, tn := range updated.Tests {
		if tn == "TestHello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TestHello in the refreshed manifest, got: %v", updated.Tests)
	}
}
