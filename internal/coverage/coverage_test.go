package coverage

import (
	"os"
	"path/filepath"
	"testing"
)

func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := "testdata/fixture"
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
	if err != nil {
		t.Fatalf("copying fixture: %v", err)
	}
	return dst
}

func TestModulePath(t *testing.T) {
	repoDir := copyFixture(t)
	got, err := ModulePath(repoDir)
	if err != nil {
		t.Fatalf("ModulePath failed: %v", err)
	}
	if got != "fixture" {
		t.Fatalf("expected module path 'fixture', got %q", got)
	}
}

func TestListPackages(t *testing.T) {
	repoDir := copyFixture(t)
	got, err := ListPackages(repoDir)
	if err != nil {
		t.Fatalf("ListPackages failed: %v", err)
	}
	if len(got) != 1 || got[0] != "./mathutil" {
		t.Fatalf("expected [\"./mathutil\"], got %v", got)
	}
}

func TestPackageTests(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	result, err := PackageTests(repoDir, "fixture", "./mathutil", workDir)
	if err != nil {
		t.Fatalf("PackageTests failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), result)
	}

	addBlocks, ok := result["TestAdd"]
	if !ok {
		t.Fatalf("expected TestAdd in results, got: %v", result)
	}

	var coveredAdd, uncoveredSub bool
	for _, b := range addBlocks {
		if b.File != "mathutil/mathutil.go" {
			t.Fatalf("unexpected file: %s", b.File)
		}
		if b.StartLine == 3 && b.EndLine == 3 && b.Count > 0 {
			coveredAdd = true
		}
		if b.StartLine == 5 && b.EndLine == 5 && b.Count == 0 {
			uncoveredSub = true
		}
	}
	if !coveredAdd {
		t.Fatalf("expected TestAdd to cover line 3 (Add), got: %+v", addBlocks)
	}
	if !uncoveredSub {
		t.Fatalf("expected TestAdd to NOT cover line 5 (Sub), got: %+v", addBlocks)
	}
}

func TestPackageTests_NoTestFiles(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(repoDir, "notested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "notested", "noop.go"), []byte("package notested\n\nfunc Noop() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := PackageTests(repoDir, "fixture", "./notested", workDir)
	if err != nil {
		t.Fatalf("expected no error for a package with no tests, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 tests, got %d: %v", len(result), result)
	}
}
