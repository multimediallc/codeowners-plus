package gh

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v63/github"
	"github.com/multimediallc/codeowners-plus/internal/git"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

type NoPRError struct{}

func (e NoPRError) Error() string {
	return "PR not initialized"
}

type UserReviewerMapNotInitError struct{}

func (e UserReviewerMapNotInitError) Error() string {
	return "User reviewer map not initialized"
}

type Client interface {
	SetWarningBuffer(writer io.Writer)
	SetInfoBuffer(writer io.Writer)
	InitPR(pr_id int) error
	PR() *github.PullRequest
	InitUserReviewerMap(reviewers []string) error
	GetTokenUser() (string, error)
	InitReviews() error
	AllApprovals() ([]*CurrentApproval, error)
	FindUserApproval(ghUser string) (*CurrentApproval, error)
	GetCurrentReviewerApprovals() ([]*CurrentApproval, error)
	GetAlreadyReviewed() ([]codeowners.Slug, error)
	GetReviewedButNotApproved() ([]codeowners.Slug, error)
	GetCurrentlyRequested() ([]codeowners.Slug, error)
	DismissStaleReviews(staleApprovals []*CurrentApproval) error
	RequestReviewers(reviewers []string) error
	ApprovePR() error
	InitComments() error
	AddComment(comment string) error
	FindExistingComment(prefix string, since *time.Time) (int64, bool, error)
	UpdateComment(commentID int64, body string) error
	IsInComments(comment string, since *time.Time) (bool, error)
	IsSubstringInComments(substring string, since *time.Time) (bool, error)
	CheckApprovals(fileReviewerMap map[string][]string, approvals []*CurrentApproval, originalDiff git.Diff) (approvers []codeowners.Slug, staleApprovals []*CurrentApproval)
	IsInLabels(labels []string) (bool, error)
	IsRepositoryAdmin(username string) (bool, error)
	ContainsValidBypassApproval(allowedUsers []string) (bool, error)
}

type GHClient struct {
	ctx             context.Context
	owner           string
	repo            string
	client          *github.Client
	pr              *github.PullRequest
	userReviewerMap ghUserReviewerMap
	comments        []*github.IssueComment
	reviews         []*github.PullRequestReview
	warningBuffer   io.Writer
	infoBuffer      io.Writer
}

func NewClient(owner, repo, token string) Client {
	client := github.NewClient(nil).WithAuthToken(token)
	return &GHClient{
		context.Background(),
		owner,
		repo,
		client,
		nil,
		nil,
		nil,
		nil,
		io.Discard,
		io.Discard,
	}
}

func (gh *GHClient) PR() *github.PullRequest {
	return gh.pr
}

func (gh *GHClient) SetWarningBuffer(writer io.Writer) {
	gh.warningBuffer = writer
}

func (gh *GHClient) SetInfoBuffer(writer io.Writer) {
	gh.infoBuffer = writer
}

func (gh *GHClient) InitPR(pr_id int) error {
	pull, res, err := gh.client.PullRequests.Get(gh.ctx, gh.owner, gh.repo, pr_id)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	gh.pr = pull
	return nil
}

func (gh *GHClient) InitUserReviewerMap(reviewers []string) error {
	teamFetch := func(org, team string) []*github.User {
		_, _ = fmt.Fprintf(gh.infoBuffer, "Fetching team members for %s/%s\n", org, team)
		allUsers := make([]*github.User, 0)
		getMembers := func(page int) (*github.Response, error) {
			listOptions := &github.TeamListTeamMembersOptions{ListOptions: github.ListOptions{PerPage: 100, Page: page}}
			users, res, err := gh.client.Teams.ListTeamMembersBySlug(gh.ctx, org, team, listOptions)
			if err != nil {
				_, _ = fmt.Fprintf(gh.warningBuffer, "WARNING: Error fetching team members for %s/%s: %v\n", org, team, err)
			}
			allUsers = append(allUsers, users...)
			return res, err
		}
		err := walkPaginatedApi(getMembers)
		if err != nil {
			_, _ = fmt.Fprintf(gh.warningBuffer, "WARNING: Error fetching team members: %v\n", err)
		}
		return allUsers
	}
	gh.userReviewerMap = makeGHUserReviwerMap(reviewers, teamFetch)
	return nil
}

