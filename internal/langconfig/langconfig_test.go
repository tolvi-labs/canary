package langconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect_Go(t *testing.T) {
	got, err := Detect("testdata/go-fixture")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if got != "go" {
		t.Fatalf("expected \"go\", got %q", got)
	}
}

func TestDetect_Python(t *testing.T) {
	got, err := Detect("testdata/python-fixture")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if got != "python" {
		t.Fatalf("expected \"python\", got %q", got)
	}
}

func TestDetect_NodeWithNoPackageJSON(t *testing.T) {
	got, err := Detect("testdata/node-fixture")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if got != "node" {
		t.Fatalf("expected \"node\", got %q", got)
	}
}

func TestDetect_Undetectable(t *testing.T) {
	dir := t.TempDir()
	if _, err := Detect(dir); err == nil {
		t.Fatal("expected an error for a repo with no detectable language")
	}
}

func TestScaffoldAndLoad(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold(dir, "python"); err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c.Language != "python" {
		t.Fatalf("expected language \"python\", got %q", c.Language)
	}
}

func TestScaffold_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold(dir, "go"); err != nil {
		t.Fatalf("first Scaffold failed: %v", err)
	}
	if err := Scaffold(dir, "python"); err == nil {
		t.Fatal("expected Scaffold to refuse overwriting an existing canary.yml")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected Load to error when no canary.yml exists")
	}
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got: %v", err)
	}
}

// TestLoad_MalformedFileIsNotConfigNotFound is what makes a bad
// canary.yml recoverable. ResolveOrScaffold only scaffolds over
// ErrConfigNotFound; if a parse failure carried the same signal, it
// would try to scaffold, Scaffold would refuse ("canary.yml already
// exists"), and the user would never learn the file was unparseable.
func TestLoad_MalformedFileIsNotConfigNotFound(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "canary.yml", "language: [this is not a string\n")
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected Load to error on a malformed canary.yml")
	}
	if errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("a malformed canary.yml must not look like a missing one: %v", err)
	}
}

func TestLoad_UnknownLanguageIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "canary.yml", "language: rubby\n")
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected Load to reject a language no backend implements")
	}
	if errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("an unknown language must not look like a missing file: %v", err)
	}
}

// TestVerifyMarker_RejectsLanguageWithNoProjectMarker covers the design's
// own error-handling rule: a canary.yml naming a language the repo has
// no trace of is a hard error, not a silent no-op that builds an empty
// manifest and green-gates everything afterward.
func TestVerifyMarker_RejectsLanguageWithNoProjectMarker(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fixture\n\ngo 1.26\n")
	err := VerifyMarker(dir, Python)
	if err == nil {
		t.Fatal("expected an error for language python in a repo with no Python marker")
	}
	if !strings.Contains(err.Error(), "python") {
		t.Fatalf("expected the error to name the language, got: %v", err)
	}
	if err := VerifyMarker(dir, Go); err != nil {
		t.Fatalf("expected go to verify against a go.mod, got: %v", err)
	}
}

func TestVerifyMarker_AcceptsEachLanguagesMarkers(t *testing.T) {
	cases := []struct {
		name, lang, file, content string
	}{
		{"go.mod", Go, "go.mod", "module fixture\n"},
		{"pyproject.toml", Python, "pyproject.toml", "[project]\nname = \"x\"\n"},
		{"pytest.ini", Python, "pytest.ini", "[pytest]\n"},
		{"a bare pytest file", Python, "tests/test_thing.py", "def test_x():\n    pass\n"},
		{"package.json", Node, "package.json", "{\"name\": \"x\"}\n"},
		{"a bare node test file", Node, "test/thing.test.js", "// none\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, c.file, c.content)
			if err := VerifyMarker(dir, c.lang); err != nil {
				t.Fatalf("expected %s to verify %s, got: %v", c.file, c.lang, err)
			}
		})
	}
}

func TestResolveOrScaffold_RejectsBadLangBeforeWritingIt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fixture\n")
	if _, _, err := ResolveOrScaffold(dir, "gogo"); err == nil {
		t.Fatal("expected an error for an unknown --lang value")
	}
	// The point of validating first: a typo must not leave a canary.yml
	// behind that every later run refuses to overwrite.
	if _, err := os.Stat(filepath.Join(dir, "canary.yml")); err == nil {
		t.Fatal("a rejected --lang value must not be written to canary.yml")
	}
}

func TestResolveOrScaffold_RejectsMismatchedLangBeforeWritingIt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fixture\n")
	if _, _, err := ResolveOrScaffold(dir, Python); err == nil {
		t.Fatal("expected an error for --lang python in a repo with no Python marker")
	}
	if _, err := os.Stat(filepath.Join(dir, "canary.yml")); err == nil {
		t.Fatal("a mismatched --lang value must not be written to canary.yml")
	}
}

// TestResolveOrScaffold_SurfacesMalformedConfig is the recovery path for
// a canary.yml that is already broken on disk: the user must be told
// what is actually wrong, not handed a "canary.yml already exists"
// refusal that never mentions the parse error.
func TestResolveOrScaffold_SurfacesMalformedConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fixture\n")
	write(t, dir, "canary.yml", "<<<<<<< HEAD\nlanguage: go\n=======\nlanguage: python\n>>>>>>> other\n")

	_, _, err := ResolveOrScaffold(dir, Go)
	if err == nil {
		t.Fatal("expected an error for a malformed canary.yml")
	}
	if strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected the real parse error, not a Scaffold refusal: %v", err)
	}
	if !strings.Contains(err.Error(), "canary.yml") {
		t.Fatalf("expected the error to name the file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "delete") {
		t.Fatalf("expected the error to say how to recover, got: %v", err)
	}
}

func TestResolveOrScaffold_ReturnsTheDeclaredBackend(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", "{\"name\": \"x\"}\n")

	cfg, backend, err := ResolveOrScaffold(dir, Node)
	if err != nil {
		t.Fatalf("ResolveOrScaffold failed: %v", err)
	}
	if cfg.Language != Node {
		t.Fatalf("expected node, got %q", cfg.Language)
	}
	exts := backend.SourceExtensions()
	if len(exts) != 3 || exts[0] != ".js" {
		t.Fatalf("expected the Node backend's extensions, got %v", exts)
	}
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
