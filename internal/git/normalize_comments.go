package git

import (
	"strings"
)

// The trailing case is the one that matters: an appended annotation leaves the
// code it annotates byte for byte identical.  Directives stay, see isDirective.
func dropComments(fileName, block string) string {
	syntax := commentSyntaxFor(fileName)
	if !syntax.spellsComments() {
		return block
	}

	lines := strings.Split(block, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if body, ok := syntax.commentBody(strings.TrimSpace(line)); ok {
			if isDirective(body) {
				kept = append(kept, line)
			}
			continue
		}
		kept = append(kept, syntax.stripTrailingComment(line))
	}
	return strings.Join(kept, "\n")
}

func (s commentSyntax) commentBody(trimmed string) (string, bool) {
	for _, prefix := range s.linePrefixes {
		if body, ok := strings.CutPrefix(trimmed, prefix); ok {
			return body, true
		}
	}
	for _, marker := range s.lineMarkers {
		if body, ok := commentBodyAfter(trimmed, marker); ok {
			return body, true
		}
	}
	return "", false
}

// A directive is left where it is: it says what a tool must do about the very line
// it is appended to, so it is compared along with that line.
func (s commentSyntax) stripTrailingComment(line string) string {
	at, body, ok := s.trailingComment(line)
	if !ok || isDirective(body) {
		return line
	}
	return strings.TrimRight(line[:at], " \t")
}

// A quoted line carries none: a marker inside a string literal is text, and telling
// the two apart needs a parser this does not have.  A marker must follow whitespace.
func (s commentSyntax) trailingComment(line string) (at int, body string, ok bool) {
	if strings.ContainsAny(line, "\"'`") {
		return 0, "", false
	}
	for i := 1; i < len(line); i++ {
		if !isLineSpace(line[i-1]) {
			continue
		}
		if body, ok := s.trailingCommentBody(line[i:]); ok {
			return i, body, true
		}
	}
	return 0, "", false
}

// Reads the block as the hunk spelled it, before a step can rewrite the punctuation
// a marker is made of.
func holdsDirective(fileName, block string) bool {
	syntax := commentSyntaxFor(fileName)
	if !syntax.spellsComments() {
		return false
	}
	for _, line := range strings.Split(block, "\n") {
		if body, ok := syntax.commentBody(strings.TrimSpace(line)); ok {
			if isDirective(body) {
				return true
			}
			continue
		}
		if _, body, ok := syntax.trailingComment(line); ok && isDirective(body) {
			return true
		}
	}
	return false
}

func (s commentSyntax) trailingCommentBody(rest string) (string, bool) {
	for _, prefix := range s.trailingPrefixes {
		if body, ok := strings.CutPrefix(rest, prefix); ok {
			return body, true
		}
	}
	for _, marker := range s.trailingMarkers {
		if body, ok := commentBodyAfter(rest, marker); ok {
			return body, true
		}
	}
	return "", false
}

// The marker must stand on its own - ending the text, followed by a space, or
// repeated - which is what separates a comment from `#define`, `--verbose`, `**kwargs`.
func commentBodyAfter(text, marker string) (string, bool) {
	rest, ok := strings.CutPrefix(text, marker)
	if !ok {
		return "", false
	}
	if rest == "" || isLineSpace(rest[0]) {
		return rest, true
	}
	return commentBodyAfter(rest, marker)
}

// A directive is configuration addressed to a tool, so changing one changes which
// findings a checker may report: compared like code rather than dropped as prose.
// Prefixes, not whole directives, so the argument each carries is compared too.
var directivePrefixes = []string{
	// Python and the checkers which read it.
	"noqa", "type:", "mypy:", "pyright:", "pytype:", "pyre-", "ruff:",
	"flake8:", "pylint:", "bandit", "nosec", "#nosec", "isort:", "yapf:",
	"fmt:", "coverage:", "pragma",

	// JavaScript, TypeScript and the web toolchain.
	"eslint", "tslint:", "prettier-ignore", "@ts-", "biome-ignore",
	"deno-lint-ignore", "istanbul", "c8 ignore", "v8 ignore", "stylelint",
	"webpack",

	// Go.  `nolint` covers golangci-lint; clang-tidy spells its own suffixes
	// into the same word, which the boundary rule below would otherwise reject.
	"go:", "+build", "golangci", "gosec", "lint:", "revive:",
	"nolint", "nolintnextline", "nolintbegin", "nolintend",

	// Other languages, and the analyzers which read across all of them.
	"checkstyle", "nosonar", "nopmd", "codeql", "suppresswarnings",
	"suppressfbwarnings", "rubocop:", "shellcheck", "hadolint", "swiftlint:",
	"ktlint", "clang-format", "clang-tidy", "phpcs:", "phpstan", "psalm",
	"markdownlint", "tflint",
}

// A prefix ending mid-word counts only where the word ends with it, so "pragmatic
// for now" and "eslintrc covers this" stay prose.
func isDirective(body string) bool {
	body = strings.ToLower(strings.TrimLeft(body, " \t"))
	for _, prefix := range directivePrefixes {
		rest, ok := strings.CutPrefix(body, prefix)
		if !ok {
			continue
		}
		if isIdentifierByte(prefix[len(prefix)-1]) && rest != "" && isIdentifierByte(rest[0]) {
			continue
		}
		return true
	}
	return false
}
