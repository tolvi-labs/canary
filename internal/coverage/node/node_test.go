package node

import (
	"os"
	"path/filepath"
	"strings"
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

// TestUnitTests_FileThatFailsToLoadIsAnError is the regression test for
// a phantom test. node:test reports a file that won't load (syntax
// error, missing require) as a single failing top-level TAP point named
// after the *file* — "not ok 1 - broken.test.js" — which the name parse
// happily took for a real test. That wrote a test named "broken.test.js"
// into the durable manifest, left DegradedPackages empty, and lost the
// file's real tests without a word. It must be an error instead, so the
// manifest records the unit as degraded.
func TestUnitTests_FileThatFailsToLoadIsAnError(t *testing.T) {
	repoDir := copyFixtureDir(t, "testdata/fixture-broken")
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture-broken", "broken.test.js", workDir)
	if err == nil {
		t.Fatalf("expected an error for a file that fails to load, got results: %v", result)
	}
	if _, phantom := result["broken.test.js"]; phantom {
		t.Fatalf("the unit file itself was recorded as a test: %v", result)
	}
	if !strings.Contains(err.Error(), "broken.test.js") {
		t.Fatalf("expected the error to name the unit, got: %v", err)
	}
	if !strings.Contains(err.Error(), "no-such-module") {
		t.Fatalf("expected the error to carry node's own diagnosis, got: %v", err)
	}
}

// TestUnitTests_SuiteLineIsNotATest is the regression test for
// describe/it suites. node:test emits a TAP point for the `describe`
// suite as well as for each `it` inside it, and trimming every line
// before matching erased the indentation and the type marker that tell
// them apart — so an N-test suite produced N+1 "tests", costing a bogus
// extra coverage invocation and writing a suite-named entry into the
// manifest that no runner can ever execute. Only the real `it`/`test`
// cases may come back.
func TestUnitTests_SuiteLineIsNotATest(t *testing.T) {
	repoDir := copyFixtureDir(t, "testdata/fixture-suite")
	workDir := t.TempDir()

	result, err := (Backend{}).UnitTests(repoDir, "fixture-suite", "suite.test.js", workDir)
	if err != nil {
		t.Fatalf("UnitTests failed: %v", err)
	}
	if _, suite := result["outer suite"]; suite {
		t.Fatalf("the describe suite was recorded as a test: %v", result)
	}
	if len(result) != 3 {
		t.Fatalf("expected exactly 3 tests (covers alpha, covers beta, covers gamma), got %d: %v", len(result), result)
	}
	for _, name := range []string{"covers alpha", "covers beta", "covers gamma"} {
		if _, ok := result[name]; !ok {
			t.Fatalf("expected %q in results, got: %v", name, result)
		}
	}
	// The nested `it` cases must still be instrumented individually —
	// excluding the suite point must not cost the tests inside it their
	// own isolated coverage.
	if !lineCovered(result["covers alpha"], "slib.js", 1) {
		t.Fatalf("expected \"covers alpha\" to cover line 1 (alpha), got: %+v", result["covers alpha"])
	}
	if lineCovered(result["covers alpha"], "slib.js", 2) {
		t.Fatalf("expected \"covers alpha\" to NOT cover line 2 (beta), got: %+v", result["covers alpha"])
	}
	if !lineCovered(result["covers beta"], "slib.js", 2) {
		t.Fatalf("expected \"covers beta\" to cover line 2 (beta), got: %+v", result["covers beta"])
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
