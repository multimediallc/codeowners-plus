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
	"github.com/multimediallc/codeowners-plus/pkg/functional"
)

type NoPRError struct{}

func (e NoPRError) Error() string {
	return "PR not initialized"
}

type UserReviewerMapNotInitError struct{}

func (e UserReviewerMapNotInitError) Error() string {
	return "User reviewer map not initialized"
}

type Client struct {
	ctx             context.Context
	owner           string
	repo            string
	client          *github.Client
	PR              *github.PullRequest
	userReviewerMap ghUserReviewerMap
	comments        []*github.IssueComment
	reviews         []*github.PullRequestReview
	warningBuffer   io.Writer
	infoBuffer      io.Writer
}

func NewClient(owner, repo, token string) *Client {
	client := github.NewClient(nil).WithAuthToken(token)
	return &Client{
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

func (gh *Client) SetWarningBuffer(writer io.Writer) {
	gh.warningBuffer = writer
}

func (gh *Client) SetInfoBuffer(writer io.Writer) {
	gh.infoBuffer = writer
}

func (gh *Client) InitPR(pr_id int) error {
	pull, res, err := gh.client.PullRequests.Get(gh.ctx, gh.owner, gh.repo, pr_id)
	defer res.Body.Close()
	if err != nil {
		return err
	}
	gh.PR = pull
	return nil
}

func (gh *Client) InitUserReviewerMap(reviewers []string) error {
	teamFetch := func(org, team string) []*github.User {
		fmt.Fprintf(gh.infoBuffer, "Fetching team members for %s/%s\n", org, team)
		allUsers := make([]*github.User, 0)
		getMembers := func(page int) (*github.Response, error) {
			listOptions := &github.TeamListTeamMembersOptions{ListOptions: github.ListOptions{PerPage: 100, Page: page}}
			users, res, err := gh.client.Teams.ListTeamMembersBySlug(gh.ctx, org, team, listOptions)
			defer res.Body.Close()
			allUsers = append(allUsers, users...)
			return res, err
		}
		err := walkPaginatedApi(getMembers)
		if err != nil {
			fmt.Fprintf(gh.warningBuffer, "WARNING: Error fetching team members for %s/%s: %v\n", org, team, err)
		}
		return allUsers
	}
	gh.userReviewerMap = makeGHUserReviwerMap(reviewers, teamFetch)
	return nil
}

func (gh *Client) GetTokenUser() (string, error) {
	user, _, err := gh.client.Users.Get(gh.ctx, "")
	if err != nil {
		return "", err
	}
	return user.GetLogin(), nil
}

func (gh *Client) InitReviews() error {
	if gh.PR == nil {
		return &NoPRError{}
	}
	allReviews := make([]*github.PullRequestReview, 0)
	listReviews := func(page int) (*github.Response, error) {
		listOptions := &github.ListOptions{PerPage: 100, Page: page}
		reviews, res, err := gh.client.PullRequests.ListReviews(gh.ctx, gh.owner, gh.repo, gh.PR.GetNumber(), listOptions)
		defer res.Body.Close()
		allReviews = append(allReviews, reviews...)
		return res, err
	}
	err := walkPaginatedApi(listReviews)
	if err != nil {
		return err
	}
	allReviews = f.Filtered(allReviews, func(review *github.PullRequestReview) bool {
		return review.User.GetLogin() != gh.PR.User.GetLogin()
	})
	// use descending chronological order (default ascending)
	slices.Reverse(allReviews)
	gh.reviews = allReviews
	return nil
}

func (gh *Client) approvals() []*github.PullRequestReview {
	seen := make(map[string]bool, 0)
	approvals := f.Filtered(gh.reviews, func(approval *github.PullRequestReview) bool {
		userName := approval.GetUser().GetLogin()
		if _, ok := seen[userName]; ok {
			// we only care about the most recent reviews for each user
			return false
		} else {
			seen[userName] = true
		}
		return approval.GetState() == "APPROVED"
	})
	return approvals
}

func (gh *Client) AllApprovals() ([]*CurrentApproval, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	return f.Map(gh.approvals(), func(approval *github.PullRequestReview) *CurrentApproval {
		return &CurrentApproval{approval.User.GetLogin(), approval.GetID(), nil, approval.GetCommitID()}
	}), nil
}

func (gh *Client) FindUserApproval(ghUser string) (*CurrentApproval, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	review, found := f.Find(gh.approvals(), func(review *github.PullRequestReview) bool {
		return review.GetUser().GetLogin() == ghUser
	})

	if !found {
		return nil, nil
	}
	return &CurrentApproval{review.User.GetLogin(), review.GetID(), nil, review.GetCommitID()}, nil
}

func (gh *Client) GetCurrentReviewerApprovals() ([]*CurrentApproval, error) {
	if gh.PR == nil {
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
		if reviewers, ok := userReviewerMap[reviewingUser]; ok {
			newApproval := &CurrentApproval{reviewingUser, review.GetID(), reviewers, review.GetCommitID()}
			filteredApprovals = append(filteredApprovals, newApproval)
		}
	}

	return filteredApprovals
}

func (gh *Client) GetAlreadyReviewed() ([]string, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	return reviewerAlreadyReviewed(gh.reviews, gh.userReviewerMap), nil
}

func reviewerAlreadyReviewed(reviews []*github.PullRequestReview, userReviewerMap ghUserReviewerMap) []string {
	reviewsReviewers := make(map[string]bool, len(reviews))
	for _, review := range reviews {
		reviewingUser := review.GetUser().GetLogin()
		if reviewers, ok := userReviewerMap[reviewingUser]; ok {
			for _, reviewer := range reviewers {
				reviewsReviewers[reviewer] = true
			}
		}
	}

	return slices.Collect(maps.Keys(reviewsReviewers))
}

func (gh *Client) GetCurrentlyRequested() ([]string, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	return currentlyRequested(gh.PR, gh.owner, gh.userReviewerMap), nil
}

func currentlyRequested(PR *github.PullRequest, owner string, userReviewerMap ghUserReviewerMap) []string {
	requestedUsers := f.Map(PR.RequestedReviewers, func(user *github.User) string {
		return user.GetLogin()
	})
	requestedTeams := f.Map(PR.RequestedTeams, func(team *github.Team) string {
		return fmt.Sprintf("%s/%s", owner, team.GetSlug())
	})
	requested := slices.Concat(requestedUsers, requestedTeams)
	reviewers := make([]string, 0, len(requested))
	for _, user := range requested {
		if reviewer, ok := userReviewerMap[user]; ok {
			reviewers = append(reviewers, reviewer...)
		}
	}
	return f.RemoveDuplicates(reviewers)
}

func (gh *Client) DismissStaleReviews(staleApprovals []*CurrentApproval) error {
	if gh.PR == nil {
		return &NoPRError{}
	}
	staleMessage := "Changes in owned files since last approval"
	for _, approval := range staleApprovals {
		dismissRequest := &github.PullRequestReviewDismissalRequest{Message: &staleMessage}
		_, res, err := gh.client.PullRequests.DismissReview(gh.ctx, gh.owner, gh.repo, gh.PR.GetNumber(), approval.ReviewID, dismissRequest)
		defer res.Body.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (gh *Client) RequestReviewers(reviewers []string) error {
	if gh.PR == nil {
		return &NoPRError{}
	}
	if len(reviewers) == 0 {
		return nil
	}
	indvidualReviewers, teamReviewers := splitReviewers(reviewers)
	reviewersRequest := github.ReviewersRequest{Reviewers: indvidualReviewers, TeamReviewers: teamReviewers}
	_, res, err := gh.client.PullRequests.RequestReviewers(gh.ctx, gh.owner, gh.repo, gh.PR.GetNumber(), reviewersRequest)
	defer res.Body.Close()
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

func (gh *Client) ApprovePR() error {
	if gh.PR == nil {
		return &NoPRError{}
	}
	createReviewOptions := &github.PullRequestReviewRequest{
		Event: github.String("APPROVE"),
		Body:  github.String("Codeowners reviews satisfied"),
	}
	_, res, err := gh.client.PullRequests.CreateReview(gh.ctx, gh.owner, gh.repo, gh.PR.GetNumber(), createReviewOptions)
	defer res.Body.Close()
	return err
}

func (gh *Client) InitComments() error {
	if gh.PR == nil {
		return &NoPRError{}
	}
	allComments := make([]*github.IssueComment, 0)
	listReviews := func(page int) (*github.Response, error) {
		listOptions := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100, Page: page}}
		comments, res, err := gh.client.Issues.ListComments(gh.ctx, gh.owner, gh.repo, gh.PR.GetNumber(), listOptions)
		defer res.Body.Close()
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

func (gh *Client) AddComment(comment string) error {
	if gh.PR == nil {
		return &NoPRError{}
	}
	createCommentOptions := &github.IssueComment{
		Body: &comment,
	}
	_, res, err := gh.client.Issues.CreateComment(gh.ctx, gh.owner, gh.repo, gh.PR.GetNumber(), createCommentOptions)
	defer res.Body.Close()
	return err
}

func (gh *Client) IsInComments(comment string, since *time.Time) (bool, error) {
	if gh.PR == nil {
		return false, &NoPRError{}
	}
	if gh.comments == nil {
		if err := gh.InitComments(); err != nil {
			fmt.Fprintf(gh.warningBuffer, "WARNING: Error initializing comments: %v\n", err)
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

func (gh *Client) IsSubstringInComments(substring string, since *time.Time) (bool, error) {
	if gh.PR == nil {
		return false, &NoPRError{}
	}
	if gh.comments == nil {
		if err := gh.InitComments(); err != nil {
			fmt.Fprintf(gh.warningBuffer, "WARNING: Error initializing comments: %v\n", err)
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

// Apply approver satisfaction to the owners map, and return the approvals which should be invalidated
func (gh *Client) CheckApprovals(
	fileReviewerMap map[string][]string,
	approvals []*CurrentApproval,
	originalDiff git.Diff,
) (approvers []string, staleApprovals []*CurrentApproval) {
	appovalsWithDiff, badApprovals := getApprovalDiffs(approvals, originalDiff, gh.warningBuffer, gh.infoBuffer)
	approvers, staleApprovals = checkStale(fileReviewerMap, appovalsWithDiff)
	return approvers, append(badApprovals, staleApprovals...)
}

type ghUserReviewerMap map[string][]string

type CurrentApproval struct {
	GHLogin   string
	ReviewID  int64
	Reviewers []string
	CommitID  string
}

func (p *CurrentApproval) String() string {
	return fmt.Sprintf("%+v", *p)
}

func makeGHUserReviwerMap(reviewers []string, teamFetcher func(string, string) []*github.User) ghUserReviewerMap {
	userReviewerMap := make(ghUserReviewerMap)

	insertReviewer := func(userName string, reviewer string) {
		reviewers, found := userReviewerMap[userName]
		if found {
			userReviewerMap[userName] = append(reviewers, reviewer)
		} else {
			userReviewerMap[userName] = []string{reviewer}
		}
	}

	for _, reviewer := range reviewers {
		reviewerString := reviewer[1:] // trim the @
		// Add the team or user to the map
		insertReviewer(reviewerString, reviewer)
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
			insertReviewer(user.GetLogin(), reviewer)
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
