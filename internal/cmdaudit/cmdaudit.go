package cmdaudit

import (
	"flag"
	"fmt"
	"os"

	"github.com/tolvi-labs/canary/internal/audit"
	"github.com/tolvi-labs/canary/internal/gate"
	"github.com/tolvi-labs/canary/internal/manifest"
	"github.com/tolvi-labs/canary/internal/report"
)

// Run implements `canary audit [--repo <dir>] [--json-out <path>]`. It
// re-runs audit mode against the already-built manifest, without
// rebuilding coverage.
func Run(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	repoDir := fs.String("repo", ".", "path to the repository")
	jsonOut := fs.String("json-out", "", "path to write the machine-readable JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	m, err := manifest.Load(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canary audit: %v (run `canary init` first)\n", err)
		return 1
	}

	a := audit.Compute(m)
	result := gate.Result{Gate: "audit", ManifestStatus: "fresh", Audit: &a}
	fmt.Println(report.ToHuman(result, "", ""))
	if *jsonOut != "" {
		raw, err := report.ToJSON(result, "", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "canary audit: %v\n", err)
			return 2
		}
		if err := os.WriteFile(*jsonOut, raw, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "canary audit: writing %s: %v\n", *jsonOut, err)
			return 2
		}
	}
	return 0
}
