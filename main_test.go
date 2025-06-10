package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/internal/app"
)

func init() {
	// Initialize test flags with default values
	flags = &Flags{
		Token:   new(string),
		RepoDir: new(string),
		PR:      new(int),
		Repo:    new(string),
		Verbose: new(bool),
	}
	*flags.Token = "test-token"
	*flags.RepoDir = "/test/dir"
	*flags.PR = 123
	*flags.Repo = "owner/repo"
	*flags.Verbose = false
}

func TestGetEnv(t *testing.T) {
	tt := []struct {
		name     string
		key      string
		fallback string
		setEnv   bool
		envValue string
		expected string
	}{
		{
			name:     "environment variable set",
			key:      "TEST_ENV",
			fallback: "fallback",
			setEnv:   true,
			envValue: "test_value",
			expected: "test_value",
		},
		{
			name:     "environment variable not set",
			key:      "TEST_ENV",
			fallback: "fallback",
			setEnv:   false,
			expected: "fallback",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				_ = os.Setenv(tc.key, tc.envValue)
				defer func() {
					_ = os.Unsetenv(tc.key)
				}()
			}

			got := getEnv(tc.key, tc.fallback)
			if got != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestIgnoreError(t *testing.T) {
	tt := []struct {
		name     string
		value    int
		err      error
		expected int
	}{
		{
			name:     "error is nil",
			value:    42,
			err:      nil,
			expected: 42,
		},
		{
			name:     "error is not nil",
			value:    42,
			err:      os.ErrNotExist,
			expected: 42,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := ignoreError(tc.value, tc.err)
			if got != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestInitFlags(t *testing.T) {
	tokenStr := "test-token"
	prInt := 123
	repoStr := "owner/repo"
	emptyStr := ""
	zeroInt := 0
	tt := []struct {
		name        string
		flags       *Flags
		expectError bool
	}{
		{
			name: "all required flags set",
			flags: &Flags{
				Token: &tokenStr,
				PR:    &prInt,
				Repo:  &repoStr,
			},
			expectError: false,
		},
		{
			name: "missing token",
			flags: &Flags{
				Token: &emptyStr,
				PR:    &prInt,
				Repo:  &repoStr,
			},
			expectError: true,
		},
		{
			name: "missing PR",
			flags: &Flags{
				Token: &tokenStr,
				PR:    &zeroInt,
				Repo:  &repoStr,
			},
			expectError: true,
		},
		{
			name: "missing repo",
			flags: &Flags{
				Token: &tokenStr,
				PR:    &prInt,
				Repo:  &emptyStr,
			},
			expectError: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := initFlags(tc.flags)
			if tc.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
		})
	}
}

func TestOuputAndExit(t *testing.T) {
	// Note: This test can't actually verify the exit behavior
	// It only verifies that the buffers are written correctly
	tt := []struct {
		name       string
		shouldFail bool
		format     string
		args       []interface{}
		verbose    bool
		warnings   string
		info       string
	}{
		{
			name:       "with warnings and info",
			shouldFail: true,
			format:     "test %s %d",
			args:       []interface{}{"message", 42},
			verbose:    true,
			warnings:   "warning message\n",
			info:       "info message\n",
		},
		{
			name:       "with warnings only",
			shouldFail: false,
			format:     "test %s %d",
			args:       []interface{}{"message", 42},
			verbose:    false,
			warnings:   "warning message\n",
			info:       "",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffers
			WarningBuffer.Reset()
			InfoBuffer.Reset()

			// Set up test data
			WarningBuffer.WriteString(tc.warnings)
			InfoBuffer.WriteString(tc.info)
			*flags.Verbose = tc.verbose

			outputAndExit(io.Discard, tc.shouldFail, fmt.Sprintf(tc.format, tc.args...))
		})
	}
}

func TestGITHUBOUTPUT(t *testing.T) {
	// Test case 1: GITHUB_OUTPUT is set
	t.Run("GITHUB_OUTPUT set", func(t *testing.T) {
		// Create a temporary file to simulate GITHUB_OUTPUT
		tmpFile, err := os.CreateTemp("", "github_output_test")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer func() {
			_ = os.Remove(tmpFile.Name())
		}()

		// Set the GITHUB_OUTPUT environment variable
		originalGithubOutput := os.Getenv("GITHUB_OUTPUT")
		defer func() {
			if originalGithubOutput != "" {
				_ = os.Setenv("GITHUB_OUTPUT", originalGithubOutput)
			} else {
				_ = os.Unsetenv("GITHUB_OUTPUT")
			}
		}()
		_ = os.Setenv("GITHUB_OUTPUT", tmpFile.Name())

		// Test data
		testOutputData := app.OutputData{
			FileOwners: map[string][]string{
				"file1.go": {"@user1", "@user2"},
				"file2.go": {"@user3"},
			},
			FileOptional: map[string][]string{
				"file1.go": {"@optional1"},
			},
			UnownedFiles:  []string{},
			StillRequired: []string{"@user1"},
			Success:       true,
			Message:       "Codeowners reviews satisfied",
		}

		// Use the actual function to write the output
		err = writeGITHUBOUTPUT(testOutputData)
		if err != nil {
			t.Fatalf("writeGITHUBOUTPUT failed: %v", err)
		}

		// Read the file back and verify the content
		content, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}

		// Verify the content format
		expectedPrefix := "data<<EOF\n"
		expectedSuffix := "\nEOF\n"
		if !strings.HasPrefix(string(content), expectedPrefix) {
			t.Errorf("Expected content to start with %q, got %q", expectedPrefix, string(content))
		}
		if !strings.HasSuffix(string(content), expectedSuffix) {
			t.Errorf("Expected content to end with %q, got %q", expectedSuffix, string(content))
		}

		// Extract the JSON content
		jsonContent := strings.TrimPrefix(string(content), expectedPrefix)
		jsonContent = strings.TrimSuffix(jsonContent, expectedSuffix)

		// Verify the JSON is valid by unmarshaling it
		var unmarshaled app.OutputData
		err = json.Unmarshal([]byte(jsonContent), &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		// Verify the unmarshaled data matches the original
		if unmarshaled.Success != testOutputData.Success {
			t.Errorf("Success mismatch: expected %t, got %t", testOutputData.Success, unmarshaled.Success)
		}
		if unmarshaled.Message != testOutputData.Message {
			t.Errorf("Message mismatch: expected %q, got %q", testOutputData.Message, unmarshaled.Message)
		}
		if len(unmarshaled.FileOwners) != len(testOutputData.FileOwners) {
			t.Errorf("FileOwners length mismatch: expected %d, got %d", len(testOutputData.FileOwners), len(unmarshaled.FileOwners))
		}
		if len(unmarshaled.FileOptional) != len(testOutputData.FileOptional) {
			t.Errorf("FileOptional length mismatch: expected %d, got %d", len(testOutputData.FileOptional), len(unmarshaled.FileOptional))
		}
		if len(unmarshaled.UnownedFiles) != len(testOutputData.UnownedFiles) {
			t.Errorf("UnownedFiles length mismatch: expected %d, got %d", len(testOutputData.UnownedFiles), len(unmarshaled.UnownedFiles))
		}
		if len(unmarshaled.StillRequired) != len(testOutputData.StillRequired) {
			t.Errorf("StillRequired length mismatch: expected %d, got %d", len(testOutputData.StillRequired), len(unmarshaled.StillRequired))
		}
	})

	// Test case 2: GITHUB_OUTPUT is not set
	t.Run("GITHUB_OUTPUT not set", func(t *testing.T) {
		// Ensure GITHUB_OUTPUT is not set
		originalGithubOutput := os.Getenv("GITHUB_OUTPUT")
		defer func() {
			if originalGithubOutput != "" {
				_ = os.Setenv("GITHUB_OUTPUT", originalGithubOutput)
			} else {
				_ = os.Unsetenv("GITHUB_OUTPUT")
			}
		}()
		_ = os.Unsetenv("GITHUB_OUTPUT")

		// Test data
		testOutputData := app.OutputData{
			FileOwners: map[string][]string{
				"file1.go": {"@user1"},
			},
			FileOptional:  map[string][]string{},
			UnownedFiles:  []string{},
			StillRequired: []string{},
			Success:       true,
			Message:       "Test message",
		}

		// Use the actual function to write the output
		err := writeGITHUBOUTPUT(testOutputData)
		if err != nil {
			t.Fatalf("writeGITHUBOUTPUT should not return error when GITHUB_OUTPUT is not set, got: %v", err)
		}
	})
}
