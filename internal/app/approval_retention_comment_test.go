package app

import (
	"slices"
	"testing"
)

const retentionCommentsOnConfig = `disable_review_status_comments = true

[approval_retention]
enabled = true
comments = true
`

func TestRunRetainsApprovalAcrossCommentOnlyChange(t *testing.T) {
	tt := []struct {
		name             string
		config           string
		expectDismissed  bool
		expectSuccess    bool
		expectStillReqd  []string
		expectedApproval string
	}{
		{
			// Pre-feature behavior: the comment lands on an owned file, so it goes stale.
			name:            "section absent",
			config:          retentionOffConfig,
			expectDismissed: true,
			expectSuccess:   false,
			expectStillReqd: []string{"@owner"},
		},
		{
			name:            "comments retained",
			config:          retentionCommentsOnConfig,
			expectDismissed: false,
			expectSuccess:   true,
			expectStillReqd: []string{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			repoDir, baseSHA, headSHA, approvalSHA := buildCommentOnlyRepo(t, tc.config)
			output, dismissed, warnings := runApp(t, repoDir, baseSHA, headSHA, approvalSHA)

			if tc.expectDismissed && len(dismissed) != 1 {
				t.Errorf("expected the approval to be dismissed, got %d dismissals", len(dismissed))
			}
			if !tc.expectDismissed && len(dismissed) != 0 {
				t.Errorf("expected the approval to survive, got %d dismissals", len(dismissed))
			}
			if output.Success != tc.expectSuccess {
				t.Errorf("expected success %t, got %t (%s)", tc.expectSuccess, output.Success, output.Message)
			}
			if !slices.Equal(output.StillRequired, tc.expectStillReqd) {
				t.Errorf("expected still required %v, got %v", tc.expectStillReqd, output.StillRequired)
			}
			if warnings != "" {
				t.Errorf("expected no warnings, got %q", warnings)
			}
		})
	}
}
