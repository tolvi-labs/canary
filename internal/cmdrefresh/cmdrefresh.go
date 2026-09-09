package cmdrefresh

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tolvi-labs/canary/internal/gitutil"
	"github.com/tolvi-labs/canary/internal/manifest"
)

// Run implements `canary refresh [--repo <dir>]`. It rebuilds coverage
// only for packages touched since the manifest's BuiltAtSHA, and merges
// the result into the existing manifest.
func Run(args []string) int {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	repoDir := fs.String("repo", ".", "path to the repository")
	if err := fs.Parse(args); err != nil {
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

	touchedPackages := touchedPackagesFor(*repoDir, changedFiles)
	if len(touchedPackages) == 0 {
		fmt.Println("✓ No Go packages touched, nothing to refresh")
		return 0
	}

	updated, err := manifest.Refresh(existing, *repoDir, touchedPackages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}
	if err := manifest.Save(*repoDir, updated); err != nil {
		fmt.Fprintf(os.Stderr, "canary refresh: %v\n", err)
		return 2
	}
	fmt.Printf("✓ Refreshed %d package(s)\n", len(touchedPackages))
	return 0
}

// touchedPackagesFor maps changed .go files to their owning package
// directories, in the "./dir" form coverage.ListPackages itself uses. A
// directory that's no longer a buildable package (e.g. it was deleted)
// is silently skipped — nothing to refresh there.
func touchedPackagesFor(repoDir string, changedFiles []string) []string {
	dirs := map[string]bool{}
	for _, f := range changedFiles {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		dir := filepath.Dir(f)
		if dir == "." {
			dirs["."] = true
		} else {
			dirs["./"+dir] = true
		}
	}
	var out []string
	for d := range dirs {
		cmd := exec.Command("go", "list", d)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}
