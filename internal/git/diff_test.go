package git

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/sourcegraph/go-diff/diff"
)

// mockGitExecutor implements GitCommandExecutor for testing
type mockGitExecutor struct {
	output string
	err    error
}

func NewMockGitExecutor(output string, err error) *mockGitExecutor {
	return &mockGitExecutor{
		output: output,
		err:    err,
	}
}

func (e *mockGitExecutor) execute(command string, args ...string) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	return []byte(e.output), nil
}

func readFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

// Test fixtures
const sampleGitDiff = `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -10,0 +11 @@ func Example() {
+       fmt.Println("New line")
diff --git a/file2.go b/file2.go
index ghi..jkl 100644
--- a/file2.go
+++ b/file2.go
@@ -20,0 +21,2 @@ func AnotherExample() {
+       fmt.Println("First new line")
+       fmt.Println("Second new line")`

func TestNewDiff(t *testing.T) {
	tt := []struct {
		name          string
		context       DiffContext
		mockOutput    string
		mockError     error
		expectedErr   bool
		expectedFiles int
		expectedHunks map[string]int // filename -> number of hunks
	}{
		{
			name: "successful diff",
			context: DiffContext{
				Base: "main",
				Head: "feature",
				Dir:  ".",
			},
			mockOutput:    sampleGitDiff,
			expectedErr:   false,
			expectedFiles: 2,
			expectedHunks: map[string]int{
				"file1.go": 1,
				"file2.go": 1,
			},
		},
		{
			name: "git command error",
			context: DiffContext{
				Base: "main",
				Head: "feature",
				Dir:  ".",
			},
			mockError:   errors.New("git command failed"),
			expectedErr: true,
		},
		{
			name: "ignore directories",
			context: DiffContext{
				Base:       "main",
				Head:       "feature",
				Dir:        ".",
				IgnoreDirs: []string{"file1"},
			},
			mockOutput:    sampleGitDiff,
			expectedErr:   false,
			expectedFiles: 1,
			expectedHunks: map[string]int{
				"file2.go": 1,
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock executor
			executor := NewMockGitExecutor(tc.mockOutput, tc.mockError)

			// Run the test
			diff, err := NewDiffWithExecutor(tc.context, executor)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if diff == nil {
				t.Error("expected non-nil diff")
				return
			}

			changes := diff.AllChanges()
			if len(changes) != tc.expectedFiles {
				t.Errorf("expected %d files, got %d", tc.expectedFiles, len(changes))
			}

			for _, file := range changes {
				expectedHunks, ok := tc.expectedHunks[file.FileName]
				if !ok {
					t.Errorf("unexpected file: %s", file.FileName)
					continue
				}
				if len(file.Hunks) != expectedHunks {
					t.Errorf("file %s: expected %d hunks, got %d", file.FileName, expectedHunks, len(file.Hunks))
				}
			}
		})
	}
}

func TestChangesSince(t *testing.T) {
	const olderDiff = `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -5,0 +6 @@ func Example() {
+       fmt.Println("Old change")`

	tt := []struct {
		name             string
		context          DiffContext
		ref              string
		currentDiff      string
		olderDiff        string
		mockError        error
		expectedErr      bool
		expectedFiles    int
		expectedNewHunks map[string]int // filename -> number of new hunks
	}{
		{
			name: "new changes detected",
			context: DiffContext{
				Base: "main",
				Head: "feature",
				Dir:  ".",
			},
			ref:           "old-ref",
			currentDiff:   sampleGitDiff,
			olderDiff:     olderDiff,
			expectedErr:   false,
			expectedFiles: 2,
			expectedNewHunks: map[string]int{
				"file1.go": 1,
				"file2.go": 1,
			},
		},
		{
			name: "error getting older diff",
			context: DiffContext{
				Base: "main",
				Head: "feature",
				Dir:  ".",
			},
			ref:         "old-ref",
			mockError:   errors.New("git command failed"),
			expectedErr: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Create initial diff with current changes
			executor := NewMockGitExecutor(tc.currentDiff, nil)
			diff, err := NewDiffWithExecutor(tc.context, executor)
			if err != nil {
				t.Fatalf("failed to create initial diff: %v", err)
			}

			// Set up mock executor for older diff
			executor = NewMockGitExecutor(tc.olderDiff, tc.mockError)
			diff.(*GitDiff).executor = executor // Update the executor in the diff

			// Run the test
			changes, err := diff.ChangesSince(tc.ref)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(changes) != tc.expectedFiles {
				t.Errorf("expected %d files, got %d", tc.expectedFiles, len(changes))
			}

			for _, file := range changes {
				expectedHunks, ok := tc.expectedNewHunks[file.FileName]
				if !ok {
					t.Errorf("unexpected file: %s", file.FileName)
					continue
				}
				if len(file.Hunks) != expectedHunks {
					t.Errorf("file %s: expected %d hunks, got %d", file.FileName, expectedHunks, len(file.Hunks))
				}
			}
		})
	}
}

