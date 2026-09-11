package langconfig

import (
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
	if _, err := Load(dir); err == nil {
		t.Fatal("expected Load to error when no canary.yml exists")
	}
}
