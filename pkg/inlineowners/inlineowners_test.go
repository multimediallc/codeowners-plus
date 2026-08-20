package inlineowners

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

func TestParse(t *testing.T) {
	tt := []struct {
		name            string
		content         string
		expectedBlocks  []Block
		expectedWarning string
	}{
		{
			name: "basic slash block",
			content: strings.Join([]string{
				"package foo",
				"// <CO-inline={@alice,@team-a}>",
				"func Sensitive() {}",
				"// </CO-inline>",
				"func Other() {}",
			}, "\n"),
			expectedBlocks: []Block{{Owners: []string{"@alice", "@team-a"}, Start: 2, End: 4}},
		},
		{
			name: "hash block with indentation",
			content: strings.Join([]string{
				"class Foo:",
				"    # <CO-inline={@model-owner}>",
				"    field = 1",
				"    # </CO-inline>",
			}, "\n"),
			expectedBlocks: []Block{{Owners: []string{"@model-owner"}, Start: 2, End: 4}},
		},
		{
			name:           "space separated owners",
			content:        "// <CO-inline={@alice @team-a}>\nx\n// </CO-inline>",
			expectedBlocks: []Block{{Owners: []string{"@alice", "@team-a"}, Start: 1, End: 3}},
		},
		{
			name:           "mixed comma and space separators",
			content:        "// <CO-inline={@a, @b @c}>\nx\n// </CO-inline>",
			expectedBlocks: []Block{{Owners: []string{"@a", "@b", "@c"}, Start: 1, End: 3}},
		},
		{
			name:           "CRLF line endings",
			content:        "// <CO-inline={@a}>\r\nx\r\n// </CO-inline>\r\n",
			expectedBlocks: []Block{{Owners: []string{"@a"}, Start: 1, End: 3}},
		},
		{
			name:           "case insensitive tags",
			content:        "// <co-INLINE={@a}>\ncode\n// </Co-Inline>",
			expectedBlocks: []Block{{Owners: []string{"@a"}, Start: 1, End: 3}},
		},
		{
			name:           "multiple blocks",
			content:        "// <CO-inline={@a}>\nx\n// </CO-inline>\ny\n// <CO-inline={@b}>\nz\n// </CO-inline>",
			expectedBlocks: []Block{{Owners: []string{"@a"}, Start: 1, End: 3}, {Owners: []string{"@b"}, Start: 5, End: 7}},
		},
		{
			name:           "owners deduped case-insensitively",
			content:        "// <CO-inline={ @a , @b , @A }>\nx\n// </CO-inline>",
			expectedBlocks: []Block{{Owners: []string{"@a", "@b"}, Start: 1, End: 3}},
		},
		{
			name:            "unclosed block extends to EOF",
			content:         "// <CO-inline={@a}>\nx\ny",
			expectedBlocks:  []Block{{Owners: []string{"@a"}, Start: 1, End: 3}},
			expectedWarning: "Unclosed",
		},
		{
			name:            "nested start ignored",
			content:         "// <CO-inline={@a}>\n// <CO-inline={@b}>\nx\n// </CO-inline>",
			expectedBlocks:  []Block{{Owners: []string{"@a"}, Start: 1, End: 4}},
			expectedWarning: "Nested",
		},
		{
			name:            "stray end tag ignored",
			content:         "x\n// </CO-inline>\ny",
			expectedBlocks:  []Block{},
			expectedWarning: "Unmatched",
		},
		{
			name:            "empty owner list ignored",
			content:         "// <CO-inline={}>\nx\n// </CO-inline>",
			expectedBlocks:  []Block{},
			expectedWarning: "Empty owner list",
		},
		{
			name:           "same line open and close",
			content:        "// <CO-inline={@a}> </CO-inline>\nx",
			expectedBlocks: []Block{{Owners: []string{"@a"}, Start: 1, End: 1}},
		},
		{
			name:           "no tags",
			content:        "just\ncode",
			expectedBlocks: []Block{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			warn := &bytes.Buffer{}
			blocks := Parse(tc.content, warn)
			if !reflect.DeepEqual(blocks, tc.expectedBlocks) {
				t.Errorf("blocks mismatch:\n got %+v\nexpected %+v", blocks, tc.expectedBlocks)
			}
			if tc.expectedWarning != "" && !strings.Contains(warn.String(), tc.expectedWarning) {
				t.Errorf("expected warning containing %q, got %q", tc.expectedWarning, warn.String())
			}
			if tc.expectedWarning == "" && warn.Len() > 0 {
				t.Errorf("unexpected warnings: %q", warn.String())
			}
		})
	}
}

