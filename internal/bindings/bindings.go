package bindings

import (
	"fmt"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/tolvi-labs/canary/internal/vault"
)

// ForcedTest is one test forced into the selection by a vault binding,
// regardless of what the coverage graph says.
type ForcedTest struct {
	Test     string
	Decision string
}

// Problem names a binding that could not be applied cleanly — e.g. it
// names a test that doesn't exist in the manifest's registry. Canary
// fails open on a bad binding: it's reported, never a hard block.
type Problem struct {
	Decision string
	Message  string
}

// Evaluate returns every test forced by an active decision's
// x-canary-bindings whose path glob matches a changed file, plus any
// problems found along the way (e.g. an unknown test name).
func Evaluate(decisions []vault.Decision, changedFiles []string, knownTests map[string]bool) ([]ForcedTest, []Problem) {
	var forced []ForcedTest
	var problems []Problem

	for _, d := range vault.ActiveOnly(decisions) {
		for _, binding := range d.XCanaryBindings {
			matched := false
			for _, f := range changedFiles {
				if matchesAny(f, binding.Paths) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			for _, test := range binding.Tests {
				if !knownTests[test] {
					problems = append(problems, Problem{
						Decision: d.Slug,
						Message:  fmt.Sprintf("x-canary-bindings references unknown test %q", test),
					})
					continue
				}
				forced = append(forced, ForcedTest{Test: test, Decision: d.Slug})
			}
		}
	}
	return forced, problems
}

func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if ok, _ := doublestar.Match(pattern, path); ok {
			return true
		}
	}
	return false
}
