package main

import (
	"fmt"
	"io"
	"os"
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
	appliedApprovals []string
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

func (m *mockCodeOwners) ApplyApprovals(approvers []string) {
	m.appliedApprovals = approvers
}

func (m *mockCodeOwners) SetAuthor(author string) {
	m.author = author
	// Remove author from reviewers
	for _, reviewers := range m.requiredOwners {
		for i, name := range reviewers.Names {
			if name == author {
				reviewers.Names = append(reviewers.Names[:i], reviewers.Names[i+1:]...)
				if len(reviewers.Names) == 0 {
					reviewers.Approved = true
				}
				break
			}
		}
	}
	for _, reviewers := range m.optionalOwners {
		for i, name := range reviewers.Names {
			if name == author {
				reviewers.Names = append(reviewers.Names[:i], reviewers.Names[i+1:]...)
				if len(reviewers.Names) == 0 {
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
	pr                      *github.PullRequest
	userReviewerMapError    error
	currentApprovals        []*gh.CurrentApproval
	currentApprovalsError   error
	tokenUser               string
	tokenUserError          error
	currentlyRequested      []string
	currentlyRequestedError error
	alreadyReviewed         []string
	alreadyReviewedError    error
	dismissError            error
	requestReviewersError   error
	warningBuffer           io.Writer
	infoBuffer              io.Writer
	comments                []*github.IssueComment
	initPRError             error
	initReviewsError        error
	initCommentsError       error
	addCommentError         error
	approvePRError          error
	AddCommentCalled        bool
	AddCommentInput         string
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
		if approval.GHLogin == user {
			return approval, nil
		}
	}
	return nil, fmt.Errorf("Not found")
}

func (m *mockGitHubClient) GetCurrentlyRequested() ([]string, error) {
	return m.currentlyRequested, m.currentlyRequestedError
}

func (m *mockGitHubClient) GetAlreadyReviewed() ([]string, error) {
	return m.alreadyReviewed, m.alreadyReviewedError
}

func (m *mockGitHubClient) DismissStaleReviews(approvals []*gh.CurrentApproval) error {
	return m.dismissError
}

func (m *mockGitHubClient) RequestReviewers(reviewers []string) error {
	return m.requestReviewersError
}

func (m *mockGitHubClient) CheckApprovals(fileReviewers map[string][]string, approvals []*gh.CurrentApproval, diff git.Diff) ([]string, []*gh.CurrentApproval) {
	// Simple mock implementation - approve all reviewers
	var approvers []string
	for _, reviewers := range fileReviewers {
		approvers = append(approvers, reviewers...)
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

func (m *mockGitHubClient) ResetCommentCallTracking() {
	m.AddCommentCalled = false
	m.AddCommentInput = ""
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

func init() {
	// Initialize test flags with default values
	flags = &Flags{
		Token:   new(string),
		RepoDir: new(string),
		PR:      new(int),
		Repo:    new(string),
		Verbose: new(bool),
	}
	*flags.Token = "test-token"
	*flags.RepoDir = "/test/dir"
	*flags.PR = 123
	*flags.Repo = "owner/repo"
	*flags.Verbose = false
}

func TestGetEnv(t *testing.T) {
	tt := []struct {
		name     string
		key      string
		fallback string
		setEnv   bool
		envValue string
		expected string
	}{
		{
			name:     "environment variable set",
			key:      "TEST_ENV",
			fallback: "fallback",
			setEnv:   true,
			envValue: "test_value",
			expected: "test_value",
		},
		{
			name:     "environment variable not set",
			key:      "TEST_ENV",
			fallback: "fallback",
			setEnv:   false,
			expected: "fallback",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				os.Setenv(tc.key, tc.envValue)
				defer os.Unsetenv(tc.key)
			}

			got := getEnv(tc.key, tc.fallback)
			if got != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestIgnoreError(t *testing.T) {
	tt := []struct {
		name     string
		value    int
		err      error
		expected int
	}{
		{
			name:     "error is nil",
			value:    42,
			err:      nil,
			expected: 42,
		},
		{
			name:     "error is not nil",
			value:    42,
			err:      os.ErrNotExist,
			expected: 42,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := ignoreError(tc.value, tc.err)
			if got != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestNewApp(t *testing.T) {
	tt := []struct {
		name        string
		config      AppConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: AppConfig{
				Token:       "test-token",
				RepoDir:     "/test/dir",
				PR:          123,
				Repo:        "owner/repo",
				Verbose:     true,
				AddComments: false,
			},
			expectError: false,
		},
		{
			name: "invalid repo name",
			config: AppConfig{
				Token:   "test-token",
				RepoDir: "/test/dir",
				PR:      123,
				Repo:    "invalid-repo",
				Verbose: true,
			},
			expectError: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			app, err := NewApp(tc.config)
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
			if app.config.AddComments != tc.config.AddComments {
				t.Errorf("expected AddComments %v, got %v", tc.config.AddComments, app.config.AddComments)
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
			InfoBuffer.Reset()
			// Set verbose flag
			*flags.Verbose = tc.verbose

			printDebug(tc.format, tc.args...)

			got := InfoBuffer.String()
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
			WarningBuffer.Reset()

			printWarning(tc.format, tc.args...)

			got := WarningBuffer.String()
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestErrorAndExit(t *testing.T) {
	// Note: This test can't actually verify the exit behavior
	// It only verifies that the buffers are written correctly
	tt := []struct {
		name       string
		shouldFail bool
		format     string
		args       []interface{}
		verbose    bool
		warnings   string
		info       string
	}{
		{
			name:       "with warnings and info",
			shouldFail: true,
			format:     "test %s %d",
			args:       []interface{}{"message", 42},
			verbose:    true,
			warnings:   "warning message\n",
			info:       "info message\n",
		},
		{
			name:       "with warnings only",
			shouldFail: false,
			format:     "test %s %d",
			args:       []interface{}{"message", 42},
			verbose:    false,
			warnings:   "warning message\n",
			info:       "",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffers
			WarningBuffer.Reset()
			InfoBuffer.Reset()

			// Set up test data
			WarningBuffer.WriteString(tc.warnings)
			InfoBuffer.WriteString(tc.info)
			*flags.Verbose = tc.verbose

			errorAndExit(tc.shouldFail, tc.format, tc.args...)
		})
	}
}

func TestInitFlags(t *testing.T) {
	tokenStr := "test-token"
	prInt := 123
	repoStr := "owner/repo"
	emptyStr := ""
	zeroInt := 0
	tt := []struct {
		name        string
		flags       *Flags
		expectError bool
	}{
		{
			name: "all required flags set",
			flags: &Flags{
				Token: &tokenStr,
				PR:    &prInt,
				Repo:  &repoStr,
			},
			expectError: false,
		},
		{
			name: "missing token",
			flags: &Flags{
				Token: &emptyStr,
				PR:    &prInt,
				Repo:  &repoStr,
			},
			expectError: true,
		},
		{
			name: "missing PR",
			flags: &Flags{
				Token: &tokenStr,
				PR:    &zeroInt,
				Repo:  &repoStr,
			},
			expectError: true,
		},
		{
			name: "missing repo",
			flags: &Flags{
				Token: &tokenStr,
				PR:    &prInt,
				Repo:  &emptyStr,
			},
			expectError: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := initFlags(tc.flags)
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
		})
	}
}

func setupAppForCommentTest(t *testing.T, addComments bool) (*App, *mockGitHubClient) {
	t.Helper()

	mockGH := &mockGitHubClient{}
	mockGH.ResetCommentCallTracking()

	cfg := AppConfig{
		AddComments: addComments,
	}

	conf := &owners.Config{
		HighPriorityLabels: []string{"high-prio"},
	}

	app := &App{
		config:     cfg,
		client:     mockGH,
		conf:       conf,
		codeowners: &mockCodeOwners{},
		gitDiff:    mockGitDiff{},
	}

	return app, mockGH
}

func TestAddReviewStatusComment_ShortCircuit(t *testing.T) {
	app, mockClient := setupAppForCommentTest(t, false) // AddComments = false

	// Prepare some data that *would* trigger a comment if AddComments were true
	unapproved := codeowners.ReviewerGroups{
		&codeowners.ReviewerGroup{Names: []string{"@pending-reviewer"}},
	}
	allRequired := codeowners.ReviewerGroups{
		&codeowners.ReviewerGroup{Names: []string{"@pending-reviewer"}},
	}

	err := app.addReviewStatusComment(allRequired, unapproved, false)
	if err != nil {
		t.Errorf("Expected no error when AddComments is false, but got: %v", err)
	}

	if mockClient.AddCommentCalled {
		t.Error("Expected AddComment not to be called when AddComments is false")
	}
}

func TestAddOptionalCcComment_ShortCircuit(t *testing.T) {
	app, mockClient := setupAppForCommentTest(t, false) // AddComments = false

	// Prepare some data that *would* trigger a comment if AddComments were true
	optionalReviewers := []string{"@optional-cc"}

	err := app.addOptionalCcComment(optionalReviewers)
	if err != nil {
		t.Errorf("Expected no error when AddComments is false, but got: %v", err)
	}

	if mockClient.AddCommentCalled {
		t.Error("Expected AddComment not to be called when AddComments is false")
	}
}

func TestAddReviewStatusComment_AddsComment(t *testing.T) {
	app, mockClient := setupAppForCommentTest(t, true) // AddComments = true

	unapproved := codeowners.ReviewerGroups{
		&codeowners.ReviewerGroup{Names: []string{"@user1"}},
	}
	allRequired := codeowners.ReviewerGroups{
		&codeowners.ReviewerGroup{Names: []string{"@user1"}},
	}
	expectedComment := allRequired.ToCommentString()

	err := app.addReviewStatusComment(allRequired, unapproved, false)

	if err != nil {
		t.Errorf("Unexpected error when adding comment: %v", err)
	}
	if !mockClient.AddCommentCalled {
		t.Error("Expected AddComment to be called when AddComments is true and unapproved exist")
	}
	if mockClient.AddCommentInput != expectedComment {
		t.Errorf("Expected comment body %q, got %q", expectedComment, mockClient.AddCommentInput)
	}
}

func TestAddOptionalCcComment_AddsComment(t *testing.T) {
	app, mockClient := setupAppForCommentTest(t, true) // AddComments = true

	optionalReviewers := []string{"@cc-user1", "@cc-user2"}
	expectedComment := "cc @cc-user1 @cc-user2"

	err := app.addOptionalCcComment(optionalReviewers)
	if err != nil {
		t.Errorf("Unexpected error when adding comment: %v", err)
	}
	if !mockClient.AddCommentCalled {
		t.Error("Expected AddComment to be called when AddComments is true and viewers need pinging")
	}
	if mockClient.AddCommentInput != expectedComment {
		t.Errorf("Expected comment body %q, got %q", expectedComment, mockClient.AddCommentInput)
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
		currentlyRequested   []string
		alreadyReviewed      []string
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
		expectedApprovals    []string
	}{
		{
			name: "successful approval process",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1", "@user2"}},
			},
			optionalOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user3"}},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: "@user1"},
			},
			currentlyRequested: []string{"@user2"},
			alreadyReviewed:    []string{"@user1"},
			expectError:        false,
			expectSuccess:      true,
			expectedApprovals:  []string{"@user1"},
		},
		{
			name: "not enough approvals",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				&codeowners.ReviewerGroup{Names: []string{"@user2"}},
				&codeowners.ReviewerGroup{Names: []string{"@user3"}},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: "@user1"},
				{GHLogin: "@user3"},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				},
				"file3.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user3"}},
				},
			},
			currentlyRequested: []string{"@user2"},
			expectError:        false,
			expectSuccess:      false,
			expectedApprovals:  []string{"@user1", "@user3"},
		},
		{
			name: "max reviews bypass",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				&codeowners.ReviewerGroup{Names: []string{"@user2"}},
				&codeowners.ReviewerGroup{Names: []string{"@user3"}},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: "@user1"},
				{GHLogin: "@user3"},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				},
				"file3.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user3"}},
				},
			},
			currentlyRequested: []string{"@user2"},
			maxReviews:         &maxReviews,
			expectError:        false,
			expectSuccess:      true,
			expectedApprovals:  []string{"@user1", "@user3"},
		},
		{
			name: "min reviews enforced",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: "@user1"},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				},
			},
			minReviews:        &minReviews,
			expectError:       false,
			expectSuccess:     false,
			expectedApprovals: []string{"@user1"},
		},
		{
			name: "token user is reviewer",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1", "@token-user"}},
			},
			tokenUser:          "@token-user",
			currentApprovals:   []*gh.CurrentApproval{},
			currentlyRequested: []string{"@user1"},
			expectError:        false,
			expectSuccess:      false,
		},
		{
			name: "token user exists",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			expectError:         false,
			enforcementApproval: true,
			expectedEnfApproval: true,
		},
		{
			name: "error getting token user",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			tokenUserError:      fmt.Errorf("failed to get token user"),
			expectError:         false,
			enforcementApproval: true,
			expectedEnfApproval: false,
		},
		{
			name: "error initializing reviewer map",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			userReviewerMapError: fmt.Errorf("failed to init reviewer map"),
			expectError:          true,
		},
		{
			name: "multiple file reviewers",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1", "@user2"}},
			},
			fileRequiredMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				},
				"file2.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user2"}},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: "@user1"},
				{GHLogin: "@user2"},
			},
			expectError:       false,
			expectSuccess:     true,
			expectedApprovals: []string{"@user1", "@user2"},
		},
		{
			name: "optional reviewers only",
			optionalOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1", "@user2"}},
			},
			fileOptionalMap: map[string]codeowners.ReviewerGroups{
				"file1.go": {
					&codeowners.ReviewerGroup{Names: []string{"@user1"}},
				},
			},
			currentApprovals: []*gh.CurrentApproval{
				{GHLogin: "@user1"},
			},
			expectError:       false,
			expectSuccess:     true,
			expectedApprovals: []string{},
		},
		{
			name: "unowned files",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			unownedFiles: []string{"unowned.go"},
			expectError:  false,
		},
		{
			name: "error getting approvals",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			approvalsError: fmt.Errorf("failed to get approvals"),
			expectError:    true,
		},
		{
			name: "error getting currently requested",
			requiredOwners: codeowners.ReviewerGroups{
				&codeowners.ReviewerGroup{Names: []string{"@user1"}},
			},
			currentApprovals: []*gh.CurrentApproval{},
			requestedError:   fmt.Errorf("failed to get requested reviewers"),
			expectError:      true,
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
				config:     AppConfig{},
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
				conf: &owners.Config{
					Enforcement: &owners.Enforcement{
						Approval:  tc.enforcementApproval,
						FailCheck: true,
					},
					MaxReviews: tc.maxReviews,
					MinReviews: tc.minReviews,
				},
			}

			success, _, err := app.processApprovalsAndReviewers()
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

			if tc.expectedEnfApproval != app.conf.Enforcement.Approval {
				t.Errorf("expected %t Enforcement.Approval, got %t", tc.expectedEnfApproval, app.conf.Enforcement.Approval)
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
