package langconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tolvi-labs/canary/internal/coverage"
	"github.com/tolvi-labs/canary/internal/coverage/golang"
	"github.com/tolvi-labs/canary/internal/coverage/node"
	"github.com/tolvi-labs/canary/internal/coverage/python"
)

// The languages a coverage backend exists for.
const (
	Go     = "go"
	Python = "python"
	Node   = "node"
)

// ErrConfigNotFound is returned (wrapped) by Load when repoDir has no
// canary.yml at all. It is deliberately distinguishable from a canary.yml
// that exists but is unreadable, unparseable, or names a language no
// backend implements: only the former may be resolved by scaffolding a
// fresh file, and treating the latter the same way is what made a
// malformed canary.yml unrecoverable — every subsequent `--lang` was
// ignored in favour of a "canary.yml already exists" refusal that never
// mentioned the parse error underneath.
var ErrConfigNotFound = errors.New("no canary.yml")

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
			return Config{}, fmt.Errorf("%w in %s (run `canary init` first)", ErrConfigNotFound, repoDir)
		}
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(content, &c); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w (fix or delete the file, then re-run)", path, err)
	}
	if c.Language == "" {
		return Config{}, fmt.Errorf("%s has no language set (fix or delete the file, then re-run)", path)
	}
	if err := ValidateLanguage(c.Language); err != nil {
		return Config{}, fmt.Errorf("%s: %w (fix or delete the file, then re-run)", path, err)
	}
	return c, nil
}

// ValidateLanguage rejects any language string no coverage backend
// implements. Callers must run it on a `--lang` value *before*
// scaffolding, so a typo is never written to disk in the first place.
func ValidateLanguage(lang string) error {
	switch lang {
	case Go, Python, Node:
		return nil
	}
	return fmt.Errorf("unknown language %q (supported: %s, %s, %s)", lang, Go, Python, Node)
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

// Backend maps a validated language to its coverage.Backend.
func Backend(lang string) (coverage.Backend, error) {
	switch lang {
	case Go:
		return golang.Backend{}, nil
	case Python:
		return python.Backend{}, nil
	case Node:
		return node.Backend{}, nil
	}
	return nil, ValidateLanguage(lang)
}

// markers lists, per language, the project files whose presence proves
// the repo really is one this backend can instrument. Deliberately
// broader than Detect's rules: Detect is picking a default with no user
// input and may be conservative, whereas VerifyMarker is second-guessing
// a language the user (or a previous Detect) already chose, so it must
// not reject a legitimate layout — a pytest suite configured by
// pytest.ini rather than pyproject.toml is still a Python repo.
var markers = map[string][]string{
	Go:     {"go.mod"},
	Python: {"pyproject.toml", "setup.py", "setup.cfg", "pytest.ini", "tox.ini", "conftest.py"},
	Node:   {"package.json"},
}

// VerifyMarker reports an error when repoDir carries no project marker
// for lang. Per the design's error-handling rule, a canary.yml naming a
// language the repo has no trace of is a hard error at init/refresh/check
// time, not a silent no-op: without this, `--lang python` in a repo with
// no Python at all builds an empty manifest and every later `canary
// check` reports a green gate having selected zero tests.
func VerifyMarker(repoDir, lang string) error {
	if err := ValidateLanguage(lang); err != nil {
		return err
	}
	looked := append([]string{}, markers[lang]...)
	for _, m := range markers[lang] {
		if _, err := os.Stat(filepath.Join(repoDir, m)); err == nil {
			return nil
		}
	}
	// Both file-shaped backends can legitimately have no project file at
	// all (birdie-os-extension has no package.json), so a test file
	// matching the backend's own discovery convention counts as a marker
	// too — it is, after all, exactly what ListUnits would find.
	switch lang {
	case Python:
		looked = append(looked, "test_*.py", "*_test.py")
		if hasFileMatching(repoDir, isPythonTestFile) {
			return nil
		}
	case Node:
		looked = append(looked, "*.test.js", "*.test.mjs", "*.test.cjs")
		if hasFileMatching(repoDir, isNodeTestFile) {
			return nil
		}
	}
	return fmt.Errorf("language %q does not match %s: no %s project marker found (looked for %s)",
		lang, repoDir, lang, strings.Join(looked, ", "))
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
		return Go, nil
	}
	if raw, err := os.ReadFile(filepath.Join(repoDir, "pyproject.toml")); err == nil {
		s := string(raw)
		if strings.Contains(s, "[tool.pytest") || strings.Contains(s, "[tool.coverage") {
			return Python, nil
		}
	}
	if hasFileMatching(repoDir, isNodeTestFile) {
		return Node, nil
	}
	return "", fmt.Errorf("could not detect a language for %s — no go.mod, no pytest-configured pyproject.toml, no *.test.js files found; pass --lang explicitly", repoDir)
}

