package git

import (
	"strings"
)

func isLineSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

// A literal left open at the end of its line cannot be read, so it is not blanked.
func stringLiteralAt(block string, i int) (quote byte, content string, end int, ok bool) {
	if i >= len(block) || !strings.ContainsRune(`"'`+"`", rune(block[i])) {
		return 0, "", 0, false
	}
	quote = block[i]
	for j := i + 1; j < len(block); j++ {
		switch block[j] {
		case '\\':
			j++
		case '\n':
			return 0, "", 0, false
		case quote:
			return quote, block[i+1 : j], j + 1, true
		}
	}
	return 0, "", 0, false
}

func isIdentifierByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func skipSpace(block string, i int) int {
	for i < len(block) && (block[i] == ' ' || block[i] == '\t' || block[i] == '\n' || block[i] == '\r') {
		i++
	}
	return i
}
