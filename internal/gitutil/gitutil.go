package gitutil

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ChangedFiles returns the list of file paths that differ between base and
// head in the git repository at repoDir.
func ChangedFiles(repoDir, base, head string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", base+".."+head)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s..%s: %w", base, head, err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// LineRange is an inclusive [Start, End] range of line numbers in a file's
// HEAD-side (post-change) content.
type LineRange struct {
	Start, End int
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
var diffGitLine = regexp.MustCompile(`^diff --git a/.* b/(.*)$`)

// ChangedRanges returns, per changed file (repo-relative path, HEAD-side),
// the line ranges added or modified between base and head. A file that
// was only deleted contributes no ranges — nothing exists on the HEAD
// side to select tests for.
func ChangedRanges(repoDir, base, head string) (map[string][]LineRange, error) {
	cmd := exec.Command("git", "diff", "--no-color", "-U0", base+".."+head)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff -U0 %s..%s: %w", base, head, err)
	}

	ranges := map[string][]LineRange{}
	var currentFile string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := diffGitLine.FindStringSubmatch(line); m != nil {
			currentFile = m[1]
			continue
		}
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			startLine, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, fmt.Errorf("parsing hunk header %q: %w", line, err)
			}
			count := 1
			if m[2] != "" {
				count, err = strconv.Atoi(m[2])
				if err != nil {
					return nil, fmt.Errorf("parsing hunk header %q: %w", line, err)
				}
			}
			if count == 0 {
				continue // pure deletion on the HEAD side
			}
			if currentFile == "" {
				return nil, fmt.Errorf("hunk header %q with no preceding diff --git line", line)
			}
			ranges[currentFile] = append(ranges[currentFile], LineRange{
				Start: startLine,
				End:   startLine + count - 1,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning diff output: %w", err)
	}
	return ranges, nil
}

// HeadSHA returns the current HEAD commit SHA for the repository at repoDir.
func HeadSHA(repoDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAncestor reports whether ancestor is an ancestor of (or equal to)
// descendant in the repository at repoDir.
func IsAncestor(repoDir, ancestor, descendant string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = repoDir
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %w", ancestor, descendant, err)
}
