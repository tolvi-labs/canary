package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDecision(t *testing.T, vaultDir, filename, content string) {
	t.Helper()
	dir := filepath.Join(vaultDir, "decisions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDecisions_ParsesXCanaryBindings(t *testing.T) {
	vaultDir := t.TempDir()
	writeDecision(t, vaultDir, "2026-07-04-stale-cache.md", `---
tags: [decision]
date: 2026-07-04
status: active
repo: example
x-canary-bindings:
  - paths: ["internal/cache/**"]
    tests: ["TestCacheInvalidation_StaleWrite", "TestCacheInvalidation_ConcurrentRead"]
---

# Stale cache incident
`)
	writeDecision(t, vaultDir, "2025-01-01-old-decision.md", `---
tags: [decision]
date: 2025-01-01
status: superseded
repo: example
---

# An old decision
`)

	decisions, err := LoadDecisions(vaultDir)
	if err != nil {
		t.Fatalf("LoadDecisions failed: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}

	active := ActiveOnly(decisions)
	if len(active) != 1 {
		t.Fatalf("expected 1 active decision, got %d", len(active))
	}
	d := active[0]
	if d.Slug != "2026-07-04-stale-cache" {
		t.Fatalf("unexpected slug: %s", d.Slug)
	}
	if len(d.XCanaryBindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(d.XCanaryBindings))
	}
	b := d.XCanaryBindings[0]
	if len(b.Paths) != 1 || b.Paths[0] != "internal/cache/**" {
		t.Fatalf("unexpected paths: %v", b.Paths)
	}
	if len(b.Tests) != 2 || b.Tests[0] != "TestCacheInvalidation_StaleWrite" {
		t.Fatalf("unexpected tests: %v", b.Tests)
	}
}

func TestLoadDecisions_MissingDirReturnsEmpty(t *testing.T) {
	vaultDir := t.TempDir()
	decisions, err := LoadDecisions(vaultDir)
	if err != nil {
		t.Fatalf("expected no error for a missing decisions dir, got: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected 0 decisions, got %d", len(decisions))
	}
}
