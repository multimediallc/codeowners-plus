// Package inlineowners implements inline ownership: comment tags inside
// source files that mark a region as owned, so changes touching that
// region require the tagged owners' approval.
//
// A block looks like:
//
//	// <CO-inline={@alice,@team-a}>
//	func Sensitive() { ... }
//	// </CO-inline>
//
// Both `//` and `#` comment prefixes are supported, tag names are matched
// case-insensitively, and owners may be separated by commas or spaces.
// The owners inside one tag form an OR group (any listed owner satisfies
// it), like a .codeowners line; overlapping blocks AND together.
//
// Blocks are parsed from both the base and head revisions of each changed
// file, so deleting an owned region or editing its tag still requires the
// owners recorded at base.
package inlineowners

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

// Requirement is a reviewer requirement derived from an inline ownership
// block touched by the diff.
type Requirement struct {
	// File is the file's name in the head revision.
	File string
	// Owners is an OR group: any one of these reviewers satisfies it.
	Owners []string
	// Reason records which block produced the requirement, for verbose output.
	Reason string
}

// Block is an inline ownership region in a single revision of a file.
// Start and End are 1-based line numbers, inclusive of the tag lines
// themselves: editing a tag line counts as touching the block, so
// tampering with an ownership tag requires the tagged owners' approval.
type Block struct {
	Owners []string
	Start  int
	End    int
}

// Overlaps reports whether the block intersects the line range [start, end].
func (b Block) Overlaps(start, end int) bool {
	return end >= b.Start && start <= b.End
}

var (
	startTagRe = regexp.MustCompile(`(?i)(?://|#)\s*<CO-inline=\{(?P<owners>[^}]*)\}>`)
	endTagRe   = regexp.MustCompile(`(?i)(?://|#)\s*</CO-inline>`)
	// After a start tag on the same line, the end tag is already inside a
	// comment, so it does not need its own comment prefix.
	endAnywhereRe = regexp.MustCompile(`(?i)</CO-inline>`)
)

// Parse scans file content for inline ownership blocks. Malformed
// constructs (nested or unmatched tags, empty owner lists) produce
// warnings on warn and are skipped, and an unclosed block extends to
// end-of-file, so a missing end tag cannot silently remove protection.
func Parse(content string, warn io.Writer) []Block {
	blocks := []Block{}
	inBlock := false
	var current Block
	lineNo := 0
	for line := range strings.SplitSeq(content, "\n") {
		lineNo++
		line = strings.TrimSuffix(line, "\r")

		startMatch := startTagRe.FindStringSubmatch(line)
		var hasEnd bool
		if startMatch != nil {
			startLoc := startTagRe.FindStringIndex(line)
			hasEnd = endAnywhereRe.MatchString(line[startLoc[1]:])
		} else {
			hasEnd = endTagRe.MatchString(line)
		}

		if startMatch != nil {
			if inBlock {
				_, _ = fmt.Fprintf(warn, "WARNING: Nested <CO-inline> tag at line %d ignored\n", lineNo)
			} else {
				owners := splitOwners(startMatch[startTagRe.SubexpIndex("owners")])
				if len(owners) == 0 {
					_, _ = fmt.Fprintf(warn, "WARNING: Empty owner list in <CO-inline> tag at line %d; block ignored\n", lineNo)
				} else {
					inBlock = true
					current = Block{Owners: owners, Start: lineNo}
				}
			}
		}

		if hasEnd {
			// An end tag on the start-tag line closes the block on that
			// same line (a block covering just the tag line).
			if inBlock {
				current.End = lineNo
				blocks = append(blocks, current)
				inBlock = false
			} else if startMatch == nil {
				_, _ = fmt.Fprintf(warn, "WARNING: Unmatched </CO-inline> tag at line %d ignored\n", lineNo)
			}
		}
	}
	if inBlock {
		_, _ = fmt.Fprintf(warn, "WARNING: Unclosed <CO-inline> tag at line %d; block extends to end of file\n", current.Start)
		current.End = lineNo
		blocks = append(blocks, current)
	}
	return blocks
}

// splitOwners splits a tag's owner list on commas and whitespace,
// dropping empties and case-insensitive duplicates.
func splitOwners(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	owners := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, owner := range parts {
		key := strings.ToLower(owner)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		owners = append(owners, owner)
	}
	return owners
}

// Requirements computes reviewer requirements for the inline ownership
// blocks touched by the diff. For each changed file it parses blocks from
// the base revision (under the file's base name, checked against
// BaseHunks) and the head revision (checked against Hunks) and emits one
// requirement per touched block. A renamed file requires approval from
// all of its base-revision block owners, since a pure rename produces no
// hunks but still relocates the protected regions. A file absent at a
// revision (added or deleted files) contributes no blocks for that side,
// but a read failure for a file that exists is a hard error: proceeding
// without its blocks would silently drop required reviews.
func Requirements(
	files []codeowners.DiffFile,
	baseReader codeowners.FileReader,
	headReader codeowners.FileReader,
	warn io.Writer,
) ([]Requirement, error) {
	requirements := []Requirement{}
	for _, file := range files {
		renamed := file.BaseFileName != ""
		baseReqs, err := revisionRequirements(file.BaseName(), file.FileName, file.BaseHunks, renamed, baseReader, "base", warn)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, baseReqs...)
		headReqs, err := revisionRequirements(file.FileName, file.FileName, file.Hunks, false, headReader, "head", warn)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, headReqs...)
	}
	return requirements, nil
}

func revisionRequirements(
	readName string,
	headName string,
	hunks []codeowners.HunkRange,
	allBlocks bool,
	reader codeowners.FileReader,
	revision string,
	warn io.Writer,
) ([]Requirement, error) {
	if (len(hunks) == 0 && !allBlocks) || !reader.PathExists(readName) {
		return nil, nil
	}
	content, err := reader.ReadFile(readName)
	if err != nil {
		return nil, fmt.Errorf("inline ownership: failed to read %s at %s: %w", readName, revision, err)
	}
	requirements := []Requirement{}
	for _, block := range Parse(string(content), warn) {
		if !allBlocks && !blockTouched(block, hunks) {
			continue
		}
		requirements = append(requirements, Requirement{
			File:   headName,
			Owners: block.Owners,
			Reason: fmt.Sprintf("inline ownership block at %s lines %d-%d", revision, block.Start, block.End),
		})
	}
	return requirements, nil
}

func blockTouched(block Block, hunks []codeowners.HunkRange) bool {
	for _, hunk := range hunks {
		// A zero-length hunk (End < Start) is an insertion point on this
		// side; it touches no lines here, and the other side's hunk
		// covers the inserted or deleted content.
		if hunk.End < hunk.Start {
			continue
		}
		if block.Overlaps(hunk.Start, hunk.End) {
			return true
		}
	}
	return false
}
