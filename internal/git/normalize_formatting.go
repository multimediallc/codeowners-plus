package git

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// Dropped entirely, so a braced body and a single-statement body compare equal.
	structuralPunctuation = "{};"
	// Binds to its neighbour, so the spacing around it is a wrapping choice.
	tightPunctuation = "()[],"
)

// Parentheses and empty brace pairs are deliberately kept: they separate a call
// from a reference and decide precedence, so dropping them would hide changes.
func collapseFormatting(block string) string {
	var b strings.Builder
	b.Grow(len(block))

	var last rune
	pendingSpace := false
	writeToken := func(text string, opening, closing rune) {
		if pendingSpace {
			pendingSpace = false
			// Space matters only where two tokens would otherwise run together.
			if !isTight(opening) && !isTight(last) {
				b.WriteRune(' ')
			}
		}
		b.WriteString(text)
		last = closing
	}

	for i := 0; i < len(block); {
		if end, ok := emptyBracePairAt(block, i); ok {
			// A brace closing over nothing is an empty argument, body or literal,
			// not structure: the code says something different without it.
			writeToken(emptyBracePair, '{', '}')
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(block[i:])
		i += size
		if unicode.IsSpace(r) || strings.ContainsRune(structuralPunctuation, r) {
			pendingSpace = b.Len() > 0
			continue
		}
		writeToken(string(r), r, r)
	}
	return dropTrailingCommas(b.String())
}

const emptyBracePair = "{}"

// Only whitespace may stand between the two, so a pair holding a body, however
// it is wrapped, is structure like any other.
func emptyBracePairAt(block string, i int) (end int, ok bool) {
	if i >= len(block) || block[i] != '{' {
		return 0, false
	}
	closing := skipSpace(block, i+1)
	if closing >= len(block) || block[closing] != '}' {
		return 0, false
	}
	return closing + 1, true
}

func isTight(r rune) bool {
	return strings.ContainsRune(tightPunctuation, r)
}

func dropTrailingCommas(block string) string {
	if !strings.Contains(block, ",") {
		return block
	}

	var b strings.Builder
	b.Grow(len(block))
	for i, r := range block {
		if r == ',' && closesAfter(block[i+1:]) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func closesAfter(rest string) bool {
	return rest == "" || rest[0] == ')' || rest[0] == ']'
}
