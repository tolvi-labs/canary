package coverage

// Block is one instrumented source-code statement block from a single
// test's coverage profile, with the module path already stripped so File
// is repo-relative, matching git's own path convention.
type Block struct {
	File               string
	StartLine, EndLine int
	Count              int
}

// Backend is one language's coverage-instrumentation engine. Each method
// mirrors the original Go-only engine's three functions plus the
// change-detection logic `canary refresh` needs — everything else in
// Canary (the two-manifest model, the gate policy, the vault bindings)
// is already language-agnostic and calls a Backend rather than a
// specific language's tools directly.
type Backend interface {
	// ModulePath returns whatever identifies the project root for this
	// language (a Go module path, a project name, a directory name) —
	// used only for trimming absolute paths to repo-relative ones.
	ModulePath(repoDir string) (string, error)

	// ListUnits returns every testable unit under repoDir, in whatever
	// form UnitTests expects as its unit argument.
	ListUnits(repoDir string) ([]string, error)

	// UnitTests returns, for one unit, a map of test name to the source
	// blocks that test covered.
	UnitTests(repoDir, modulePath, unit, workDir string) (map[string][]Block, error)

	// TouchedUnits maps a list of changed repo-relative file paths to
	// the units `canary refresh` should re-instrument. A backend whose
	// unit granularity doesn't let it precisely reverse-map a non-test
	// file to the unit(s) that exercise it may fall back to "every
	// unit" for that case — a documented, deliberate v1 simplification
	// (see internal/manifest/global.go's own precedent for a test that
	// no longer exists anywhere not being pruned until a full
	// `canary init`), not an oversight.
	TouchedUnits(repoDir string, changedFiles []string) []string
}
