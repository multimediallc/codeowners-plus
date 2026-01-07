package gh

import (
	"testing"

	"io"

	"github.com/multimediallc/codeowners-plus/internal/git"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

type fakeDiff struct{}

func (gd *fakeDiff) AllChanges() []codeowners.DiffFile {
	return []codeowners.DiffFile{}
}

func (gd *fakeDiff) ChangesSince(ref string) ([]codeowners.DiffFile, error) {
	if ref == "bad" {
		return nil, &NoPRError{}
	}
	return []codeowners.DiffFile{}, nil
}

func (gd *fakeDiff) Context() git.DiffContext {
	return git.DiffContext{}
}

func TestGetApprovalDiffs(t *testing.T) {
	approvals := []*CurrentApproval{
		{Reviewers: codeowners.NewSlugs([]string{"@base", "@core"}), CommitID: "123"},
		{Reviewers: codeowners.NewSlugs([]string{"@b-owner"}), CommitID: "123"},
		{Reviewers: codeowners.NewSlugs([]string{"@frontend"}), CommitID: "bad"},
	}
	diff := &fakeDiff{}

	approvalsWithDiff, badApprovals := getApprovalDiffs(approvals, diff, io.Discard, io.Discard)
	if len(approvalsWithDiff) != 2 {
		t.Errorf("expected 2 approvals with diff, got %d", len(approvalsWithDiff))
	}
	if len(badApprovals) != 1 {
		if badApprovals[0].CommitID != "bad" {
			t.Errorf("expected bad approval to have commit ID bad, got %s", badApprovals[0].CommitID)
		}
		t.Errorf("expected 1 bad approval, got %d", len(badApprovals))
	}
}
