package owners

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadConfig(t *testing.T) {
	tt := []struct {
		name          string
		configContent string
		path          string
		expected      *Config
		expectedErr   bool
	}{
		{
			name: "default config when no file exists",
			path: "nonexistent/",
			expected: &Config{
				MaxReviews:           nil,
				MinReviews:           nil,
				UnskippableReviewers: []string{},
				Ignore:               []string{},
				Enforcement:          &Enforcement{Approval: false, FailCheck: true},
			},
			expectedErr: false,
		},
		{
			name: "valid config with all fields",
			configContent: `
max_reviews = 2
min_reviews = 1
unskippable_reviewers = ["@user1", "@user2"]
ignore = ["ignored/"]
[enforcement]
approval = true
fail_check = false
high_priority_labels = ["high-priority", "urgent"]
detailed_reviewers = true
disable_review_status_comments = true
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:                  intPtr(2),
				MinReviews:                  intPtr(1),
				UnskippableReviewers:        []string{"@user1", "@user2"},
				Ignore:                      []string{"ignored/"},
				Enforcement:                 &Enforcement{Approval: true, FailCheck: false},
				HighPriorityLabels:          []string{"high-priority", "urgent"},
				DetailedReviewers:           true,
				DisableReviewStatusComments: true,
			},
			expectedErr: false,
		},
		{
			name: "partial config with defaults",
			configContent: `
max_reviews = 3
unskippable_reviewers = ["@user1"]
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:           intPtr(3),
				MinReviews:           nil,
				UnskippableReviewers: []string{"@user1"},
				Ignore:               []string{},
				Enforcement:          &Enforcement{Approval: false, FailCheck: true},
				HighPriorityLabels:   []string{},
				DetailedReviewers:    false,
			},
			expectedErr: false,
		},
		{
			name: "config with require_both_branch_reviewers enabled",
			configContent: `
require_both_branch_reviewers = true
max_reviews = 2
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:                 intPtr(2),
				MinReviews:                 nil,
				UnskippableReviewers:       []string{},
				Ignore:                     []string{},
				Enforcement:                &Enforcement{Approval: false, FailCheck: true},
				HighPriorityLabels:         []string{},
				DetailedReviewers:          false,
				DisableSmartDismissal:      false,
				RequireBothBranchReviewers: true,
			},
			expectedErr: false,
		},
		{
			name: "config with suppress_unowned_warning enabled",
			configContent: `
suppress_unowned_warning = true
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:             nil,
				MinReviews:             nil,
				UnskippableReviewers:   []string{},
				Ignore:                 []string{},
				Enforcement:            &Enforcement{Approval: false, FailCheck: true},
				HighPriorityLabels:     []string{},
				SuppressUnownedWarning: true,
			},
			expectedErr: false,
		},
		{
			name: "config with allow_self_approval enabled",
			configContent: `
allow_self_approval = true
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:           nil,
				MinReviews:           nil,
				UnskippableReviewers: []string{},
				Ignore:               []string{},
				Enforcement:          &Enforcement{Approval: false, FailCheck: true},
				HighPriorityLabels:   []string{},
				AllowSelfApproval:    true,
			},
			expectedErr: false,
		},
		{
			name: "invalid toml",
			configContent: `
max_reviews = invalid
`,
			path:        "testdata/",
			expectedErr: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Create test directory
			testDir := t.TempDir()
			configPath := filepath.Join(testDir, tc.path)

			// Create config file if content is provided
			if tc.configContent != "" {
				err := os.MkdirAll(configPath, 0755)
				if err != nil {
					t.Fatalf("failed to create test directory: %v", err)
				}
				err = os.WriteFile(filepath.Join(configPath, "codeowners.toml"), []byte(tc.configContent), 0644)
				if err != nil {
					t.Fatalf("failed to write test config: %v", err)
				}
			}

			// Test with and without trailing slash
			paths := []string{configPath, configPath + "/"}
			for _, path := range paths {
				got, err := ReadConfig(path, nil)
				if tc.expectedErr {
					if err == nil {
						t.Error("expected error but got none")
					}
					continue
				}

				if err != nil {
					t.Errorf("unexpected error: %v", err)
					continue
				}

				if got == nil {
					t.Error("got nil config")
					continue
				}

				// Compare fields
				if tc.expected.MaxReviews != nil {
					if got.MaxReviews == nil {
						t.Error("expected MaxReviews to be set")
					} else if *got.MaxReviews != *tc.expected.MaxReviews {
						t.Errorf("MaxReviews: expected %d, got %d", *tc.expected.MaxReviews, *got.MaxReviews)
					}
				} else if got.MaxReviews != nil {
					t.Errorf("MaxReviews: expected nil, got %d", *got.MaxReviews)
				}

				if tc.expected.MinReviews != nil {
					if got.MinReviews == nil {
						t.Error("expected MinReviews to be set")
					} else if *got.MinReviews != *tc.expected.MinReviews {
						t.Errorf("MinReviews: expected %d, got %d", *tc.expected.MinReviews, *got.MinReviews)
					}
				} else if got.MinReviews != nil {
					t.Errorf("MinReviews: expected nil, got %d", *got.MinReviews)
				}

				if !sliceEqual(got.UnskippableReviewers, tc.expected.UnskippableReviewers) {
					t.Errorf("UnskippableReviewers: expected %v, got %v", tc.expected.UnskippableReviewers, got.UnskippableReviewers)
				}

				if !sliceEqual(got.Ignore, tc.expected.Ignore) {
					t.Errorf("Ignore: expected %v, got %v", tc.expected.Ignore, got.Ignore)
				}

				if got.RequireBothBranchReviewers != tc.expected.RequireBothBranchReviewers {
					t.Errorf("RequireBothBranchReviewers: expected %v, got %v", tc.expected.RequireBothBranchReviewers, got.RequireBothBranchReviewers)
				}

				if got.SuppressUnownedWarning != tc.expected.SuppressUnownedWarning {
					t.Errorf("SuppressUnownedWarning: expected %v, got %v", tc.expected.SuppressUnownedWarning, got.SuppressUnownedWarning)
				}

				if got.AllowSelfApproval != tc.expected.AllowSelfApproval {
					t.Errorf("AllowSelfApproval: expected %v, got %v", tc.expected.AllowSelfApproval, got.AllowSelfApproval)
				}

				if tc.expected.Enforcement != nil {
					if got.Enforcement == nil {
						t.Error("expected Enforcement to be set")
					} else {
						if got.Enforcement.Approval != tc.expected.Enforcement.Approval {
							t.Errorf("Enforcement.Approval: expected %v, got %v", tc.expected.Enforcement.Approval, got.Enforcement.Approval)
						}
						if got.Enforcement.FailCheck != tc.expected.Enforcement.FailCheck {
							t.Errorf("Enforcement.FailCheck: expected %v, got %v", tc.expected.Enforcement.FailCheck, got.Enforcement.FailCheck)
						}
					}
				} else if got.Enforcement != nil {
					t.Error("Enforcement: expected nil, got non-nil")
				}
			}
		})
	}
}

func TestReadConfigFileError(t *testing.T) {
	// Create a directory with no read permissions
	testDir := t.TempDir()
	configPath := filepath.Join(testDir, "test/")
	err := os.MkdirAll(configPath, 0000)
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Try to read config from directory with no permissions
	_, err = ReadConfig(configPath, nil)
	if err == nil {
		t.Error("expected error when reading from directory with no permissions")
	}
}

func TestReadConfigInvalidTomlReturnsDefaults(t *testing.T) {
	testDir := t.TempDir()
	content := `
max_reviews = 9
min_reviews = 9
unskippable_reviewers = ["@someone"]
ignore = ["vendor"]
high_priority_labels = ["urgent"]
detailed_reviewers = true
disable_smart_dismissal = true
require_both_branch_reviewers = true
suppress_unowned_warning = true
allow_self_approval = true
self_approval_via_teams = true
disable_review_status_comments = true
[enforcement]
approval = true
fail_check = false
[admin_bypass]
enabled = true
allowed_users = ["someone"]
trailing = invalid
`
	if err := os.WriteFile(filepath.Join(testDir, "codeowners.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	config, err := ReadConfig(testDir, nil)
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if config == nil {
		t.Fatal("expected a config alongside the error")
	}

	if !reflect.DeepEqual(config, newDefaultConfig()) {
		t.Errorf("expected pristine defaults after a failed parse, got %s", configDiff(config, newDefaultConfig()))
	}
}

func TestReadConfigNeverReturnsNilOnError(t *testing.T) {
	testDir := t.TempDir()
	unreadable := filepath.Join(testDir, "unreadable")
	if err := os.Mkdir(unreadable, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	malformed := t.TempDir()
	if err := os.WriteFile(filepath.Join(malformed, "codeowners.toml"), []byte("trailing = invalid\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, dir := range []string{unreadable, malformed} {
		config, err := ReadConfig(dir, nil)
		if err == nil {
			continue
		}
		if config == nil {
			t.Fatalf("%s: callers warn and then dereference the config, so an error path must still return one", dir)
		}
		if !reflect.DeepEqual(config, newDefaultConfig()) {
			t.Errorf("%s: expected pristine defaults, got %s", dir, configDiff(config, newDefaultConfig()))
		}
	}
}

func configDiff(got, want *Config) string {
	problems := make([]string, 0, 8)
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if got.MaxReviews != nil {
		add("MaxReviews=%d (want nil)", *got.MaxReviews)
	}
	if got.MinReviews != nil {
		add("MinReviews=%d (want nil)", *got.MinReviews)
	}
	if !sliceEqual(got.UnskippableReviewers, want.UnskippableReviewers) {
		add("UnskippableReviewers=%v", got.UnskippableReviewers)
	}
	if !sliceEqual(got.Ignore, want.Ignore) {
		add("Ignore=%v", got.Ignore)
	}
	if !sliceEqual(got.HighPriorityLabels, want.HighPriorityLabels) {
		add("HighPriorityLabels=%v", got.HighPriorityLabels)
	}
	if got.Enforcement == nil {
		add("Enforcement=nil")
	} else if *got.Enforcement != *want.Enforcement {
		add("Enforcement=%+v (want %+v)", *got.Enforcement, *want.Enforcement)
	}
	if got.AdminBypass == nil {
		add("AdminBypass=nil")
	} else {
		if got.AdminBypass.Enabled != want.AdminBypass.Enabled {
			add("AdminBypass.Enabled=%v", got.AdminBypass.Enabled)
		}
		if !sliceEqual(got.AdminBypass.AllowedUsers, want.AdminBypass.AllowedUsers) {
			add("AdminBypass.AllowedUsers=%v", got.AdminBypass.AllowedUsers)
		}
	}
	for _, f := range []struct {
		name string
		got  bool
		want bool
	}{
		{"DetailedReviewers", got.DetailedReviewers, want.DetailedReviewers},
		{"DisableSmartDismissal", got.DisableSmartDismissal, want.DisableSmartDismissal},
		{"RequireBothBranchReviewers", got.RequireBothBranchReviewers, want.RequireBothBranchReviewers},
		{"SuppressUnownedWarning", got.SuppressUnownedWarning, want.SuppressUnownedWarning},
		{"AllowSelfApproval", got.AllowSelfApproval, want.AllowSelfApproval},
		{"SelfApprovalViaTeams", got.SelfApprovalViaTeams, want.SelfApprovalViaTeams},
		{"DisableReviewStatusComments", got.DisableReviewStatusComments, want.DisableReviewStatusComments},
	} {
		if f.got != f.want {
			add("%s=%v (want %v)", f.name, f.got, f.want)
		}
	}
	return strings.Join(problems, "; ")
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
