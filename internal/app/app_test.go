package app

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v63/github"
	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/multimediallc/codeowners-plus/internal/git"
	gh "github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

// Mock implementations
type mockGitDiff struct {
	changes           []string
	context           git.DiffContext
	changesSinceError error
}

func (m mockGitDiff) AllChanges() []codeowners.DiffFile {
	files := make([]codeowners.DiffFile, 0, len(m.changes))
	for _, change := range m.changes {
		files = append(files, codeowners.DiffFile{
			FileName: change,
			Hunks: []codeowners.HunkRange{
				{Start: 1, End: 1}, // Mock hunk for testing
			},
		})
	}
	return files
}

func (m mockGitDiff) ChangesSince(ref string) ([]codeowners.DiffFile, error) {
	if m.changesSinceError != nil {
		return nil, m.changesSinceError
	}
	return m.AllChanges(), nil
}

func (m mockGitDiff) Context() git.DiffContext {
	return m.context
}

type mockCodeOwners struct {
	requiredOwners   codeowners.ReviewerGroups
	optionalOwners   codeowners.ReviewerGroups
	fileRequiredMap  map[string]codeowners.ReviewerGroups
	fileOptionalMap  map[string]codeowners.ReviewerGroups
	appliedApprovals []codeowners.Slug
	author           string
	unownedFiles     []string
}

func (m *mockCodeOwners) AllRequired() codeowners.ReviewerGroups {
	return m.requiredOwners.FilterOut(m.appliedApprovals...)
}

func (m *mockCodeOwners) AllOptional() codeowners.ReviewerGroups {
	return m.optionalOwners
}

func (m *mockCodeOwners) FileRequired() map[string]codeowners.ReviewerGroups {
	return m.fileRequiredMap
}

func (m *mockCodeOwners) FileOptional() map[string]codeowners.ReviewerGroups {
	return m.fileOptionalMap
}

func (m *mockCodeOwners) ApplyApprovals(approvers []codeowners.Slug) {
	m.appliedApprovals = approvers
}

func (m *mockCodeOwners) SetAuthor(author string, mode codeowners.AuthorMode) {
	m.author = author
	for _, reviewers := range m.requiredOwners {
		for i, name := range reviewers.Names {
			if name.EqualsString(author) {
				reviewers.Names = append(reviewers.Names[:i], reviewers.Names[i+1:]...)
				if len(reviewers.Names) == 0 || mode == codeowners.AuthorModeSelfApproval {
					reviewers.Approved = true
				}
				break
			}
		}
	}
	for _, reviewers := range m.optionalOwners {
		for i, name := range reviewers.Names {
			if name.EqualsString(author) {
				reviewers.Names = append(reviewers.Names[:i], reviewers.Names[i+1:]...)
				if len(reviewers.Names) == 0 || mode == codeowners.AuthorModeSelfApproval {
					reviewers.Approved = true
				}
				break
			}
		}
	}
}

func (m *mockCodeOwners) UnownedFiles() []string {
	return m.unownedFiles
}

type mockGitHubClient struct {
	pr                        *github.PullRequest
	userReviewerMapError      error
	currentApprovals          []*gh.CurrentApproval
	currentApprovalsError     error
	tokenUser                 string
	tokenUserError            error
	currentlyRequested        []codeowners.Slug
	currentlyRequestedError   error
	alreadyReviewed           []codeowners.Slug
	alreadyReviewedError      error
	dismissError              error
	requestReviewersError     error
	warningBuffer             io.Writer
	infoBuffer                io.Writer
	comments                  []*github.IssueComment
	initPRError               error
	initReviewsError          error
	initCommentsError         error
	addCommentError           error
	approvePRError            error
	AddCommentCalled          bool
	AddCommentInput           string
	RequestReviewersCalled    bool
	FindExistingCommentCalled bool
	FindExistingCommentInput  string
	UpdateCommentCalled       bool
	UpdateCommentInput        string
}

func (m *mockGitHubClient) PR() *github.PullRequest {
	return m.pr
}

func (m *mockGitHubClient) InitUserReviewerMap(owners []string) error {
	return m.userReviewerMapError
}

