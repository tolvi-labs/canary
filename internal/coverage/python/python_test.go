package python

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
	got, err := (Backend{}).ModulePath(repoDir)
	if err != nil {
		t.Fatalf("ModulePath failed: %v", err)
	}
	if got != "fixture" {
		t.Fatalf("expected module path \"fixture\", got %q", got)
	}
}

func TestListUnits(t *testing.T) {
	repoDir := copyFixture(t)
	got, err := (Backend{}).ListUnits(repoDir)
	if err != nil {
		t.Fatalf("ListUnits failed: %v", err)
	}
	if len(got) != 1 || got[0] != "mathutil/test_mathutil.py" {
		t.Fatalf("expected [\"mathutil/test_mathutil.py\"], got %v", got)
	}
}

func TestUnitTests(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture", "mathutil/test_mathutil.py", workDir)
	if err != nil {
		t.Fatalf("UnitTests failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), result)
	}

	blocks, ok := result["test_add"]
	if !ok {
		t.Fatalf("expected test_add in results, got: %v", result)
	}

	var coveredAdd, uncoveredSub bool
	for _, b := range blocks {
		if b.File != "mathutil/__init__.py" {
			continue
		}
		if b.StartLine == 2 && b.Count > 0 {
			coveredAdd = true
		}
		if b.StartLine == 6 && b.Count == 0 {
			uncoveredSub = true
		}
	}
	if !coveredAdd {
		t.Fatalf("expected test_add to cover line 2 (add's body), got: %+v", blocks)
	}
	if !uncoveredSub {
		t.Fatalf("expected test_add to NOT cover line 6 (sub's body), got: %+v", blocks)
	}
}

func TestUnitTests_NoTests(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(repoDir, "mathutil", "test_empty.py"), []byte("# no tests here\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := (Backend{}).UnitTests(repoDir, "fixture", "mathutil/test_empty.py", workDir)
	if err != nil {
		t.Fatalf("expected no error for a file with no tests, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 tests, got %d: %v", len(result), result)
	}
}

// TestUnitTests_DoesNotLeakUnrelatedFiles guards against UnitTests
// attributing blocks from a file the unit under test never imports.
// testdata/fixture/other/unused.py is never imported by
// mathutil/test_mathutil.py, so it must never appear in that unit's
// results — even though coverage.py's own JSON report (scoped to the
// whole repoDir, not just this unit's module) includes it as a
// 0%-covered file regardless.
func TestUnitTests_DoesNotLeakUnrelatedFiles(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture", "mathutil/test_mathutil.py", workDir)
	if err != nil {
		t.Fatalf("UnitTests failed: %v", err)
	}

	for testName, blocks := range result {
		for _, b := range blocks {
			if b.File == "other/unused.py" {
				t.Fatalf("test %q unexpectedly covers unrelated file %q: %+v", testName, b.File, b)
			}
		}
	}
}
