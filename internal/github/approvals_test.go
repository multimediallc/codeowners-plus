package gh

import (
	"testing"

	"io"

	"github.com/multimediallc/codeowners-plus/internal/diff"
)

type fakeDiff struct{}

func (gd *fakeDiff) AllChanges() []diff.DiffFile {
	return []diff.DiffFile{}
}

func (gd *fakeDiff) ChangesSince(ref string) ([]diff.DiffFile, error) {
	if ref == "bad" {
		return nil, &NoPRError{}
	}
	return []diff.DiffFile{}, nil
}

func (gd *fakeDiff) Context() diff.DiffContext {
	return diff.DiffContext{}
}

func TestGetApprovalDiffs(t *testing.T) {
	approvals := []*CurrentApproval{
		{Reviewers: []string{"@base", "@core"}, CommitID: "123"},
		{Reviewers: []string{"@b-owner"}, CommitID: "123"},
		{Reviewers: []string{"@frontend"}, CommitID: "bad"},
	}
	diff := &fakeDiff{}

	approvalsWithDiff, badApprovals := getApprovalDiffs(approvals, diff, io.Discard, io.Discard)
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