func (m *mockGitHubClient) GetCurrentReviewerApprovals() ([]*gh.CurrentApproval, error) {
	return m.currentApprovals, m.currentApprovalsError
}

func (m *mockGitHubClient) GetTokenUser() (string, error) {
	return m.tokenUser, m.tokenUserError
}

func (m *mockGitHubClient) FindUserApproval(user string) (*gh.CurrentApproval, error) {
	for _, approval := range m.currentApprovals {
		if approval.GHLogin.EqualsString(user) {
			return approval, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockGitHubClient) GetCurrentlyRequested() ([]codeowners.Slug, error) {
	return m.currentlyRequested, m.currentlyRequestedError
}

func (m *mockGitHubClient) GetAlreadyReviewed() ([]codeowners.Slug, error) {
	return m.alreadyReviewed, m.alreadyReviewedError
}

func (m *mockGitHubClient) DismissStaleReviews(approvals []*gh.CurrentApproval) error {
	return m.dismissError
}

func (m *mockGitHubClient) RequestReviewers(reviewers []string) error {
	m.RequestReviewersCalled = true
	return m.requestReviewersError
}

func (m *mockGitHubClient) CheckApprovals(fileReviewers map[string][]string, approvals []*gh.CurrentApproval, diff git.Diff) ([]codeowners.Slug, []*gh.CurrentApproval) {
	// Simple mock implementation - approve all reviewers
	var approvers []codeowners.Slug
	for _, reviewers := range fileReviewers {
		approvers = append(approvers, codeowners.NewSlugs(reviewers)...)
	}
	return approvers, nil
}

func (m *mockGitHubClient) SetWarningBuffer(writer io.Writer) {
	m.warningBuffer = writer
}

func (m *mockGitHubClient) SetInfoBuffer(writer io.Writer) {
	m.infoBuffer = writer
}

func (m *mockGitHubClient) InitPR(pr_id int) error {
	if m.initPRError != nil {
		return m.initPRError
	}
	if m.pr == nil {
		m.pr = &github.PullRequest{Number: github.Int(pr_id)}
	}
	return nil
}

func (m *mockGitHubClient) InitReviews() error {
	if m.initReviewsError != nil {
		return m.initReviewsError
	}
	return nil
}

func (m *mockGitHubClient) AllApprovals() ([]*gh.CurrentApproval, error) {
	return m.currentApprovals, m.currentApprovalsError
}

func (m *mockGitHubClient) InitComments() error {
	if m.initCommentsError != nil {
		return m.initCommentsError
	}
	return nil
}

func (m *mockGitHubClient) AddComment(comment string) error {
	m.AddCommentCalled = true
	m.AddCommentInput = comment
	if m.addCommentError != nil {
		return m.addCommentError
	}
	if m.comments == nil {
		m.comments = make([]*github.IssueComment, 0)
	}
	m.comments = append(m.comments, &github.IssueComment{Body: github.String(comment)})
	return nil
}

func (m *mockGitHubClient) ApprovePR() error {
	return m.approvePRError
}

func (m *mockGitHubClient) IsInComments(comment string, since *time.Time) (bool, error) {
	if m.comments == nil {
		return false, nil
	}
	for _, c := range m.comments {
		if since != nil && c.GetCreatedAt().Before(*since) {
			continue
		}
		if c.GetBody() == comment {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockGitHubClient) ResetGHClientTracking() {
	m.AddCommentCalled = false
	m.AddCommentInput = ""
	m.RequestReviewersCalled = false
}

func (m *mockGitHubClient) IsSubstringInComments(substring string, since *time.Time) (bool, error) {
	if m.comments == nil {
		return false, nil
	}
	for _, c := range m.comments {
		if since != nil && c.GetCreatedAt().Before(*since) {
			continue
		}
		if strings.Contains(c.GetBody(), substring) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockGitHubClient) IsInLabels(labels []string) (bool, error) {
	if m.pr == nil {
		return false, &gh.NoPRError{}
	}
	if len(labels) == 0 {
		return false, nil
	}
	for _, label := range m.pr.Labels {
		for _, targetLabel := range labels {
			if label.GetName() == targetLabel {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockGitHubClient) FindExistingComment(prefix string, since *time.Time) (int64, bool, error) {
	m.FindExistingCommentCalled = true
	m.FindExistingCommentInput = prefix
	for _, comment := range m.comments {
		if strings.HasPrefix(comment.GetBody(), prefix) {
			return comment.GetID(), true, nil
		}
	}
	return 0, false, nil
}

func (m *mockGitHubClient) UpdateComment(commentID int64, body string) error {
	m.UpdateCommentCalled = true
	m.UpdateCommentInput = body
	return nil
}

func (m *mockGitHubClient) IsRepositoryAdmin(username string) (bool, error) {
	// For testing, assume any user with "admin" in the name is an admin
	return strings.Contains(username, "admin"), nil
}

func (m *mockGitHubClient) ContainsValidBypassApproval(allowedUsers []string) (bool, error) {
	// For testing, check if any approval is from an admin-user with review ID 999
	for _, approval := range m.currentApprovals {
		if approval.ReviewID == 999 && strings.Contains(approval.GHLogin.Original(), "admin") {
			return true, nil
		}
		// Also check if user is in allowed users list
		for _, allowedUser := range allowedUsers {
			if approval.GHLogin.EqualsString(allowedUser) {
				return true, nil
			}
		}
	}
	return false, nil
}

func TestNewApp(t *testing.T) {
	tt := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Token:         "test-token",
				RepoDir:       "/test/dir",
				PR:            123,
				Repo:          "owner/repo",
				Verbose:       true,
				Quiet:         true,
				InfoBuffer:    io.Discard,
				WarningBuffer: io.Discard,
			},
			expectError: false,
		},
		{
			name: "invalid repo name",
			config: Config{
				Token:         "test-token",
				RepoDir:       "/test/dir",
				PR:            123,
				Repo:          "invalid-repo",
				Verbose:       true,
				InfoBuffer:    io.Discard,
				WarningBuffer: io.Discard,
			},
			expectError: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			app, err := New(tc.config)
			if tc.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if app == nil {
				t.Error("expected app to be non-nil")
				return
			}

			if app.config.Token != tc.config.Token {
				t.Errorf("expected token %s, got %s", tc.config.Token, app.config.Token)
			}
			if app.config.RepoDir != tc.config.RepoDir {
				t.Errorf("expected repo dir %s, got %s", tc.config.RepoDir, app.config.RepoDir)
			}
			if app.config.PR != tc.config.PR {
				t.Errorf("expected PR %d, got %d", tc.config.PR, app.config.PR)
			}
			if app.config.Repo != tc.config.Repo {
				t.Errorf("expected repo %s, got %s", tc.config.Repo, app.config.Repo)
			}
			if app.config.Verbose != tc.config.Verbose {
				t.Errorf("expected verbose %v, got %v", tc.config.Verbose, app.config.Verbose)
			}
			if app.config.Quiet != tc.config.Quiet {
				t.Errorf("expected Quiet %v, got %v", tc.config.Quiet, app.config.Quiet)
			}
		})
	}
}

func TestPrintDebug(t *testing.T) {
	tt := []struct {
		name     string
		verbose  bool
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "verbose enabled",
			verbose:  true,
			format:   "test %s %d",
			args:     []interface{}{"message", 42},
			expected: "test message 42",
		},
		{
			name:     "verbose disabled",
			verbose:  false,
			format:   "test %s %d",
			args:     []interface{}{"message", 42},
			expected: "",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffer
			infoBuffer := bytes.NewBuffer([]byte{})
			app := &App{
				config: &Config{
					Verbose:    tc.verbose,
					InfoBuffer: infoBuffer,
				},
			}

			app.printDebug(tc.format, tc.args...)

			got := infoBuffer.String()
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestPrintWarning(t *testing.T) {
	tt := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "simple warning",
			format:   "test %s %d",
			args:     []interface{}{"message", 42},
			expected: "test message 42",
		},
		{
			name:     "no args",
			format:   "test message",
			args:     []interface{}{},
			expected: "test message",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffer
			warningBuffer := bytes.NewBuffer([]byte{})
			app := &App{
				config: &Config{
					WarningBuffer: warningBuffer,
				},
			}

			app.printWarn(tc.format, tc.args...)

			got := warningBuffer.String()
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func setupAppForTest(t *testing.T, quiet bool) (*App, *mockGitHubClient) {
	t.Helper()

	mockGH := &mockGitHubClient{}
	mockGH.ResetGHClientTracking()

	cfg := Config{
		Quiet:         quiet,
		InfoBuffer:    io.Discard,
		WarningBuffer: io.Discard,
	}

	conf := &owners.Config{
		HighPriorityLabels: []string{"high-prio"},
	}
	mockOwners := &mockCodeOwners{
		requiredOwners: codeowners.ReviewerGroups{
			&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
		},
	}

	app := &App{
		config:     &cfg,
		client:     mockGH,
		Conf:       conf,
		codeowners: mockOwners,
		gitDiff:    mockGitDiff{},
	}

	return app, mockGH
}

func TestAddReviewStatusComment(t *testing.T) {
	tt := []struct {
		name                string
		requiredOwners      codeowners.ReviewerGroups
		maxReviewsMet       bool
		minReviewsNeeded    int
		currentApprovals    int
		existingComments    []*github.IssueComment
		expectAddComment    bool
		expectUpdateComment bool
		expectError         bool
		expectMinReviewNote bool
	}{
		{
			name: "no existing comment",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			expectAddComment: true,
			expectError:      false,
		},
		{
			name: "update existing comment",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			existingComments: []*github.IssueComment{
				{
					ID:   github.Int64(1),
					Body: github.String("Codeowners approval required for this PR:\n[ ] @user1"),
				},
			},
			expectUpdateComment: true,
			expectError:         false,
		},
		{
			name: "quiet mode",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			expectAddComment: false,
			expectError:      false,
		},
		{
			name: "min reviews not met shows note",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			minReviewsNeeded:    3,
			currentApprovals:    2,
			expectAddComment:    true,
			expectMinReviewNote: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mockGH := &mockGitHubClient{
				comments: tc.existingComments,
			}

			app := &App{
				config: &Config{
					Quiet:         tc.name == "quiet mode",
					InfoBuffer:    io.Discard,
					WarningBuffer: io.Discard,
				},
				client: mockGH,
				codeowners: &mockCodeOwners{
					requiredOwners: tc.requiredOwners,
				},
				Conf: &owners.Config{},
			}

			err := app.addReviewStatusComment(tc.requiredOwners, tc.maxReviewsMet, tc.minReviewsNeeded, tc.currentApprovals)
			if tc.expectError {
				if err == nil {
					t.Error("expected an error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if tc.expectAddComment && !mockGH.AddCommentCalled {
				t.Error("expected AddComment to be called")
			}
			if !tc.expectAddComment && mockGH.AddCommentCalled {
				t.Error("expected AddComment not to be called")
			}

			if tc.expectUpdateComment && !mockGH.UpdateCommentCalled {
				t.Error("expected UpdateComment to be called")
			}
			if !tc.expectUpdateComment && mockGH.UpdateCommentCalled {
				t.Error("expected UpdateComment not to be called")
			}

			// Check min reviews note
			if tc.expectMinReviewNote {
				expectedNote := "Minimum review requirement not met"
				if !strings.Contains(mockGH.AddCommentInput, expectedNote) {
					t.Errorf("expected comment to contain min reviews note, got: %s", mockGH.AddCommentInput)
				}
			}
		})
	}
}

func TestAddOptionalCcComment(t *testing.T) {
	optionalSingle := []string{"@optional-cc"}
	optionalMultiple := []string{"@cc-user1", "@cc-user2"}

	tt := []struct {
		name               string
		quiet              bool
		optionalReviewers  []string
		expectedShouldCall bool
		expectedComment    string
	}{
		{
			name:               "short circuits in quiet mode",
			quiet:              true,
			optionalReviewers:  optionalSingle,
			expectedShouldCall: false,
			expectedComment:    "",
		},
		{
			name:               "adds comment when not in quiet mode",
			quiet:              false,
			optionalReviewers:  optionalMultiple,
			expectedShouldCall: true,
			expectedComment:    fmt.Sprintf("cc %s", strings.Join(optionalMultiple, " ")),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			app, mockClient := setupAppForTest(t, tc.quiet)
			mockClient.ResetGHClientTracking()

			err := app.addOptionalCcComment(tc.optionalReviewers)
			if err != nil {
				t.Errorf("Unexpected error when adding optional cc comment: %v", err)
			}
			if mockClient.AddCommentCalled != tc.expectedShouldCall {
				t.Errorf("Expected mockClient.AddCommentCalled to be %t, but got %t", tc.expectedShouldCall, mockClient.AddCommentCalled)
			}
			if tc.expectedShouldCall && mockClient.AddCommentInput != tc.expectedComment {
				t.Errorf("Expected comment body %q, got %q", tc.expectedComment, mockClient.AddCommentInput)
			}
			if !tc.expectedShouldCall && mockClient.AddCommentInput != "" {
				t.Errorf("Expected empty comment body when AddCommentCalled is false, but got %q", mockClient.AddCommentInput)
			}
		})
	}
}

func TestRequestReviews(t *testing.T) {
	tt := []struct {
		name                   string
		quiet                  bool
		mockCurrentlyRequested []codeowners.Slug
		mockAlreadyReviewed    []codeowners.Slug
		expectedShouldCall     bool
	}{
		{
			name:                   "short circuits in quiet mode",
			quiet:                  true,
			mockCurrentlyRequested: []codeowners.Slug{},
			mockAlreadyReviewed:    []codeowners.Slug{},
			expectedShouldCall:     false,
		},
		{
			name:                   "sends requests when not in quiet mode",
			quiet:                  false,
			mockCurrentlyRequested: []codeowners.Slug{},
			mockAlreadyReviewed:    []codeowners.Slug{},
			expectedShouldCall:     true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			app, mockClient := setupAppForTest(t, tc.quiet)
			mockClient.ResetGHClientTracking()

			mockClient.currentlyRequested = tc.mockCurrentlyRequested
			mockClient.alreadyReviewed = tc.mockAlreadyReviewed

			err := app.requestReviews()

			if err != nil {
				t.Errorf("Unexpected error during requestReviews: %v", err)
			}

			if mockClient.RequestReviewersCalled != tc.expectedShouldCall {
				t.Errorf("Expected mockClient.RequestReviewersCalled to be %t, but got %t", tc.expectedShouldCall, mockClient.RequestReviewersCalled)
			}
		})
	}
}

func TestProcessApprovalsAndReviewers(t *testing.T) {
	maxReviews := 2
	minReviews := 2
	tt := []struct {
		name                 string
		requiredOwners       codeowners.ReviewerGroups
		optionalOwners       codeowners.ReviewerGroups
		fileRequiredMap      map[string]codeowners.ReviewerGroups
		fileOptionalMap      map[string]codeowners.ReviewerGroups
		currentApprovals     []*gh.CurrentApproval
		currentlyRequested   []codeowners.Slug
		alreadyReviewed      []codeowners.Slug
		tokenUser            string
		tokenUserError       error
		userReviewerMapError error
		approvalsError       error
		requestedError       error
		reviewedError        error
		dismissError         error
		requestError         error
		enforcementApproval  bool
		expectedEnfApproval  bool
		minReviews           *int
		maxReviews           *int
		unownedFiles         []string
		expectError          bool
		expectSuccess        bool
		expectedApprovals    []codeowners.Slug
	}{
		{
			name: "successful approval process",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
			},
			optionalOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1")},
			},
			currentlyRequested: codeowners.NewSlugs([]string{"@user2"}),
			alreadyReviewed:    codeowners.NewSlugs([]string{"@user1"}),
			expectError:        false,
			expectSuccess:      true,
			expectedApprovals:  codeowners.NewSlugs([]string{"@user1"}),
		},
		{
			name: "not enough approvals",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user2"})},
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1")},
				{GHLogin: codeowners.NewSlug("@user3")},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
				"file3.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
				},
			},
			currentlyRequested: codeowners.NewSlugs([]string{"@user2"}),
			expectError:        false,
			expectSuccess:      false,
			expectedApprovals:  codeowners.NewSlugs([]string{"@user1", "@user3"}),
		},
		{
			name: "max reviews bypass",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user2"})},
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1")},
				{GHLogin: codeowners.NewSlug("@user3")},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
				"file3.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
				},
			},
			currentlyRequested: codeowners.NewSlugs([]string{"@user2"}),
			maxReviews:         &maxReviews,
			expectError:        false,
			expectSuccess:      true,
			expectedApprovals:  codeowners.NewSlugs([]string{"@user1", "@user3"}),
		},
		{
			name: "min reviews enforced",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1")},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
			},
			minReviews:        &minReviews,
			expectError:       false,
			expectSuccess:     false,
			expectedApprovals: codeowners.NewSlugs([]string{"@user1"}),
		},
		{
			name: "min reviews enforced - re-request from satisfied team",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@team/eng", "@user1", "@user2"})},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1"), Reviewers: codeowners.NewSlugs([]string{"@team/eng"})},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@team/eng"})},
				},
			},
			currentlyRequested: codeowners.NewSlugs([]string{}),
			alreadyReviewed:    codeowners.NewSlugs([]string{"@team/eng"}),
			minReviews:         &minReviews,
			expectError:        false,
			expectSuccess:      false,
			expectedApprovals:  codeowners.NewSlugs([]string{"@team/eng"}),
		},
		{
			name: "token user is reviewer",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@token-user"})},
			},
			tokenUser:          "@token-user",
			currentApprovals:   []*gh.CurrentApproval{},
			currentlyRequested: codeowners.NewSlugs([]string{"@user1"}),
			expectError:        false,
			expectSuccess:      false,
		},
		{
			name: "token user exists",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			expectError:         false,
			enforcementApproval: true,
			expectedEnfApproval: true,
		},
		{
			name: "error getting token user",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			tokenUserError:      fmt.Errorf("failed to get token user"),
			expectError:         false,
			enforcementApproval: true,
			expectedEnfApproval: false,
		},
		{
			name: "error initializing reviewer map",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			userReviewerMapError: fmt.Errorf("failed to init reviewer map"),
			expectError:          true,
		},
		{
			name: "multiple file reviewers",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
				"file2.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user2"})},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1")},
				{GHLogin: codeowners.NewSlug("@user2")},
			},
			expectError:       false,
			expectSuccess:     true,
			expectedApprovals: codeowners.NewSlugs([]string{"@user1", "@user2"}),
		},
		{
			name: "optional reviewers only",
			optionalOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
			},
			fileOptionalMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("@user1")},
			},
			expectError:       false,
			expectSuccess:     true,
			expectedApprovals: codeowners.NewSlugs([]string{}),
		},
		{
			name: "unowned files",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			unownedFiles: []string{"unowned.go"},
			expectError:  false,
		},
		{
			name: "error getting approvals",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			approvalsError: fmt.Errorf("failed to get approvals"),
			expectError:    true,
		},
		{
			name: "error getting currently requested",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
			},
			currentApprovals: []*gh.CurrentApproval{},
			requestedError:   fmt.Errorf("failed to get requested reviewers"),
			expectError:      true,
		},
		{
			name: "admin bypass approval succeeds",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user2"})},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
				"file2.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user2"})},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: codeowners.NewSlug("admin-user"), ReviewID: 999, Reviewers: codeowners.NewSlugs([]string{})},
			},
			expectError:       false,
			expectSuccess:     true,
			expectedApprovals: codeowners.NewSlugs([]string{"@user1", "@user2"}),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock instances
			mockGH := &mockGitHubClient{
				currentApprovals:        tc.currentApprovals,
				currentApprovalsError:   tc.approvalsError,
				currentlyRequested:      tc.currentlyRequested,
				currentlyRequestedError: tc.requestedError,
				alreadyReviewed:         tc.alreadyReviewed,
				alreadyReviewedError:    tc.reviewedError,
				dismissError:            tc.dismissError,
				requestReviewersError:   tc.requestError,
				userReviewerMapError:    tc.userReviewerMapError,
				tokenUser:               tc.tokenUser,
				tokenUserError:          tc.tokenUserError,
			}

			mockOwners := &mockCodeOwners{
				requiredOwners:  tc.requiredOwners,
				optionalOwners:  tc.optionalOwners,
				fileRequiredMap: tc.fileRequiredMap,
				fileOptionalMap: tc.fileOptionalMap,
				unownedFiles:    tc.unownedFiles,
			}

			app := &App{
				config: &Config{
					Quiet:         false,
					InfoBuffer:    io.Discard,
					WarningBuffer: io.Discard,
				},
				client:     mockGH,
				codeowners: mockOwners,
				gitDiff: mockGitDiff{
					changes: []string{"file1.go", "file2.go", "unowned.go"},
					context: git.DiffContext{
						Base: "main",
						Head: "feature",
						Dir:  "/test/dir",
					},
				},
				Conf: &owners.Config{
					Enforcement: &owners.Enforcement{
						Approval:  tc.enforcementApproval,
						FailCheck: true,
					},
					MaxReviews: tc.maxReviews,
					MinReviews: tc.minReviews,
					AdminBypass: &owners.AdminBypass{
						Enabled:      tc.name == "admin bypass approval succeeds",
						AllowedUsers: []string{},
					},
				},
			}

			success, _, _, err := app.processApprovalsAndReviewers()
			if tc.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if tc.expectSuccess != success {
				t.Errorf("expected %t, got %t for process success", tc.expectSuccess, success)
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tc.expectedEnfApproval != app.Conf.Enforcement.Approval {
				t.Errorf("expected %t Enforcement.Approval, got %t", tc.expectedEnfApproval, app.Conf.Enforcement.Approval)
			}

			// Verify that approvals were applied correctly
			if len(mockOwners.appliedApprovals) != len(tc.expectedApprovals) {
				t.Log(mockOwners.appliedApprovals)
				t.Errorf("expected %d approvals, got %d", len(tc.expectedApprovals), len(mockOwners.appliedApprovals))
				return
			}

			if !f.SlicesItemsMatch(tc.expectedApprovals, mockOwners.appliedApprovals) {
				t.Errorf("expected approvals %x, got %x", tc.expectedApprovals, mockOwners.appliedApprovals)
			}
		})
	}
}

