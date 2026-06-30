package codeowners

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

// inMemoryReader is an in-memory FileReader that serves a single .codeowners file.
type inMemoryReader struct {
	content []byte
}

func (r *inMemoryReader) ReadFile(string) ([]byte, error) {
	return r.content, nil
}

func (r *inMemoryReader) PathExists(string) bool {
	return true
}

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
			name:       "non-directory file",
			path:       "../../test_project/a.py",
			fallback:   false,
			owners:     0,
			additional: 0,
			optional:   0,
		},
		{
			name:       "empty directory",
			path:       "../../test_project/empty",
			fallback:   false,
			owners:     0,
			additional: 0,
			optional:   0,
		},
	}

	rgMan := NewReviewerGroupMemo()
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			rules := Read(tc.path, rgMan, nil, io.Discard)

			if !tc.fallback && rules.Fallback != nil {
				t.Errorf("Expected fallback to be nil, got %+v", rules.Fallback)
			}

			if tc.fallback && rules.Fallback.Names[0].Original() != tc.fallbackName {
				t.Errorf("Expected fallback to be %s, got %s", tc.fallbackName, rules.Fallback.Names[0].Original())
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

// TestReadLastMatchWinsSamePriority verifies that within a single priority tier,
// the last-declared rule wins, per the documented "last declared wins" semantics.
// Since ownerTestRecursive returns the first match, same-tier rules must appear in
// reverse declaration order (last-declared first). In a large file the sort must be
// stable to preserve that order for equal-priority patterns — an unstable sort
// scrambles them.
func TestReadLastMatchWinsSamePriority(t *testing.T) {
	rgMan := NewReviewerGroupMemo()

	const globstarRules = 200

	var b strings.Builder
	// Many globstar rules (service0/**, service1/**, ...) interleaved with rules from
	// the other two priority tiers. The mix forces the sort to move elements, which
	// surfaces reordering of the equal-priority globstar rules under an unstable sort.
	for i := 0; i < globstarRules; i++ {
		fmt.Fprintf(&b, "service%d/** @team%d\n", i, i)
		fmt.Fprintf(&b, "file%d.go @file%d\n", i, i) // no-wildcard tier
		fmt.Fprintf(&b, "lib%d/*.go @lib%d\n", i, i) // wildcard tier
	}

	reader := &inMemoryReader{content: []byte(b.String())}
	rules := Read("any/dir", rgMan, reader, io.Discard)

	// Extract the globstar rules in result order. They must be in reverse declaration
	// order (highest index first): for any two same-tier rules, the later-declared one
	// must precede the earlier one so that first-match-wins picks the last declaration.
	prev := globstarRules
	for _, test := range rules.OwnerTests {
		var idx int
		if n, _ := fmt.Sscanf(test.Match, "service%d/**", &idx); n != 1 {
			continue
		}
		if idx >= prev {
			t.Errorf("globstar rules out of last-declared-wins order: service%d/** appears after service%d/** (expected strictly decreasing)", idx, prev)
			break
		}
		prev = idx
	}
}
