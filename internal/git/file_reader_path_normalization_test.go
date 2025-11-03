package git

import (
	"testing"
)

// TestGitRefFileReader_PathNormalization tests that absolute paths are correctly
// normalized to be relative to the repository root
func TestGitRefFileReader_PathNormalization(t *testing.T) {
	tt := []struct {
		name     string
		repoDir  string
		input    string
		expected string
	}{
		{
			name:     "relative path unchanged",
			repoDir:  "/repo",
			input:    ".codeowners",
			expected: ".codeowners",
		},
		{
			name:     "absolute path stripped",
			repoDir:  "/repo",
			input:    "/repo/.codeowners",
			expected: ".codeowners",
		},
		{
			name:     "nested path with repo prefix",
			repoDir:  "/repo",
			input:    "/repo/internal/app/.codeowners",
			expected: "internal/app/.codeowners",
		},
		{
			name:     "current directory repo with relative path",
			repoDir:  ".",
			input:    "./.codeowners",
			expected: ".codeowners",
		},
		{
			name:     "github workspace path",
			repoDir:  "/github/workspace",
			input:    "/github/workspace/.codeowners",
			expected: ".codeowners",
		},
		{
			name:     "github workspace nested path",
			repoDir:  "/github/workspace",
			input:    "/github/workspace/internal/app/.codeowners",
			expected: "internal/app/.codeowners",
		},
		{
			name:     "path not under repo dir",
			repoDir:  "/repo",
			input:    "/other/.codeowners",
			expected: "other/.codeowners",
		},
		{
			name:     "repo dir without trailing slash",
			repoDir:  "/repo/",
			input:    "/repo/subdir/.codeowners",
			expected: "subdir/.codeowners",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			reader := &GitRefFileReader{
				ref: "test-ref",
				dir: tc.repoDir,
			}

			result := reader.normalizePathForGit(tc.input)
			if result != tc.expected {
				t.Errorf("normalizePathForGit(%q) with dir=%q = %q, want %q",
					tc.input, tc.repoDir, result, tc.expected)
			}
		})
	}
}
