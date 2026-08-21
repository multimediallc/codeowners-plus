package git

import (
	"bufio"
	"bytes"
	"strings"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/sourcegraph/go-diff/diff"
)

// normalizer reports whether a hunk's two sides are the same code once the enabled
// flags' noise is stripped.  Renames is not a step: it compares what they leave.
type normalizer struct {
	steps []normalizeStep
}

// normalizeStep rewrites one side of a hunk, its lines joined by newlines.  The
// file name comes too: what a run of punctuation means depends on the language.
type normalizeStep func(fileName, block string) string

// Adapts a step which reads a block the same way whatever the language.
func inAnyLanguage(step func(string) string) normalizeStep {
	return func(_, block string) string { return step(block) }
}

func newNormalizer(retention *owners.ApprovalRetention) normalizer {
	n := normalizer{}
	if retention.FormattingEnabled() {
		n.steps = append(n.steps, inAnyLanguage(collapseFormatting))
	}
	return n
}

// A flag may only retain a hunk it actually read, so anything the enabled steps
// cannot account for stays non-trivial and an approval is kept only on purpose.
func (n normalizer) isTrivial(fileName string, hunk *diff.Hunk) bool {
	if len(n.steps) == 0 {
		return false
	}
	added, removed, ok := hunkBlocks(hunk.Body)
	if !ok {
		return false
	}
	// Checked before the steps run, since they drop the indentation that changed.
	if reindentsBlock(fileName, added, removed) {
		return false
	}
	return n.normalize(fileName, added) == n.normalize(fileName, removed)
}

// approvalKey identifies a hunk by what it says once the enabled flags' noise is
// stripped, so it matches an approval-time counterpart whose bytes differ.
func (n normalizer) approvalKey(fileName string, hunk *diff.Hunk) (string, bool) {
	// No key at all, so de-duplication is unchanged while every flag is off.
	if len(n.steps) == 0 {
		return "", false
	}
	added, removed, ok := hunkBlocks(hunk.Body)
	if !ok {
		return "", false
	}
	// The file name joins the key, unlike in the raw hash: a normalized body says
	// less than a raw one, so identity should not reach across files as far.
	return fileName + "\x00" + n.normalize(fileName, added) +
		"\x00" + n.normalize(fileName, removed), true
}

func (n normalizer) normalize(fileName, block string) string {
	for _, step := range n.steps {
		block = step(fileName, block)
	}
	return block
}

// Rewrapping a statement removes one line and adds several, so the sides are joined
// and compared whole; line-wise comparison could never pair them up.
func hunkBlocks(body []byte) (added, removed string, ok bool) {
	var addedLines, removedLines []string

	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			addedLines = append(addedLines, line[1:])
		case '-':
			removedLines = append(removedLines, line[1:])
		}
	}
	// A hunk we could not read whole is not a hunk we can vouch for.
	if scanner.Err() != nil || (len(addedLines) == 0 && len(removedLines) == 0) {
		return "", "", false
	}
	return strings.Join(addedLines, "\n"), strings.Join(removedLines, "\n"), true
}
