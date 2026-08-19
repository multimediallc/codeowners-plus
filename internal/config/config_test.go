package owners

import (
	"os"
	"path/filepath"
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

func TestApprovalRetention(t *testing.T) {
	type resolved struct {
		whitespace            bool
		comments              bool
		formatting            bool
		stringLiterals        bool
		renames               bool
		fetchOrphanedApproval bool
	}

	tt := []struct {
		name          string
		configContent string
		expected      resolved
	}{
		{
			name:          "section absent",
			configContent: "max_reviews = 2",
			expected:      resolved{},
		},
		{
			name: "umbrella off with nothing else set",
			configContent: `
[approval_retention]
enabled = false
`,
			expected: resolved{},
		},
		{
			name: "umbrella off ignores explicitly enabled flags",
			configContent: `
[approval_retention]
enabled = false
whitespace = true
string_literals = true
renames = true
fetch_orphaned_approval = true
`,
			expected: resolved{},
		},
		{
			name: "master switch on retains nothing on its own",
			configContent: `
[approval_retention]
enabled = true
`,
			expected: resolved{},
		},
		// One flag per case, so an accessor reading the wrong field fails one.
		{
			name: "only whitespace asked for",
			configContent: `
[approval_retention]
enabled = true
whitespace = true
`,
			expected: resolved{whitespace: true},
		},
		{
			name: "only comments asked for",
			configContent: `
[approval_retention]
enabled = true
comments = true
`,
			expected: resolved{comments: true},
		},
		{
			name: "only formatting asked for",
			configContent: `
[approval_retention]
enabled = true
formatting = true
`,
			expected: resolved{formatting: true},
		},
		{
			name: "only string_literals asked for",
			configContent: `
[approval_retention]
enabled = true
string_literals = true
`,
			expected: resolved{stringLiterals: true},
		},
		{
			name: "only renames asked for",
			configContent: `
[approval_retention]
enabled = true
renames = true
`,
			expected: resolved{renames: true},
		},
		{
			name: "only fetch_orphaned_approval asked for",
			configContent: `
[approval_retention]
enabled = true
fetch_orphaned_approval = true
`,
			expected: resolved{fetchOrphanedApproval: true},
		},
		{
			name: "a flag asked for without the master switch stays off",
			configContent: `
[approval_retention]
whitespace = true
comments = true
formatting = true
`,
			expected: resolved{},
		},
		{
			name: "every flag asked for",
			configContent: `
[approval_retention]
enabled = true
whitespace = true
comments = true
formatting = true
string_literals = true
renames = true
fetch_orphaned_approval = true
`,
			expected: resolved{
				whitespace: true, comments: true, formatting: true,
				stringLiterals: true, renames: true, fetchOrphanedApproval: true,
			},
		},
		{
			name: "every flag explicitly false",
			configContent: `
[approval_retention]
enabled = true
whitespace = false
comments = false
formatting = false
string_literals = false
renames = false
fetch_orphaned_approval = false
`,
			expected: resolved{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
			err := os.WriteFile(filepath.Join(testDir, "codeowners.toml"), []byte(tc.configContent), 0644)
			if err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			config, err := ReadConfig(testDir, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if config.ApprovalRetention == nil {
				t.Fatal("expected ApprovalRetention to be set")
			}

			got := resolved{
				whitespace:            config.ApprovalRetention.WhitespaceEnabled(),
				comments:              config.ApprovalRetention.CommentsEnabled(),
				formatting:            config.ApprovalRetention.FormattingEnabled(),
				stringLiterals:        config.ApprovalRetention.StringLiteralsEnabled(),
				renames:               config.ApprovalRetention.RenamesEnabled(),
				fetchOrphanedApproval: config.ApprovalRetention.FetchOrphanedApprovalEnabled(),
			}
			if got != tc.expected {
				t.Errorf("resolved flags: expected %+v, got %+v", tc.expected, got)
			}
		})
	}
}

func TestApprovalRetentionNilSection(t *testing.T) {
	var retention *ApprovalRetention

	if retention.WhitespaceEnabled() || retention.CommentsEnabled() || retention.FormattingEnabled() ||
		retention.StringLiteralsEnabled() || retention.RenamesEnabled() || retention.FetchOrphanedApprovalEnabled() {
		t.Error("expected all flags to be disabled for a nil section")
	}
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
