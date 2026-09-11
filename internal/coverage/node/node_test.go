package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tolvi-labs/canary/internal/coverage"
)

func copyFixture(t *testing.T) string {
	t.Helper()
	return copyFixtureDir(t, "testdata/fixture")
}

// copyFixtureDir copies src (a testdata fixture directory) into a fresh
// t.TempDir(). Generalized out of copyFixture so a second, independent
// fixture (testdata/fixture-namecollision) can be copied without adding
// files to testdata/fixture and perturbing the other tests' assumptions
// about what ListUnits finds there.
func copyFixtureDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
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
		t.Fatalf("copying fixture %s: %v", src, err)
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

// TestUnitTests_ExactNameMatch guards against --test-name-pattern doing
// an unanchored substring match. testdata/fixture-namecollision has two
// tests, "sends" and "sends twice" — "sends" is a substring of "sends
// twice" — each exercising a distinct, otherwise-never-called function
// in substr.js. An unanchored pattern for "sends" would also match
// "sends twice" (node:test runs both together as one process), merging
// sendsTwice's coverage into the "sends" result; the fix anchors the
// pattern with ^...$ so each name's run is isolated to just that test.
func TestUnitTests_ExactNameMatch(t *testing.T) {
	repoDir := copyFixtureDir(t, "testdata/fixture-namecollision")
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture-namecollision", "substr.test.js", workDir)
	if err != nil {
		t.Fatalf("UnitTests failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 tests, got %d: %v", len(result), result)
	}

	sends, ok := result["sends"]
	if !ok {
		t.Fatalf("expected \"sends\" in results, got: %v", result)
	}
	sendsTwice, ok := result["sends twice"]
	if !ok {
		t.Fatalf("expected \"sends twice\" in results, got: %v", result)
	}

	if !lineCovered(sends, "substr.js", 1) {
		t.Fatalf("expected \"sends\" to cover line 1 (sendsOnce), got: %+v", sends)
	}
	if lineCovered(sends, "substr.js", 2) {
		t.Fatalf("expected \"sends\" to NOT cover line 2 (sendsTwice) — substring-match leak, got: %+v", sends)
	}

	if !lineCovered(sendsTwice, "substr.js", 2) {
		t.Fatalf("expected \"sends twice\" to cover line 2 (sendsTwice), got: %+v", sendsTwice)
	}
	if lineCovered(sendsTwice, "substr.js", 1) {
		t.Fatalf("expected \"sends twice\" to NOT cover line 1 (sendsOnce) — substring-match leak, got: %+v", sendsTwice)
	}
}

func lineCovered(blocks []coverage.Block, file string, line int) bool {
	for _, b := range blocks {
		if b.File == file && b.StartLine == line && b.Count > 0 {
			return true
		}
	}
	return false
}