func (gh *GHClient) GetTokenUser() (string, error) {
	user, _, err := gh.client.Users.Get(gh.ctx, "")
	if err != nil {
		return "", err
	}
	return user.GetLogin(), nil
}

func (gh *GHClient) InitReviews() error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	allReviews := make([]*github.PullRequestReview, 0)
	listReviews := func(page int) (*github.Response, error) {
		listOptions := &github.ListOptions{PerPage: 100, Page: page}
		reviews, res, err := gh.client.PullRequests.ListReviews(gh.ctx, gh.owner, gh.repo, gh.pr.GetNumber(), listOptions)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = res.Body.Close()
		}()
		allReviews = append(allReviews, reviews...)
		return res, err
	}
	err := walkPaginatedApi(listReviews)
	if err != nil {
		return err
	}
	allReviews = f.Filtered(allReviews, func(review *github.PullRequestReview) bool {
		return review.User.GetLogin() != gh.pr.User.GetLogin()
	})
	// use descending chronological order (default ascending)
	slices.Reverse(allReviews)
	gh.reviews = allReviews
	return nil
}

func (gh *GHClient) approvals() []*github.PullRequestReview {
	seen := make(map[string]bool, 0)
	approvals := f.Filtered(gh.reviews, func(approval *github.PullRequestReview) bool {
		userName := strings.ToLower(approval.GetUser().GetLogin())
		if _, ok := seen[userName]; ok {
			// we only care about the most recent reviews for each user
			return false
		} else if approval.GetState() == "APPROVED" {
			seen[userName] = true
		}
		return approval.GetState() == "APPROVED"
	})
	return approvals
}

// ContainsValidBypassApproval checks if any approval is a valid admin bypass approval
func (gh *GHClient) ContainsValidBypassApproval(allowedUsers []string) (bool, error) {
	if gh.pr == nil {
		return false, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return false, err
		}
	}

	for _, review := range gh.reviews {
		// Check if the review body contains bypass text
		body := review.GetBody()
		if !strings.Contains(strings.ToLower(body), "codeowners bypass") {
			continue
		}

		username := review.GetUser().GetLogin()
		usernameSlug := codeowners.NewSlug(username)

		// Check if user is in allowed users list
		for _, allowedUser := range allowedUsers {
			if usernameSlug.EqualsString(allowedUser) {
				return true, nil
			}
		}

		// Check if user is repository admin
		isAdmin, err := gh.IsRepositoryAdmin(username)
		if err != nil {
			// Log error but continue checking other approvals
			_, _ = fmt.Fprintf(gh.warningBuffer, "Warning: Could not check admin status for user %s: %v\n", username, err)
			continue
		}

		if isAdmin {
			return true, nil
		}
	}

	return false, nil
}

func (gh *GHClient) AllApprovals() ([]*CurrentApproval, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	return f.Map(gh.approvals(), func(approval *github.PullRequestReview) *CurrentApproval {
		return &CurrentApproval{codeowners.NewSlug(approval.User.GetLogin()), approval.GetID(), nil, approval.GetCommitID()}
	}), nil
}

func (gh *GHClient) FindUserApproval(ghUser string) (*CurrentApproval, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	ghUserSlug := codeowners.NewSlug(ghUser)
	review, found := f.Find(gh.approvals(), func(review *github.PullRequestReview) bool {
		return ghUserSlug.EqualsString(review.GetUser().GetLogin())
	})

	if !found {
		return nil, nil
	}
	return &CurrentApproval{codeowners.NewSlug(review.User.GetLogin()), review.GetID(), nil, review.GetCommitID()}, nil
}

func (gh *GHClient) GetCurrentReviewerApprovals() ([]*CurrentApproval, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	return currentReviewerApprovalsFromReviews(gh.approvals(), gh.userReviewerMap), nil
}

