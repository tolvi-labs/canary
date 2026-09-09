package manifest

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

func copyFixtureRepo(t *testing.T) string {
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
	runGit(t, dst, "init", "-q")
	runGit(t, dst, "config", "user.email", "test@example.com")
	runGit(t, dst, "config", "user.name", "test")
	runGit(t, dst, "add", "-A")
	runGit(t, dst, "commit", "-q", "-m", "base")
	return dst
}

func coveredRange(m GlobalManifest, file string, start, end int) (CoveredRange, bool) {
	for _, r := range m.Coverage[file] {
		if r.StartLine == start && r.EndLine == end {
			return r, true
		}
	}
	return CoveredRange{}, false
}

func TestBuild(t *testing.T) {
	repoDir := copyFixtureRepo(t)
	head := gitRevParse(t, repoDir)

	m, err := Build(repoDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if m.BuiltAtSHA != head {
		t.Fatalf("BuiltAtSHA = %s, want %s", m.BuiltAtSHA, head)
	}
	if len(m.Tests) != 2 || m.Tests[0] != "TestAdd" || m.Tests[1] != "TestHello" {
		t.Fatalf("unexpected Tests: %v", m.Tests)
	}
	if len(m.Packages) != 2 || m.Packages[0] != "./greet" || m.Packages[1] != "./mathutil" {
		t.Fatalf("unexpected Packages: %v", m.Packages)
	}

	add, ok := coveredRange(m, "mathutil/mathutil.go", 3, 3)
	if !ok || len(add.Tests) != 1 || add.Tests[0] != "TestAdd" {
		t.Fatalf("expected Add covered by TestAdd, got: %+v (ok=%v)", add, ok)
	}
	sub, ok := coveredRange(m, "mathutil/mathutil.go", 5, 5)
	if !ok || len(sub.Tests) != 0 {
		t.Fatalf("expected Sub to be untested, got: %+v (ok=%v)", sub, ok)
	}
	hello, ok := coveredRange(m, "greet/greet.go", 3, 3)
	if !ok || len(hello.Tests) != 1 || hello.Tests[0] != "TestHello" {
		t.Fatalf("expected Hello covered by TestHello, got: %+v (ok=%v)", hello, ok)
	}
}

func TestRefresh_OnlyRebuildsTouchedPackage(t *testing.T) {
	repoDir := copyFixtureRepo(t)
	existing, err := Build(repoDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "greet", "greet.go"), []byte("package greet\n\nfunc Hello() string { return \"hello\" }\n\nfunc Bye() string { return \"bye\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "greet", "greet_test.go"), []byte("package greet\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif Hello() != \"hello\" {\n\t\tt.Fatal(\"bad hello\")\n\t}\n}\n\nfunc TestBye(t *testing.T) {\n\tif Bye() != \"bye\" {\n\t\tt.Fatal(\"bad bye\")\n\t}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "commit", "-q", "-am", "add Bye")
	newHead := gitRevParse(t, repoDir)

	updated, err := Refresh(existing, repoDir, []string{"./greet"})
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if updated.BuiltAtSHA != newHead {
		t.Fatalf("BuiltAtSHA = %s, want %s", updated.BuiltAtSHA, newHead)
	}
	// mathutil's coverage is untouched.
	add, ok := coveredRange(updated, "mathutil/mathutil.go", 3, 3)
	if !ok || len(add.Tests) != 1 || add.Tests[0] != "TestAdd" {
		t.Fatalf("expected mathutil coverage preserved, got: %+v (ok=%v)", add, ok)
	}
	// greet picked up the new function and test.
	bye, ok := coveredRange(updated, "greet/greet.go", 5, 5)
	if !ok || len(bye.Tests) != 1 || bye.Tests[0] != "TestBye" {
		t.Fatalf("expected Bye covered by TestBye, got: %+v (ok=%v)", bye, ok)
	}
	found := false
	for _, tn := range updated.Tests {
		if tn == "TestBye" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TestBye in Tests, got: %v", updated.Tests)
	}
}

func copyFixtureDegradedRepo(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := "testdata/fixture-degraded"
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
	runGit(t, dst, "init", "-q")
	runGit(t, dst, "config", "user.email", "test@example.com")
	runGit(t, dst, "config", "user.name", "test")
	runGit(t, dst, "add", "-A")
	runGit(t, dst, "commit", "-q", "-m", "base")
	return dst
}

func TestBuild_DegradesCompileFailingPackageInsteadOfFailing(t *testing.T) {
	repoDir := copyFixtureDegradedRepo(t)

	m, err := Build(repoDir)
	if err != nil {
		t.Fatalf("expected Build to succeed despite one broken package, got: %v", err)
	}

	if len(m.DegradedPackages) != 1 || m.DegradedPackages[0] != "broken" {
		t.Fatalf("expected [\"broken\"] in DegradedPackages, got: %v", m.DegradedPackages)
	}
	if len(m.BuildWarnings) != 1 {
		t.Fatalf("expected 1 build warning, got: %v", m.BuildWarnings)
	}
	found := false
	for _, tn := range m.Tests {
		if tn == "TestOk" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TestOk from the healthy package in Tests, got: %v", m.Tests)
	}
	if len(m.Packages) != 2 {
		t.Fatalf("expected both packages still listed, got: %v", m.Packages)
	}
}