func TestBlockOverlaps(t *testing.T) {
	block := Block{Start: 5, End: 10}
	tt := []struct {
		name       string
		start, end int
		expected   bool
	}{
		{name: "before block", start: 1, end: 4, expected: false},
		{name: "touching start", start: 1, end: 5, expected: true},
		{name: "inside block", start: 7, end: 8, expected: true},
		{name: "touching end", start: 10, end: 20, expected: true},
		{name: "after block", start: 11, end: 20, expected: false},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if result := block.Overlaps(tc.start, tc.end); result != tc.expected {
				t.Errorf("Overlaps(%d, %d) = %t, expected %t", tc.start, tc.end, result, tc.expected)
			}
		})
	}
}

// mapReader is an in-memory FileReader keyed by path.
type mapReader map[string]string

func (m mapReader) ReadFile(path string) ([]byte, error) {
	content, ok := m[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return []byte(content), nil
}

func (m mapReader) PathExists(path string) bool {
	_, ok := m[path]
	return ok
}

func ownersByFile(requirements []Requirement) map[string][]string {
	result := make(map[string][]string)
	for _, requirement := range requirements {
		for _, owner := range requirement.Owners {
			if !slices.Contains(result[requirement.File], owner) {
				result[requirement.File] = append(result[requirement.File], owner)
			}
		}
	}
	return result
}

func TestRequirements(t *testing.T) {
	ownedBase := "// <CO-inline={@guard}>\nline2\nline3\n// </CO-inline>\nline5"
	base := mapReader{
		"owned.go":   ownedBase,
		"deleted.go": "// <CO-inline={@keeper}>\nprecious\n// </CO-inline>",
		"plain.go":   "nothing\nspecial",
	}
	head := mapReader{
		"owned.go": ownedBase + "\nline6",
		// deleted.go removed at head
		"plain.go": "nothing\nspecial\nnew line",
		"added.go": "// <CO-inline={@newbie}>\nfresh\n// </CO-inline>",
	}

	files := []codeowners.DiffFile{
		// change inside the owned block (head line 2, base line 2)
		{FileName: "owned.go", Hunks: []codeowners.HunkRange{{Start: 2, End: 2}}, BaseHunks: []codeowners.HunkRange{{Start: 2, End: 2}}},
		// whole file deleted: only base hunks
		{FileName: "deleted.go", Hunks: []codeowners.HunkRange{{Start: 0, End: -1}}, BaseHunks: []codeowners.HunkRange{{Start: 1, End: 3}}},
		// change outside any block
		{FileName: "plain.go", Hunks: []codeowners.HunkRange{{Start: 3, End: 3}}, BaseHunks: []codeowners.HunkRange{{Start: 2, End: 2}}},
		// new file with a block
		{FileName: "added.go", Hunks: []codeowners.HunkRange{{Start: 1, End: 3}}, BaseHunks: []codeowners.HunkRange{{Start: 0, End: -1}}},
	}

	warn := &bytes.Buffer{}
	requirements, err := Requirements(files, base, head, warn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	owners := ownersByFile(requirements)
	expected := map[string][]string{
		"owned.go":   {"@guard"},  // touched block (base and head)
		"deleted.go": {"@keeper"}, // block only exists at base
		"added.go":   {"@newbie"}, // block only exists at head
	}
	if !reflect.DeepEqual(owners, expected) {
		t.Errorf("owners mismatch:\n got %v\nexpected %v", owners, expected)
	}
	// owned.go is touched at base and head: one requirement per revision
	if len(requirements) != 4 {
		t.Errorf("expected 4 requirements (owned x2, deleted, added), got %+v", requirements)
	}
}

func TestRequirementsChangeOutsideBlock(t *testing.T) {
	content := "line1\n// <CO-inline={@guard}>\nline3\n// </CO-inline>\nline5"
	readers := mapReader{"a.go": content}
	files := []codeowners.DiffFile{
		{FileName: "a.go", Hunks: []codeowners.HunkRange{{Start: 5, End: 5}}, BaseHunks: []codeowners.HunkRange{{Start: 5, End: 5}}},
	}
	requirements, err := Requirements(files, readers, readers, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requirements) != 0 {
		t.Errorf("expected no requirements for change outside block, got %+v", requirements)
	}
}

func TestRequirementsEmptyHunkDoesNotTouchBlock(t *testing.T) {
	// A pure insertion right after a block produces a zero-length hunk on
	// the base side (e.g. @@ -3,0 +4 @@ -> base range {3, 2}); it touches
	// no base lines, so the block's owners must not be required.
	content := "// <CO-inline={@guard}>\nline2\n// </CO-inline>\nline4"
	readers := mapReader{"a.go": content}
	files := []codeowners.DiffFile{
		{FileName: "a.go", Hunks: []codeowners.HunkRange{{Start: 4, End: 4}}, BaseHunks: []codeowners.HunkRange{{Start: 3, End: 2}}},
	}
	requirements, err := Requirements(files, readers, readers, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requirements) != 0 {
		t.Errorf("zero-length hunk must not touch the block, got %+v", requirements)
	}
}

func TestRequirementsTagLineEditTriggersOwners(t *testing.T) {
	baseContent := "line1\n// <CO-inline={@guard}>\nline3\n// </CO-inline>\nline5"
	headContent := "line1\n// <CO-inline={@attacker}>\nline3\n// </CO-inline>\nline5"
	base := mapReader{"a.go": baseContent}
	head := mapReader{"a.go": headContent}
	// only the tag line (line 2) changed
	files := []codeowners.DiffFile{
		{FileName: "a.go", Hunks: []codeowners.HunkRange{{Start: 2, End: 2}}, BaseHunks: []codeowners.HunkRange{{Start: 2, End: 2}}},
	}
	requirements, err := Requirements(files, base, head, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(ownersByFile(requirements)["a.go"], "@guard") {
		t.Errorf("editing the tag line must still require the base owners, got %+v", requirements)
	}
}

func TestRequirementsRenamedFile(t *testing.T) {
	// A pure rename produces no hunks, but relocating protected regions
	// still requires the base blocks' owners; requirements target the
	// head file name.
	base := mapReader{"old.go": "// <CO-inline={@guard}>\ncontent\n// </CO-inline>\nplain"}
	head := mapReader{"new.go": "// <CO-inline={@guard}>\ncontent\n// </CO-inline>\nplain"}
	files := []codeowners.DiffFile{
		{FileName: "new.go", BaseFileName: "old.go"},
	}
	requirements, err := Requirements(files, base, head, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requirements) != 1 {
		t.Fatalf("expected 1 requirement for renamed file's block, got %+v", requirements)
	}
	if requirements[0].File != "new.go" || !slices.Contains(requirements[0].Owners, "@guard") {
		t.Errorf("expected @guard requirement on new.go, got %+v", requirements[0])
	}
}

func TestRequirementsMissingFilesAreSilent(t *testing.T) {
	files := []codeowners.DiffFile{
		{FileName: "gone.go", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}, BaseHunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
	}
	warn := &bytes.Buffer{}
	requirements, err := Requirements(files, mapReader{}, mapReader{}, warn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requirements) != 0 {
		t.Errorf("expected no requirements, got %+v", requirements)
	}
	if warn.Len() != 0 {
		t.Errorf("missing files should be silent (added/deleted are normal), got %q", warn.String())
	}
}

// failReader claims every path exists but fails to read it.
type failReader struct{}

func (failReader) ReadFile(path string) ([]byte, error) { return nil, errors.New("boom") }
func (failReader) PathExists(path string) bool          { return true }

func TestRequirementsReadErrorFailsClosed(t *testing.T) {
	files := []codeowners.DiffFile{
		{FileName: "a.go", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}, BaseHunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
	}
	_, err := Requirements(files, failReader{}, failReader{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Errorf("expected hard error for unreadable existing file, got %v", err)
	}
}