func currentReviewerApprovalsFromReviews(approvals []*github.PullRequestReview, userReviewerMap ghUserReviewerMap) []*CurrentApproval {
	filteredApprovals := make([]*CurrentApproval, 0, len(approvals))
	for _, review := range approvals {
		reviewingUser := review.GetUser().GetLogin()
		reviewingUserSlug := codeowners.NewSlug(reviewingUser)
		if reviewers, ok := userReviewerMap[strings.ToLower(reviewingUser)]; ok {
			newApproval := &CurrentApproval{reviewingUserSlug, review.GetID(), reviewers, review.GetCommitID()}
			filteredApprovals = append(filteredApprovals, newApproval)
		} else {
			newApproval := &CurrentApproval{reviewingUserSlug, review.GetID(), []codeowners.Slug{}, review.GetCommitID()}
			filteredApprovals = append(filteredApprovals, newApproval)
		}
	}

	return filteredApprovals
}

func (gh *GHClient) GetAlreadyReviewed() ([]codeowners.Slug, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	return reviewerAlreadyReviewed(gh.reviews, gh.userReviewerMap), nil
}

func reviewerAlreadyReviewed(reviews []*github.PullRequestReview, userReviewerMap ghUserReviewerMap) []codeowners.Slug {
	reviewsReviewers := make(map[string]codeowners.Slug, len(reviews))
	for _, review := range reviews {
		reviewingUser := review.GetUser().GetLogin()
		if reviewers, ok := userReviewerMap[strings.ToLower(reviewingUser)]; ok {
			for _, reviewer := range reviewers {
				reviewsReviewers[reviewer.Normalized()] = reviewer
			}
		}
	}

	return slices.Collect(maps.Values(reviewsReviewers))
}

func (gh *GHClient) GetReviewedButNotApproved() ([]codeowners.Slug, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	return reviewerReviewedButNotApproved(gh.reviews, gh.userReviewerMap), nil
}

func reviewerReviewedButNotApproved(reviews []*github.PullRequestReview, userReviewerMap ghUserReviewerMap) []codeowners.Slug {
	// Track the most recent review state for each user
	// Since reviews are in descending chronological order (most recent first),
	// the first occurrence of each user is their most recent review
	userReviewStates := make(map[string]string)
	for _, review := range reviews {
		reviewingUser := strings.ToLower(review.GetUser().GetLogin())
		// Only track if we haven't seen this user yet (most recent review)
		if _, seen := userReviewStates[reviewingUser]; !seen {
			state := review.GetState()
			// Only track non-approved states (CHANGES_REQUESTED or COMMENTED)
			if state == github.ReviewStateChangesRequested || state == github.ReviewStateCommented {
				userReviewStates[reviewingUser] = state
			}
		}
	}

	reviewsReviewers := make(map[string]codeowners.Slug)
	for reviewingUserLower := range userReviewStates {
		// Include reviewers whose most recent review is CHANGES_REQUESTED or COMMENTED
		if reviewers, ok := userReviewerMap[reviewingUserLower]; ok {
			for _, reviewer := range reviewers {
				reviewsReviewers[reviewer.Normalized()] = reviewer
			}
		}
	}

	return slices.Collect(maps.Values(reviewsReviewers))
}

func (gh *GHClient) GetCurrentlyRequested() ([]codeowners.Slug, error) {
	if gh.pr == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	return currentlyRequested(gh.pr, gh.owner, gh.userReviewerMap), nil
}

func currentlyRequested(pr *github.PullRequest, owner string, userReviewerMap ghUserReviewerMap) []codeowners.Slug {
	requestedUsers := f.Map(pr.RequestedReviewers, func(user *github.User) string {
		return user.GetLogin()
	})
	requestedTeams := f.Map(pr.RequestedTeams, func(team *github.Team) string {
		return fmt.Sprintf("%s/%s", owner, team.GetSlug())
	})
	requested := slices.Concat(requestedUsers, requestedTeams)
	reviewers := make([]codeowners.Slug, 0, len(requested))
	for _, user := range requested {
		if reviewer, ok := userReviewerMap[strings.ToLower(user)]; ok {
			reviewers = append(reviewers, reviewer...)
		}
	}
	// Deduplicate based on normalized form
	seen := make(map[string]codeowners.Slug)
	for _, rev := range reviewers {
		seen[rev.Normalized()] = rev
	}
	return slices.Collect(maps.Values(seen))
}

