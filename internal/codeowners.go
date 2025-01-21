package owners

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type CodeOwners interface {
	SetAuthor(author string)
	FileReviewers() map[string]ReviewerGroups
	AllRequiredReviewers() ReviewerGroups
	AllOptionalReviewers() ReviewerGroups
	ApplyApprovals(approvals []*CurrentApproval, diff Diff) []*CurrentApproval
	UnownedFiles() []string
}

func NewCodeOwners(root string, diffFiles []DiffFile) (CodeOwners, error) {
	reviewerGroupManager := NewReviewerGroupMemo()
	tree := initOwnerTreeNode(root, root, reviewerGroupManager, nil)
	testMap := tree.BuildFromFiles(diffFiles, reviewerGroupManager)
	fileNames := Map(diffFiles, func(diffFile DiffFile) string { return diffFile.FileName })
	ownersMap, err := testMap.getOwners(fileNames)
	return ownersMap, err
}

// A collection of owned files, with reverse lookups for owners and reviewers
type ownersMap struct {
	author          string
	fileToOwner     map[string]fileOwners
	nameReviewerMap map[string]ReviewerGroups
	unownedFiles    []string
}

func (om *ownersMap) SetAuthor(author string) {
	for _, reviewers := range om.nameReviewerMap[author] {
		// remove author from the reviewers list
		reviewers.Names = RemoveValue(reviewers.Names, author)
		if len(reviewers.Names) == 0 {
			// mark the reviewer as approved if they are the author
			reviewers.Approved = true
		}
	}
	om.author = author
}

func (om *ownersMap) FileReviewers() map[string]ReviewerGroups {
	return FilteredMap(
		MapMap(om.fileToOwner, func(fileOwner fileOwners) ReviewerGroups {
			return fileOwner.RequiredReviewers()
		}),
		func(reviewers ReviewerGroups) bool {
			return len(reviewers) > 0
		})
}

func (om *ownersMap) AllRequiredReviewers() ReviewerGroups {
	reviewers := make([]*ReviewerGroup, 0)
	for _, fileOwner := range om.fileToOwner {
		reviewers = append(reviewers, fileOwner.RequiredReviewers()...)
	}
	return RemoveDuplicates(reviewers)
}

func (om *ownersMap) AllOptionalReviewers() ReviewerGroups {
	reviewers := make([]*ReviewerGroup, 0)
	for _, fileOwner := range om.fileToOwner {
		reviewers = append(reviewers, fileOwner.OptionalReviewers()...)
	}
	return RemoveDuplicates(reviewers)
}

func (om *ownersMap) UnownedFiles() []string {
	return om.unownedFiles
}

type ApprovalWithDiff struct {
	inner *CurrentApproval
	Diff  []DiffFile
}

type previousDiffRes struct {
	diff []DiffFile
	err  error
}

// Apply approver satisfaction to the owners map, and return the approvals which should be invalidated
func (om *ownersMap) ApplyApprovals(approvals []*CurrentApproval, diff Diff) []*CurrentApproval {
	appovalsWithDiff, badApprovals := getApprovalDiffs(approvals, diff)
	staleApprovals := om.applyApprovalDiffs(appovalsWithDiff)
	return append(badApprovals, staleApprovals...)
}

func getApprovalDiffs(approvals []*CurrentApproval, diff Diff) ([]*ApprovalWithDiff, []*CurrentApproval) {
	badApprovals := make([]*CurrentApproval, 0)
	seenDiffs := make(map[string]previousDiffRes)
	approvalsWithDiff := Map(approvals, func(approval *CurrentApproval) *ApprovalWithDiff {
		var diffFiles []DiffFile
		var err error

		if seenDiff, ok := seenDiffs[approval.CommitID]; ok {
			diffFiles, err = seenDiff.diff, seenDiff.err
		} else {
			diffFiles, err = diff.GetChangesSince(approval.CommitID)
			seenDiffs[approval.CommitID] = previousDiffRes{diffFiles, err}
		}
		if err != nil {
			fmt.Fprintf(WarningBuffer, "WARNING: Error getting changes since %s: %v\n", approval.CommitID, err)
			badApprovals = append(badApprovals, approval)
			return nil
		}
		return &ApprovalWithDiff{approval, diffFiles}
	})
	approvalsWithDiff = slices.DeleteFunc(approvalsWithDiff, func(approval *ApprovalWithDiff) bool { return approval == nil })
	return approvalsWithDiff, badApprovals
}

func (om *ownersMap) applyApprovalDiffs(approvals []*ApprovalWithDiff) []*CurrentApproval {
	staleApprovals := make([]*CurrentApproval, 0)
	applyApproved := func(approval *CurrentApproval) {
		for _, user := range approval.Reviewers {
			for _, reviewer := range om.nameReviewerMap[user] {
				reviewer.Approved = true
			}
		}
	}
	for _, approval := range approvals {
		// for each file in the changes since approval
		// if the file is owned by the approval owner, mark stale
		// else, mark all overlapping owners as satisfied
		if len(approval.Diff) == 0 {
			applyApproved(approval.inner)
			continue
		}
		stale := false
		badFiles := make([]string, 0)
		for _, diffFile := range approval.Diff {
			fileOwner, ok := om.fileToOwner[diffFile.FileName]
			if !ok {
				fmt.Fprintf(WarningBuffer, "WARNING: File %s not found in owners map\n", diffFile.FileName)
				badFiles = append(badFiles, diffFile.FileName)
				continue
			}
			if len(Intersection(fileOwner.RequiredNames(), approval.inner.Reviewers)) > 0 {
				stale = true
			}
		}
		if stale {
			staleApprovals = append(staleApprovals, approval.inner)
		} else if len(badFiles) < len(approval.Diff) {
			applyApproved(approval.inner)
		}
	}
	return staleApprovals
}

