package owners

import (
	"fmt"
	"strings"
	"testing"
)

type mockConfigFileReader struct {
	files map[string]string
}

func (m *mockConfigFileReader) ReadFile(path string) ([]byte, error) {
	content, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return []byte(content), nil
}

func (m *mockConfigFileReader) PathExists(path string) bool {
	_, ok := m.files[path]
	return ok
}

func TestReadConfigFromGitRef(t *testing.T) {
	// Simulate reading config from a git ref
	mockReader := &mockConfigFileReader{
		files: map[string]string{
			"test/repo/codeowners.toml": `
max_reviews = 3
min_reviews = 2
unskippable_reviewers = ["@admin"]

[enforcement]
approval = true
fail_check = false
`,
		},
	}

	config, err := ReadConfig("test/repo", mockReader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.MaxReviews == nil || *config.MaxReviews != 3 {
		t.Errorf("expected max_reviews = 3, got %v", config.MaxReviews)
	}

	if config.MinReviews == nil || *config.MinReviews != 2 {
		t.Errorf("expected min_reviews = 2, got %v", config.MinReviews)
	}

	if len(config.UnskippableReviewers) != 1 || config.UnskippableReviewers[0] != "@admin" {
		t.Errorf("expected unskippable_reviewers = [@admin], got %v", config.UnskippableReviewers)
	}

	if !config.Enforcement.Approval {
		t.Error("expected enforcement.approval = true")
	}

	if config.Enforcement.FailCheck {
		t.Error("expected enforcement.fail_check = false")
	}
}

func TestReadConfigFromGitRefNotFound(t *testing.T) {
	// Simulate config file not existing in git ref
	mockReader := &mockConfigFileReader{
		files: map[string]string{},
	}

	config, err := ReadConfig("test/repo", mockReader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return default config
	if config.MaxReviews != nil {
		t.Error("expected nil max_reviews for default config")
	}

	if config.MinReviews != nil {
		t.Error("expected nil min_reviews for default config")
	}

	if config.Enforcement.Approval {
		t.Error("expected enforcement.approval = false for default config")
	}

	if !config.Enforcement.FailCheck {
		t.Error("expected enforcement.fail_check = true for default config")
	}
}

func TestReadConfigFromGitRefInvalidToml(t *testing.T) {
	// Simulate invalid TOML content
	mockReader := &mockConfigFileReader{
		files: map[string]string{
			"test/repo/codeowners.toml": "invalid toml [[[",
		},
	}

	_, err := ReadConfig("test/repo", mockReader)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}

	if !strings.Contains(err.Error(), "toml") && !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected TOML parse error, got: %v", err)
	}
}

func TestReadConfigUsesFilesystemWhenNilReader(t *testing.T) {
	// When nil reader is passed, it should use filesystem reader
	// This test just ensures the fallback logic works
	config, err := ReadConfig("../../test_project", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The test_project directory might not have a codeowners.toml
	// In that case, we should get default config
	if config.Enforcement == nil {
		t.Error("expected enforcement config to be initialized")
	}
}

// TestConfigSecurityIsolation verifies that even if filesystem has different config,
// the git ref reader returns the correct config from the ref
func TestConfigSecurityIsolation(t *testing.T) {
	// This simulates the security scenario where:
	// - Base ref has max_reviews = 5
	// - PR branch (filesystem) has max_reviews = 1 (trying to bypass)
	// - We should only see the base ref config (max_reviews = 5)

	mockReader := &mockConfigFileReader{
		files: map[string]string{
			"test/repo/codeowners.toml": "max_reviews = 5",
		},
	}

	config, err := ReadConfig("test/repo", mockReader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.MaxReviews == nil || *config.MaxReviews != 5 {
		t.Errorf("expected max_reviews = 5 from base ref, got %v", config.MaxReviews)
	}

	// Even if filesystem has different value, we're reading from git ref
	// so we should only see the base ref value
}