func (gh *GHClient) DismissStaleReviews(staleApprovals []*CurrentApproval) error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	staleMessage := "Changes in owned files since last approval"
	for _, approval := range staleApprovals {
		dismissRequest := &github.PullRequestReviewDismissalRequest{Message: &staleMessage}
		_, res, err := gh.client.PullRequests.DismissReview(gh.ctx, gh.owner, gh.repo, gh.pr.GetNumber(), approval.ReviewID, dismissRequest)
		if err != nil {
			return err
		}
		defer func() {
			_ = res.Body.Close()
		}()
	}
	return nil
}

func (gh *GHClient) RequestReviewers(reviewers []string) error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	if len(reviewers) == 0 {
		return nil
	}
	indvidualReviewers, teamReviewers := splitReviewers(reviewers)
	reviewersRequest := github.ReviewersRequest{Reviewers: indvidualReviewers, TeamReviewers: teamReviewers}
	_, res, err := gh.client.PullRequests.RequestReviewers(gh.ctx, gh.owner, gh.repo, gh.pr.GetNumber(), reviewersRequest)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	return err
}

func splitReviewers(reviewers []string) ([]string, []string) {
	indvidualReviewers := make([]string, 0, len(reviewers))
	teamReviewers := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		reviewerString := reviewer[1:] // trim the @
		if strings.Contains(reviewerString, "/") {
			split := strings.SplitN(reviewerString, "/", 2)
			teamReviewers = append(teamReviewers, split[1])
		} else {
			indvidualReviewers = append(indvidualReviewers, reviewerString)
		}
	}
	return indvidualReviewers, teamReviewers
}

func (gh *GHClient) ApprovePR() error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	createReviewOptions := &github.PullRequestReviewRequest{
		Event: github.String("APPROVE"),
		Body:  github.String("Codeowners reviews satisfied"),
	}
	_, res, err := gh.client.PullRequests.CreateReview(gh.ctx, gh.owner, gh.repo, gh.pr.GetNumber(), createReviewOptions)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	return err
}

func (gh *GHClient) InitComments() error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	allComments := make([]*github.IssueComment, 0)
	listReviews := func(page int) (*github.Response, error) {
		listOptions := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100, Page: page}}
		comments, res, err := gh.client.Issues.ListComments(gh.ctx, gh.owner, gh.repo, gh.pr.GetNumber(), listOptions)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = res.Body.Close()
		}()
		allComments = append(allComments, comments...)
		return res, err
	}
	err := walkPaginatedApi(listReviews)
	if err != nil {
		return err
	}
	gh.comments = allComments
	return nil
}

func (gh *GHClient) AddComment(comment string) error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	createCommentOptions := &github.IssueComment{
		Body: &comment,
	}
	_, res, err := gh.client.Issues.CreateComment(gh.ctx, gh.owner, gh.repo, gh.pr.GetNumber(), createCommentOptions)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	return err
}

func (gh *GHClient) FindExistingComment(prefix string, since *time.Time) (int64, bool, error) {
	if gh.pr == nil {
		return 0, false, &NoPRError{}
	}
	if err := gh.InitComments(); err != nil {
		return 0, false, err
	}

	for _, comment := range gh.comments {
		if since != nil && comment.GetCreatedAt().Before(*since) {
			continue
		}
		if strings.HasPrefix(comment.GetBody(), prefix) {
			return comment.GetID(), true, nil
		}
	}
	return 0, false, nil
}

func (gh *GHClient) UpdateComment(commentID int64, body string) error {
	if gh.pr == nil {
		return &NoPRError{}
	}
	comment := &github.IssueComment{
		Body: &body,
	}
	_, res, err := gh.client.Issues.EditComment(gh.ctx, gh.owner, gh.repo, commentID, comment)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	return nil
}