type ownerTreeNode struct {
	name                    string
	parent                  *ownerTreeNode
	children                map[string]*ownerTreeNode
	ownerTests              FileTestCases
	additionalReviewerTests FileTestCases
	optionalReviewerTests   FileTestCases
	fallback                *ReviewerGroup
}

func initOwnerTreeNode(name string, path string, reviewerGroupManager ReviewerGroupManager, parent *ownerTreeNode) *ownerTreeNode {
	rules := ReadCodeownersFile(path, reviewerGroupManager)
	fallback := rules.Fallback
	ownerTests := rules.OwnerTests
	additionalReviewerTests := rules.AdditionalReviewerTests
	optionalReviewerTests := rules.OptionalReviewerTests

	if parent != nil {
		if fallback == nil {
			fallback = parent.fallback
		}
	}
	return &ownerTreeNode{
		name:                    name,
		parent:                  parent,
		children:                make(map[string]*ownerTreeNode),
		ownerTests:              ownerTests,
		additionalReviewerTests: additionalReviewerTests,
		optionalReviewerTests:   optionalReviewerTests,
		fallback:                fallback,
	}
}

func (tree *ownerTreeNode) BuildFromFiles(diffFiles []DiffFile, reviewerGroupManager ReviewerGroupManager) ownerTestFileMap {
	fileMap := make(ownerTestFileMap, len(diffFiles))
	for _, diffFile := range diffFiles {
		file := diffFile.FileName
		parts := strings.Split(file, "/")
		currNode := tree
		currPath := tree.name
		// loop the file path parts, creating nodes as needed
		for _, part := range parts[:len(parts)-1] {
			currPath = currPath + "/" + part
			partNode, ok := currNode.children[part]
			if !ok {
				partNode = initOwnerTreeNode(part, currPath, reviewerGroupManager, currNode)
				currNode.children[part] = partNode
			}
			currNode = partNode
		}
		fileMap[file] = currNode
	}
	return fileMap
}

// returns the owner of the file and a boolean indicating if the owner was found
func (tree *ownerTreeNode) ownerTestRecursive(path string) (*ReviewerGroup, bool) {
	if tree == nil {
		return nil, false
	}
	for _, test := range tree.ownerTests {
		if test.Matches(path) {
			return test.Reviewer, true
		}
	}
	return tree.parent.ownerTestRecursive(tree.name + "/" + path)
}

// returns the additional reviewers owner of the file and a boolean indicating if the owner was found
func (tree *ownerTreeNode) additionalOwnersRecursive(path string) ReviewerGroups {
	owners := []*ReviewerGroup{}
	if tree == nil {
		return owners
	}
	for _, test := range tree.additionalReviewerTests {
		if test.Matches(path) {
			owners = append(owners, test.Reviewer)
		}
	}
	return append(owners, tree.parent.additionalOwnersRecursive(tree.name+"/"+path)...)
}

// returns the owner of the file and a boolean indicating if the owner was found
func (tree *ownerTreeNode) optionalOwnersRecursive(path string) ReviewerGroups {
	owners := []*ReviewerGroup{}
	if tree == nil {
		return owners
	}
	for _, test := range tree.optionalReviewerTests {
		if test.Matches(path) {
			owners = append(owners, test.Reviewer)
		}
	}
	return append(owners, tree.parent.optionalOwnersRecursive(tree.name+"/"+path)...)
}

type ownerTestFileMap map[string]*ownerTreeNode

func (otfm ownerTestFileMap) getOwners(fileNames []string) (*ownersMap, error) {
	owners := make(map[string]fileOwners, len(fileNames))
	nameReviewerMap := make(map[string]ReviewerGroups)
	unownedFiles := make([]string, 0)
	// for each file, get the owners and add to the owners map
	for _, file := range fileNames {
		node, ok := otfm[file]
		if !ok {
			return nil, errors.New("Path not found in owner tree")
		}
		fileOwner := newFileOwners()
		fileParts := strings.Split(file, "/")

		if owner, found := node.ownerTestRecursive(fileParts[len(fileParts)-1]); found {
			fileOwner.requiredReviewers = append(fileOwner.requiredReviewers, owner)
		} else if node.fallback != nil {
			fileOwner.requiredReviewers = append(fileOwner.requiredReviewers, node.fallback)
		} else {
			unownedFiles = append(unownedFiles, file)
		}

		pathSegment := fileParts[len(fileParts)-1]
		fileOwner.requiredReviewers = append(fileOwner.requiredReviewers, node.additionalOwnersRecursive(pathSegment)...)
		fileOwner.requiredReviewers = RemoveDuplicates(fileOwner.requiredReviewers)

		fileOwner.optionalReviewers = node.optionalOwnersRecursive(pathSegment)
		fileOwner.optionalReviewers = RemoveDuplicates(fileOwner.optionalReviewers)

		for _, reviewer := range fileOwner.requiredReviewers {
			for _, name := range reviewer.Names {
				if v, ok := nameReviewerMap[name]; ok {
					nameReviewerMap[name] = append(v, reviewer)
				} else {
					nameReviewerMap[name] = []*ReviewerGroup{reviewer}
				}
			}
		}
		owners[file] = *fileOwner
	}
	return &ownersMap{
		fileToOwner:     owners,
		nameReviewerMap: nameReviewerMap,
		unownedFiles:    unownedFiles,
	}, nil
}
