package codeowners

import (
	"strings"
	"testing"
)

func TestMergeCodeOwners(t *testing.T) {
	tt := []struct {
		name             string
		baseRequired     map[string]ReviewerGroups
		headRequired     map[string]ReviewerGroups
		baseOptional     map[string]ReviewerGroups
		headOptional     map[string]ReviewerGroups
		baseUnowned      []string
		headUnowned      []string
		expectedRequired map[string][]string // file -> list of reviewer group strings
		expectedOptional map[string][]string // file -> list of reviewer group strings
		expectedUnowned  []string
	}{
		{
			name: "basic merge with different owners",
			baseRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-a"}), Approved: false},
				},
			},
			headRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-b"}), Approved: false},
				},
			},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{},
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"file.py": {"@team-a", "@team-b"},
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{},
		},
		{
			name: "file only in base (deleted file)",
			baseRequired: map[string]ReviewerGroups{
				"deleted.py": {
					{Names: NewSlugs([]string{"@team-a"}), Approved: false},
				},
			},
			headRequired: map[string]ReviewerGroups{},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{},
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"deleted.py": {"@team-a"},
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{},
		},
		{
			name:         "file only in head (new file)",
			baseRequired: map[string]ReviewerGroups{},
			headRequired: map[string]ReviewerGroups{
				"new.py": {
					{Names: NewSlugs([]string{"@team-b"}), Approved: false},
				},
			},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{},
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"new.py": {"@team-b"},
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{},
		},
		{
			name: "duplicate reviewers across branches",
			baseRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-a"}), Approved: false},
				},
			},
			headRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-a"}), Approved: false},
				},
			},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{},
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"file.py": {"@team-a"}, // Only one @team-a due to deduplication
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{},
		},
		{
			name: "multiple reviewers in each branch",
			baseRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-a"}), Approved: false},
					{Names: NewSlugs([]string{"@security"}), Approved: false},
				},
			},
			headRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-b"}), Approved: false},
					{Names: NewSlugs([]string{"@compliance"}), Approved: false},
				},
			},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{},
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"file.py": {"@team-a", "@security", "@team-b", "@compliance"},
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{},
		},
		{
			name:         "optional reviewers merged",
			baseRequired: map[string]ReviewerGroups{},
			headRequired: map[string]ReviewerGroups{},
			baseOptional: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@junior-dev"}), Approved: false},
				},
			},
			headOptional: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@intern"}), Approved: false},
				},
			},
			baseUnowned:      []string{},
			headUnowned:      []string{},
			expectedRequired: map[string][]string{},
			expectedOptional: map[string][]string{
				"file.py": {"@junior-dev", "@intern"},
			},
			expectedUnowned: []string{},
		},
		{
			name:             "unowned files merged",
			baseRequired:     map[string]ReviewerGroups{},
			headRequired:     map[string]ReviewerGroups{},
			baseOptional:     map[string]ReviewerGroups{},
			headOptional:     map[string]ReviewerGroups{},
			baseUnowned:      []string{"unowned1.txt"},
			headUnowned:      []string{"unowned2.txt"},
			expectedRequired: map[string][]string{},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{"unowned1.txt", "unowned2.txt"},
		},
		{
			name: "unowned file in one branch, owned in another",
			baseRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-a"}), Approved: false},
				},
			},
			headRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@team-b"}), Approved: false},
				},
			},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{"file.py"}, // Base says unowned, but we have explicit rules
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"file.py": {"@team-a", "@team-b"},
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{}, // Should not be in unowned since it has owners
		},
		{
			name: "OR groups within each branch",
			baseRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@user1", "@user2"}), Approved: false}, // OR group
				},
			},
			headRequired: map[string]ReviewerGroups{
				"file.py": {
					{Names: NewSlugs([]string{"@user3", "@user4"}), Approved: false}, // OR group
				},
			},
			baseOptional: map[string]ReviewerGroups{},
			headOptional: map[string]ReviewerGroups{},
			baseUnowned:  []string{},
			headUnowned:  []string{},
			expectedRequired: map[string][]string{
				"file.py": {"@user1 or @user2", "@user3 or @user4"},
			},
			expectedOptional: map[string][]string{},
			expectedUnowned:  []string{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock CodeOwners objects
			base := createMockCodeOwners(tc.baseRequired, tc.baseOptional, tc.baseUnowned)
			head := createMockCodeOwners(tc.headRequired, tc.headOptional, tc.headUnowned)

			// Merge
			merged := MergeCodeOwners(base, head)

			// Verify required reviewers
			mergedRequired := merged.FileRequired()
			if len(tc.expectedRequired) == 0 && len(mergedRequired) > 0 {
				t.Errorf("expected no required reviewers, got %v", mergedRequired)
			}
			for file, expectedGroups := range tc.expectedRequired {
				actualGroups, found := mergedRequired[file]
				if !found {
					t.Errorf("file %s not found in merged required reviewers", file)
					continue
				}
				if !reviewerGroupsMatch(actualGroups, expectedGroups) {
					t.Errorf("file %s: expected reviewers %v, got %v",
						file, expectedGroups, reviewerGroupsToStrings(actualGroups))
				}
			}

			// Verify optional reviewers
			mergedOptional := merged.FileOptional()
			if len(tc.expectedOptional) == 0 && len(mergedOptional) > 0 {
				t.Errorf("expected no optional reviewers, got %v", mergedOptional)
			}
			for file, expectedGroups := range tc.expectedOptional {
				actualGroups, found := mergedOptional[file]
				if !found {
					t.Errorf("file %s not found in merged optional reviewers", file)
					continue
				}
				if !reviewerGroupsMatch(actualGroups, expectedGroups) {
					t.Errorf("file %s: expected optional reviewers %v, got %v",
						file, expectedGroups, reviewerGroupsToStrings(actualGroups))
				}
			}

			// Verify unowned files
			mergedUnowned := merged.UnownedFiles()
			if !stringSlicesEqual(mergedUnowned, tc.expectedUnowned) {
				t.Errorf("unowned files: expected %v, got %v", tc.expectedUnowned, mergedUnowned)
			}
		})
	}
}

