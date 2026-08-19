package git

import (
	"path"
	"strings"
)

// Languages which delimit a block by how far its lines are indented, so moving a
// statement in or out of one changes what runs rather than how it reads.
var indentDelimitedExtensions = map[string]bool{
	".py": true, ".pyi": true, ".pyx": true,
	".yaml": true, ".yml": true,
	".sass": true, ".haml": true, ".slim": true, ".pug": true, ".jade": true,
	".nim": true, ".coffee": true,
}

// PEP 8's indent width, weighing a tab against spaces rather than rendering
// anything, so a tabs-to-spaces conversion at unchanged depth compares equal.
const tabColumns = 4

func indentDepth(line string) int {
	depth := 0
	for _, r := range line {
		switch r {
		case '\t':
			depth += tabColumns
		case ' ':
			depth++
		default:
			return depth
		}
	}
	return depth
}

func indentIsSyntax(fileName string) bool {
	return indentDelimitedExtensions[strings.ToLower(path.Ext(fileName))]
}

// reindentsBlock reports whether the two sides are the same lines in the same
// order at a different depth, which collapseWhitespace reads as nothing at all.
func reindentsBlock(fileName, added, removed string) bool {
	if !indentIsSyntax(fileName) {
		return false
	}
	addedLines, removedLines := strings.Split(added, "\n"), strings.Split(removed, "\n")
	// What keeps this narrow enough to be worth having: rewrapping an argument
	// list changes the line count, so it never reaches the depth comparison.
	if len(addedLines) != len(removedLines) {
		return false
	}
	moved := false
	for i := range addedLines {
		a, r := addedLines[i], removedLines[i]
		aBody, rBody := strings.TrimLeft(a, " \t"), strings.TrimLeft(r, " \t")
		if aBody != rBody {
			return false
		}
		// A blank line has no depth to carry, so its padding is not a move.
		if aBody != "" && indentDepth(a) != indentDepth(r) {
			moved = true
		}
	}
	return moved
}