func (gh *GHClient) IsInComments(comment string, since *time.Time) (bool, error) {
	if gh.pr == nil {
		return false, &NoPRError{}
	}
	if gh.comments == nil {
		if err := gh.InitComments(); err != nil {
			_, _ = fmt.Fprintf(gh.warningBuffer, "WARNING: Error initializing comments: %v\n", err)
		}
	}
	for _, c := range gh.comments {
		if since != nil && c.GetCreatedAt().Before(*since) {
			continue
		}
		if c.GetBody() == comment {
			return true, nil
		}
	}
	return false, nil
}

func (gh *GHClient) IsSubstringInComments(substring string, since *time.Time) (bool, error) {
	if gh.pr == nil {
		return false, &NoPRError{}
	}
	if gh.comments == nil {
		if err := gh.InitComments(); err != nil {
			_, _ = fmt.Fprintf(gh.warningBuffer, "WARNING: Error initializing comments: %v\n", err)
		}
	}
	for _, c := range gh.comments {
		if since != nil && c.GetCreatedAt().Before(*since) {
			continue
		}
		if strings.Contains(c.GetBody(), substring) {
			return true, nil
		}
	}
	return false, nil
}

// IsInLabels checks if the PR has any of the given labels
func (gh *GHClient) IsInLabels(labels []string) (bool, error) {
	if gh.pr == nil {
		return false, &NoPRError{}
	}
	if len(labels) == 0 {
		return false, nil
	}
	for _, label := range gh.pr.Labels {
		for _, targetLabel := range labels {
			if label.GetName() == targetLabel {
				return true, nil
			}
		}
	}
	return false, nil
}

// Apply approver satisfaction to the owners map, and return the approvals which should be invalidated
func (gh *GHClient) CheckApprovals(
	fileReviewerMap map[string][]string,
	approvals []*CurrentApproval,
	originalDiff git.Diff,
) (approvers []codeowners.Slug, staleApprovals []*CurrentApproval) {
	appovalsWithDiff, badApprovals := getApprovalDiffs(approvals, originalDiff, gh.warningBuffer, gh.infoBuffer)
	approvers, staleApprovals = checkStale(fileReviewerMap, appovalsWithDiff)
	return approvers, append(badApprovals, staleApprovals...)
}

// IsRepositoryAdmin checks if a user has admin permissions for the repository
func (gh *GHClient) IsRepositoryAdmin(username string) (bool, error) {
	permission, _, err := gh.client.Repositories.GetPermissionLevel(
		gh.ctx,
		gh.owner,
		gh.repo,
		username,
	)
	if err != nil {
		return false, err
	}
	return permission.GetPermission() == "admin", nil
}

type ghUserReviewerMap map[string][]codeowners.Slug

type CurrentApproval struct {
	GHLogin   codeowners.Slug
	ReviewID  int64
	Reviewers []codeowners.Slug
	CommitID  string
}

func (p *CurrentApproval) String() string {
	return fmt.Sprintf("%+v", *p)
}

func makeGHUserReviwerMap(reviewers []string, teamFetcher func(string, string) []*github.User) ghUserReviewerMap {
	userReviewerMap := make(ghUserReviewerMap)

	insertReviewer := func(userName string, reviewer codeowners.Slug) {
		reviewers, found := userReviewerMap[userName]
		if found {
			userReviewerMap[userName] = append(reviewers, reviewer)
		} else {
			userReviewerMap[userName] = []codeowners.Slug{reviewer}
		}
	}

	for _, reviewer := range reviewers {
		reviewerSlug := codeowners.NewSlug(reviewer)
		reviewerString := reviewer[1:] // trim the @
		// Add the team or user to the map
		insertReviewer(strings.ToLower(reviewerString), reviewerSlug)
		if !strings.Contains(reviewerString, "/") {
			// This is a user
			continue
		}
		split := strings.SplitN(reviewerString, "/", 2)
		org := split[0]
		team := split[1]
		users := teamFetcher(org, team)
		// Add the team members to the map
		for _, user := range users {
			insertReviewer(strings.ToLower(user.GetLogin()), reviewerSlug)
		}
	}
	return userReviewerMap
}

func walkPaginatedApi(apiCall func(int) (*github.Response, error)) error {
	page := 1
	for {
		res, err := apiCall(page)
		if err != nil {
			return err
		}
		if res.NextPage == 0 {
			break
		}
		page = res.NextPage
	}
	return nil
}
