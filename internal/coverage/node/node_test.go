package node

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
	if got == "" {
		t.Fatal("expected a non-empty module path")
	}
}

func TestListUnits(t *testing.T) {
	repoDir := copyFixture(t)
	got, err := (Backend{}).ListUnits(repoDir)
	if err != nil {
		t.Fatalf("ListUnits failed: %v", err)
	}
	if len(got) != 1 || got[0] != "lib.test.js" {
		t.Fatalf("expected [\"lib.test.js\"], got %v", got)
	}
}

func TestUnitTests(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture", "lib.test.js", workDir)
	if err != nil {
		t.Fatalf("UnitTests failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), result)
	}

	blocks, ok := result["add works"]
	if !ok {
		t.Fatalf("expected \"add works\" in results, got: %v", result)
	}

	var coveredAdd, uncoveredSub bool
	for _, b := range blocks {
		if b.File != "lib.js" {
			continue
		}
		if b.StartLine == 1 && b.Count > 0 {
			coveredAdd = true
		}
		if b.StartLine == 2 && b.Count == 0 {
			uncoveredSub = true
		}
	}
	if !coveredAdd {
		t.Fatalf("expected \"add works\" to cover line 1 (add), got: %+v", blocks)
	}
	if !uncoveredSub {
		t.Fatalf("expected \"add works\" to NOT cover line 2 (sub), got: %+v", blocks)
	}
}

// TestUnitTests_DoesNotLeakUnrelatedFiles guards against UnitTests
// attributing blocks from a file the unit under test never requires.
// testdata/fixture/other/unused.js is never required by lib.test.js, so
// it must never appear in that unit's results. This mirrors the Python
// backend's equivalent regression test (added after a real bug there:
// --cov=repoDir scoped coverage to the whole repo rather than the unit
// under test). Node's --experimental-test-coverage is scoped by the V8
// coverage data itself — it only reports files actually loaded by the
// process, not a static walk of repoDir — so this test is expected to
// pass without a corresponding fix, but it's here to catch a regression
// if that ever changes (e.g. a future flag change broadening the scope).
func TestUnitTests_DoesNotLeakUnrelatedFiles(t *testing.T) {
	repoDir := copyFixture(t)
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture", "lib.test.js", workDir)
	if err != nil {
		t.Fatalf("UnitTests failed: %v", err)
	}

	for testName, blocks := range result {
		for _, b := range blocks {
			if b.File == "other/unused.js" {
				t.Fatalf("test %q unexpectedly covers unrelated file %q: %+v", testName, b.File, b)
			}
		}
	}
}
