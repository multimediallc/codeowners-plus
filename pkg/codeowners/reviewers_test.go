package codeowners

import (
	"sort"
	"testing"

	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

func TestToReviewers(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	existing := rgMan.ToReviewerGroup("@a")
	tt := []struct {
		name          string
		input         []string
		expected      []string
		checkExisting bool
	}{
		{
			name:          "empty input",
			input:         []string{},
			expected:      []string{},
			checkExisting: false,
		},
		{
			name:          "single input",
			input:         []string{"@a"},
			expected:      []string{"@a"},
			checkExisting: true,
		},
		{
			name:          "multiple inputs",
			input:         []string{"@a", "@b"},
			expected:      []string{"@a", "@b"},
			checkExisting: false,
		},
		{
			name:          "maintain order",
			input:         []string{"@b", "@a"},
			expected:      []string{"@b", "@a"},
			checkExisting: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r := rgMan.ToReviewerGroup(tc.input...)
			if r == nil {
				t.Error("ToReviewers should never return nil")
				return
			}
			if r.Approved {
				t.Error("ToReviewers should always initialize not Approved")
			}
			if !f.SlicesItemsMatch(r.Names, tc.expected) {
				t.Error("Expected reviewer names do not match")
			}
			if tc.checkExisting && r != existing {
				t.Error("ToReviewers should memoize reviewers")
			}
		})
	}
}

func TestReviewerTestCasesSort(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	ownerTests := FileTestCases{
		&reviewerTest{Match: "*a", Reviewer: rgMan.ToReviewerGroup("@a")},
		&reviewerTest{Match: "b", Reviewer: rgMan.ToReviewerGroup("@b")},
		&reviewerTest{Match: "**/b", Reviewer: rgMan.ToReviewerGroup("@b")},
		&reviewerTest{Match: "c*", Reviewer: rgMan.ToReviewerGroup("@c")},
		&reviewerTest{Match: "*d*", Reviewer: rgMan.ToReviewerGroup("@c")},
		&reviewerTest{Match: "e", Reviewer: rgMan.ToReviewerGroup("@c")},
		&reviewerTest{Match: "f*j", Reviewer: rgMan.ToReviewerGroup("@c")},
	}

	sort.Sort(ownerTests)

	// No wildcard should come first
	expectedOrder := []string{"b", "e", "*a", "c*", "*d*", "f*j", "**/b"}

	for i, test := range ownerTests {
		if test.Match != expectedOrder[i] {
			t.Errorf("Case %d: Expected match %s, got %s", i, expectedOrder[i], test.Match)
		}
	}
}

func TestFileOwners(t *testing.T) {
	fo := newFileOwners()
	if fo == nil {
		t.Error("NewFileOwners should return a non-nil fileOwners")
		return
	}

	rgMan := NewReviewerGroupMemo()
	fo.requiredReviewers = append(fo.requiredReviewers, rgMan.ToReviewerGroup("@a", "@b"), rgMan.ToReviewerGroup("@c"))
	fo.optionalReviewers = append(fo.optionalReviewers, rgMan.ToReviewerGroup("@d"))

	if !f.SlicesItemsMatch(fo.RequiredNames(), []string{"@a", "@b", "@c"}) {
		t.Error("RequiredNames should return all required reviewers")
	}

	if !f.SlicesItemsMatch(fo.OptionalNames(), []string{"@d"}) {
		t.Error("OptionalNames should return the names of optional reviewers")
	}

	fo.requiredReviewers[0].Approved = true
	if !f.SlicesItemsMatch(fo.RequiredNames(), []string{"@c"}) {
		t.Error("RequiredNames should exclude reviewers who have already approved")
	}
	fo.optionalReviewers[0].Approved = true
	if !f.SlicesItemsMatch(fo.OptionalNames(), []string{"@d"}) {
		t.Error("OptionalNames should not worry about reviewers who have already approved")
	}
}

func TestToCommentString(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	rg := rgMan.ToReviewerGroup("@a", "@b", "@c")
	if rg.ToCommentString() != "@a or @b or @c" {
		t.Error("ToCommentString should return a string of reviewers")
	}

	rgs := ReviewerGroups{rgMan.ToReviewerGroup("@a"), rgMan.ToReviewerGroup("@b")}
	if rgs.ToCommentString(true) != "- [ ] @a\n- [ ] @b" {
		t.Error("ToCommentString should match expected format")
	}
	if rgs.ToCommentString(false) != "- @a\n- @b" {
		t.Error("ToCommentString should match expected format")
	}
	// Test sorting is working in ToCommentString
	rgs = ReviewerGroups{rgMan.ToReviewerGroup("@b"), rgMan.ToReviewerGroup("@a")}
	if rgs.ToCommentString(true) != "- [ ] @a\n- [ ] @b" {
		t.Error("ToCommentString should use sorted reviewers")
	}
}

func TestReviewerGroupsFlatten(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	rgs := ReviewerGroups{rgMan.ToReviewerGroup("@a", "@c"), rgMan.ToReviewerGroup("@b"), rgMan.ToReviewerGroup("@b", "@c")}
	if !f.SlicesItemsMatch(rgs.Flatten(), []string{"@a", "@b", "@c"}) {
		t.Error("Flatten should return a list of sorted reviewer names with duplicates removed")
	}
}

func TestReviewerGroupsFilter(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	rgs := ReviewerGroups{rgMan.ToReviewerGroup("@a", "@c"), rgMan.ToReviewerGroup("@b")}
	rgs = rgs.FilterOut("@a")
	// Filtering "@a" should remove the whole ReviewerGroup from the list
	if !f.SlicesItemsMatch(rgs.Flatten(), []string{"@b"}) {
		t.Error("Filter should remove ReviewerGroup[s] with names in the filter list")
	}
	rgMan = NewReviewerGroupMemo()
	rgs = ReviewerGroups{rgMan.ToReviewerGroup("@a", "@c"), rgMan.ToReviewerGroup("@b"), rgMan.ToReviewerGroup("@c", "@d")}
	rgs = rgs.FilterOut("@a", "@b")

	// Filtering "@a" should remove the whole ReviewerGroup from the list
	if !f.SlicesItemsMatch(rgs.Flatten(), []string{"@c", "@d"}) {
		t.Error("Filter should work with multiple names")
	}
}
