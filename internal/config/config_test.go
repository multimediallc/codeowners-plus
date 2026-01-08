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
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:           intPtr(2),
				MinReviews:           intPtr(1),
				UnskippableReviewers: []string{"@user1", "@user2"},
				Ignore:               []string{"ignored/"},
				Enforcement:          &Enforcement{Approval: true, FailCheck: false},
				HighPriorityLabels:   []string{"high-priority", "urgent"},
				DetailedReviewers:    true,
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
			name: "config with sum_owners enabled",
			configContent: `
sum_owners = true
max_reviews = 2
`,
			path: "testdata/",
			expected: &Config{
				MaxReviews:            intPtr(2),
				MinReviews:            nil,
				UnskippableReviewers:  []string{},
				Ignore:                []string{},
				Enforcement:           &Enforcement{Approval: false, FailCheck: true},
				HighPriorityLabels:    []string{},
				DetailedReviewers:     false,
				DisableSmartDismissal: false,
				SumOwners:             true,
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

				if got.SumOwners != tc.expected.SumOwners {
					t.Errorf("SumOwners: expected %v, got %v", tc.expected.SumOwners, got.SumOwners)
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
