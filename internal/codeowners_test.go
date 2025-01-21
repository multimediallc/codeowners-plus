package owners

import (
	"reflect"
	"testing"
)

func TestInitOwnerTree(t *testing.T) {
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../test_project", "../test_project", rgMan, nil)

	if tree.name != "../test_project" {
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
	diffFiles := []DiffFile{
		{FileName: "a.py", Hunks: []HunkRange{}},
		{FileName: "b.py", Hunks: []HunkRange{}},
		{FileName: "test_a.py", Hunks: []HunkRange{}},
		{FileName: "models.py", Hunks: []HunkRange{}},
		{FileName: "frontend/a.ts", Hunks: []HunkRange{}},
		{FileName: "frontend/b.ts", Hunks: []HunkRange{}},
		{FileName: "frontend/a.test.ts", Hunks: []HunkRange{}},
		{FileName: "frontend/inner/a.js", Hunks: []HunkRange{}},
		{FileName: "frontend/inner/b.ts", Hunks: []HunkRange{}},
		{FileName: "frontend/inner/a.test.js", Hunks: []HunkRange{}},
	}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../test_project", "../test_project", rgMan, nil)
	testMap := tree.BuildFromFiles(diffFiles, rgMan)
	owners, err := testMap.getOwners(Map(diffFiles, func(df DiffFile) string { return df.FileName }))
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}

	if len(owners.fileToOwner) != len(diffFiles) {
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
		return Map(input, deref)
	}

	actual := owners.FileReviewers()
	for file, expected := range expectedRequired {
		if !SlicesItemsMatch(actual[file].Flatten(), expected.Flatten()) {
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
	diffFiles := []DiffFile{
		{FileName: "a.py", Hunks: []HunkRange{}},
	}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../test_project", "../test_project", rgMan, nil)
	testMap := tree.BuildFromFiles(diffFiles, rgMan)
	diffFiles = append(diffFiles, DiffFile{FileName: "non_existant_file", Hunks: []HunkRange{}})
	_, err := testMap.getOwners(Map(diffFiles, func(df DiffFile) string { return df.FileName }))
	if err == nil {
		t.Errorf("Expected error getting owners: %v", err)
	}
}

func TestGetOwnersNoFallback(t *testing.T) {
	diffFiles := []DiffFile{
		{FileName: "a.py", Hunks: []HunkRange{}},
	}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../test_project", "../test_project", rgMan, nil)
	testMap := tree.BuildFromFiles(diffFiles, rgMan)
	testMap["a.py"].fallback = nil
	owners, err := testMap.getOwners(Map(diffFiles, func(df DiffFile) string { return df.FileName }))
	if err != nil {
		t.Errorf("Expected error getting owners: %v", err)
	}
	if len(owners.fileToOwner["a.py"].requiredReviewers) > 0 {
		t.Errorf("Expected no required reviewers, got %v", owners.fileToOwner["a.py"].requiredReviewers)
	}
}

func TestNewCodeOwners(t *testing.T) {
	diffFiles := []DiffFile{
		{FileName: "a.py", Hunks: []HunkRange{}},
		{FileName: "frontend/b.ts", Hunks: []HunkRange{}},
		{FileName: "frontend/inner/a.test.js", Hunks: []HunkRange{}},
	}
	_, err := NewCodeOwners("../test_project", diffFiles)
	if err != nil {
		t.Errorf("NewCodeOwners error: %v", err)
	}
}

func setupOwnersMap() (*ownersMap, map[string]bool, error) {
	diffFiles := []DiffFile{
		{FileName: "frontend/a.ts", Hunks: []HunkRange{}},
		{FileName: "frontend/a.test.ts", Hunks: []HunkRange{}},
		{FileName: "a.py", Hunks: []HunkRange{}},
		{FileName: "b.py", Hunks: []HunkRange{}},
		{FileName: "test_a.py", Hunks: []HunkRange{}},
		{FileName: "models.py", Hunks: []HunkRange{}},
		{FileName: "frontend/inner/a.js", Hunks: []HunkRange{}},
	}
	rgMan := NewReviewerGroupMemo()
	tree := initOwnerTreeNode("../test_project", "../test_project", rgMan, nil)
	testMap := tree.BuildFromFiles(diffFiles, rgMan)
	owners, error := testMap.getOwners(Map(diffFiles, func(df DiffFile) string { return df.FileName }))
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

	allReviewers := RemoveDuplicates(append(owners.AllRequiredReviewers().Flatten(), owners.AllOptionalReviewers().Flatten()...))
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

type fakeDiff struct{}

func (gd *fakeDiff) GetAllChanges() []DiffFile {
	return []DiffFile{}
}

func (gd *fakeDiff) GetChangesSince(ref string) ([]DiffFile, error) {
	if ref == "bad" {
		return nil, &NoPRError{}
	}
	return []DiffFile{}, nil
}

func TestGetApprovalDiffs(t *testing.T) {
	approvals := []*CurrentApproval{
		{Reviewers: []string{"@base", "@core"}, CommitID: "123"},
		{Reviewers: []string{"@b-owner"}, CommitID: "123"},
		{Reviewers: []string{"@frontend"}, CommitID: "bad"},
	}
	diff := &fakeDiff{}

	approvalsWithDiff, badApprovals := getApprovalDiffs(approvals, diff)
	if len(approvalsWithDiff) != 2 {
		t.Errorf("Expected 2 approvals with diff, got %d", len(approvalsWithDiff))
	}
	if len(badApprovals) != 1 {
		if badApprovals[0].CommitID != "bad" {
			t.Errorf("Expected bad approval to have commit ID bad, got %s", badApprovals[0].CommitID)
		}
		t.Errorf("Expected 1 bad approval, got %d", len(badApprovals))
	}
}

func TestApplyAprovalDiffs(t *testing.T) {
	owners, expectedOwners, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	diffWithOwnedByCore := []DiffFile{{FileName: "test_a.py", Hunks: []HunkRange{}}}
	diffWithoutOwnedByBOwner := []DiffFile{{FileName: "a.py", Hunks: []HunkRange{}}}
	nonExistantDiff := []DiffFile{{FileName: "non_existant_file", Hunks: []HunkRange{}}}
	staleApprovals := owners.applyApprovalDiffs([]*ApprovalWithDiff{
		{inner: &CurrentApproval{Reviewers: []string{"@base", "@core"}, CommitID: "123"}, Diff: diffWithOwnedByCore},
		{inner: &CurrentApproval{Reviewers: []string{"@b-owner"}, CommitID: "123"}, Diff: diffWithoutOwnedByBOwner},
		{inner: &CurrentApproval{Reviewers: []string{"@frontend"}, CommitID: "123"}, Diff: nonExistantDiff},
	})

	if len(staleApprovals) != 1 {
		t.Errorf("Expected 1 stale approval, got %d", len(staleApprovals))
	}
	if !SlicesItemsMatch(staleApprovals[0].Reviewers, []string{"@base", "@core"}) {
		t.Errorf("Expected stale approval to be [@base, @core], got %v", staleApprovals[0].Reviewers)
	}

	delete(expectedOwners, "@b-owner") // This should be removed by applyApprovalDiffs

	allReviewers := RemoveDuplicates(append(owners.AllRequiredReviewers().Flatten(), owners.AllOptionalReviewers().Flatten()...))
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

func TestApplyAprovalsNoChanges(t *testing.T) {
	owners, expectedOwners, err := setupOwnersMap()
	if err != nil {
		t.Errorf("Error getting owners: %v", err)
	}
	diffWithNoChanges := []DiffFile{}
	staleApprovals := owners.applyApprovalDiffs([]*ApprovalWithDiff{
		{inner: &CurrentApproval{Reviewers: []string{"@base", "@core"}, CommitID: "123"}, Diff: diffWithNoChanges},
	})

	if len(staleApprovals) != 0 {
		t.Errorf("Expected 0 stale approval, got %d", len(staleApprovals))
	}

	delete(expectedOwners, "@base") // This should be removed by applyApprovalDiffs
	delete(expectedOwners, "@core") // This should be removed by applyApprovalDiffs

	allReviewers := RemoveDuplicates(append(owners.AllRequiredReviewers().Flatten(), owners.AllOptionalReviewers().Flatten()...))
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

	allReviewers := RemoveDuplicates(append(owners.AllRequiredReviewers().Flatten(), owners.AllOptionalReviewers().Flatten()...))
	if len(allReviewers) != len(expectedOwners) {
		t.Errorf("Expected %d owners, got %d", len(expectedOwners), len(allReviewers))
	}

	for _, fileOwners := range owners.fileToOwner {
		testOwners := RemoveDuplicates(append(fileOwners.RequiredNames(), fileOwners.OptionalNames()...))
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
