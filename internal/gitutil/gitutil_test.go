package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
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

func gitRevParse(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	s := string(out)
	return s[:len(s)-1]
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}

func TestChangedFiles(t *testing.T) {
	dir := initRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-q", "-m", "base")
	base := gitRevParse(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "head")
	head := gitRevParse(t, dir)

	files, err := ChangedFiles(dir, base, head)
	if err != nil {
		t.Fatalf("ChangedFiles failed: %v", err)
	}
	if len(files) != 1 || files[0] != "b.go" {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestChangedRanges_AddedLines(t *testing.T) {
	dir := initRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc One() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-q", "-m", "base")
	base := gitRevParse(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc One() {}\n\nfunc Two() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "commit", "-q", "-am", "add Two")
	head := gitRevParse(t, dir)

	ranges, err := ChangedRanges(dir, base, head)
	if err != nil {
		t.Fatalf("ChangedRanges failed: %v", err)
	}
	got, ok := ranges["a.go"]
	if !ok || len(got) != 1 {
		t.Fatalf("expected 1 range for a.go, got: %v", ranges)
	}
	if got[0].Start != 4 || got[0].End != 5 {
		t.Fatalf("unexpected range: %+v", got[0])
	}
}

func TestChangedRanges_PureDeletionContributesNoRanges(t *testing.T) {
	dir := initRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc One() {}\n\nfunc Two() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-q", "-m", "base")
	base := gitRevParse(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc One() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "commit", "-q", "-am", "remove Two")
	head := gitRevParse(t, dir)

	ranges, err := ChangedRanges(dir, base, head)
	if err != nil {
		t.Fatalf("ChangedRanges failed: %v", err)
	}
	if _, ok := ranges["a.go"]; ok {
		t.Fatalf("expected no ranges for a pure deletion, got: %v", ranges["a.go"])
	}
}

func TestHeadSHA(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-q", "-m", "base")
	want := gitRevParse(t, dir)

	got, err := HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA failed: %v", err)
	}
	if got != want {
		t.Fatalf("HeadSHA = %s, want %s", got, want)
	}
}

func TestIsAncestor(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-q", "-m", "base")
	base := gitRevParse(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "b.go")
	runGit(t, dir, "commit", "-q", "-m", "head")
	head := gitRevParse(t, dir)

	ok, err := IsAncestor(dir, base, head)
	if err != nil {
		t.Fatalf("IsAncestor failed: %v", err)
	}
	if !ok {
		t.Fatal("expected base to be an ancestor of head")
	}

	ok, err = IsAncestor(dir, head, base)
	if err != nil {
		t.Fatalf("IsAncestor failed: %v", err)
	}
	if ok {
		t.Fatal("expected head to NOT be an ancestor of base")
	}
}
