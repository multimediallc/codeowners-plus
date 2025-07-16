package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

func setupTestRepo(t *testing.T) (string, func()) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "codeowners-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create .git directory
	err = os.Mkdir(filepath.Join(tmpDir, ".git"), 0755)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create .git dir: %v", err)
	}

	// Create test files and directories
	files := map[string]string{
		".codeowners": `
.codeowners @default-owner
**/.codeowners @default-owner

*.go @backend-team

# Additional reviewers
& internal/* @security-team

# Optional reviewers
? **/*.test.js @qa-team
? **/*.test.ts @qa-team`,
		"main.go": "package main",
		"internal/.codeowners": `
* @backend-team`,
		"internal/util.go": "package internal",
		"frontend/.codeowners": `
* @frontend-team`,
		"frontend/app.js":         "// Frontend code",
		"frontend/app.ts":         "// TypeScript code",
		"unowned/file.txt":        "No owner",
		"unowned/inner/file2.txt": "No owner",
		"unowned2/file3.txt":      "No owner",
		"tests/.codeowners": `
*.go @backend-team
*.js @frontend-team`,
		"tests/some.test.js": "// Test file",
		"tests/some.test.go": "// Test file",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			t.Fatalf("Failed to create directory %s: %v", filepath.Dir(fullPath), err)
		}
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

func TestUnownedFiles(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tests := []struct {
		name     string
		targets  []string
		depth    int
		dirsOnly bool
		want     []string
		wantErr  bool
	}{
		{
			name:    "all unowned files",
			targets: []string{""},
			depth:   0,
			want:    []string{"unowned/file.txt", "unowned/inner/file2.txt", "unowned2/file3.txt"},
		},
		{
			name:     "unowned directories",
			targets:  []string{""},
			depth:    0,
			dirsOnly: true,
			want:     []string{"unowned", "unowned/inner", "unowned2"},
		},
		{
			name:    "specific directory",
			targets: []string{"unowned2"},
			depth:   0,
			want:    []string{"unowned2/file3.txt"},
		},
		{
			name:    "with depth limit",
			targets: []string{""},
			depth:   1,
			want:    []string{"unowned/file.txt", "unowned2/file3.txt"},
		},
		{
			name:    "multiple targets",
			targets: []string{"unowned", "unowned2"},
			depth:   1,
			want:    []string{"unowned:", "unowned/file.txt", "unowned/inner/file2.txt", "", "unowned2:", "unowned2/file3.txt"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := unownedFilesWithFormat(testRepo, tc.targets, tc.depth, tc.dirsOnly, FormatDefault)
			if (err != nil) != tc.wantErr {
				t.Errorf("unownedFilesWithFormat() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr {
				return
			}

			// Restore stdout and get output
			if err := w.Close(); err != nil {
				t.Errorf("failed to close pipe writer: %v", err)
				return
			}
			os.Stdout = oldStdout
			out, _ := io.ReadAll(r)
			got := strings.Split(strings.TrimSpace(string(out)), "\n")

			if len(got) != len(tc.want) {
				t.Errorf("unownedFilesWithFormat() got %d files, want %d", len(got), len(tc.want))
				return
			}

			if !f.SlicesItemsMatch(got, tc.want) {
				t.Errorf("unownedFilesWithFormat() got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFileOwner(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tt := []struct {
		name      string
		target    []string
		wantErr   bool
		wantOwner []string
	}{
		{
			name:      "go file",
			target:    []string{"main.go"},
			wantErr:   false,
			wantOwner: []string{"@backend-team"},
		},
		{
			name:      "internal file",
			target:    []string{"internal/util.go"},
			wantErr:   false,
			wantOwner: []string{"@backend-team", "@security-team"},
		},
		{
			name:      "frontend file",
			target:    []string{"frontend/app.js"},
			wantErr:   false,
			wantOwner: []string{"@frontend-team"},
		},
		{
			name:      "test file",
			target:    []string{"tests/some.test.js"},
			wantErr:   false,
			wantOwner: []string{"@frontend-team", "@qa-team (Optional)"},
		},
		{
			name:    "non-existent file",
			target:  []string{"does-not-exist.go"},
			wantErr: true,
		},
		{
			name:    "empty target",
			target:  []string{""},
			wantErr: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := fileOwner(testRepo, tc.target, "default")
			if (err != nil) != tc.wantErr {
				t.Errorf("fileOwner() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr {
				return
			}

			// Restore stdout and get output
			if err := w.Close(); err != nil {
				t.Errorf("failed to close pipe writer: %v", err)
				return
			}
			os.Stdout = oldStdout
			out, _ := io.ReadAll(r)
			got := strings.Split(strings.TrimSpace(string(out)), "\n")

			// Remove "Optional:" line and empty lines
			got = func(lines []string) []string {
				result := make([]string, 0)
				for _, line := range lines {
					if line != "" {
						result = append(result, line)
					}
				}
				return result
			}(got)

			if len(got) != len(tc.wantOwner) {
				t.Errorf("fileOwner() got %d owners, want %d", len(got), len(tc.wantOwner))
				return
			}

			if !f.SlicesItemsMatch(tc.wantOwner, got) {
				t.Errorf("fileOwner() got %+v, want to contain %+v", got, tc.wantOwner)
			}
		})
	}
}

func TestValidateCodeowners(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tt := []struct {
		name     string
		setup    func(string) error
		target   string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid codeowners",
			target:  "",
			wantErr: false,
		},
		{
			name: "invalid owner format",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, ".codeowners"), []byte("* invalid-owner"), 0644)
			},
			target:   "",
			wantErr:  true,
			errMatch: "doesn't start with @",
		},
		{
			name:    "non-existent directory",
			target:  "does-not-exist",
			wantErr: true,
		},
		{
			name: "missing codeowners file",
			setup: func(dir string) error {
				return os.Remove(filepath.Join(dir, ".codeowners"))
			},
			target:  "",
			wantErr: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				err := tc.setup(testRepo)
				if err != nil {
					t.Fatalf("Setup failed: %v", err)
				}
			}

			err := validateCodeowners(testRepo, tc.target)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateCodeowners() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr && tc.errMatch != "" && !strings.Contains(err.Error(), tc.errMatch) {
				t.Errorf("validateCodeowners() error = %v, want to contain %v", err, tc.errMatch)
			}
		})
	}
}

