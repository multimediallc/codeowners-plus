package codeowners

import (
	"io"
	"reflect"
	"testing"

	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

func TestInitOwnerTree(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, nil, io.Discard)

	if tree.name != "../../test_project" {
		t.Errorf("Expected name to be ../test_project, got %s", tree.name)
	}

	if (*tree.fallback).Names[0].Original() != "@base" {
		t.Errorf("Expected fallback to be @base, got %+v", tree.fallback)
	}

	expectedOwnerTests := FileTestCases{
		&reviewerTest{Match: "b.py", Reviewer: rgMan.ToReviewerGroup("@b-owner")},
		&reviewerTest{Match: "test_*", Reviewer: rgMan.ToReviewerGroup("@base-test")},
		&reviewerTest{Match: "backend/**", Reviewer: rgMan.ToReviewerGroup("@backend")},
		&reviewerTest{Match: "**/*test.ts", Reviewer: rgMan.ToReviewerGroup("@base-test")},
	}

	if len(tree.ownerTests) != len(expectedOwnerTests) {
		t.Errorf("Expected %d owner tests, got %d", len(expectedOwnerTests), len(tree.ownerTests))
		return
	}

	for i, test := range tree.ownerTests {
		if !reflect.DeepEqual(test, expectedOwnerTests[i]) {
			t.Errorf("Expected owner test %v, got %v", expectedOwnerTests[i], test)
		}
	}

	expectedAdditionalTests := FileTestCases{
		&reviewerTest{Match: "models*", Reviewer: rgMan.ToReviewerGroup("@devops")},
		&reviewerTest{Match: "**/*test*", Reviewer: rgMan.ToReviewerGroup("@core")},
	}

	for i, test := range tree.additionalReviewerTests {
		if !reflect.DeepEqual(test, expectedAdditionalTests[i]) {
			t.Errorf("Expected additional test %v, got %v", expectedOwnerTests[i], test)
		}
	}
}

func TestGetOwners(t *testing.T) {
	files := []string{
		"a.py",
		"b.py",
		"test_a.py",
		"models.py",
		"frontend/a.ts",
		"frontend/b.ts",
		"frontend/a.test.ts",
		"frontend/inner/a.js",
		"frontend/inner/b.ts",
		"frontend/inner/a.test.js",
	}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, nil, io.Discard)
	testMap := tree.BuildFromFiles(files, rgMan)
	owners, err := testMap.getOwners(files)
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	if len(owners.fileToOwner) != len(files) {
		t.Errorf("Expected 8 owners, got %d", len(owners.fileToOwner))
	}

	expectedRequired := map[string]ReviewerGroups{
		"a.py":                     {rgMan.ToReviewerGroup("@base")},
		"b.py":                     {rgMan.ToReviewerGroup("@b-owner")},
		"test_a.py":                {rgMan.ToReviewerGroup("@base-test"), rgMan.ToReviewerGroup("@core")},
		"models.py":                {rgMan.ToReviewerGroup("@base"), rgMan.ToReviewerGroup("@devops")},
		"frontend/a.ts":            {rgMan.ToReviewerGroup("@frontend")},
		"frontend/b.ts":            {rgMan.ToReviewerGroup("@b-owner")},
		"frontend/a.test.ts":       {rgMan.ToReviewerGroup("@frontend-test"), rgMan.ToReviewerGroup("@frontend-core"), rgMan.ToReviewerGroup("@core")},
		"frontend/inner/a.js":      {rgMan.ToReviewerGroup("@inner-owner")},
		"frontend/inner/b.ts":      {rgMan.ToReviewerGroup("@frontend")},
		"frontend/inner/a.test.js": {rgMan.ToReviewerGroup("@frontend-test"), rgMan.ToReviewerGroup("@frontend-core"), rgMan.ToReviewerGroup("@core")},
	}

	derefMap := func(input ReviewerGroups) []ReviewerGroup {
		deref := func(x *ReviewerGroup) ReviewerGroup { return *x }
		return f.Map(input, deref)
	}

	actual := owners.FileRequired()
	for file, expected := range expectedRequired {
		if !f.SlicesItemsMatch(actual[file].Flatten(), expected.Flatten()) {
			t.Errorf("Expected %s to have %+v additional reviewers, got %+v", file, derefMap(expected), derefMap(actual[file]))
			return
		}
	}

	expectedOptionals := map[string]ReviewerGroups{
		"a.py": {rgMan.ToReviewerGroup("@junior-devs")},
	}

	for file, expected := range expectedOptionals {
		actual := owners.fileToOwner[file].optionalReviewers
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected %s to have %+v additional reviewers, got %+v", file, derefMap(expected), derefMap(actual))
			return
		}
	}
}

