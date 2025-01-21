package owners

import (
	"fmt"
	"sort"
	"testing"
)

func TestToReviewers(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	existing := rgMan.ToReviewerGroup("@a")
	tt := []struct {
		input         []string
		output        []string
		checkExisting bool
		failMessage   string
	}{
		{[]string{}, []string{}, false, "Empty input should return empty output"},
		{[]string{"@a"}, []string{"@a"}, true, "Single input should return single output"},
		{[]string{"@a", "@b"}, []string{"@a", "@b"}, false, "Multiple inputs should return multiple outputs"},
		{[]string{"@b", "@a"}, []string{"@b", "@a"}, false, "Multiple inputs should maintain input order"},
	}

	for i, tc := range tt {
		r := rgMan.ToReviewerGroup(tc.input...)
		if r == nil {
			t.Errorf("Case %d: ToReviewers should never return nil", i)
			continue
		}
		if r.Approved {
			t.Errorf("Case %d: ToReviewers should always initialize not Approved", i)
		}
		if !SlicesItemsMatch(r.Names, tc.output) {
			t.Error(tc.failMessage)
		}
		if tc.checkExisting && r != existing {
			t.Error("ToReviewers should memoize reviewers")
		}
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

	if !SlicesItemsMatch(fo.RequiredNames(), []string{"@a", "@b", "@c"}) {
		t.Error("RequiredNames should return all required reviewers")
	}

	if !SlicesItemsMatch(fo.OptionalNames(), []string{"@d"}) {
		t.Error("OptionalNames should return the names of optional reviewers")
	}

	fo.requiredReviewers[0].Approved = true
	if !SlicesItemsMatch(fo.RequiredNames(), []string{"@c"}) {
		t.Error("RequiredNames should exclude reviewers who have already approved")
	}
	fo.optionalReviewers[0].Approved = true
	if !SlicesItemsMatch(fo.OptionalNames(), []string{"@d"}) {
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
	if rgs.ToCommentString() != fmt.Sprintf("%s- @a\n- @b", commentPrefix) {
		t.Error("ToCommentString should match expected format")
	}
	// Test sorting is working in ToCommentString
	rgs = ReviewerGroups{rgMan.ToReviewerGroup("@b"), rgMan.ToReviewerGroup("@a")}
	if rgs.ToCommentString() != fmt.Sprintf("%s- @a\n- @b", commentPrefix) {
		t.Error("ToCommentString should use sorted reviewers")
	}
}

func TestReviewerGroupsFlatten(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	rgs := ReviewerGroups{rgMan.ToReviewerGroup("@a", "@c"), rgMan.ToReviewerGroup("@b"), rgMan.ToReviewerGroup("@b", "@c")}
	if !SlicesItemsMatch(rgs.Flatten(), []string{"@a", "@b", "@c"}) {
		t.Error("Flatten should return a list of sorted reviewer names with duplicates removed")
	}
}

func TestReviewerGroupsFilter(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	rgs := ReviewerGroups{rgMan.ToReviewerGroup("@a", "@c"), rgMan.ToReviewerGroup("@b")}
	rgs = rgs.FilterOut("@a")
	// Filtering "@a" should remove the whole ReviewerGroup from the list
	if !SlicesItemsMatch(rgs.Flatten(), []string{"@b"}) {
		t.Error("Filter should remove ReviewerGroup[s] with names in the filter list")
	}
	rgMan = NewReviewerGroupMemo()
	rgs = ReviewerGroups{rgMan.ToReviewerGroup("@a", "@c"), rgMan.ToReviewerGroup("@b"), rgMan.ToReviewerGroup("@c", "@d")}
	rgs = rgs.FilterOut("@a", "@b")

	// Filtering "@a" should remove the whole ReviewerGroup from the list
	if !SlicesItemsMatch(rgs.Flatten(), []string{"@c", "@d"}) {
		t.Error("Filter should work with multiple names")
	}
}
