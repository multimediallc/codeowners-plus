package git

import (
	"fmt"
	"strings"
)

// MergeBase returns the merge base of two refs. Diffs are produced with
// three-dot syntax (base...head), so hunk line numbers are relative to
// the merge base, not the base branch tip; consumers that map hunk
// coordinates onto base-revision file content must read files at this
// ref.
func MergeBase(dir string, base string, head string) (string, error) {
	executor := newRealGitExecutor(dir)
	output, err := executor.execute("git", "merge-base", base, head)
	if err != nil {
		return "", fmt.Errorf("git merge-base %s %s: %s\n%s", base, head, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}
