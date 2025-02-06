package codeowners

import (
	"io"
	"testing"
)

func TestRead(t *testing.T) {
	tt := []struct {
		path         string
		fallback     bool
		fallbackName string
		owners       int
		additional   int
		optional     int
	}{
		{"../../test_project", true, "@base", 4, 2, 1},
		{"../../test_project/frontend/", true, "@frontend", 2, 1, 0},
		{"../../test_project/a.py", false, "", 0, 0, 0},  // test non-directory
		{"../../test_project/empty", false, "", 0, 0, 0}, // test directory with no .codeowners file
	}

	rgMan := NewReviewerGroupMemo()
	for i, tc := range tt {
		rules := Read(tc.path, rgMan, io.Discard)

		if !tc.fallback && rules.Fallback != nil {
			t.Errorf("Case %d: Expected fallback to be nil, got %+v", i, rules.Fallback)
		}

		if tc.fallback && rules.Fallback.Names[0] != tc.fallbackName {
			t.Errorf("Case %d: Expected fallback to be %s, got %s", i, tc.fallbackName, rules.Fallback.Names[0])
		}

		if len(rules.OwnerTests) != tc.owners {
			t.Errorf("Case %d: Expected %d owner tests, got %d", i, tc.owners, len(rules.OwnerTests))
		}

		if len(rules.AdditionalReviewerTests) != tc.additional {
			t.Errorf("Case %d: Expected %d additional tests, got %d", i, tc.additional, len(rules.AdditionalReviewerTests))
		}

		if len(rules.OptionalReviewerTests) != tc.optional {
			t.Errorf("Case %d: Expected %d optional tests, got %d", i, tc.optional, len(rules.OptionalReviewerTests))
		}
	}
}
