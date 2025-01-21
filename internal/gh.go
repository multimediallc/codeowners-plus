package owners

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v63/github"
)

type NoPRError struct{}

func (e NoPRError) Error() string {
	return "PR not initialized"
}

type UserReviewerMapNotInitError struct{}

func (e UserReviewerMapNotInitError) Error() string {
	return "User reviewer map not initialized"
}

type GitHubClient struct {
	ctx             context.Context
	owner           string
	repo            string
	client          *github.Client
	PR              *github.PullRequest
	userReviewerMap ghUserReviewerMap
	comments        []*github.IssueComment
	reviews         []*github.PullRequestReview
}

func NewGithubClient(owner, repo, token string) *GitHubClient {
	client := github.NewClient(nil).WithAuthToken(token)
	return &GitHubClient{
		context.Background(),
		owner,
		repo,
		client,
		nil,
		nil,
		nil,
		nil,
	}
}

func (gh *GitHubClient) InitPR(pr_id int) error {
	pull, res, err := gh.client.PullRequests.Get(gh.ctx, gh.owner, gh.repo, pr_id)
	defer res.Body.Close()
	if err != nil {
		return err
	}
	gh.PR = pull
	return nil
}

func (gh *GitHubClient) InitUserReviewerMap(reviewers []string) error {
	teamFetch := func(org, team string) []*github.User {
		fmt.Fprintf(InfoBuffer, "Fetching team members for %s/%s\n", org, team)
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
			fmt.Fprintf(WarningBuffer, "WARNING: Error fetching team members for %s/%s: %v\n", org, team, err)
		}
		return allUsers
	}
	gh.userReviewerMap = makeGHUserReviwerMap(reviewers, teamFetch)
	return nil
}

func (gh *GitHubClient) GetTokenUser() (string, error) {
	user, _, err := gh.client.Users.Get(gh.ctx, "")
	if err != nil {
		return "", err
	}
	return user.GetLogin(), nil
}

func (gh *GitHubClient) InitReviews() error {
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
	gh.reviews = allReviews
	return nil
}

func (gh *GitHubClient) approvals() []*github.PullRequestReview {
	approvals := Filtered(gh.reviews, func(approval *github.PullRequestReview) bool {
		return approval.GetState() == "APPROVED"
	})
	return approvals
}

func (gh *GitHubClient) AllApprovals() ([]*CurrentApproval, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	return Map(gh.approvals(), func(approval *github.PullRequestReview) *CurrentApproval {
		return &CurrentApproval{approval.User.GetLogin(), approval.GetID(), nil, approval.GetCommitID()}
	}), nil
}

func (gh *GitHubClient) FindUserApproval(ghUser string) (*CurrentApproval, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.reviews == nil {
		err := gh.InitReviews()
		if err != nil {
			return nil, err
		}
	}
	review, found := Find(gh.approvals(), func(review *github.PullRequestReview) bool {
		return review.GetUser().GetLogin() == ghUser
	})

	if !found {
		return nil, nil
	}
	return &CurrentApproval{review.User.GetLogin(), review.GetID(), nil, review.GetCommitID()}, nil
}

func (gh *GitHubClient) GetCurrentReviewerApprovals() ([]*CurrentApproval, error) {
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

func (gh *GitHubClient) GetAlreadyReviewed() ([]string, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	return reviewerAlreadyReviewed(gh.reviews, gh.userReviewerMap), nil
}

func reviewerAlreadyReviewed(reviews []*github.PullRequestReview, userReviewerMap ghUserReviewerMap) []string {
	reviewsReviewers := make([]string, 0, len(reviews))
	for _, review := range reviews {
		reviewingUser := review.GetUser().GetLogin()
		if reviewers, ok := userReviewerMap[reviewingUser]; ok {
			reviewsReviewers = append(reviewsReviewers, reviewers...)
		}
	}

	return reviewsReviewers
}

func (gh *GitHubClient) GetCurrentlyRequested() ([]string, error) {
	if gh.PR == nil {
		return nil, &NoPRError{}
	}
	if gh.userReviewerMap == nil {
		return nil, &UserReviewerMapNotInitError{}
	}
	return currentlyRequested(gh.PR, gh.owner, gh.userReviewerMap), nil
}

func currentlyRequested(PR *github.PullRequest, owner string, userReviewerMap ghUserReviewerMap) []string {
	requestedUsers := Map(PR.RequestedReviewers, func(user *github.User) string {
		return user.GetLogin()
	})
	requestedTeams := Map(PR.RequestedTeams, func(team *github.Team) string {
		return fmt.Sprintf("%s/%s", owner, team.GetSlug())
	})
	requested := slices.Concat(requestedUsers, requestedTeams)
	reviewers := make([]string, 0, len(requested))
	for _, user := range requested {
		if reviewer, ok := userReviewerMap[user]; ok {
			reviewers = append(reviewers, reviewer...)
		}
	}
	return RemoveDuplicates(reviewers)
}

func (gh *GitHubClient) DismissStaleReviews(staleApprovals []*CurrentApproval) error {
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

func (gh *GitHubClient) RequestReviewers(reviewers []string) error {
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

func (gh *GitHubClient) ApprovePR() error {
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

func (gh *GitHubClient) InitComments() error {
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

func (gh *GitHubClient) AddComment(comment string) error {
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

func (gh *GitHubClient) IsInComments(comment string, since *time.Time) (bool, error) {
	if gh.PR == nil {
		return false, &NoPRError{}
	}
	if gh.comments == nil {
		if err := gh.InitComments(); err != nil {
			fmt.Fprintf(WarningBuffer, "WARNING: Error initializing comments: %v\n", err)
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

func (gh *GitHubClient) IsSubstringInComments(substring string, since *time.Time) (bool, error) {
	if gh.PR == nil {
		return false, &NoPRError{}
	}
	if gh.comments == nil {
		if err := gh.InitComments(); err != nil {
			fmt.Fprintf(WarningBuffer, "WARNING: Error initializing comments: %v\n", err)
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
