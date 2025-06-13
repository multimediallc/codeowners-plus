package inlineowners

// Block represents an inline ownership region in a file.
// Start and End are inclusive line numbers (1-based).
// Empty-content blocks have StartLine > EndLine; they are ignored in range look-ups.
// Owners must already be trimmed/deduped.

type Block struct {
	Owners []string
	Start  int
	End    int
}

// Oracle maps a file path (relative to repository root) to its inline ownership blocks.
// It can answer ownership queries for any line range.

type Oracle map[string][]Block

// OwnersForRange returns a slice of owner lists for every block that overlaps the
// given line interval [start,end] (inclusive).  If no block overlaps, nil is returned.
// Empty-content blocks (Start > End) never match.
// The caller may deduplicate the returned owners if needed.
func (o Oracle) OwnersForRange(file string, start, end int) [][]string {
	blocks, ok := o[file]
	if !ok {
		return nil
	}
	matches := [][]string{}
	for _, b := range blocks {
		if b.Start > b.End { // empty block, ignore
			continue
		}
		if end >= b.Start && start <= b.End {
			matches = append(matches, b.Owners)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}