func TestGetOwnersFail(t *testing.T) {
	files := []string{"a.py"}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, nil, io.Discard)
	testMap := tree.BuildFromFiles(files, rgMan)
	files = append(files, "non_existent_file")
	_, err := testMap.getOwners(files)
	if err == nil {
		t.Errorf("Expected error getting owners: %v", err)
	}
}

func TestGetOwnersNoFallback(t *testing.T) {
	files := []string{"a.py"}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, nil, io.Discard)
	testMap := tree.BuildFromFiles(files, rgMan)
	testMap["a.py"].fallback = nil
	owners, err := testMap.getOwners(files)
	if err != nil {
		t.Errorf("Expected error getting owners: %v", err)
	}
	if len(owners.fileToOwner["a.py"].requiredReviewers) > 0 {
		t.Errorf("Expected no required reviewers, got %v", owners.fileToOwner["a.py"].requiredReviewers)
	}
}

func TestNewCodeOwners(t *testing.T) {
	files := []DiffFile{
		{FileName: "a.py"},
		{FileName: "frontend/b.ts"},
		{FileName: "frontend/inner/a.test.js"},
	}
	_, err := New("../../test_project", files, nil, io.Discard)
	if err != nil {
		t.Errorf("NewCodeOwners error: %v", err)
	}
}

func setupOwnersMap() (*ownersMap, map[string]bool, error) {
	files := []string{
		"frontend/a.ts",
		"frontend/a.test.ts",
		"a.py",
		"b.py",
		"test_a.py",
		"models.py",
		"frontend/inner/a.js",
	}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, nil, io.Discard)
	testMap := tree.BuildFromFiles(files, rgMan)
	owners, error := testMap.getOwners(files)
	expectedOwners := map[string]bool{
		"@base":          false,
		"@b-owner":       false,
		"@base-test":     false,
		"@core":          false,
		"@devops":        false,
		"@frontend":      false,
		"@frontend-core": false,
		"@frontend-test": false,
		"@inner-owner":   false,
		"@junior-devs":   false,
	}
	return owners, expectedOwners, error
}

func TestAllReviewers(t *testing.T) {
	owners, expectedOwners, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	allReviewers := f.RemoveDuplicates(append(owners.AllRequired().Flatten(), owners.AllOptional().Flatten()...))
	if len(allReviewers) != len(expectedOwners) {
		t.Errorf("Expected %d owners, got %d", len(expectedOwners), len(allReviewers))
	}

	for _, owner := range allReviewers {
		ownerStr := owner.Original()
		if _, ok := expectedOwners[ownerStr]; ok {
			expectedOwners[ownerStr] = true
		} else {
			t.Errorf("Unexpected owner %s", owner)
		}
	}

	for owner, found := range expectedOwners {
		if !found {
			t.Errorf("Expected owner %s not found", owner)
		}
	}
}

func TestSetAuthor(t *testing.T) {
	owners, expectedOwners, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	owners.SetAuthor("@b-owner", AuthorModeDefault)
	delete(expectedOwners, "@b-owner") // This should be removed by SetAuthor

	allReviewers := f.RemoveDuplicates(append(owners.AllRequired().Flatten(), owners.AllOptional().Flatten()...))
	if len(allReviewers) != len(expectedOwners) {
		t.Errorf("Expected %d owners, got %d", len(expectedOwners), len(allReviewers))
	}

	for _, fileOwners := range owners.fileToOwner {
		testOwners := f.RemoveDuplicates(append(fileOwners.RequiredNames(), fileOwners.OptionalNames()...))
		for _, owner := range testOwners {
			if _, ok := expectedOwners[owner]; ok {
				expectedOwners[owner] = true
			} else {
				t.Errorf("Unexpected owner %s", owner)
			}
		}
	}

	for owner, found := range expectedOwners {
		if !found {
			t.Errorf("Expected owner %s not found", owner)
		}
	}
}

