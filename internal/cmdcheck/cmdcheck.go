package cmdcheck

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tolvi-labs/canary/internal/gate"
	"github.com/tolvi-labs/canary/internal/gitutil"
	"github.com/tolvi-labs/canary/internal/langconfig"
	"github.com/tolvi-labs/canary/internal/manifest"
	"github.com/tolvi-labs/canary/internal/provenancereport"
	"github.com/tolvi-labs/canary/internal/report"
	"github.com/tolvi-labs/canary/internal/vault"
)

// Run implements:
//
//	canary check --base <ref> --head <ref> [--gate pr|merge|release]
//	             [--provenance-report <path>] [--json-out <path>] [--repo <dir>]
func Run(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	base := fs.String("base", "", "base ref to diff from")
	head := fs.String("head", "HEAD", "head ref to diff to")
	gateMode := fs.String("gate", "pr", "gate mode: pr, merge, or release")
	provenanceReportPath := fs.String("provenance-report", "", "path to Provenance's tolvi-provenance-report-v1 JSON (optional)")
	jsonOut := fs.String("json-out", "", "path to write the machine-readable JSON report")
	repoDir := fs.String("repo", ".", "path to the repository")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *base == "" {
		fmt.Fprintln(os.Stderr, "canary check: --base is required")
		return 2
	}
	if *gateMode != "pr" && *gateMode != "merge" && *gateMode != "release" {
		fmt.Fprintf(os.Stderr, "canary check: --gate must be pr, merge, or release, got %q\n", *gateMode)
		return 2
	}

	// The gate's unmapped-code safety net is scoped by the repo's own
	// backend, so `check` has to resolve canary.yml exactly the way
	// `init` and `refresh` do. Without it the net was hardcoded to Go and
	// silently selected nothing for a brand-new untested file in any
	// other language — the one failure mode a test gate exists to
	// prevent.
	_, backend, err := langconfig.Resolve(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
		return 2
	}

	m, loadErr := manifest.Load(*repoDir)
	manifestStale := true
	if loadErr == nil {
		stale, err := manifest.Stale(m, *repoDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
			return 2
		}
		manifestStale = stale
	} else if loadErr != manifest.ErrManifestNotFound {
		fmt.Fprintf(os.Stderr, "canary check: %v\n", loadErr)
		return 2
	}

	var changedFiles []string
	var changedRanges map[string][]gitutil.LineRange
	if *gateMode == "pr" {
		changedFiles, err = gitutil.ChangedFiles(*repoDir, *base, *head)
		if err != nil {
			fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
			return 2
		}
		changedRanges, err = gitutil.ChangedRanges(*repoDir, *base, *head)
		if err != nil {
			fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
			return 2
		}
	}

	decisions, err := vault.LoadDecisions(filepath.Join(*repoDir, "vault"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
		return 2
	}

	provReport, err := provenancereport.Load(*provenanceReportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
		return 2
	}

	result := gate.Check(*gateMode, m, manifestStale, changedFiles, changedRanges, decisions, provReport, backend.SourceExtensions())

	fmt.Println(report.ToHuman(result, *base, *head))

	if *jsonOut != "" {
		raw, err := report.ToJSON(result, *base, *head)
		if err != nil {
			fmt.Fprintf(os.Stderr, "canary check: %v\n", err)
			return 2
		}
		if err := os.WriteFile(*jsonOut, raw, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "canary check: writing %s: %v\n", *jsonOut, err)
			return 2
		}
	}
	return 0
}