func TestPrintFileOwners(t *testing.T) {
	tt := []struct {
		name           string
		verbose        bool
		fileRequired   map[string]codeowners.ReviewerGroups
		fileOptional   map[string]codeowners.ReviewerGroups
		expectedOutput string
	}{
		{
			name:    "verbose enabled with both required and optional owners",
			verbose: true,
			fileRequired: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
				},
			},
			fileOptional: map[string]codeowners.ReviewerGroups{
				"file2.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
				},
			},
			expectedOutput: "File Reviewers:\n- file1.go: [@user1 @user2]\nFile Optional:\n- file2.go: [@user3]\n",
		},
		{
			name:    "verbose enabled with only required owners",
			verbose: true,
			fileRequired: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
			},
			fileOptional:   map[string]codeowners.ReviewerGroups{},
			expectedOutput: "File Reviewers:\n- file1.go: [@user1]\nFile Optional:\n",
		},
		{
			name:         "verbose enabled with only optional owners",
			verbose:      true,
			fileRequired: map[string]codeowners.ReviewerGroups{},
			fileOptional: map[string]codeowners.ReviewerGroups{
				"file2.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
				},
			},
			expectedOutput: "File Reviewers:\nFile Optional:\n- file2.go: [@user3]\n",
		},
		{
			name:    "verbose disabled",
			verbose: false,
			fileRequired: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
				},
			},
			fileOptional: map[string]codeowners.ReviewerGroups{
				"file2.go": {
					&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})},
				},
			},
			expectedOutput: "",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Create a buffer to capture output
			infoBuffer := bytes.NewBuffer([]byte{})

			// Create mock codeowners
			mockOwners := &mockCodeOwners{
				fileRequiredMap: tc.fileRequired,
				fileOptionalMap: tc.fileOptional,
			}

			// Create app instance
			app := &App{
				config: &Config{
					Verbose:    tc.verbose,
					InfoBuffer: infoBuffer,
				},
				codeowners: mockOwners,
			}

			// Call the method
			app.printFileOwners(mockOwners)

			// Check the output
			got := infoBuffer.String()
			if got != tc.expectedOutput {
				t.Errorf("expected output:\n%q\ngot:\n%q", tc.expectedOutput, got)
			}
		})
	}
}