func TestMergeCodeOwnersApprovalTracking(t *testing.T) {
	// Test that approval tracking works correctly after merging
	baseRequired := map[string]ReviewerGroups{
		"file.py": {
			{Names: NewSlugs([]string{"@team-a"}), Approved: false},
		},
	}
	headRequired := map[string]ReviewerGroups{
		"file.py": {
			{Names: NewSlugs([]string{"@team-b"}), Approved: false},
		},
	}

	base := createMockCodeOwners(baseRequired, map[string]ReviewerGroups{}, []string{})
	head := createMockCodeOwners(headRequired, map[string]ReviewerGroups{}, []string{})

	merged := MergeCodeOwners(base, head)

	// Initially, both team-a and team-b should be required (unapproved)
	allRequired := merged.AllRequired()
	if len(allRequired) != 2 {
		t.Fatalf("expected 2 required reviewer groups initially, got %d", len(allRequired))
	}

	// Apply approval from team-a
	merged.ApplyApprovals(NewSlugs([]string{"@team-a"}))

	// After team-a approves, only team-b should be in AllRequired (since AllRequired filters approved)
	allRequired = merged.AllRequired()
	if len(allRequired) != 1 {
		t.Logf("AllRequired contents after team-a approval:")
		for i, rg := range allRequired {
			t.Logf("  %d: %v (approved: %v)", i, rg.ToCommentString(), rg.Approved)
		}
		t.Fatalf("expected 1 required reviewer group after team-a approval, got %d", len(allRequired))
	}

	if allRequired[0].ToCommentString() != "@team-b" {
		t.Errorf("expected @team-b to be the remaining required reviewer, got %s", allRequired[0].ToCommentString())
	}

	// Apply approval from team-b
	merged.ApplyApprovals(NewSlugs([]string{"@team-b"}))

	// After both approve, AllRequired should return empty (all approved)
	allRequired = merged.AllRequired()
	if len(allRequired) != 0 {
		t.Errorf("expected 0 required reviewer groups after both approvals, got %d", len(allRequired))
	}
}

func TestMergeCodeOwnersSetAuthor(t *testing.T) {
	// Test that SetAuthor works correctly after merging
	baseRequired := map[string]ReviewerGroups{
		"file.py": {
			{Names: NewSlugs([]string{"@author", "@team-a"}), Approved: false},
		},
	}
	headRequired := map[string]ReviewerGroups{
		"file.py": {
			{Names: NewSlugs([]string{"@author", "@team-b"}), Approved: false},
		},
	}

	base := createMockCodeOwners(baseRequired, map[string]ReviewerGroups{}, []string{})
	head := createMockCodeOwners(headRequired, map[string]ReviewerGroups{}, []string{})

	merged := MergeCodeOwners(base, head)

	// Set author
	merged.SetAuthor("@author", AuthorModeDefault)

	// Check that author is removed from reviewer groups or groups are auto-approved
	allRequired := merged.AllRequired()
	for _, rg := range allRequired {
		for _, name := range rg.Names {
			if name.Original() == "@author" {
				t.Errorf("author should be removed from reviewer groups, but found in %v", rg.Names)
			}
		}
	}
}

// With require_both_branch_reviewers, ownership rules from base and head are AND'd together.
// Self-approval should only satisfy groups the author belongs to. Here the author is in the
// base branch's OR group but not the head branch's group, so only the base group is satisfied
// and the head's @team-b still requires an independent review.
func TestMergeCodeOwnersSetAuthorSelfApproval(t *testing.T) {
	baseRequired := map[string]ReviewerGroups{
		"file.py": {
			{Names: NewSlugs([]string{"@author", "@team-a"}), Approved: false},
		},
	}
	headRequired := map[string]ReviewerGroups{
		"file.py": {
			{Names: NewSlugs([]string{"@team-b"}), Approved: false},
		},
	}

	base := createMockCodeOwners(baseRequired, map[string]ReviewerGroups{}, []string{})
	head := createMockCodeOwners(headRequired, map[string]ReviewerGroups{}, []string{})

	merged := MergeCodeOwners(base, head)
	merged.SetAuthor("@author", AuthorModeSelfApproval)

	allRequired := merged.AllRequired()
	if len(allRequired) != 1 {
		t.Fatalf("Expected 1 still-required group (head's @team-b), got %d", len(allRequired))
	}
	if allRequired[0].Names[0].Normalized() != "@team-b" {
		t.Errorf("Expected @team-b to still be required, got %v", OriginalStrings(allRequired[0].Names))
	}
}

