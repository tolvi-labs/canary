package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tolvi-labs/canary/internal/gitutil"
)

// ManifestRelPath is where the global manifest is stored, relative to the
// repository root.
const ManifestRelPath = ".canary/global-manifest.json"

// ErrManifestNotFound is returned by Load when no manifest has been built
// yet — callers should instruct the user to run `canary init`.
var ErrManifestNotFound = errors.New("global manifest not found")

// Save writes m to <repoDir>/.canary/global-manifest.json.
func Save(repoDir string, m GlobalManifest) error {
	path := filepath.Join(repoDir, ManifestRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// Load reads the global manifest from <repoDir>/.canary/global-manifest.json.
func Load(repoDir string) (GlobalManifest, error) {
	path := filepath.Join(repoDir, ManifestRelPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GlobalManifest{}, ErrManifestNotFound
		}
		return GlobalManifest{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var m GlobalManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return GlobalManifest{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// Stale reports whether m's BuiltAtSHA is NOT an ancestor of the
// repository's current HEAD — meaning it was built on a divergent
// branch, is missing, or is otherwise unusable. A manifest whose
// BuiltAtSHA is an ancestor of HEAD is treated as fresh enough — commits
// made since then are covered by incremental `canary refresh`, not by
// rebuilding from scratch on every check.
func Stale(m GlobalManifest, repoDir string) (bool, error) {
	if m.BuiltAtSHA == "" {
		return true, nil
	}
	head, err := gitutil.HeadSHA(repoDir)
	if err != nil {
		return true, err
	}
	ancestor, err := gitutil.IsAncestor(repoDir, m.BuiltAtSHA, head)
	if err != nil {
		return true, err
	}
	return !ancestor, nil
}