func TestBuildOutputData(t *testing.T) {
	mockOwners := &mockCodeOwners{
		fileRequiredMap: map[string]codeowners.ReviewerGroups{
			"file1.go": {&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})}},
			"file2.go": {&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user3"})}},
		},
		fileOptionalMap: map[string]codeowners.ReviewerGroups{
			"file1.go": {&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@optional1"})}},
		},
		unownedFiles: []string{"unowned.go"},
	}
	app := &App{
		codeowners: mockOwners,
	}

	success := true
	message := "Test message"
	stillRequired := []string{"@user1"}

	output := NewOutputData(app.codeowners)
	output.UpdateOutputData(success, message, stillRequired)

	if !output.Success {
		t.Errorf("expected Success true, got false")
	}
	if output.Message != message {
		t.Errorf("expected Message %q, got %q", message, output.Message)
	}
	if len(output.FileOwners) != 2 {
		t.Errorf("expected 2 FileOwners, got %d", len(output.FileOwners))
	}
	if len(output.FileOptional) != 1 {
		t.Errorf("expected 1 FileOptional, got %d", len(output.FileOptional))
	}
	if len(output.UnownedFiles) != 1 || output.UnownedFiles[0] != "unowned.go" {
		t.Errorf("expected UnownedFiles [unowned.go], got %v", output.StillRequired)
	}
	if len(output.StillRequired) != 1 || output.StillRequired[0] != "@user1" {
		t.Errorf("expected StillRequired [@user1], got %v", output.StillRequired)
	}
}

