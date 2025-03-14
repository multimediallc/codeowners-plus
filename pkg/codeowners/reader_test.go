package codeowners

import (
	"io"
	"testing"
)

func TestRead(t *testing.T) {
	tt := []struct {
		name         string
		path         string
		fallback     bool
		fallbackName string
		owners       int
		additional   int
		optional     int
	}{
		{
			name:         "test project root",
			path:         "../../test_project",
			fallback:     true,
			fallbackName: "@base",
			owners:       4,
			additional:   2,
			optional:     1,
		},
		{
			name:         "frontend directory",
			path:         "../../test_project/frontend/",
			fallback:     true,
			fallbackName: "@frontend",
			owners:       2,
			additional:   1,
			optional:     0,
		},
		{
			name:     "non-directory file",
			path:     "../../test_project/a.py",
			fallback: false,
			owners:   0,
			additional: 0,
			optional:   0,
		},
		{
			name:     "empty directory",
			path:     "../../test_project/empty",
			fallback: false,
			owners:   0,
			additional: 0,
			optional:   0,
		},
	}

	rgMan := NewReviewerGroupMemo()
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			rules := Read(tc.path, rgMan, io.Discard)

			if !tc.fallback && rules.Fallback != nil {
				t.Errorf("Expected fallback to be nil, got %+v", rules.Fallback)
			}

			if tc.fallback && rules.Fallback.Names[0] != tc.fallbackName {
				t.Errorf("Expected fallback to be %s, got %s", tc.fallbackName, rules.Fallback.Names[0])
			}

			if len(rules.OwnerTests) != tc.owners {
				t.Errorf("Expected %d owner tests, got %d", tc.owners, len(rules.OwnerTests))
			}

			if len(rules.AdditionalReviewerTests) != tc.additional {
				t.Errorf("Expected %d additional tests, got %d", tc.additional, len(rules.AdditionalReviewerTests))
			}

			if len(rules.OptionalReviewerTests) != tc.optional {
				t.Errorf("Expected %d optional tests, got %d", tc.optional, len(rules.OptionalReviewerTests))
			}
		})
	}
}
