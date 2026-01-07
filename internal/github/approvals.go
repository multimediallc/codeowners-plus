package gh

import (
	"fmt"
	"io"
	"slices"

	"github.com/multimediallc/codeowners-plus/internal/git"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

type approvalWithDiff struct {
	inner *CurrentApproval
	Diff  []codeowners.DiffFile
}

type previousDiffRes struct {
	diff []codeowners.DiffFile
	err  error
}

func getApprovalDiffs(
	approvals []*CurrentApproval,
	originalDiff git.Diff,
	warningWriter io.Writer,
	infoWriter io.Writer,
) ([]*approvalWithDiff, []*CurrentApproval) {
	badApprovals := make([]*CurrentApproval, 0)
	seenDiffs := make(map[string]previousDiffRes)
	approvalsWithDiff := f.Map(approvals, func(approval *CurrentApproval) *approvalWithDiff {
		var diffFiles []codeowners.DiffFile
		var err error

		if seenDiff, ok := seenDiffs[approval.CommitID]; ok {
			diffFiles, err = seenDiff.diff, seenDiff.err
		} else {
			_, _ = fmt.Fprintf(infoWriter, "Getting diff for %s...%s\n", originalDiff.Context().Base, approval.CommitID)
			diffFiles, err = originalDiff.ChangesSince(approval.CommitID)
			seenDiffs[approval.CommitID] = previousDiffRes{diffFiles, err}
		}
		if err != nil {
			_, _ = fmt.Fprintf(warningWriter, "WARNING: Error getting changes since %s: %v\n", approval.CommitID, err)
			badApprovals = append(badApprovals, approval)
			return nil
		}
		return &approvalWithDiff{approval, diffFiles}
	})
	approvalsWithDiff = slices.DeleteFunc(approvalsWithDiff, func(approval *approvalWithDiff) bool { return approval == nil })
	return approvalsWithDiff, badApprovals
}

func checkStale(
	fileReviewerMap map[string][]string,
	approvals []*approvalWithDiff,
) ([]codeowners.Slug, []*CurrentApproval) {
	staleApprovals := make([]*CurrentApproval, 0)
	approvers := make([]codeowners.Slug, 0)
	for _, approval := range approvals {
		// for each file in the changes since approval
		// if the file is owned by the approval owner, mark stale
		// else, mark all overlapping owners as satisfied
		// Note: fileReviewerMap values are normalized strings, approval.inner.Reviewers are Slugs
		stale := false
		for _, diffFile := range approval.Diff {
			fileOwners, ok := fileReviewerMap[diffFile.FileName]
			if !ok {
				continue
			}
			// Convert Reviewers to normalized strings for Intersection
			reviewersNormalized := codeowners.NormalizedStrings(approval.inner.Reviewers)
			if len(f.Intersection(fileOwners, reviewersNormalized)) > 0 {
				stale = true
			}
		}
		if stale {
			staleApprovals = append(staleApprovals, approval.inner)
		} else {
			approvers = append(approvers, approval.inner.Reviewers...)
		}
	}
	return approvers, staleApprovals
}
