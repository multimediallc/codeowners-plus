package app

import (
	"slices"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

// overlayOwners implements codeowners.CodeOwners by overriding the required reviewer
// groups for specific files. All other behaviour is delegated to the base object.
// Only precedence for required owners is currently affected.

type overlayOwners struct {
	base     codeowners.CodeOwners
	required map[string]codeowners.ReviewerGroups
}

func newOverlayOwners(base codeowners.CodeOwners, overrides map[string]codeowners.ReviewerGroups) codeowners.CodeOwners {
	return &overlayOwners{base: base, required: overrides}
}

// SetAuthor propagates to base and ensures author removal from override groups.
func (o *overlayOwners) SetAuthor(author string) {
	o.base.SetAuthor(author)
	for _, groups := range o.required {
		for _, g := range groups {
			g.Names = f.RemoveValue(g.Names, author)
			if len(g.Names) == 0 {
				g.Approved = true
			}
		}
	}
}

func (o *overlayOwners) FileRequired() map[string]codeowners.ReviewerGroups { return o.required }

func (o *overlayOwners) FileOptional() map[string]codeowners.ReviewerGroups {
	return o.base.FileOptional()
}

func (o *overlayOwners) AllRequired() codeowners.ReviewerGroups {
	agg := make(codeowners.ReviewerGroups, 0)
	for _, grps := range o.required {
		agg = append(agg, grps...)
	}
	return f.RemoveDuplicates(agg)
}

func (o *overlayOwners) AllOptional() codeowners.ReviewerGroups { return o.base.AllOptional() }

func (o *overlayOwners) UnownedFiles() []string { return o.base.UnownedFiles() }

func (o *overlayOwners) ApplyApprovals(approvers []string) {
	o.base.ApplyApprovals(approvers)
	for _, a := range approvers {
		for _, groups := range o.required {
			for _, g := range groups {
				if slices.Contains(g.Names, a) {
					g.Approved = true
				}
			}
		}
	}
}

func (o *overlayOwners) AddInlineOwners(file string, owners codeowners.ReviewerGroups) {
	// Not used; overrides already applied.
}
