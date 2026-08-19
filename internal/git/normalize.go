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
	// Set when a step reads where a line sits; owns a hunk which only moved lines.
	layout bool
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
	if retention.WhitespaceEnabled() {
		n.steps = append(n.steps, inAnyLanguage(collapseWhitespace))
	}
	if retention.FormattingEnabled() {
		n.steps = append(n.steps, inAnyLanguage(collapseFormatting))
	}
	// Formatting rewrites whitespace around the punctuation it drops, so it reads
	// where a line sits just as whitespace does.
	n.layout = retention.WhitespaceEnabled() || retention.FormattingEnabled()
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
	// Sides identical before any step ran (a line deleted and added back, or moved)
	// changed only where the lines sit, so only a layout step may vouch for it.
	if added == removed {
		return n.layout
	}
	// Checked before the steps run, since they drop the indentation that changed.
	if n.layout && reindentsBlock(fileName, added, removed) {
		return false
	}
	return n.normalize(fileName, added) == n.normalize(fileName, removed)
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
