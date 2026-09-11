package langconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is canary.yml's contents: which language backend applies to
// this repo.
type Config struct {
	Language string `yaml:"language"`
}

// Load reads canary.yml from repoDir. Unlike provenance.yml, there is no
// usable default — a missing canary.yml means backend selection hasn't
// happened yet, which callers must resolve via Detect + Scaffold, not by
// silently guessing on every run.
func Load(repoDir string) (Config, error) {
	path := filepath.Join(repoDir, "canary.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("no canary.yml in %s (run `canary init` first)", repoDir)
		}
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(content, &c); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if c.Language == "" {
		return Config{}, fmt.Errorf("%s has no language set", path)
	}
	return c, nil
}

// Scaffold writes a canary.yml declaring lang, refusing to overwrite an
// existing one — the file is authoritative once it exists; nothing
// re-detects on a later run.
func Scaffold(repoDir, lang string) error {
	path := filepath.Join(repoDir, "canary.yml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	content := fmt.Sprintf("language: %s\n", lang)
	return os.WriteFile(path, []byte(content), 0644)
}

// Detect guesses a repo's language from its file layout, for scaffolding
// a first-time canary.yml default only — never called once the file
// exists. go.mod wins if present; otherwise a pyproject.toml carrying a
// pytest or coverage table means python; otherwise any file matching
// *.test.js/.mjs/.cjs anywhere under repoDir means node (this is the
// fallback that correctly detects birdie-os-extension, which has no
// package.json at all). Returns an error if nothing matches, so the
// caller can fall back to an explicit --lang flag instead of guessing
// wrong.
func Detect(repoDir string) (string, error) {
	if _, err := os.Stat(filepath.Join(repoDir, "go.mod")); err == nil {
		return "go", nil
	}
	if raw, err := os.ReadFile(filepath.Join(repoDir, "pyproject.toml")); err == nil {
		s := string(raw)
		if strings.Contains(s, "[tool.pytest") || strings.Contains(s, "[tool.coverage") {
			return "python", nil
		}
	}
	var foundNodeTest bool
	filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || foundNodeTest || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".test.js") || strings.HasSuffix(name, ".test.mjs") || strings.HasSuffix(name, ".test.cjs") {
			foundNodeTest = true
		}
		return nil
	})
	if foundNodeTest {
		return "node", nil
	}
	return "", fmt.Errorf("could not detect a language for %s — no go.mod, no pytest-configured pyproject.toml, no *.test.js files found; pass --lang explicitly", repoDir)
}
