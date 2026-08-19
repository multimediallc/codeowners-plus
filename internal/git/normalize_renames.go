package git

import (
	"strings"
)

// Swapping `and` for `or` wears the shape of a one-word rename while changing
// behavior, so no keyword may take part in a substitution.
var keywords = wordSet(`
	and or not in is if elif else elseif unless for foreach while do switch case
	default break continue return yield await async try catch except finally
	throw raise new delete typeof instanceof void null nil none true false this
	self super var let const def fn func function class struct interface enum
	trait impl import export include from as with using namespace package module
	pass lambda global nonlocal assert del defer go select chan range map static
	public private protected final abstract extends implements override readonly
	goto sizeof begin end then`)

func wordSet(words string) map[string]bool {
	set := make(map[string]bool)
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// A name is the only kind a rename may move; everything else spells structure,
// a value, or text which is read rather than called.
type token struct {
	text   string
	isName bool
}

// Deliberately strict: same tokens in the same order and number, exactly one name
// changed, the old name surviving nowhere, and the name one the change owns.
func isPureRename(added, removed string) bool {
	was, now := tokenize(removed), tokenize(added)
	if len(was) != len(now) {
		return false
	}

	var from, to token
	var at []int
	for i := range was {
		if was[i] == now[i] {
			continue
		}
		if !was[i].isName || !now[i].isName {
			return false
		}
		if len(at) > 0 && (was[i] != from || now[i] != to) {
			return false
		}
		from, to = was[i], now[i]
		at = append(at, i)
	}
	if len(at) == 0 {
		return false
	}
	// An occurrence the change left standing rules the rename out.
	for i := range was {
		if was[i] == from && now[i] == from {
			return false
		}
	}
	return ownsName(now, at, from, to)
}

// What catches an error names the same type, from a handler somewhere else.
var raiseOperands = wordSet("raise throw")

// A definition is referred to everywhere except the hunk declaring it.
var declarationKeywords = wordSet(`
	class def func function fn struct interface enum trait`)

// Owned only where the hunk holds the code that uses it.  Elsewhere the other half
// is out of sight, and the substitution repoints the code or points it at nothing.
func ownsName(tokens []token, at []int, from, to token) bool {
	// A member the hunk assigns is one it brings into being.
	defined := definesMember(tokens, at)

	for _, i := range at {
		previous, next := tokenAt(tokens, i-1), tokenAt(tokens, i+1)
		// Read before a call: a definition spells its name in front of the same
		// parentheses a call would.
		switch {
		case previous.text == "@":
			return false
		case raiseOperands[strings.ToLower(previous.text)]:
			return false
		case declarationKeywords[strings.ToLower(previous.text)]:
			// A runner finds a test by its prefix, not its full name, so a test
			// renamed to another test name is still run the same way.
			if !namesATest(from.text) || !namesATest(to.text) {
				return false
			}
		case next.text == "(":
			return false
		case previous.text == ".":
			if !defined {
				return false
			}
		}
	}
	return true
}

// Only a plain assignment defines: an augmented one writes its operator before the
// `=`, and a comparison doubles it, so neither is a single `=` following the name.
func definesMember(tokens []token, at []int) bool {
	for _, i := range at {
		if tokenAt(tokens, i+1).text != "=" {
			continue
		}
		if tokenAt(tokens, i+2).text == "=" {
			continue
		}
		return true
	}
	return false
}

const testNamePrefix = "test"

func namesATest(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), testNamePrefix)
}

// An index off either end returns an empty token, so a hunk edge matches no rule.
func tokenAt(tokens []token, i int) token {
	if i < 0 || i >= len(tokens) {
		return token{}
	}
	return tokens[i]
}

func tokenize(block string) []token {
	var tokens []token
	for i := skipSpace(block, 0); i < len(block); i = skipSpace(block, i) {
		if _, _, end, ok := stringLiteralAt(block, i); ok {
			// A literal is opaque: a word changed inside it is not a rename.
			tokens = append(tokens, token{text: block[i:end]})
			i = end
			continue
		}
		if end := identifierEnd(block, i); end > i {
			tokens = append(tokens, token{text: block[i:end], isName: isName(block[i:end])})
			i = end
			continue
		}
		tokens = append(tokens, token{text: block[i : i+1]})
		i++
	}
	return tokens
}

func identifierEnd(block string, i int) int {
	end := i
	for end < len(block) && isIdentifierByte(block[end]) {
		end++
	}
	return end
}

// A run opening with a digit is a value spelled out, not a name for one.
func isName(text string) bool {
	return !(text[0] >= '0' && text[0] <= '9') && !keywords[strings.ToLower(text)]
}
