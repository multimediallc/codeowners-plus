package git

import (
	"fmt"
	"strings"
	"testing"
)

type mockFileReaderExecutor struct {
	outputs map[string][]byte
	errors  map[string]error
}

func (m *mockFileReaderExecutor) execute(command string, args ...string) ([]byte, error) {
	key := fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	if err, ok := m.errors[key]; ok {
		return nil, err
	}
	if output, ok := m.outputs[key]; ok {
		return output, nil
	}
	return nil, fmt.Errorf("unexpected command: %s", key)
}

func TestGitRefFileReader_ReadFile(t *testing.T) {
	mockExec := &mockFileReaderExecutor{
		outputs: map[string][]byte{
			"git show baseref123:.codeowners":        []byte("* @owner1\n"),
			"git show baseref123:subdir/.codeowners": []byte("*.js @owner2\n"),
		},
		errors: map[string]error{
			"git show baseref123:nonexistent": fmt.Errorf("file not found"),
		},
	}

	reader := &GitRefFileReader{
		ref:      "baseref123",
		dir:      "/repo",
		executor: mockExec,
	}

	tt := []struct {
		name        string
		path        string
		expected    string
		expectError bool
	}{
		{
			name:        "read root codeowners",
			path:        ".codeowners",
			expected:    "* @owner1\n",
			expectError: false,
		},
		{
			name:        "read subdirectory codeowners",
			path:        "subdir/.codeowners",
			expected:    "*.js @owner2\n",
			expectError: false,
		},
		{
			name:        "read nonexistent file",
			path:        "nonexistent",
			expectError: true,
		},
		{
			name:        "read with leading slash",
			path:        "/.codeowners",
			expected:    "* @owner1\n",
			expectError: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			content, err := reader.ReadFile(tc.path)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if string(content) != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, string(content))
			}
		})
	}
}

func TestGitRefFileReader_PathExists(t *testing.T) {
	mockExec := &mockFileReaderExecutor{
		outputs: map[string][]byte{
			"git cat-file -e baseref123:.codeowners": []byte(""),
		},
		errors: map[string]error{
			"git cat-file -e baseref123:nonexistent": fmt.Errorf("file not found"),
		},
	}

	reader := &GitRefFileReader{
		ref:      "baseref123",
		dir:      "/repo",
		executor: mockExec,
	}

	tt := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "existing file",
			path:     ".codeowners",
			expected: true,
		},
		{
			name:     "nonexistent file",
			path:     "nonexistent",
			expected: false,
		},
		{
			name:     "existing file with leading slash",
			path:     "/.codeowners",
			expected: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			exists := reader.PathExists(tc.path)
			if exists != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, exists)
			}
		})
	}
}
