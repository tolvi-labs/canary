package cmdinit

import (
	"flag"
	"fmt"
	"os"

	"github.com/tolvi-labs/canary/internal/audit"
	"github.com/tolvi-labs/canary/internal/gate"
	"github.com/tolvi-labs/canary/internal/manifest"
	"github.com/tolvi-labs/canary/internal/report"
)

// Run implements `canary init [--audit] [--repo <dir>] [--json-out <path>]`.
func Run(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	repoDir := fs.String("repo", ".", "path to the repository")
	withAudit := fs.Bool("audit", false, "also run audit mode after building the manifest")
	jsonOut := fs.String("json-out", "", "path to write the machine-readable JSON report (audit mode only)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	m, err := manifest.Build(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary init: %v\n", err)
		return 2
	}
	if err := manifest.Save(*repoDir, m); err != nil {
		fmt.Fprintf(os.Stderr, "canary init: %v\n", err)
		return 2
	}
	fmt.Printf("✓ Built global manifest: %d packages, %d tests\n", len(m.Packages), len(m.Tests))

	if !*withAudit {
		return 0
	}

	a := audit.Compute(m)
	result := gate.Result{Gate: "audit", ManifestStatus: "fresh", Audit: &a}
	fmt.Println(report.ToHuman(result, "", ""))
	if *jsonOut != "" {
		raw, err := report.ToJSON(result, "", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "canary init: %v\n", err)
			return 2
		}
		if err := os.WriteFile(*jsonOut, raw, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "canary init: writing %s: %v\n", *jsonOut, err)
			return 2
		}
	}
	return 0
}