func TestSetAuthorCaseInsensitive(t *testing.T) {
	owners, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	// Set author with different casing than what's in the .codeowners file
	owners.SetAuthor("@B-OWNER", AuthorModeDefault) // .codeowners has @b-owner

	// Verify that @b-owner was removed from reviewers
	allReviewers := owners.AllRequired().Flatten()
	for _, reviewer := range allReviewers {
		if reviewer.Normalized() == "@b-owner" {
			t.Errorf("Expected @b-owner to be removed, but found %s", reviewer.Original())
		}
	}

	// Verify with lowercase
	owners2, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	owners2.SetAuthor("@b-owner", AuthorModeDefault)
	allReviewers2 := owners2.AllRequired().Flatten()

	// Both should have the same result
	if len(allReviewers) != len(allReviewers2) {
		t.Errorf("Case-insensitive SetAuthor produced different results")
	}
}

// When the author is in a multi-member OR group (e.g. `*.py @alice @bob`) and self-approval
// is enabled, the author implicitly vouches for their own code — the entire group should be
// satisfied without needing @bob to also approve.
func TestSetAuthorSelfApprovalMultiMemberORGroup(t *testing.T) {
	co := createMockCodeOwners(
		map[string]ReviewerGroups{
			"file.py": {{Names: NewSlugs([]string{"@alice", "@bob"}), Approved: false}},
		},
		map[string]ReviewerGroups{},
		[]string{},
	)
	co.SetAuthor("@alice", AuthorModeSelfApproval)

	allRequired := co.AllRequired()
	if len(allRequired) != 0 {
		t.Errorf("Expected OR group to be auto-satisfied with self-approval, got %d still required", len(allRequired))
	}
}

// Contrast to the self-approval case: with default mode, the author is removed from the OR
// group but the remaining members still need to approve. So `*.py @alice @bob` with @alice as
// author means @bob must still review.
func TestSetAuthorDefaultMultiMemberORGroup(t *testing.T) {
	co := createMockCodeOwners(
		map[string]ReviewerGroups{
			"file.py": {{Names: NewSlugs([]string{"@alice", "@bob"}), Approved: false}},
		},
		map[string]ReviewerGroups{},
		[]string{},
	)
	co.SetAuthor("@alice", AuthorModeDefault)

	allRequired := co.AllRequired()
	if len(allRequired) != 1 {
		t.Fatalf("Expected 1 still-required group, got %d", len(allRequired))
	}
	if len(allRequired[0].Names) != 1 || allRequired[0].Names[0].Normalized() != "@bob" {
		t.Errorf("Expected remaining required reviewer to be @bob, got %v", OriginalStrings(allRequired[0].Names))
	}
}

// When the author appears in multiple OR groups across different files, self-approval should
// satisfy all of them — not just the first one encountered.
func TestSetAuthorSelfApprovalMultipleORGroups(t *testing.T) {
	orGroup1 := &ReviewerGroup{Names: NewSlugs([]string{"@alice", "@bob"}), Approved: false}
	orGroup2 := &ReviewerGroup{Names: NewSlugs([]string{"@alice", "@charlie"}), Approved: false}
	co := createMockCodeOwners(
		map[string]ReviewerGroups{
			"file1.py": {orGroup1},
			"file2.py": {orGroup2},
		},
		map[string]ReviewerGroups{},
		[]string{},
	)
	co.SetAuthor("@alice", AuthorModeSelfApproval)

	allRequired := co.AllRequired()
	if len(allRequired) != 0 {
		t.Errorf("Expected all OR groups containing author to be satisfied, got %d still required", len(allRequired))
	}
}

// Self-approval only applies to OR groups the author belongs to. If a file also has a separate
// AND group (e.g. `&*.py @security`), that group must still be independently satisfied —
// the author being in one OR group doesn't grant them approval power over unrelated AND groups.
func TestSetAuthorSelfApprovalDoesNotAffectOtherANDGroups(t *testing.T) {
	orGroup := &ReviewerGroup{Names: NewSlugs([]string{"@alice", "@bob"}), Approved: false}
	andGroup := &ReviewerGroup{Names: NewSlugs([]string{"@security"}), Approved: false}
	co := createMockCodeOwners(
		map[string]ReviewerGroups{
			"file.py": {orGroup, andGroup},
		},
		map[string]ReviewerGroups{},
		[]string{},
	)
	co.SetAuthor("@alice", AuthorModeSelfApproval)

	allRequired := co.AllRequired()
	if len(allRequired) != 1 {
		t.Fatalf("Expected 1 still-required AND group, got %d", len(allRequired))
	}
	if allRequired[0].Names[0].Normalized() != "@security" {
		t.Errorf("Expected @security to still be required, got %v", OriginalStrings(allRequired[0].Names))
	}
}

