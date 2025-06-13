package inlineowners

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// InlineBlock represents an inline ownership block extracted from a source file.
// Owners contains the list parsed from the start tag. StartLine/EndLine are
// 1-based indices of the lines inside the block (exclusive of the tag lines).
// If EndLine < StartLine the block is effectively empty but still returned so
// the caller can decide how to handle it.
//
// Example of a block in a Go file:
//
//	// <CO-inline={@alice,@bob}>
//	code here
//	// </CO-inline>
//
// Lines containing the two "//" tags are NOT included in the range.
// The first code line has index StartLine and the last code line is EndLine.
//
// The parser is tolerant: malformed or unmatched tags generate warnings via
// the provided writer but do not fail the parse completely.
//
// Currently supported comment prefixes: "//" and "#".  Both may be preceded
// by arbitrary whitespace.
// Tag names are matched case-insensitively.
//
// The parser also supports the edge-case where the start and end tag both
// appear on the same line (e.g. `// <CO-inline={@a}> /* code */ // </CO-inline>`).
// In such a scenario the block's content lines are considered empty; the
// returned InlineBlock will have StartLine > EndLine so that callers can treat
// it as a zero-length range.
type InlineBlock struct {
	Owners    []string
	StartLine int
	EndLine   int
}

var (
	startTagRe = regexp.MustCompile(`(?i)^\s*(?P<prefix>//|#)\s*<CO-inline=\{(?P<owners>[^}]*)}>\s*$`)
	endTagRe   = regexp.MustCompile(`(?i)^\s*(?://|#)\s*</CO-inline>\s*$`)
	// start tag that can exist *anywhere* in a line (used when start & end
	// tags share the same line; we don't anchor with ^ and $)
	inlineStartAnywhereRe = regexp.MustCompile(`(?i)<CO-inline=\{(?P<owners>[^}]*)}>`)
	endAnywhereRe         = regexp.MustCompile(`(?i)</CO-inline>`) // end tag anywhere in line
)

// Parse scans the given file content and returns all inline ownership blocks.
// Any warnings (unclosed tags, malformed owner list, nested blocks, etc.) are
// written to warn. The function never returns an error; severe issues are also
// reported through warn so callers can decide whether to fail the build.
func Parse(content string, warn io.Writer) ([]InlineBlock, error) {
	blocks := []InlineBlock{}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0
	inBlock := false
	var current InlineBlock
	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")

		lower := strings.ToLower(line)

		// Fast path: both start & end on same line
		if openIdx := inlineStartAnywhereRe.FindStringIndex(line); openIdx != nil {
			endIdx := endAnywhereRe.FindStringIndex(line[openIdx[1]:])
			if endIdx != nil {
				ownersRaw := inlineStartAnywhereRe.FindStringSubmatch(line)[inlineStartAnywhereRe.SubexpIndex("owners")]
				ownersSlice := splitOwners(strings.TrimSpace(ownersRaw))
				if len(ownersSlice) == 0 {
					_, _ = fmt.Fprintf(warn, "WARNING: Empty owner list in inline tag at line %d\n", lineNo)
				}
				blk := InlineBlock{
					Owners:    ownersSlice,
					StartLine: lineNo + 1, // no interior lines
					EndLine:   lineNo,     // StartLine > EndLine -> empty content
				}
				blocks = append(blocks, blk)
				// Remove this occurence so outer logic doesn't treat it twice
				continue
			}
		}

		if m := startTagRe.FindStringSubmatch(line); m != nil {
			if inBlock {
				// nested start tag – disallowed
				_, _ = fmt.Fprintf(warn, "WARNING: Nested <CO-inline> tag at line %d ignored\n", lineNo)
				continue
			}
			inBlock = true
			ownersRaw := strings.TrimSpace(m[startTagRe.SubexpIndex("owners")])
			ownersSlice := splitOwners(ownersRaw)
			if len(ownersSlice) == 0 {
				_, _ = fmt.Fprintf(warn, "WARNING: Empty owner list in inline tag at line %d\n", lineNo)
			}
			current = InlineBlock{
				Owners:    ownersSlice,
				StartLine: lineNo + 1, // first line AFTER the start tag
			}
			continue
		}

		if endTagRe.MatchString(line) {
			if !inBlock {
				// stray end tag
				_, _ = fmt.Fprintf(warn, "WARNING: Unmatched </CO-inline> tag at line %d ignored\n", lineNo)
				continue
			}
			// Finalise current block
			current.EndLine = lineNo - 1 // last line BEFORE the end tag
			blocks = append(blocks, current)
			inBlock = false
			continue
		}

		// Detect malformed starting attempt e.g. missing braces or >
		if strings.Contains(lower, "<co-inline") && !inBlock {
			// If it wasn't matched by the valid regex path earlier, it's malformed
			_, _ = fmt.Fprintf(warn, "WARNING: Malformed <CO-inline> tag at line %d ignored\n", lineNo)
		}
	}

	if inBlock {
		// EOF reached without closing tag
		_, _ = fmt.Fprintf(warn, "WARNING: <CO-inline> tag starting at line %d not closed before EOF\n", current.StartLine-1)
		// Set EndLine to last line of file
		current.EndLine = lineNo
		blocks = append(blocks, current)
	}

	return blocks, nil
}

func splitOwners(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	owners := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			owners = append(owners, trimmed)
		}
	}
	return owners
}
