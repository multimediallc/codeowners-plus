package git

import (
	"path/filepath"
	"strings"
)

// Decided per file: `//` divides in Python, `--` decrements in C, `#` opens an id
// selector in CSS, so a marker from the wrong language reads a real change as prose.
type commentSyntax struct {
	// Unambiguous: no statement begins this way, so the prefix alone is enough.
	linePrefixes []string
	// Ambiguous: only a marker standing on its own opens a comment, so `#define`
	// and `--verbose` stay code.
	lineMarkers []string
	// A subset of the above: a marker which only continues or closes a block
	// comment opens nothing mid-line.
	trailingPrefixes []string
	trailingMarkers  []string
}

// A language names the styles it uses rather than repeating their markers, so each
// table entry below carries one claim worth checking.
var (
	slashStyle = commentSyntax{
		linePrefixes:     []string{"//"},
		trailingPrefixes: []string{"//"},
	}
	// `*` is a marker, not a prefix: a continuation line shares it with dereferences
	// and splats, and it opens no trailing comment, so multiplication is safe.
	blockStyle = commentSyntax{
		linePrefixes:     []string{"/*", "*/"},
		lineMarkers:      []string{"*"},
		trailingPrefixes: []string{"/*"},
	}
	hashStyle = commentSyntax{
		lineMarkers:     []string{"#"},
		trailingMarkers: []string{"#"},
	}
	dashStyle = commentSyntax{
		lineMarkers:     []string{"--"},
		trailingMarkers: []string{"--"},
	}
	// Opens no trailing comment: a `<!--` after content is as likely escaped as parsed.
	markupStyle = commentSyntax{
		linePrefixes: []string{"<!--", "-->"},
	}
	docstringStyle = commentSyntax{
		linePrefixes: []string{`"""`, `'''`},
	}
)

var (
	cFamilyComments = mergeCommentSyntax(slashStyle, blockStyle)
	pythonComments  = mergeCommentSyntax(hashStyle, docstringStyle)
	phpComments     = mergeCommentSyntax(slashStyle, blockStyle, hashStyle)
	sqlComments     = mergeCommentSyntax(dashStyle, blockStyle)
)

// A language left out of the table keeps its comments, costing a review round; one
// given a marker it does not have loses a real change, costing the review itself.
var commentSyntaxByExtension = map[string]commentSyntax{
	".c":     cFamilyComments,
	".cc":    cFamilyComments,
	".cjs":   cFamilyComments,
	".cpp":   cFamilyComments,
	".cs":    cFamilyComments,
	".dart":  cFamilyComments,
	".go":    cFamilyComments,
	".h":     cFamilyComments,
	".hpp":   cFamilyComments,
	".java":  cFamilyComments,
	".js":    cFamilyComments,
	".jsx":   cFamilyComments,
	".kt":    cFamilyComments,
	".less":  cFamilyComments,
	".mjs":   cFamilyComments,
	".proto": cFamilyComments,
	".rs":    cFamilyComments,
	".scala": cFamilyComments,
	".scss":  cFamilyComments,
	".swift": cFamilyComments,
	".ts":    cFamilyComments,
	".tsx":   cFamilyComments,

	// CSS has no `//` line comment: the declaration it was meant to hide still applies.
	".css": blockStyle,

	".py":  pythonComments,
	".pyi": pythonComments,

	".bash": hashStyle,
	".ini":  hashStyle,
	".rb":   hashStyle,
	".sh":   hashStyle,
	".toml": hashStyle,
	".yaml": hashStyle,
	".yml":  hashStyle,
	".zsh":  hashStyle,

	".sql": sqlComments,
	".php": phpComments,

	".htm":  markupStyle,
	".html": markupStyle,
	".svg":  markupStyle,
	".xml":  markupStyle,
	// Three languages in one file, and a diff cannot see the block bounds, so only
	// the markup markers are safe across the whole of it.
	".vue": markupStyle,
}

var commentSyntaxByName = map[string]commentSyntax{
	"dockerfile":    hashStyle,
	"gemfile":       hashStyle,
	"jenkinsfile":   cFamilyComments,
	"makefile":      hashStyle,
	"rakefile":      hashStyle,
	"vagrantfile":   hashStyle,
	".gitignore":    hashStyle,
	".dockerignore": hashStyle,
}

func mergeCommentSyntax(styles ...commentSyntax) commentSyntax {
	var merged commentSyntax
	for _, style := range styles {
		merged.linePrefixes = append(merged.linePrefixes, style.linePrefixes...)
		merged.lineMarkers = append(merged.lineMarkers, style.lineMarkers...)
		merged.trailingPrefixes = append(merged.trailingPrefixes, style.trailingPrefixes...)
		merged.trailingMarkers = append(merged.trailingMarkers, style.trailingMarkers...)
	}
	return merged
}

// An unrecognized name gets no markers rather than a permissive guess: applying a
// marker to a language that lacks it is the one mistake this package cannot make.
func commentSyntaxFor(fileName string) commentSyntax {
	base := strings.ToLower(filepath.Base(fileName))
	if syntax, ok := commentSyntaxByExtension[filepath.Ext(base)]; ok {
		return syntax
	}
	return commentSyntaxByName[base]
}

func (s commentSyntax) spellsComments() bool {
	return len(s.linePrefixes) > 0 || len(s.lineMarkers) > 0
}

// A file this package cannot name is one no punctuation guard was written for.
func knownLanguage(fileName string) bool {
	return commentSyntaxFor(fileName).spellsComments()
}