// GitHub usernames are case-insensitive. If the PR author is `@ALICE` but the .codeowners
// file lists `@alice`, self-approval should still match and satisfy the group.
func TestSetAuthorSelfApprovalCaseInsensitive(t *testing.T) {
	co := createMockCodeOwners(
		map[string]ReviewerGroups{
			"file.py": {{Names: NewSlugs([]string{"@alice", "@bob"}), Approved: false}},
		},
		map[string]ReviewerGroups{},
		[]string{},
	)
	co.SetAuthor("@ALICE", AuthorModeSelfApproval)

	allRequired := co.AllRequired()
	if len(allRequired) != 0 {
		t.Errorf("Expected case-insensitive self-approval to satisfy the group, got %d still required", len(allRequired))
	}
}

// When self-approval is enabled but the author isn't listed in any ownership group,
// nothing should change — all groups remain required with their original members.
func TestSetAuthorSelfApprovalAuthorNotInAnyGroup(t *testing.T) {
	co := createMockCodeOwners(
		map[string]ReviewerGroups{
			"file.py": {{Names: NewSlugs([]string{"@bob", "@charlie"}), Approved: false}},
		},
		map[string]ReviewerGroups{},
		[]string{},
	)
	co.SetAuthor("@outsider", AuthorModeSelfApproval)

	allRequired := co.AllRequired()
	if len(allRequired) != 1 {
		t.Fatalf("Expected group to remain required when author is not a member, got %d", len(allRequired))
	}
	if len(allRequired[0].Names) != 2 {
		t.Errorf("Expected group members unchanged, got %v", OriginalStrings(allRequired[0].Names))
	}
}

func TestApplyApprovalsCaseInsensitive(t *testing.T) {
	owners, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	// Apply approvals with different casing
	owners.ApplyApprovals(NewSlugs([]string{"@BASE", "@B-OWNER"})) // .codeowners has @base, @b-owner

	// Verify approvals were applied regardless of case
	allRequired := owners.AllRequired()
	for _, rg := range allRequired {
		for _, name := range rg.Names {
			normalized := name.Normalized()
			if normalized == "@base" || normalized == "@b-owner" {
				if !rg.Approved {
					t.Errorf("Expected %v to be approved", OriginalStrings(rg.Names))
				}
			}
		}
	}

	// Test with lowercase
	owners2, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	owners2.ApplyApprovals(NewSlugs([]string{"@base", "@b-owner"}))

	// Both should produce the same approval state
	allRequired2 := owners2.AllRequired()
	approvedCount1 := 0
	approvedCount2 := 0
	for _, rg := range allRequired {
		if rg.Approved {
			approvedCount1++
		}
	}
	for _, rg := range allRequired2 {
		if rg.Approved {
			approvedCount2++
		}
	}

	if approvedCount1 != approvedCount2 {
		t.Errorf("Case-insensitive ApplyApprovals produced different results: %d vs %d", approvedCount1, approvedCount2)
	}
}

func TestNameReviewerMapCaseInsensitive(t *testing.T) {
	owners, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	// First, verify @frontend is in the required reviewers before approval
	foundBefore := false
	for _, fileOwner := range owners.fileToOwner {
		for _, rg := range fileOwner.requiredReviewers {
			for _, name := range rg.Names {
				if name.Normalized() == "@frontend" {
					foundBefore = true
					t.Logf("Found @frontend before approval: %v, approved=%v", OriginalStrings(rg.Names), rg.Approved)
				}
			}
		}
	}

	if !foundBefore {
		t.Errorf("@frontend not found in required reviewers - test setup issue")
		return
	}

	// Check that the nameReviewerMap uses normalized keys
	// Apply an approval with uppercase
	owners.ApplyApprovals(NewSlugs([]string{"@FRONTEND"}))

	// Verify it matches @frontend by checking if it was approved
	foundApproved := false
	for _, fileOwner := range owners.fileToOwner {
		for _, rg := range fileOwner.requiredReviewers {
			for _, name := range rg.Names {
				if name.Normalized() == "@frontend" {
					t.Logf("After approval: %v, approved=%v", OriginalStrings(rg.Names), rg.Approved)
					if rg.Approved {
						foundApproved = true
					}
				}
			}
		}
	}

	if !foundApproved {
		t.Errorf("Expected @frontend to be approved when @FRONTEND was used")
	}
}