func TestHunkHash(t *testing.T) {
	tt := []struct {
		name         string
		hunkBody     []byte
		hunk2Body    []byte
		expectedSame bool
	}{
		{
			name: "same content different context",
			hunkBody: []byte(`-old line
+new line
 context line 1`),
			hunk2Body: []byte(`-old line
+new line
 different context`),
			expectedSame: true,
		},
		{
			name: "different content",
			hunkBody: []byte(`-old line
+different line
 context line 1`),
			hunk2Body: []byte(`-old line
+another different line
 context line 1`),
			expectedSame: false,
		},
		{
			name:         "empty hunk",
			hunkBody:     []byte(``),
			hunk2Body:    []byte(``),
			expectedSame: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			hunk1 := &diff.Hunk{Body: tc.hunkBody}
			hunk2 := &diff.Hunk{Body: tc.hunk2Body}

			hash1 := hunkHash(hunk1)
			hash2 := hunkHash(hunk2)

			if tc.expectedSame {
				if hash1 != hash2 {
					t.Error("hashes should be equal")
				}
			} else {
				if hash1 == hash2 {
					t.Error("hashes should be different")
				}
			}
		})
	}
}

func TestToDiffFiles(t *testing.T) {
	tt := []struct {
		name        string
		fileDiffs   []*diff.FileDiff
		expected    []codeowners.DiffFile
		expectedErr bool
	}{
		{
			name: "single file single hunk",
			fileDiffs: []*diff.FileDiff{
				{
					NewName: "b/file1.go",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 10,
							NewLines:     1,
						},
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "file1.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 10,
							End:   10,
						},
					},
				},
			},
		},
		{
			name: "multiple files multiple hunks",
			fileDiffs: []*diff.FileDiff{
				{
					NewName: "b/file1.go",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 10,
							NewLines:     2,
						},
					},
				},
				{
					NewName: "b/file2.go",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 20,
							NewLines:     3,
						},
						{
							NewStartLine: 30,
							NewLines:     1,
						},
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "file1.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 10,
							End:   11,
						},
					},
				},
				{
					FileName: "file2.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 20,
							End:   22,
						},
						{
							Start: 30,
							End:   30,
						},
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toDiffFiles(tc.fileDiffs)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(got) != len(tc.expected) {
				t.Errorf("expected %d files, got %d", len(tc.expected), len(got))
				return
			}

			for i, expectedFile := range tc.expected {
				gotFile := got[i]
				if gotFile.FileName != expectedFile.FileName {
					t.Errorf("file %d: expected name %s, got %s", i, expectedFile.FileName, gotFile.FileName)
				}
				if len(gotFile.Hunks) != len(expectedFile.Hunks) {
					t.Errorf("file %s: expected %d hunks, got %d", gotFile.FileName, len(expectedFile.Hunks), len(gotFile.Hunks))
				}
				for j, expectedHunk := range expectedFile.Hunks {
					gotHunk := gotFile.Hunks[j]
					if gotHunk.Start != expectedHunk.Start {
						t.Errorf("file %s, hunk %d: expected start %d, got %d", gotFile.FileName, j, expectedHunk.Start, gotHunk.Start)
					}
					if gotHunk.End != expectedHunk.End {
						t.Errorf("file %s, hunk %d: expected end %d, got %d", gotFile.FileName, j, expectedHunk.End, gotHunk.End)
					}
				}
			}
		})
	}
}

func TestDiff(t *testing.T) {
	// Test case 1
	diffChangesOutput, err := readFile("../../test_project/.diff_changes")
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

	expectedDiffOutput := []codeowners.DiffFile{
		{FileName: "a.py", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "models.py", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}, {Start: 3, End: 3}}},
		{FileName: "test_a.py", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/a.ts", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/b.ts", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/a.test.ts", Hunks: []codeowners.HunkRange{{Start: 1, End: 4}}},
		{FileName: "frontend/inner/a.js", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/inner/b.ts", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "frontend/inner/a.test.js", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "backend/test.txt", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
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
	diffChangesOutput, err = readFile("../../test_project/.diff_nochanges")
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
	newDiffData, err := readFile("../../test_project/.diff_changes")
	if err != nil {
		t.Errorf("Error reading diff changes file: %v", err)
	}
	newDiff, err := diff.ParseMultiFileDiff(newDiffData)
	if err != nil {
		t.Errorf("Error parsing diff changes: %v", err)
	}

	oldDiffData, err := readFile("../../test_project/.diff_changes_old")
	if err != nil {
		t.Errorf("Error reading diff changes file: %v", err)
	}
	oldDiff, err := diff.ParseMultiFileDiff(oldDiffData)
	if err != nil {
		t.Errorf("Error parsing diff changes: %v", err)
	}

	diffOutput, err := changesSince(changesSinceContext{newDiff, oldDiff})
	if err != nil {
		t.Errorf("Error getting diff of diffs: %v", err)
	}

	expectedDiffOutput := []codeowners.DiffFile{
		{FileName: "a.py", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "models.py", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}}, // 1 of 2 hunks not in old
		{FileName: "frontend/inner/a.test.js", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
		{FileName: "backend/test.txt", Hunks: []codeowners.HunkRange{{Start: 1, End: 1}}},
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
