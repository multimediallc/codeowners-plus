package codeowners

import (
	"io"
	"reflect"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/functional"
)

func TestInitOwnerTree(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, io.Discard)

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
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, io.Discard)
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
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, io.Discard)
	testMap := tree.BuildFromFiles(files, rgMan)
	files = append(files, "non_existant_file")
	_, err := testMap.getOwners(files)
	if err == nil {
		t.Errorf("Expected error getting owners: %v", err)
	}
}

func TestGetOwnersNoFallback(t *testing.T) {
	files := []string{"a.py"}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, io.Discard)
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
	files := []string{
		"a.py",
		"frontend/b.ts",
		"frontend/inner/a.test.js",
	}
	_, err := NewCodeOwners("../../test_project", files, io.Discard)
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
	tree := initOwnerTreeNode("../../test_project", "../../test_project", rgMan, nil, io.Discard)
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
