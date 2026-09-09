package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tolvi-labs/canary/internal/gate"
)

const Schema = "tolvi-canary-report-v1"

type jsonReport struct {
	Schema string `json:"schema"`
	Gate   string `json:"gate"`
	Range  struct {
		Base string `json:"base"`
		Head string `json:"head"`
	} `json:"range"`
	ManifestStatus   string             `json:"manifest_status"`
	SelectedTests    []jsonSelectedTest `json:"selected_tests"`
	Impact           []jsonImpactEntry  `json:"impact"`
	Audit            *jsonAudit         `json:"audit,omitempty"`
	DegradedPackages []string           `json:"degraded_packages"`
}

type jsonSelectedTest struct {
	Test            string `json:"test"`
	Reason          string `json:"reason"`
	BindingDecision string `json:"binding_decision"`
}

type jsonImpactEntry struct {
	Path           string   `json:"path"`
	Tests          []string `json:"tests"`
	ProvenanceRisk []string `json:"provenance_risk"`
}

type jsonAudit struct {
	OrphanTests   []string            `json:"orphan_tests"`
	UntestedPaths []jsonUntestedRange `json:"untested_paths"`
}

type jsonUntestedRange struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ToJSON renders a gate.Result as the tolvi-canary-report-v1 JSON schema.
func ToJSON(result gate.Result, base, head string) ([]byte, error) {
	r := jsonReport{
		Schema:           Schema,
		Gate:             result.Gate,
		ManifestStatus:   result.ManifestStatus,
		SelectedTests:    []jsonSelectedTest{},
		Impact:           []jsonImpactEntry{},
		DegradedPackages: emptyIfNil(result.DegradedPackages),
	}
	r.Range.Base = base
	r.Range.Head = head

	for _, s := range result.SelectedTests {
		r.SelectedTests = append(r.SelectedTests, jsonSelectedTest{
			Test: s.Test, Reason: s.Reason, BindingDecision: s.BindingDecision,
		})
	}
	for _, i := range result.Impact {
		r.Impact = append(r.Impact, jsonImpactEntry{
			Path: i.Path, Tests: emptyIfNil(i.Tests), ProvenanceRisk: emptyIfNil(i.ProvenanceRisk),
		})
	}
	if result.Audit != nil {
		ja := &jsonAudit{OrphanTests: emptyIfNil(result.Audit.OrphanTests), UntestedPaths: []jsonUntestedRange{}}
		for _, u := range result.Audit.UntestedPaths {
			ja.UntestedPaths = append(ja.UntestedPaths, jsonUntestedRange{
				Path: u.File, StartLine: u.StartLine, EndLine: u.EndLine,
			})
		}
		r.Audit = ja
	}

	return json.MarshalIndent(r, "", "  ")
}

// ToHuman renders a gate.Result as a human-readable report, suitable for
// both terminal output and a PR comment body.
func ToHuman(result gate.Result, base, head string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "canary %s: %s..%s\n", result.Gate, base, head)
	fmt.Fprintf(&b, "Manifest: %s\n\n", result.ManifestStatus)

	if len(result.DegradedPackages) > 0 {
		fmt.Fprintf(&b, "Degraded packages (excluded from selection-narrowing): %s\n\n", strings.Join(result.DegradedPackages, ", "))
	}

	if len(result.SelectedTests) > 0 {
		b.WriteString("Selected tests:\n")
		for _, s := range result.SelectedTests {
			if s.Reason == "never-skip-binding" {
				fmt.Fprintf(&b, "  - %s (never-skip: %s)\n", s.Test, s.BindingDecision)
			} else {
				fmt.Fprintf(&b, "  - %s (%s)\n", s.Test, s.Reason)
			}
		}
		b.WriteString("\n")
	}

	if len(result.Impact) > 0 {
		b.WriteString("Impact:\n")
		for _, i := range result.Impact {
			fmt.Fprintf(&b, "  - %s -> %s\n", i.Path, strings.Join(i.Tests, ", "))
			if len(i.ProvenanceRisk) > 0 {
				fmt.Fprintf(&b, "      provenance risk: %s\n", strings.Join(i.ProvenanceRisk, ", "))
			}
		}
		b.WriteString("\n")
	}

	if len(result.Problems) > 0 {
		b.WriteString("Problems (fail-open, not blocking):\n")
		for _, p := range result.Problems {
			fmt.Fprintf(&b, "  - %s: %s\n", p.Decision, p.Message)
		}
		b.WriteString("\n")
	}

	if result.Audit != nil {
		b.WriteString("Audit:\n")
		if len(result.Audit.OrphanTests) == 0 {
			b.WriteString("  orphan tests: none\n")
		} else {
			fmt.Fprintf(&b, "  orphan tests: %s\n", strings.Join(result.Audit.OrphanTests, ", "))
		}
		if len(result.Audit.UntestedPaths) == 0 {
			b.WriteString("  untested paths: none\n")
		} else {
			b.WriteString("  untested paths:\n")
			for _, u := range result.Audit.UntestedPaths {
				fmt.Fprintf(&b, "    - %s:%d-%d\n", u.File, u.StartLine, u.EndLine)
			}
		}
	}

	return b.String()
}
