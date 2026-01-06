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

	if (*tree.fallback).Names[0] != "@base" {
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
		if _, ok := expectedOwners[owner]; ok {
			expectedOwners[owner] = true
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
	owners.SetAuthor("@b-owner")
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
	owners.SetAuthor("@B-OWNER") // .codeowners has @b-owner

	// Verify that @b-owner was removed from reviewers
	allReviewers := owners.AllRequired().Flatten()
	for _, reviewer := range allReviewers {
		if NormalizeUsername(reviewer) == "@b-owner" {
			t.Errorf("Expected @b-owner to be removed, but found %s", reviewer)
		}
	}

	// Verify with lowercase
	owners2, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	owners2.SetAuthor("@b-owner")
	allReviewers2 := owners2.AllRequired().Flatten()

	// Both should have the same result
	if len(allReviewers) != len(allReviewers2) {
		t.Errorf("Case-insensitive SetAuthor produced different results")
	}
}

func TestApplyApprovalsCaseInsensitive(t *testing.T) {
	owners, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	// Apply approvals with different casing
	owners.ApplyApprovals([]string{"@BASE", "@B-OWNER"}) // .codeowners has @base, @b-owner

	// Verify approvals were applied regardless of case
	allRequired := owners.AllRequired()
	for _, rg := range allRequired {
		normalizedNames := NormalizeUsernames(rg.Names)
		for _, name := range normalizedNames {
			if name == "@base" || name == "@b-owner" {
				if !rg.Approved {
					t.Errorf("Expected %v to be approved", rg.Names)
				}
			}
		}
	}

	// Test with lowercase
	owners2, _, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	owners2.ApplyApprovals([]string{"@base", "@b-owner"})

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
				if NormalizeUsername(name) == "@frontend" {
					foundBefore = true
					t.Logf("Found @frontend before approval: %v, approved=%v", rg.Names, rg.Approved)
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
	owners.ApplyApprovals([]string{"@FRONTEND"})

	// Verify it matches @frontend by checking if it was approved
	foundApproved := false
	for _, fileOwner := range owners.fileToOwner {
		for _, rg := range fileOwner.requiredReviewers {
			for _, name := range rg.Names {
				if NormalizeUsername(name) == "@frontend" {
					t.Logf("After approval: %v, approved=%v", rg.Names, rg.Approved)
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
