package cmdhook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndUninstall(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := install(dir, false); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	path := filepath.Join(dir, ".git", "hooks", "pre-commit")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected hook file to exist: %v", err)
	}
	if string(content) != shim {
		t.Fatalf("unexpected hook content: %s", content)
	}

	if err := install(dir, false); err == nil {
		t.Fatal("expected install to refuse to overwrite without force")
	}

	if err := uninstall(dir); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected hook file to be removed, stat err: %v", err)
	}
}

func TestUninstall_LeavesForeignHookInPlace(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho not canary\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := uninstall(dir); err == nil {
		t.Fatal("expected uninstall to refuse to remove a foreign hook")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected foreign hook to remain, stat err: %v", err)
	}
}
