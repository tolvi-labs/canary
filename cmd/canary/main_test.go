package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("canary version failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "canary 0.1.0") {
		t.Fatalf("expected version output, got: %s", out)
	}
}

func TestHelpMentionsUsage(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "canary — vault-informed CI test selection") {
		t.Fatalf("expected usage banner, got: %s", out)
	}
}

func TestUnknownCommandExitsNonZero(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "bogus")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected a non-zero exit for an unknown command")
	}
}