func TestStripRoot(t *testing.T) {
	tt := []struct {
		name string
		root string
		path string
		want string
	}{
		{
			name: "current directory",
			root: ".",
			path: "file.txt",
			want: "file.txt",
		},
		{
			name: "subdirectory",
			root: "/test",
			path: "/test/file.txt",
			want: "file.txt",
		},
		{
			name: "nested subdirectory",
			root: "/test",
			path: "/test/dir/file.txt",
			want: "dir/file.txt",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := stripRoot(tc.root, tc.path)
			if got != tc.want {
				t.Errorf("stripRoot() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDepthCheck(t *testing.T) {
	tt := []struct {
		name   string
		path   string
		target string
		depth  int
		want   bool
	}{
		{
			name:   "within depth",
			path:   "dir/file.txt",
			target: "",
			depth:  1,
			want:   false,
		},
		{
			name:   "exceeds depth",
			path:   "dir1/dir2/dir3/file.txt",
			target: "",
			depth:  1,
			want:   true,
		},
		{
			name:   "with target within depth",
			path:   "target/dir/file.txt",
			target: "target",
			depth:  1,
			want:   false,
		},
		{
			name:   "with target exceeds depth",
			path:   "target/dir1/dir2/file.txt",
			target: "target",
			depth:  1,
			want:   true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := depthCheck(tc.path, tc.target, tc.depth)
			if got != tc.want {
				t.Errorf("depthCheck() = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeCodeOwners struct {
	required map[string]codeowners.ReviewerGroups
	optional map[string]codeowners.ReviewerGroups
}

func (f *fakeCodeOwners) FileRequired() map[string]codeowners.ReviewerGroups {
	return f.required
}
func (f *fakeCodeOwners) FileOptional() map[string]codeowners.ReviewerGroups {
	return f.optional
}
func (f *fakeCodeOwners) SetAuthor(author string)                {}
func (f *fakeCodeOwners) AllRequired() codeowners.ReviewerGroups { return nil }
func (f *fakeCodeOwners) AllOptional() codeowners.ReviewerGroups { return nil }
func (f *fakeCodeOwners) UnownedFiles() []string                 { return nil }
func (f *fakeCodeOwners) ApplyApprovals(approvers []string)      {}

func TestJsonTargets(t *testing.T) {
	owners := &fakeCodeOwners{
		required: map[string]codeowners.ReviewerGroups{
			"file1.txt": {&codeowners.ReviewerGroup{Names: []string{"@alice"}}},
			"file2.txt": {&codeowners.ReviewerGroup{Names: []string{"@bob"}}},
		},
		optional: map[string]codeowners.ReviewerGroups{
			"file1.txt": {&codeowners.ReviewerGroup{Names: []string{"@carol"}}},
			"file2.txt": {},
		},
	}
	targets := []string{"file1.txt", "file2.txt"}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	jsonTargets(targets, owners)

	_ = w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	output := string(out)

	// Unmarshal and check associations
	type targetJSON struct {
		Required []string `json:"required"`
		Optional []string `json:"optional"`
	}
	var parsed map[string]targetJSON
	err := json.Unmarshal([]byte(output), &parsed)
	if err != nil {
		t.Fatalf("jsonTargets output is not valid JSON: %v\noutput: %s", err, output)
	}

	want := map[string]struct {
		required []string
		optional []string
	}{
		"file1.txt": {required: []string{"@alice"}, optional: []string{"@carol"}},
		"file2.txt": {required: []string{"@bob"}, optional: []string{}},
	}

	for file, expect := range want {
		got, ok := parsed[file]
		if !ok {
			t.Errorf("jsonTargets output missing file: %s", file)
			continue
		}
		if !f.SlicesItemsMatch(got.Required, expect.required) {
			t.Errorf("jsonTargets required for %s: got %v, want %v", file, got.Required, expect.required)
		}
		if !f.SlicesItemsMatch(got.Optional, expect.optional) {
			t.Errorf("jsonTargets optional for %s: got %v, want %v", file, got.Optional, expect.optional)
		}
	}
}

func TestPrintTargets(t *testing.T) {
	owners := &fakeCodeOwners{
		required: map[string]codeowners.ReviewerGroups{
			"file1.txt": {&codeowners.ReviewerGroup{Names: []string{"@alice"}}},
			"file2.txt": {&codeowners.ReviewerGroup{Names: []string{"@bob"}}},
		},
		optional: map[string]codeowners.ReviewerGroups{
			"file1.txt": {&codeowners.ReviewerGroup{Names: []string{"@carol"}}},
			"file2.txt": {},
		},
	}
	targets := []string{"file1.txt", "file2.txt"}

	want := map[string]struct {
		required []string
		optional []string
	}{
		"file1.txt": {required: []string{"@alice"}, optional: []string{"@carol (Optional)"}},
		"file2.txt": {required: []string{"@bob"}, optional: []string{}},
	}

	t.Run("default format", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printTargets(targets, owners, false)

		_ = w.Close()
		os.Stdout = oldStdout
		out, _ := io.ReadAll(r)
		output := string(out)

		// Parse output by file
		for file, expect := range want {
			idx := strings.Index(output, file+":")
			if idx == -1 {
				t.Errorf("printTargets output missing file: %s", file)
				continue
			}
			// Get the section for this file
			section := output[idx:]
			if next := strings.Index(section, "\n\n"); next != -1 {
				section = section[:next]
			}
			for _, req := range expect.required {
				if !strings.Contains(section, req) {
					t.Errorf("printTargets output for %s missing required owner: %s", file, req)
				}
			}
			for _, opt := range expect.optional {
				if !strings.Contains(section, opt) {
					t.Errorf("printTargets output for %s missing optional owner: %s", file, opt)
				}
			}
		}
	})

	t.Run("one-line format", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		printTargets(targets, owners, true)

		_ = w.Close()
		os.Stdout = oldStdout
		out, _ := io.ReadAll(r)
		output := string(out)

		// Each file should be on a single line, check association
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			found := false
			for file, expect := range want {
				if strings.HasPrefix(line, file+": ") {
					found = true
					for _, req := range expect.required {
						if !strings.Contains(line, req) {
							t.Errorf("printTargets (one-line) for %s missing required owner: %s", file, req)
						}
					}
					for _, opt := range expect.optional {
						if !strings.Contains(line, opt) {
							t.Errorf("printTargets (one-line) for %s missing optional owner: %s", file, opt)
						}
					}
				}
			}
			if !found {
				t.Errorf("printTargets (one-line) output missing file: %s", line)
			}
		}
		if strings.Contains(output, "\n\n") {
			t.Errorf("printTargets (one-line) should not have double newlines: %s", output)
		}
	})
}

func TestUnownedFilesWithFormat(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tests := []struct {
		name     string
		targets  []string
		depth    int
		dirsOnly bool
		format   OutputFormat
		want     []string
		wantErr  bool
	}{
		{
			name:   "default format",
			format: FormatDefault,
			want:   []string{"unowned/file.txt", "unowned/inner/file2.txt", "unowned2/file3.txt"},
		},
		{
			name:    "one-line format",
			targets: []string{""},
			format:  FormatOneLine,
			want:    []string{"unowned/file.txt, unowned/inner/file2.txt, unowned2/file3.txt"},
		},
		{
			name:   "json format",
			format: FormatJSON,
			want:   []string{"{\".\":[\"unowned/file.txt\",\"unowned/inner/file2.txt\",\"unowned2/file3.txt\"]}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			_ = unownedFilesWithFormat(testRepo, []string{""}, 0, false, tt.format)

			// Restore stdout and get output
			if err := w.Close(); err != nil {
				t.Errorf("failed to close pipe writer: %v", err)
				return
			}
			os.Stdout = oldStdout
			out, _ := io.ReadAll(r)
			got := strings.Split(strings.TrimSpace(string(out)), "\n")

			if len(got) != len(tt.want) {
				t.Errorf("unownedFilesWithFormat() got %d files, want %d", len(got), len(tt.want))
				return
			}

			if !f.SlicesItemsMatch(got, tt.want) {
				t.Errorf("unownedFilesWithFormat() got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGenerateOwnershipMap(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Run("by file", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := generateOwnershipMap(testRepo, "file")
		if err != nil {
			t.Fatalf("generateOwnershipMap() error = %v", err)
		}

		_ = w.Close()
		os.Stdout = oldStdout
		out, _ := io.ReadAll(r)

		var got map[string][]string
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("failed to unmarshal json: %v", err)
		}

		want := map[string][]string{
			".codeowners":          {"@default-owner"},
			"main.go":              {"@backend-team"},
			"internal/.codeowners": {"@default-owner", "@security-team"},
			"internal/util.go":     {"@backend-team", "@security-team"},
			"frontend/.codeowners": {"@default-owner"},
			"frontend/app.js":      {"@frontend-team"},
			"frontend/app.ts":      {"@frontend-team"},
			"tests/.codeowners":    {"@default-owner"},
			"tests/some.test.js":   {"@frontend-team", "@qa-team"},
			"tests/some.test.go":   {"@backend-team"},
		}

		if len(got) != len(want) {
			t.Errorf("map by file: got %d files, want %d", len(got), len(want))
		}

		for file, wantOwners := range want {
			gotOwners, ok := got[file]
			if !ok {
				t.Errorf("map by file: missing file %s in output", file)
				continue
			}
			if !f.SlicesItemsMatch(gotOwners, wantOwners) {
				t.Errorf("map by file: for file %s, got %v, want %v", file, gotOwners, wantOwners)
			}
		}
	})

	t.Run("by owner", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := generateOwnershipMap(testRepo, "owner")
		if err != nil {
			t.Fatalf("generateOwnershipMap() error = %v", err)
		}

		_ = w.Close()
		os.Stdout = oldStdout
		out, _ := io.ReadAll(r)

		var got map[string][]string
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("failed to unmarshal json: %v", err)
		}

		want := map[string][]string{
			"@default-owner": {".codeowners", "frontend/.codeowners", "internal/.codeowners", "tests/.codeowners"},
			"@backend-team":  {"internal/util.go", "main.go", "tests/some.test.go"},
			"@security-team": {"internal/util.go"},
			"@frontend-team": {"frontend/app.js", "frontend/app.ts", "tests/some.test.js"},
			"@qa-team":       {"tests/some.test.js"},
		}

		if len(got) != len(want) {
			t.Errorf("map by owner: got %d owners, want %d", len(got), len(want))
		}

		for owner, wantFiles := range want {
			gotFiles, ok := got[owner]
			if !ok {
				t.Errorf("map by owner: missing owner %s in output", owner)
				continue
			}
			// Use a map for efficient lookup
			gotFilesSet := make(map[string]struct{}, len(gotFiles))
			for _, file := range gotFiles {
				gotFilesSet[file] = struct{}{}
			}

			for _, wantFile := range wantFiles {
				if _, ok := gotFilesSet[wantFile]; !ok {
					t.Errorf("map by owner: for owner %s, missing expected file %s in got %v", owner, wantFile, gotFiles)
				}
			}
		}
	})
}
