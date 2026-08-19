package git

import (
	"strings"
)

// Nothing is blanked unless the call around it says the string is copy: a query,
// a path, a key and a sentence are all spelled the same way.
var translationFunctions = []string{"gettext", "ngettext", "trans", "_", "t"}

// A substitution names something outside the string, so text around one is not copy.
const placeholderChars = "{}%"

// An address is followed and matched on, so a word changed inside one moves it.
const schemeSeparator = "://"

// A string must hold a space to qualify: a single-token argument is a lookup key,
// and swapping keys asks for different copy rather than rewording it.
func blankTranslatedCopy(block string) string {
	var b strings.Builder
	b.Grow(len(block))

	for i := 0; i < len(block); {
		name, ok := translationCallAt(block, i)
		if !ok {
			b.WriteByte(block[i])
			i++
			continue
		}
		b.WriteString(name)
		b.WriteByte('(')
		i = blankLeadingCopy(&b, block, i+len(name)+1)
	}
	return b.String()
}

// The name must start a token, so a helper named `format_t` is not a call to `t`.
func translationCallAt(block string, i int) (string, bool) {
	if i > 0 && isIdentifierByte(block[i-1]) {
		return "", false
	}
	for _, name := range translationFunctions {
		if strings.HasPrefix(block[i:], name+"(") {
			return name, true
		}
	}
	return "", false
}

// Stops at the first non-copy argument: later arguments are the values the copy
// is rendered with, and those are code.
func blankLeadingCopy(b *strings.Builder, block string, i int) int {
	for {
		start := skipSpace(block, i)
		quote, content, end, ok := stringLiteralAt(block, start)
		if !ok || !isCopy(content) {
			return i
		}
		b.WriteString(block[i:start])
		b.WriteByte(quote)
		b.WriteByte(quote)

		i = end
		separator := skipSpace(block, end)
		if separator >= len(block) || block[separator] != ',' {
			return i
		}
		b.WriteString(block[i : separator+1])
		i = separator + 1
	}
}

func isCopy(content string) bool {
	return strings.Contains(content, " ") &&
		!strings.ContainsAny(content, placeholderChars) &&
		!strings.Contains(content, schemeSeparator)
}
