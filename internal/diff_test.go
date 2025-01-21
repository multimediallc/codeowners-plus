package owners

import (
	"io"
	"os"
	"testing"

	"github.com/sourcegraph/go-diff/diff"
)

func readFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

func TestDiff(t *testing.T) {
	// Test case 1
	diffChangesOutput, err := readFile("../test_project/.diff_changes")
	if err != nil {
		t.Errorf("Error reading diff changes file: %v", err)
	}
	parsedDiff, err := diff.ParseMultiFileDiff(diffChangesOutput)
	if err != nil {
		t.Errorf("Error parsing diff changes: %v", err)
	}
	diffOutput, err := toDiffFiles(parsedDiff)
	if err != nil {
		t.Errorf("Error getting diff files: %v", err)
	}

	expectedDiffOutput := []DiffFile{
		{FileName: "a.py", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "models.py", Hunks: []HunkRange{{Start: 1, End: 1}, {Start: 3, End: 3}}},
		{FileName: "test_a.py", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/a.ts", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/b.ts", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/a.test.ts", Hunks: []HunkRange{{Start: 1, End: 4}}},
		{FileName: "frontend/inner/a.js", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/inner/b.ts", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/inner/a.test.js", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "backend/test.txt", Hunks: []HunkRange{{Start: 1, End: 1}}},
	}

	if len(diffOutput) != len(expectedDiffOutput) {
		t.Errorf("Expected %d diff files, got %d", len(expectedDiffOutput), len(diffOutput))
		return
	}

	for i, d := range diffOutput {
		d2 := expectedDiffOutput[i]
		if d.FileName != d2.FileName {
			t.Errorf("Expected file name %s, got %s", d2.FileName, d.FileName)
		}
		if len(d.Hunks) != len(d2.Hunks) {
			t.Errorf("Expected %d hunks, got %d", len(d2.Hunks), len(d.Hunks))
		}
		for j, h := range d.Hunks {
			h2 := d2.Hunks[j]
			if h.Start != h2.Start {
				t.Errorf("Expected start %d, got %d", h2.Start, h.Start)
			}
			if h.End != h2.End {
				t.Errorf("Expected end %d, got %d", h2.End, h.End)
			}
		}
	}

	// Test case 2
	diffChangesOutput, err = readFile("../test_project/.diff_nochanges")
	if err != nil {
		t.Errorf("Error reading diff changes file: %v", err)
	}
	parsedDiff, err = diff.ParseMultiFileDiff(diffChangesOutput)
	if err != nil {
		t.Errorf("Error parsing diff changes: %v", err)
	}
	diffOutput, err = toDiffFiles(parsedDiff)
	if err != nil {
		t.Errorf("Error getting diff files: %v", err)
	}

	if len(diffOutput) != 0 {
		t.Errorf("Expected 0 diff files, got %d", len(diffOutput))
	}
}

func TestDiffOfDiffs(t *testing.T) {
	newDiffData, err := readFile("../test_project/.diff_changes")
	if err != nil {
		t.Errorf("Error reading diff changes file: %v", err)
	}
	newDiff, err := diff.ParseMultiFileDiff(newDiffData)
	if err != nil {
		t.Errorf("Error parsing diff changes: %v", err)
	}

	oldDiffData, err := readFile("../test_project/.diff_changes_old")
	if err != nil {
		t.Errorf("Error reading diff changes file: %v", err)
	}
	oldDiff, err := diff.ParseMultiFileDiff(oldDiffData)
	if err != nil {
		t.Errorf("Error parsing diff changes: %v", err)
	}

	diffOutput, err := getChangesSince(changesSinceContext{newDiff, oldDiff})
	if err != nil {
		t.Errorf("Error getting diff of diffs: %v", err)
	}

	expectedDiffOutput := []DiffFile{
		{FileName: "a.py", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "models.py", Hunks: []HunkRange{{Start: 1, End: 1}}}, // 1 of 2 hunks not in old
		{FileName: "frontend/inner/a.test.js", Hunks: []HunkRange{{Start: 1, End: 1}}},
		{FileName: "backend/test.txt", Hunks: []HunkRange{{Start: 1, End: 1}}},
	}

	if len(diffOutput) != len(expectedDiffOutput) {
		t.Errorf("Expected %d diff files, got %d", len(expectedDiffOutput), len(diffOutput))
		return
	}

	for i, d := range diffOutput {
		d2 := expectedDiffOutput[i]
		if d.FileName != d2.FileName {
			t.Errorf("Expected file name %s, got %s", d2.FileName, d.FileName)
		}
		if len(d.Hunks) != len(d2.Hunks) {
			t.Errorf("Expected %d hunks, got %d", len(d2.Hunks), len(d.Hunks))
		}
		for j, h := range d.Hunks {
			h2 := d2.Hunks[j]
			if h.Start != h2.Start {
				t.Errorf("Expected start %d, got %d", h2.Start, h.Start)
			}
			if h.End != h2.End {
				t.Errorf("Expected end %d, got %d", h2.End, h.End)
			}
		}
	}
}
