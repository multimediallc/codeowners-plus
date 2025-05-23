package main

import (
	"os"
	"testing"
)

func TestIsStdinPiped(t *testing.T) {
	if isStdinPiped() {
		t.Error("IsStdinPiped() returned true when stdin is not piped")
	}
}

func TestScanStdin(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single line",
			input:    "test",
			expected: []string{"test"},
		},
		{
			name:     "multiple lines",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "lines with whitespace",
			input:    "  line1  \n\tline2\t\n  line3  ",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "empty lines",
			input:    "line1\n\nline2\n\n\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original stdin and restore it after the test
			oldStdin := os.Stdin
			defer func() { os.Stdin = oldStdin }()

			// Create a pipe and set it as stdin
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			os.Stdin = r

			go func() {
				defer func() {
					_ = w.Close()
				}()
				if _, err := w.Write([]byte(tt.input)); err != nil {
					t.Errorf("Failed to write to pipe: %v", err)
				}
			}()

			got, err := scanStdin()
			if err != nil {
				t.Errorf("ScanStdin() error = %v", err)
				return
			}

			if len(got) != len(tt.expected) {
				t.Errorf("ScanStdin() got %d lines, want %d", len(got), len(tt.expected))
				return
			}

			for i, line := range got {
				if line != tt.expected[i] {
					t.Errorf("ScanStdin() line %d = %q, want %q", i, line, tt.expected[i])
				}
			}
		})
	}
}