func TestCommentDetailedReviewers(t *testing.T) {
	tt := []struct {
		name              string
		detailedReviewers bool
	}{
		{
			name:              "exclude detailed owners when config off",
			detailedReviewers: false,
		},
		{
			name:              "include detailed owners when config on",
			detailedReviewers: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mockGH := &mockGitHubClient{}

			requiredOwners := codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
			}
			app := &App{
				config: &Config{
					InfoBuffer:    io.Discard,
					WarningBuffer: io.Discard,
				},
				client: mockGH,
				codeowners: &mockCodeOwners{
					fileRequiredMap: map[string]codeowners.ReviewerGroups{
						"file1.go": {
							&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1", "@user2"})},
						},
						"file2.go": {
							&codeowners.ReviewerGroup{Names: codeowners.NewSlugs([]string{"@user1"})},
						},
					},
					requiredOwners: requiredOwners,
				},
				Conf: &owners.Config{
					HighPriorityLabels: []string{},
					DetailedReviewers:  tc.detailedReviewers,
				},
			}
			err := app.addReviewStatusComment(requiredOwners, false, 0, 0)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			DetailedReviewersSnippet := "\n\n<details><summary>Show detailed file reviewers</summary>\n\n" +
				"- file1.go: [@user1 @user2]\n" +
				"- file2.go: [@user1]\n\n"

			containsDetailedReviewersSnippet := strings.Contains(mockGH.AddCommentInput, DetailedReviewersSnippet)

			if tc.detailedReviewers != containsDetailedReviewersSnippet {
				t.Logf("AddCommentInput: %s", mockGH.AddCommentInput)
				t.Errorf("expected comment to include detailed owners to be %t, got %t ",
					tc.detailedReviewers, containsDetailedReviewersSnippet)
			}

		})
	}
}
