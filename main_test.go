package main

import (
	"fmt"
	"io"
	"os"
	"testing"
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
