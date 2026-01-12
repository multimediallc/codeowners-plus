package codeowners

import (
	"slices"
	"strings"

	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

// MergeCodeOwners combines two CodeOwners objects using AND logic.
// The result requires satisfaction of ownership rules from BOTH base and head.
// This is useful for ownership handoffs where both outgoing and incoming teams must approve.
func MergeCodeOwners(base CodeOwners, head CodeOwners) CodeOwners {
	// Get file ownership maps from both branches
	baseRequired := base.FileRequired()
	headRequired := head.FileRequired()
	baseOptional := base.FileOptional()
	headOptional := head.FileOptional()

	// Get all unique file names from both branches
	allFiles := getAllFileNames(baseRequired, headRequired, baseOptional, headOptional)

	// Build merged ownership map
	mergedFileToOwner := make(map[string]fileOwners)
	for _, file := range allFiles {
		baseReq := baseRequired[file]
		headReq := headRequired[file]
		baseOpt := baseOptional[file]
		headOpt := headOptional[file]

		// Merge required reviewers (AND logic - both must be satisfied)
		mergedRequired := mergeReviewerGroups(baseReq, headReq)

		// Merge optional reviewers (both are CC'd)
		mergedOptional := mergeReviewerGroups(baseOpt, headOpt)

		mergedFileToOwner[file] = fileOwners{
			requiredReviewers: mergedRequired,
			optionalReviewers: mergedOptional,
		}
	}

	// Merge unowned files
	mergedUnowned := mergeUnownedFiles(base.UnownedFiles(), head.UnownedFiles(), allFiles)

	// Build nameReviewerMap for approval tracking
	nameReviewerMap := buildNameReviewerMap(mergedFileToOwner)

	return &ownersMap{
		author:          "",
		fileToOwner:     mergedFileToOwner,
		nameReviewerMap: nameReviewerMap,
		unownedFiles:    mergedUnowned,
	}
}

// getAllFileNames returns a deduplicated, sorted list of all file names from multiple maps
func getAllFileNames(maps ...map[string]ReviewerGroups) []string {
	fileSet := make(map[string]bool)
	for _, m := range maps {
		for file := range m {
			fileSet[file] = true
		}
	}

	files := make([]string, 0, len(fileSet))
	for file := range fileSet {
		files = append(files, file)
	}

	// Sort for deterministic output
	slices.Sort(files)
	return files
}

// mergeReviewerGroups combines two ReviewerGroups using AND logic.
func mergeReviewerGroups(base ReviewerGroups, head ReviewerGroups) ReviewerGroups {
	// Combine both groups
	combined := make([]*ReviewerGroup, 0, len(base)+len(head))
	seen := make(map[string]bool)

	// add reviewer group with deduplication
	addGroup := func(rg *ReviewerGroup, checkSeen bool) {
		key := createReviewerGroupKey(rg)
		if checkSeen && seen[key] {
			return
		}
		combined = append(combined, rg)
		seen[key] = true
	}

	for _, rg := range base {
		addGroup(rg, false)
	}

	for _, rg := range head {
		addGroup(rg, true)
	}

	return combined
}

// createReviewerGroupKey creates a unique key for a ReviewerGroup based on its normalized names
func createReviewerGroupKey(rg *ReviewerGroup) string {
	normalizedNames := f.Map(rg.Names, func(s Slug) string { return s.Normalized() })
	slices.Sort(normalizedNames)
	return strings.Join(normalizedNames, ",")
}

// mergeUnownedFiles combines unowned files from both branches, excluding files that have owners
func mergeUnownedFiles(baseUnowned []string, headUnowned []string, filesWithOwners []string) []string {
	// Create a set of files that have owners
	ownedSet := f.NewSet[string]()
	for _, file := range filesWithOwners {
		ownedSet.Add(file)
	}

	// Combine unowned files from both branches
	unownedSet := f.NewSet[string]()
	for _, file := range baseUnowned {
		if !ownedSet.Contains(file) {
			unownedSet.Add(file)
		}
	}
	for _, file := range headUnowned {
		if !ownedSet.Contains(file) {
			unownedSet.Add(file)
		}
	}

	// Convert to sorted slice
	unowned := unownedSet.Items()
	slices.Sort(unowned)
	return unowned
}

// buildNameReviewerMap creates a reverse lookup from normalized reviewer names to their ReviewerGroups
func buildNameReviewerMap(fileToOwner map[string]fileOwners) map[string]ReviewerGroups {
	nameReviewerMap := make(map[string]ReviewerGroups)

	for _, owners := range fileToOwner {
		// Process required reviewers
		for _, rg := range owners.requiredReviewers {
			for _, name := range rg.Names {
				normalizedName := name.Normalized()
				nameReviewerMap[normalizedName] = append(nameReviewerMap[normalizedName], rg)
			}
		}

		// Process optional reviewers
		for _, rg := range owners.optionalReviewers {
			for _, name := range rg.Names {
				normalizedName := name.Normalized()
				nameReviewerMap[normalizedName] = append(nameReviewerMap[normalizedName], rg)
			}
		}
	}

	// Deduplicate ReviewerGroups in nameReviewerMap
	for name, groups := range nameReviewerMap {
		nameReviewerMap[name] = f.RemoveDuplicates(groups)
	}

	return nameReviewerMap
}
