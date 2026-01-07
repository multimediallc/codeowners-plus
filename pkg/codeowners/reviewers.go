package codeowners

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

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
	newReviewers := &ReviewerGroup{NewSlugs(names), false}
	rgm[key] = newReviewers
	return newReviewers
}

// Represents a group of ReviewerGroup, with a list of names and an approved status
type ReviewerGroup struct {
	Names    []Slug
	Approved bool
}

type ReviewerGroups []*ReviewerGroup

func (rg *ReviewerGroup) ToCommentString() string {
	return strings.Join(OriginalStrings(rg.Names), " or ")
}

func (rgs ReviewerGroups) Flatten() []Slug {
	names := make([]Slug, 0)
	for _, rg := range rgs {
		names = append(names, rg.Names...)
	}
	// Use a map for deduplication based on normalized form
	seen := make(map[string]Slug)
	for _, name := range names {
		seen[name.Normalized()] = name
	}
	// Extract unique slugs
	unique := make([]Slug, 0, len(seen))
	for _, slug := range seen {
		unique = append(unique, slug)
	}
	// Sort by normalized form
	slices.SortFunc(unique, func(a, b Slug) int {
		return strings.Compare(a.Normalized(), b.Normalized())
	})
	return unique
}

// ContainsAny returns true if any of the provided names (case-insensitive)
// are present in any of the reviewer groups
func (rgs ReviewerGroups) ContainsAny(names []Slug) bool {
	for _, rg := range rgs {
		for _, rgName := range rg.Names {
			for _, name := range names {
				if rgName.Equals(name) {
					return true
				}
			}
		}
	}
	return false
}

func (rgs ReviewerGroups) FilterOut(names ...Slug) ReviewerGroups {
	return f.Filtered(rgs, func(rg *ReviewerGroup) bool {
		for _, rgName := range rg.Names {
			for _, name := range names {
				if rgName.Equals(name) {
					return false
				}
			}
		}
		return true
	})
}

// FilterOutNames returns a new slice with names from 'names' that are NOT present
// in 'exclude' (case-insensitive comparison)
func FilterOutNames(names []Slug, exclude []Slug) []Slug {
	return f.Filtered(names, func(name Slug) bool {
		for _, ex := range exclude {
			if name.Equals(ex) {
				return false
			}
		}
		return true
	})
}

func (rgs ReviewerGroups) ToCommentString(includeCheckbox bool) string {
	ownersList := f.Map(rgs, func(s *ReviewerGroup) string {
		prefix := "- "
		if includeCheckbox {
			if s.Approved {
				prefix += "✅ "
			}
		}
		return fmt.Sprintf("%s%s", prefix, s.ToCommentString())
	})
	slices.Sort(ownersList)
	return strings.Join(ownersList, "\n")
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
	return OriginalStrings(fo.RequiredReviewers().Flatten())
}

// Returns the names of the opitonal reviewers
func (fo *fileOwners) OptionalNames() []string {
	return OriginalStrings(fo.OptionalReviewers().Flatten())
}

type reviewerTest struct {
	Match    string
	Reviewer *ReviewerGroup
}

func (rt *reviewerTest) Matches(path string, warningBuffer io.Writer) bool {
	match, err := doublestar.Match(rt.Match, path)
	if err != nil {
		_, _ = fmt.Fprintf(warningBuffer, "WARNING: PatternError for pattern '%s': %s", rt.Match, err)
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
