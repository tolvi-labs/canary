package cmdrefresh

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tolvi-labs/canary/internal/gitutil"
	"github.com/tolvi-labs/canary/internal/langconfig"
	"github.com/tolvi-labs/canary/internal/manifest"
)

// Run implements `canary refresh [--repo <dir>]`. It rebuilds coverage
// only for units touched since the manifest's BuiltAtSHA, and merges
// the result into the existing manifest.
func Run(args []string) int {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	repoDir := fs.String("repo", ".", "path to the repository")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	_, backend, err := langconfig.Resolve(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}

	existing, err := manifest.Load(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v (run `canary init` first)\n", err)
		return 1
	}

	head, err := gitutil.HeadSHA(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}
	if head == existing.BuiltAtSHA {
		fmt.Println("✓ Manifest already up to date")
		return 0
	}

	changedFiles, err := gitutil.ChangedFiles(*repoDir, existing.BuiltAtSHA, head)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}

	touchedPackages := backend.TouchedUnits(*repoDir, changedFiles)
	if len(touchedPackages) == 0 {
		fmt.Println("✓ No units touched, nothing to refresh")
		return 0
	}

	updated, err := manifest.Refresh(existing, *repoDir, touchedPackages, backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}
	if err := manifest.Save(*repoDir, updated); err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}
	fmt.Printf("✓ Refreshed %d unit(s)\n", len(touchedPackages))
	if len(updated.DegradedPackages) > 0 {
		fmt.Printf("⚠ %d package(s) could not be built for coverage and are excluded from selection-narrowing: %s\n", len(updated.DegradedPackages), strings.Join(updated.DegradedPackages, ", "))
		for _, w := range updated.BuildWarnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	return 0
}
