package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := initGitRepo(t)
	want := GlobalManifest{
		Schema:     Schema,
		BuiltAtSHA: gitRevParse(t, dir),
		Packages:   []string{"./mathutil"},
		Tests:      []string{"TestAdd"},
		Coverage: map[string][]CoveredRange{
			"mathutil/mathutil.go": {{StartLine: 3, EndLine: 3, Tests: []string{"TestAdd"}}},
		},
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestRelPath)); err != nil {
		t.Fatalf("expected manifest file to exist: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.BuiltAtSHA != want.BuiltAtSHA || len(got.Tests) != 1 || got.Tests[0] != "TestAdd" {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoad_MissingReturnsErrManifestNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if !errors.Is(err, ErrManifestNotFound) {
		t.Fatalf("expected ErrManifestNotFound, got: %v", err)
	}
}

func TestStale_FreshWhenBuiltAtSHAIsAncestorOfHead(t *testing.T) {
	dir := initGitRepo(t)
	base := gitRevParse(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "b.txt")
	runGit(t, dir, "commit", "-q", "-m", "second")

	stale, err := Stale(GlobalManifest{BuiltAtSHA: base}, dir)
	if err != nil {
		t.Fatalf("Stale failed: %v", err)
	}
	if stale {
		t.Fatal("expected fresh (BuiltAtSHA is an ancestor of HEAD)")
	}
}

func TestStale_TrueForEmptyBuiltAtSHA(t *testing.T) {
	dir := initGitRepo(t)
	stale, err := Stale(GlobalManifest{}, dir)
	if err != nil {
		t.Fatalf("Stale failed: %v", err)
	}
	if !stale {
		t.Fatal("expected stale for an empty BuiltAtSHA")
	}
}