// Helper functions

// createMockCodeOwners creates a mock CodeOwners object for testing
func createMockCodeOwners(required, optional map[string]ReviewerGroups, unowned []string) CodeOwners {
	fileToOwner := make(map[string]fileOwners)
	for file, reviewers := range required {
		fileToOwner[file] = fileOwners{
			requiredReviewers: reviewers,
			optionalReviewers: optional[file],
		}
	}
	for file, reviewers := range optional {
		if _, exists := fileToOwner[file]; !exists {
			fileToOwner[file] = fileOwners{
				requiredReviewers: nil,
				optionalReviewers: reviewers,
			}
		}
	}

	nameReviewerMap := buildNameReviewerMap(fileToOwner)

	return &ownersMap{
		author:          "",
		fileToOwner:     fileToOwner,
		nameReviewerMap: nameReviewerMap,
		unownedFiles:    unowned,
	}
}

// reviewerGroupsToStrings converts ReviewerGroups to strings for comparison
func reviewerGroupsToStrings(groups ReviewerGroups) []string {
	result := make([]string, len(groups))
	for i, rg := range groups {
		result[i] = rg.ToCommentString()
	}
	return result
}

// reviewerGroupsMatch checks if actual ReviewerGroups match expected string representations
func reviewerGroupsMatch(actual ReviewerGroups, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}

	actualStrings := reviewerGroupsToStrings(actual)

	// Sort both for comparison
	actualSet := make(map[string]bool)
	for _, s := range actualStrings {
		actualSet[s] = true
	}

	for _, exp := range expected {
		if !actualSet[exp] {
			return false
		}
	}

	return true
}

// stringSlicesEqual checks if two string slices are equal (order-independent)
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]bool)
	for _, s := range a {
		aMap[s] = true
	}

	for _, s := range b {
		if !aMap[s] {
			return false
		}
	}

	return true
}

func TestGetAllFileNames(t *testing.T) {
	map1 := map[string]ReviewerGroups{
		"file1.py": {},
		"file2.py": {},
	}
	map2 := map[string]ReviewerGroups{
		"file2.py": {}, // duplicate
		"file3.py": {},
	}
	map3 := map[string]ReviewerGroups{
		"file4.py": {},
	}

	result := getAllFileNames(map1, map2, map3)

	expected := []string{"file1.py", "file2.py", "file3.py", "file4.py"}
	if !stringSlicesEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}

	// Verify sorted
	expectedSorted := strings.Join(expected, ",")
	actualSorted := strings.Join(result, ",")
	if expectedSorted != actualSorted {
		t.Errorf("result is not sorted: expected %s, got %s", expectedSorted, actualSorted)
	}
}

func TestCreateReviewerGroupKey(t *testing.T) {
	tt := []struct {
		name     string
		rg       *ReviewerGroup
		expected string
	}{
		{
			name:     "single reviewer",
			rg:       &ReviewerGroup{Names: NewSlugs([]string{"@team-a"})},
			expected: "@team-a",
		},
		{
			name:     "multiple reviewers sorted",
			rg:       &ReviewerGroup{Names: NewSlugs([]string{"@team-b", "@team-a"})},
			expected: "@team-a,@team-b", // Should be sorted
		},
		{
			name:     "case insensitive normalization",
			rg:       &ReviewerGroup{Names: NewSlugs([]string{"@Team-A"})},
			expected: "@team-a",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := createReviewerGroupKey(tc.rg)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestMergeUnownedFiles(t *testing.T) {
	tt := []struct {
		name            string
		baseUnowned     []string
		headUnowned     []string
		filesWithOwners []string
		expectedUnowned []string
	}{
		{
			name:            "no overlap",
			baseUnowned:     []string{"unowned1.txt"},
			headUnowned:     []string{"unowned2.txt"},
			filesWithOwners: []string{},
			expectedUnowned: []string{"unowned1.txt", "unowned2.txt"},
		},
		{
			name:            "files with owners excluded",
			baseUnowned:     []string{"unowned1.txt", "owned.txt"},
			headUnowned:     []string{"unowned2.txt"},
			filesWithOwners: []string{"owned.txt"},
			expectedUnowned: []string{"unowned1.txt", "unowned2.txt"},
		},
		{
			name:            "duplicate unowned",
			baseUnowned:     []string{"unowned.txt"},
			headUnowned:     []string{"unowned.txt"},
			filesWithOwners: []string{},
			expectedUnowned: []string{"unowned.txt"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := mergeUnownedFiles(tc.baseUnowned, tc.headUnowned, tc.filesWithOwners)
			if !stringSlicesEqual(result, tc.expectedUnowned) {
				t.Errorf("expected %v, got %v", tc.expectedUnowned, result)
			}
		})
	}
}
