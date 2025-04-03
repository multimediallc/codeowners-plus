package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		os.RemoveAll(tmpDir)
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
		"tests/test.js": "// Test file",
		"tests/test.go": "// Test file",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("Failed to create directory %s: %v", filepath.Dir(fullPath), err)
		}
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

func TestUnownedFiles(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tt := []struct {
		name     string
		target   string
		depth    int
		dirsOnly bool
		want     []string
	}{
		{
			name:   "all unowned files",
			target: "",
			depth:  0,
			want:   []string{"unowned/file.txt", "unowned/inner/file2.txt", "unowned2/file3.txt"},
		},
		{
			name:     "unowned directories",
			target:   "",
			depth:    0,
			dirsOnly: true,
			want:     []string{"unowned", "unowned/inner", "unowned2"},
		},
		{
			name:   "specific directory",
			target: "unowned2",
			depth:  0,
			want:   []string{"unowned2/file3.txt"},
		},
		{
			name:   "with depth limit",
			target: "",
			depth:  1,
			want:   []string{"unowned/file.txt", "unowned2/file3.txt"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := unownedFiles(testRepo, tc.target, tc.depth, tc.dirsOnly)
			if err != nil {
				t.Errorf("unownedFiles() error = %v", err)
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
				t.Errorf("unownedFiles() got %d files, want %d", len(got), len(tc.want))
				return
			}

			if !f.SlicesItemsMatch(got, tc.want) {
				t.Errorf("unownedFiles() got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFileOwner(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tt := []struct {
		name      string
		target    string
		wantErr   bool
		wantOwner []string
	}{
		{
			name:      "go file",
			target:    "main.go",
			wantErr:   false,
			wantOwner: []string{"@backend-team"},
		},
		{
			name:      "internal file",
			target:    "internal/util.go",
			wantErr:   false,
			wantOwner: []string{"@backend-team", "@security-team"},
		},
		{
			name:      "frontend file",
			target:    "frontend/app.js",
			wantErr:   false,
			wantOwner: []string{"@frontend-team"},
		},
		{
			name:      "test file",
			target:    "tests/test.js",
			wantErr:   false,
			wantOwner: []string{"@frontend-team"},
		},
		{
			name:    "non-existent file",
			target:  "does-not-exist.go",
			wantErr: true,
		},
		{
			name:    "empty target",
			target:  "",
			wantErr: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := fileOwner(testRepo, tc.target)
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
					if line != "" && line != "Optional:" {
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

func TestVerifyCodeowners(t *testing.T) {
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

			err := verifyCodeowners(testRepo, tc.target)
			if (err != nil) != tc.wantErr {
				t.Errorf("verifyCodeowners() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr && tc.errMatch != "" && !strings.Contains(err.Error(), tc.errMatch) {
				t.Errorf("verifyCodeowners() error = %v, want to contain %v", err, tc.errMatch)
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
