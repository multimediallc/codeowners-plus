package codeowners

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

var commentPrefix = "Codeowners approval required for this PR:\n"

type ReviewerGroupManager interface {
	ToReviewerGroup(names ...string) *ReviewerGroup
}

func NewReviewerGroupMemo() ReviewerGroupManager {
	return make(ReviewerGroupMemo)
}

type ReviewerGroupMemo map[string]*ReviewerGroup

// Create a new Reviewers, memoizing the Reviewers so it is only created once
func (rgm ReviewerGroupMemo) ToReviewerGroup(names ...string) *ReviewerGroup {
	key := strings.Join(names, ",")
	if item, found := rgm[key]; found {
		return item
	}
	newReviewers := &ReviewerGroup{names, false}
	rgm[key] = newReviewers
	return newReviewers
}

// Represents a group of ReviewerGroup, with a list of names and an approved status
type ReviewerGroup struct {
	Names    []string
	Approved bool
}

type ReviewerGroups []*ReviewerGroup

func (rg *ReviewerGroup) ToCommentString() string {
	return strings.Join(rg.Names, " or ")
}

func (rgs ReviewerGroups) Flatten() []string {
	names := make([]string, 0)
	for _, rg := range rgs {
		names = append(names, rg.Names...)
	}
	names = f.RemoveDuplicates(names)
	slices.Sort(names)
	return names
}

func (rgs ReviewerGroups) FilterOut(names ...string) ReviewerGroups {
	return f.Filtered(rgs, func(rg *ReviewerGroup) bool {
		found := false
		for _, name := range rg.Names {
			if slices.Contains(names, name) {
				found = true
				break
			}
		}
		return !found
	})
}

func (rgs ReviewerGroups) ToCommentString() string {
	ownersList := f.Map(rgs, func(s *ReviewerGroup) string {
		return "- " + s.ToCommentString()
	})
	slices.Sort(ownersList)
	ownersListString := strings.Join(ownersList, "\n")
	return fmt.Sprintf("%s%s", commentPrefix, ownersListString)
}

// Represents the owners of a file, with a list of required and optional reviewers
type fileOwners struct {
	requiredReviewers ReviewerGroups
	optionalReviewers ReviewerGroups
}

func newFileOwners() *fileOwners {
	return &fileOwners{make(ReviewerGroups, 0), make(ReviewerGroups, 0)}
}

// Returns the required reviewers, excluding those who have already approved
func (fo *fileOwners) RequiredReviewers() ReviewerGroups {
	owners := f.NewSet[*ReviewerGroup]()
	for _, reviewers := range fo.requiredReviewers {
		if !reviewers.Approved {
			owners.Add(reviewers)
		}
	}
	return owners.Items()
}

// Returns the optional reviewers
func (fo *fileOwners) OptionalReviewers() ReviewerGroups {
	owners := f.NewSet[*ReviewerGroup]()
	for _, reviewers := range fo.optionalReviewers {
		owners.Add(reviewers)
	}
	return owners.Items()
}

// Returns the names of the required reviewers, excluding those who have already approved
func (fo *fileOwners) RequiredNames() []string {
	return fo.RequiredReviewers().Flatten()
}

// Returns the names of the opitonal reviewers
func (fo *fileOwners) OptionalNames() []string {
	return fo.OptionalReviewers().Flatten()
}

type reviewerTest struct {
	Match    string
	Reviewer *ReviewerGroup
}

func (rt *reviewerTest) Matches(path string, warningBuffer io.Writer) bool {
	match, err := doublestar.Match(rt.Match, path)
	if err != nil {
		fmt.Fprintf(warningBuffer, "WARNING: PatternError for pattern '%s': %s", rt.Match, err)
	}
	return match
}

type FileTestCases []*reviewerTest

func (ftc FileTestCases) Len() int {
	return len(ftc)
}

func (ftc FileTestCases) Swap(i, j int) {
	ftc[i], ftc[j] = ftc[j], ftc[i]
}

func (ftc FileTestCases) Less(i, j int) bool {
	it := ftc[i]
	other := ftc[j]

	itHasGlobstar := strings.Contains(it.Match, "**/") || strings.Contains(it.Match, "/**")
	otherHasGlobstar := strings.Contains(other.Match, "**/") || strings.Contains(other.Match, "/**")
	if !itHasGlobstar && otherHasGlobstar {
		// no globstar is more specific
		return true
	}

	itHasWC := strings.Contains(it.Match, "*")
	otherHasWC := strings.Contains(other.Match, "*")
	if !itHasWC && otherHasWC {
		// no wildcards is more specific
		return true
	}

	return false
}
