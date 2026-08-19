package git

import (
	"strings"
)

// Collapsed to one space, never deleted: whitespace holds two tokens apart, so
// dropping it would let `foo bar` compare equal to `foobar`.
func collapseWhitespace(block string) string {
	return strings.Join(strings.Fields(block), " ")
}
