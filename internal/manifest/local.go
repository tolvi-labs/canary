package manifest

import (
	"sort"
	"strings"

	"github.com/tolvi-labs/canary/internal/gitutil"
)

// SelectByCoverage returns the sorted, de-duplicated set of test names
// whose coverage overlaps any changed line range, for any changed file
// present in the manifest.
func SelectByCoverage(m GlobalManifest, changedRanges map[string][]gitutil.LineRange) []string {
	set := map[string]bool{}
	for file, ranges := range changedRanges {
		covered, ok := m.Coverage[file]
		if !ok {
			continue
		}
		for _, cr := range covered {
			for _, r := range ranges {
				if overlaps(cr.StartLine, cr.EndLine, r.Start, r.End) {
					for _, t := range cr.Tests {
						set[t] = true
					}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func overlaps(aStart, aEnd, bStart, bEnd int) bool {
	return aStart <= bEnd && bStart <= aEnd
}

// AllTestsUnderPackage returns every test with any coverage entry for a
// file under the given repo-relative package directory (empty string for
// the module root). Used as the safe fallback when that package's
// coverage could not be rebuilt (a compile failure): force-include
// everything known about it rather than trust a selection that might no
// longer align with the code.
func AllTestsUnderPackage(m GlobalManifest, packageDir string) []string {
	set := map[string]bool{}
	for file, ranges := range m.Coverage {
		if !isUnderPackage(file, packageDir) {
			continue
		}
		for _, r := range ranges {
			for _, t := range r.Tests {
				set[t] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func isUnderPackage(file, packageDir string) bool {
	if packageDir == "" {
		return !strings.Contains(file, "/")
	}
	return file == packageDir || strings.HasPrefix(file, packageDir+"/")
}
