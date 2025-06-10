package app

import (
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

type simpleOwners struct {
	required map[string]codeowners.ReviewerGroups
}

func (s *simpleOwners) SetAuthor(a string)                                 {}
func (s *simpleOwners) FileRequired() map[string]codeowners.ReviewerGroups { return s.required }
func (s *simpleOwners) FileOptional() map[string]codeowners.ReviewerGroups { return nil }
func (s *simpleOwners) AllRequired() codeowners.ReviewerGroups {
	agg := codeowners.ReviewerGroups{}
	for _, g := range s.required {
		agg = append(agg, g...)
	}
	return agg
}
func (s *simpleOwners) AllOptional() codeowners.ReviewerGroups            { return nil }
func (s *simpleOwners) UnownedFiles() []string                            { return nil }
func (s *simpleOwners) ApplyApprovals([]string)                           {}
func (s *simpleOwners) AddInlineOwners(string, codeowners.ReviewerGroups) {}

func TestOverlayOwnersPrecedence(t *testing.T) {
	rgm := codeowners.NewReviewerGroupMemo()
	baseGrp := rgm.ToReviewerGroup("@file")
	inGrp := rgm.ToReviewerGroup("@inline")

	base := &simpleOwners{required: map[string]codeowners.ReviewerGroups{
		"file.go": {baseGrp},
	}}

	overlay := newOverlayOwners(base, map[string]codeowners.ReviewerGroups{
		"file.go": {inGrp},
	}).(*overlayOwners)

	req := overlay.FileRequired()
	if len(req) != (1) {
		t.Fatalf("expected 1 entry")
	}
	grps := req["file.go"]
	if len(grps) != 1 || grps[0] != inGrp {
		t.Fatalf("precedence failed, got %+v", grps)
	}
}
