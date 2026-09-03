package git

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

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

type mockResult struct {
	output string
	err    error
}

type scriptedGitExecutor struct {
	diffResults []mockResult
	fetchResult mockResult
	refResolves bool
	calls       [][]string
}

func (e *scriptedGitExecutor) execute(command string, args ...string) ([]byte, error) {
	e.calls = append(e.calls, append([]string{command}, args...))
	res := e.fetchResult
	if len(args) > 0 && args[0] == "cat-file" {
		if e.refResolves {
			return nil, nil
		}
		return nil, errors.New("not a valid object name")
	}
	if len(args) > 0 && args[0] == "diff" {
		if len(e.diffResults) == 0 {
			return nil, errors.New("unexpected git diff call")
		}
		res, e.diffResults = e.diffResults[0], e.diffResults[1:]
	}
	// CombinedOutput returns both, and the output is where git puts the reason.
	return []byte(res.output), res.err
}

type timeoutScriptedExecutor struct {
	scriptedGitExecutor
	timeouts []time.Duration
}

func (e *timeoutScriptedExecutor) executeWithTimeout(timeout time.Duration, command string, args ...string) ([]byte, error) {
	e.timeouts = append(e.timeouts, timeout)
	return e.execute(command, args...)
}

func (e *scriptedGitExecutor) fetchCalls() [][]string {
	fetches := make([][]string, 0, len(e.calls))
	for _, call := range e.calls {
		if len(call) > 1 && call[1] == "fetch" {
			fetches = append(fetches, call)
		}
	}
	return fetches
}

func readFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("WARNING: Error closing file: %v\n", err)
		}
	}()

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
		{
			name: "binary file change does not panic",
			context: DiffContext{
				Base: "main",
				Head: "feature",
				Dir:  ".",
			},
			mockOutput: `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -10,0 +11 @@ func Example() {
+       fmt.Println("New line")
diff --git a/assets/img/offline.png b/assets/img/offline.png
index 1111111..2222222 100644
Binary files a/assets/img/offline.png and b/assets/img/offline.png differ`,
			expectedErr:   false,
			expectedFiles: 2,
			expectedHunks: map[string]int{
				"file1.go":               1,
				"assets/img/offline.png": 0,
			},
		},
		{
			// Regression for the production panic: a "mode change + binary
			// patch" entry is parsed by go-diff into a FileDiff with empty
			// OrigName/NewName (its handleEmpty logic does not cover this
			// 5-extended-header shape). The pre-fix code did NewName[2:]
			// and panicked.
			name: "mode change + binary patch",
			context: DiffContext{
				Base: "main",
				Head: "feature",
				Dir:  ".",
			},
			mockOutput: `diff --git a/assets/img.png b/assets/img.png
old mode 100755
new mode 100644
index 8ed9b15..0bfca45
Binary files a/assets/img.png and b/assets/img.png differ
diff --git a/notes.txt b/notes.txt
index abc..def 100644
--- a/notes.txt
+++ b/notes.txt
@@ -1,0 +2 @@ trailing entry forces parser to flush the mode-change+binary above
+ok
`,
			expectedErr:   false,
			expectedFiles: 2,
			expectedHunks: map[string]int{
				"assets/img.png": 0,
				"notes.txt":      1,
			},
		},
		{
			name: "binary file in ignored directory is filtered",
			context: DiffContext{
				Base:       "main",
				Head:       "feature",
				Dir:        ".",
				IgnoreDirs: []string{"assets/"},
			},
			mockOutput: `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -10,0 +11 @@ func Example() {
+       fmt.Println("New line")
diff --git a/assets/img/offline.png b/assets/img/offline.png
index 1111111..2222222 100644
Binary files a/assets/img/offline.png and b/assets/img/offline.png differ`,
			expectedErr:   false,
			expectedFiles: 1,
			expectedHunks: map[string]int{
				"file1.go": 1,
			},
		},
		{
			name: "ignore deleted files in ignored directories",
			context: DiffContext{
				Base:       "main",
				Head:       "feature",
				Dir:        ".",
				IgnoreDirs: []string{"ignored/"},
			},
			mockOutput: `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -10,0 +11 @@ func Example() {
+       fmt.Println("New line")
diff --git a/ignored/deleted.go b/dev/null
deleted file mode 100644
index ghi..0000000
--- a/ignored/deleted.go
+++ /dev/null
@@ -1 +0,0 @@
-content`,
			expectedErr:   false,
			expectedFiles: 1,
			expectedHunks: map[string]int{
				"file1.go": 1,
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

func TestFetchRefSurfacesGitOutput(t *testing.T) {
	executor := &scriptedGitExecutor{
		diffResults: []mockResult{{output: sampleGitDiff}, {err: errors.New("fatal: bad object deadbeef")}},
		fetchResult: mockResult{output: "fatal: remote error: upload-pack not permitted", err: errors.New("exit status 128")},
	}
	diff, err := NewDiffWithExecutor(DiffContext{Base: "main", Head: "feature", Dir: "."}, executor, WithFetchOrphanedRefs())
	if err != nil {
		t.Fatalf("failed to create initial diff: %v", err)
	}

	_, err = diff.ChangesSince("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "upload-pack not permitted") {
		t.Errorf("expected git's own reason to reach the caller, got %v", err)
	}
}

func TestChangesSinceFetchOrphanedRef(t *testing.T) {
	const olderDiff = `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -5,0 +6 @@ func Example() {
+       fmt.Println("Old change")`

	diffFailure := errors.New("fatal: bad object deadbeef")
	retryFailure := errors.New("fatal: ambiguous argument 'deadbeefdeadbeefdeadbeefdeadbeefdeadbeef'")
	fetchFailure := errors.New("fatal: could not read from remote")

	tt := []struct {
		name               string
		fetchOrphanedRefs  bool
		refResolves        bool
		ref                string
		olderDiffResults   []mockResult
		fetchResult        mockResult
		expectedErr        error // the error the caller must see as the cause
		alsoInErr          error // secondary error which must stay visible
		expectedFetchCalls int
		expectedFiles      int
	}{
		{
			name:               "older diff succeeds, no fetch attempted",
			fetchOrphanedRefs:  true,
			olderDiffResults:   []mockResult{{output: olderDiff}},
			expectedFetchCalls: 0,
			expectedFiles:      2,
		},
		{
			name:               "fetch recovers the orphaned ref",
			fetchOrphanedRefs:  true,
			olderDiffResults:   []mockResult{{err: diffFailure}, {output: olderDiff}},
			expectedFetchCalls: 1,
			expectedFiles:      2,
		},
		{
			name:               "fetch fails",
			fetchOrphanedRefs:  true,
			olderDiffResults:   []mockResult{{err: diffFailure}},
			fetchResult:        mockResult{err: fetchFailure},
			expectedErr:        diffFailure,
			alsoInErr:          fetchFailure,
			expectedFetchCalls: 1,
		},
		{
			name:               "retry after fetch still fails",
			fetchOrphanedRefs:  true,
			olderDiffResults:   []mockResult{{err: diffFailure}, {err: retryFailure}},
			expectedErr:        diffFailure,
			alsoInErr:          retryFailure,
			expectedFetchCalls: 1,
		},
		{
			name:               "disabled, no fetch attempted",
			fetchOrphanedRefs:  false,
			olderDiffResults:   []mockResult{{err: diffFailure}},
			expectedErr:        diffFailure,
			expectedFetchCalls: 0,
		},
		{
			name:               "ref is not an object name, so nothing is fetched",
			fetchOrphanedRefs:  true,
			ref:                "refs/heads/main:refs/heads/injected",
			olderDiffResults:   []mockResult{{err: diffFailure}},
			expectedErr:        diffFailure,
			expectedFetchCalls: 0,
		},
		{
			name:               "empty ref is never fetched",
			fetchOrphanedRefs:  true,
			ref:                "",
			olderDiffResults:   []mockResult{{err: diffFailure}},
			expectedErr:        diffFailure,
			expectedFetchCalls: 0,
		},
		{
			name:               "ref resolves locally, so the diff failed for another reason",
			fetchOrphanedRefs:  true,
			refResolves:        true,
			olderDiffResults:   []mockResult{{err: diffFailure}},
			expectedErr:        diffFailure,
			expectedFetchCalls: 0,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			executor := &scriptedGitExecutor{
				diffResults: append([]mockResult{{output: sampleGitDiff}}, tc.olderDiffResults...),
				fetchResult: tc.fetchResult,
				refResolves: tc.refResolves,
			}

			var opts []DiffOption
			if tc.fetchOrphanedRefs {
				opts = append(opts, WithFetchOrphanedRefs())
			}
			context := DiffContext{Base: "main", Head: "feature", Dir: "."}
			diff, err := NewDiffWithExecutor(context, executor, opts...)
			if err != nil {
				t.Fatalf("failed to create initial diff: %v", err)
			}

			ref := tc.ref
			if ref == "" && tc.name != "empty ref is never fetched" {
				ref = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
			}
			changes, err := diff.ChangesSince(ref)

			fetches := executor.fetchCalls()
			if len(fetches) != tc.expectedFetchCalls {
				t.Errorf("expected %d fetch calls, got %d", tc.expectedFetchCalls, len(fetches))
			}
			for _, fetch := range fetches {
				want := []string{"git", "fetch", "--no-tags", "origin", "--", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"}
				if !slices.Equal(fetch, want) {
					t.Errorf("expected fetch command %v, got %v", want, fetch)
				}
			}

			if tc.expectedErr != nil {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				wantPrefix := "failed to get older diff: diff Error: " + tc.expectedErr.Error()
				if !strings.HasPrefix(err.Error(), wantPrefix) {
					t.Errorf("expected error to start with %q, got %v", wantPrefix, err)
				}
				if tc.alsoInErr != nil && !strings.Contains(err.Error(), tc.alsoInErr.Error()) {
					t.Errorf("expected %q to stay visible, got %v", tc.alsoInErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(changes) != tc.expectedFiles {
				t.Errorf("expected %d files, got %d", tc.expectedFiles, len(changes))
			}
		})
	}
}

func TestChangesSinceFetchIsBounded(t *testing.T) {
	const olderDiff = `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -5,0 +6 @@ func Example() {
+       fmt.Println("Old change")`

	executor := &timeoutScriptedExecutor{
		scriptedGitExecutor: scriptedGitExecutor{
			diffResults: []mockResult{
				{output: sampleGitDiff},
				{err: errors.New("fatal: bad object deadbeef")},
				{output: olderDiff},
			},
		},
	}

	context := DiffContext{Base: "main", Head: "feature", Dir: "."}
	diff, err := NewDiffWithExecutor(context, executor, WithFetchOrphanedRefs())
	if err != nil {
		t.Fatalf("failed to create initial diff: %v", err)
	}
	if _, err := diff.ChangesSince("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executor.timeouts) != 1 {
		t.Fatalf("expected 1 bounded command, got %d", len(executor.timeouts))
	}
	if executor.timeouts[0] != fetchTimeout {
		t.Errorf("expected the fetch to be bounded by %s, got %s", fetchTimeout, executor.timeouts[0])
	}
}

func TestExecuteWithTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	executor := newRealGitExecutor(".")

	if _, err := executor.executeWithTimeout(time.Minute, "sleep", "0"); err != nil {
		t.Errorf("unexpected error for a command within the timeout: %v", err)
	}

	_, err := executor.executeWithTimeout(10*time.Millisecond, "sleep", "30")
	if err == nil {
		t.Fatal("expected an error when the command outlives the timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout error, got %v", err)
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
		{
			name: "deleted file",
			fileDiffs: []*diff.FileDiff{
				{
					OrigName: "a/deleted.go",
					NewName:  "/dev/null",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 0,
							NewLines:     0,
						},
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "deleted.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 0,
							End:   -1,
						},
					},
				},
			},
		},
		{
			name: "binary file (no --- / +++ headers, filename in extended)",
			fileDiffs: []*diff.FileDiff{
				{
					Extended: []string{
						"diff --git a/assets/img/offline.png b/assets/img/offline.png",
						"index abc123..def456 100644",
						"Binary files a/assets/img/offline.png and b/assets/img/offline.png differ",
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "assets/img/offline.png",
					Hunks:    []codeowners.HunkRange{},
				},
			},
		},
		{
			name: "binary file with quoted path",
			fileDiffs: []*diff.FileDiff{
				{
					Extended: []string{
						`diff --git "a/assets/some image.png" "b/assets/some image.png"`,
						"Binary files differ",
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "assets/some image.png",
					Hunks:    []codeowners.HunkRange{},
				},
			},
		},
		{
			name: "binary rename uses new path",
			fileDiffs: []*diff.FileDiff{
				{
					Extended: []string{
						"diff --git a/old/foo.png b/new/foo.png",
						"similarity index 100%",
						"rename from old/foo.png",
						"rename to new/foo.png",
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "new/foo.png",
					Hunks:    []codeowners.HunkRange{},
				},
			},
		},
		{
			name: "mixed: added, modified, and deleted files",
			fileDiffs: []*diff.FileDiff{
				{
					NewName: "b/added.go",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 1,
							NewLines:     5,
						},
					},
				},
				{
					NewName: "b/modified.go",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 10,
							NewLines:     2,
						},
					},
				},
				{
					OrigName: "a/deleted.go",
					NewName:  "/dev/null",
					Hunks: []*diff.Hunk{
						{
							NewStartLine: 0,
							NewLines:     0,
						},
					},
				},
			},
			expected: []codeowners.DiffFile{
				{
					FileName: "added.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 1,
							End:   5,
						},
					},
				},
				{
					FileName: "modified.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 10,
							End:   11,
						},
					},
				},
				{
					FileName: "deleted.go",
					Hunks: []codeowners.HunkRange{
						{
							Start: 0,
							End:   -1,
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