// Resolve reads repoDir's canary.yml, verifies the language it declares
// is actually present in the repo, and returns the matching backend. It
// is the single entry point `canary refresh` and `canary check` use, so
// neither can drift from `canary init`'s view of which backend a repo is.
func Resolve(repoDir string) (Config, coverage.Backend, error) {
	cfg, err := Load(repoDir)
	if err != nil {
		return Config{}, nil, err
	}
	if err := VerifyMarker(repoDir, cfg.Language); err != nil {
		return Config{}, nil, fmt.Errorf("%s: %w", filepath.Join(repoDir, "canary.yml"), err)
	}
	b, err := Backend(cfg.Language)
	if err != nil {
		return Config{}, nil, err
	}
	return cfg, b, nil
}

// ResolveOrScaffold is Resolve for `canary init`, which is allowed to
// create the canary.yml it then resolves: langOverride if set, otherwise
// Detect. The language is validated and marker-checked *before* the file
// is written, so a typo'd or mismatched --lang never leaves a bad
// canary.yml behind for Scaffold to refuse to overwrite on the next run.
// An existing canary.yml is never rewritten — and if it exists but is
// malformed, that error is surfaced rather than being mistaken for "this
// repo just needs scaffolding".
func ResolveOrScaffold(repoDir, langOverride string) (Config, coverage.Backend, error) {
	cfg, b, err := Resolve(repoDir)
	if err == nil {
		return cfg, b, nil
	}
	if !errors.Is(err, ErrConfigNotFound) {
		return Config{}, nil, err
	}
	lang := langOverride
	if lang == "" {
		detected, detectErr := Detect(repoDir)
		if detectErr != nil {
			return Config{}, nil, detectErr
		}
		lang = detected
	}
	if err := ValidateLanguage(lang); err != nil {
		return Config{}, nil, err
	}
	if err := VerifyMarker(repoDir, lang); err != nil {
		return Config{}, nil, err
	}
	if err := Scaffold(repoDir, lang); err != nil {
		return Config{}, nil, err
	}
	return Resolve(repoDir)
}

func isNodeTestFile(name string) bool {
	return strings.HasSuffix(name, ".test.js") ||
		strings.HasSuffix(name, ".test.mjs") ||
		strings.HasSuffix(name, ".test.cjs")
}

func isPythonTestFile(name string) bool {
	return strings.HasSuffix(name, ".py") &&
		(strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py"))
}

// hasFileMatching walks repoDir for a file whose base name satisfies
// match, skipping the directories that hold other projects' code rather
// than this repo's own (a *.test.js under node_modules says nothing
// about what language this repo is written in).
func hasFileMatching(repoDir string, match func(name string) bool) bool {
	var found bool
	filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "__pycache__", ".venv", "venv", ".tox":
				return filepath.SkipDir
			}
			return nil
		}
		if match(d.Name()) {
			found = true
		}
		return nil
	})
	return found
}
