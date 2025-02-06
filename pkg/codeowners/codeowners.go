package codeowners

import (
	"errors"
	"io"
	"strings"

	"github.com/multimediallc/codeowners-plus/pkg/functional"
)

// CodeOwners represents a collection of owned files, with reverse lookups for owners and reviewers
type CodeOwners interface {
	// SetAuthor sets the author of the PR
	SetAuthor(author string)

	// FileRequired returns a map of file names to their required reviewers
	FileRequired() map[string]ReviewerGroups

	// FileReviewers returns a map of file names to their required reviewers
	FileOptional() map[string]ReviewerGroups

	// AllRequired returns a list of the required reviewers for all files in the PR
	AllRequired() ReviewerGroups

	// AllOptional return a list of the optional reviewers for all files in the PR
	AllOptional() ReviewerGroups

	// UnownedFiles returns a list of files in the diff which are not
	UnownedFiles() []string

	// ApplyApprovals marks the given approvers as satisfied
	ApplyApprovals(approvers []string)
}

// New creates a new CodeOwners object from a root path and a list of diff files
func New(root string, files []DiffFile, warningWriter io.Writer) (CodeOwners, error) {
	reviewerGroupManager := NewReviewerGroupMemo()
	tree := initOwnerTreeNode(root, root, reviewerGroupManager, nil, warningWriter)
	tree.warningWriter = warningWriter
	// TODO - support inline ownership rules (issue #3)
	fileNames := f.Map(files, func(file DiffFile) string { return file.FileName })
	testMap := tree.BuildFromFiles(fileNames, reviewerGroupManager)
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
		reviewers.Names = f.RemoveValue(reviewers.Names, author)
		if len(reviewers.Names) == 0 {
			// mark the reviewer as approved if they are the author
			reviewers.Approved = true
		}
	}
	om.author = author
}

func (om *ownersMap) FileRequired() map[string]ReviewerGroups {
	return f.FilteredMap(
		f.MapMap(om.fileToOwner, func(fileOwner fileOwners) ReviewerGroups {
			return fileOwner.RequiredReviewers()
		}),
		func(reviewers ReviewerGroups) bool {
			return len(reviewers) > 0
		})
}

func (om *ownersMap) FileOptional() map[string]ReviewerGroups {
	return f.FilteredMap(
		f.MapMap(om.fileToOwner, func(fileOwner fileOwners) ReviewerGroups {
			return fileOwner.OptionalReviewers()
		}),
		func(reviewers ReviewerGroups) bool {
			return len(reviewers) > 0
		})
}

func (om *ownersMap) AllRequired() ReviewerGroups {
	reviewers := make([]*ReviewerGroup, 0)
	for _, fileOwner := range om.fileToOwner {
		reviewers = append(reviewers, fileOwner.RequiredReviewers()...)
	}
	return f.RemoveDuplicates(reviewers)
}

func (om *ownersMap) AllOptional() ReviewerGroups {
	reviewers := make([]*ReviewerGroup, 0)
	for _, fileOwner := range om.fileToOwner {
		reviewers = append(reviewers, fileOwner.OptionalReviewers()...)
	}
	return f.RemoveDuplicates(reviewers)
}

func (om *ownersMap) UnownedFiles() []string {
	return om.unownedFiles
}

// Apply approver satisfaction to the owners map, and return the approvals which should be invalidated
func (om *ownersMap) ApplyApprovals(approvers []string) {
	applyApproved := func(user string) {
		for _, reviewer := range om.nameReviewerMap[user] {
			reviewer.Approved = true
		}
	}
	for _, user := range approvers {
		applyApproved(user)
	}
}

type ownerTreeNode struct {
	name                    string
	parent                  *ownerTreeNode
	children                map[string]*ownerTreeNode
	ownerTests              FileTestCases
	additionalReviewerTests FileTestCases
	optionalReviewerTests   FileTestCases
	fallback                *ReviewerGroup
	warningWriter           io.Writer
}

func initOwnerTreeNode(
	name string,
	path string,
	reviewerGroupManager ReviewerGroupManager,
	parent *ownerTreeNode,
	warningWriter io.Writer,
) *ownerTreeNode {
	rules := Read(path, reviewerGroupManager, warningWriter)
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
		warningWriter:           io.Discard,
	}
}

func (tree *ownerTreeNode) BuildFromFiles(
	files []string,
	reviewerGroupManager ReviewerGroupManager,
) ownerTestFileMap {
	fileMap := make(ownerTestFileMap, len(files))
	for _, file := range files {
		file := file
		parts := strings.Split(file, "/")
		currNode := tree
		currPath := tree.name
		// loop the file path parts, creating nodes as needed
		for _, part := range parts[:len(parts)-1] {
			currPath = currPath + "/" + part
			partNode, ok := currNode.children[part]
			if !ok {
				partNode = initOwnerTreeNode(part, currPath, reviewerGroupManager, currNode, tree.warningWriter)
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
		if test.Matches(path, io.Discard) {
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
		if test.Matches(path, io.Discard) {
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
		if test.Matches(path, io.Discard) {
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
		fileOwner.requiredReviewers = f.RemoveDuplicates(fileOwner.requiredReviewers)

		fileOwner.optionalReviewers = node.optionalOwnersRecursive(pathSegment)
		fileOwner.optionalReviewers = f.RemoveDuplicates(fileOwner.optionalReviewers)

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
