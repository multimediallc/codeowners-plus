package owners

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-github/v63/github"
)

func setupReviews() *GitHubClient {
	reviews := []*github.PullRequestReview{
		{User: &github.User{Login: github.String("reviewer1")}, State: github.String("APPROVED"), ID: github.Int64(1), CommitID: github.String("commit1")},
		{User: &github.User{Login: github.String("reviewer2")}, State: github.String("REQUEST_CHANGES"), ID: github.Int64(2), CommitID: github.String("commit2")},
		{User: &github.User{Login: github.String("reviewer3")}, State: github.String("APPROVED"), ID: github.Int64(1), CommitID: github.String("commit1")},
		{User: &github.User{Login: github.String("reviewer4")}, State: github.String("APPROVED"), ID: github.Int64(3), CommitID: github.String("commit3")},
	}
	userReviewerMap := ghUserReviewerMap{
		"reviewer1": []string{"@a", "@b"},
		"reviewer2": []string{"@c"},
		"reviewer4": []string{"@e"},
	}
	gh := &GitHubClient{
		reviews:         reviews,
		userReviewerMap: userReviewerMap,
		PR:              &github.PullRequest{Number: github.Int(1)},
	}
	return gh
}

func TestApprovals(t *testing.T) {
	gh := setupReviews()
	approvals := gh.approvals()
	expectedApprovals := []*github.PullRequestReview{
		{User: &github.User{Login: github.String("reviewer1")}, State: github.String("APPROVED"), ID: github.Int64(1), CommitID: github.String("commit1")},
		{User: &github.User{Login: github.String("reviewer3")}, State: github.String("APPROVED"), ID: github.Int64(1), CommitID: github.String("commit1")},
		{User: &github.User{Login: github.String("reviewer4")}, State: github.String("APPROVED"), ID: github.Int64(3), CommitID: github.String("commit3")},
	}

	if len(approvals) != len(expectedApprovals) {
		t.Errorf("Expected %d current approval, got %d", len(expectedApprovals), len(approvals))
	}

	seen := make(map[int]bool)
	for _, approval := range approvals {
		for i, expected := range expectedApprovals {
			if approval.CommitID == expected.CommitID && approval.User.GetLogin() == expected.User.GetLogin() {
				seen[i] = true
			}
		}
	}
	for i, found := range seen {
		if !found {
			t.Errorf("Expected approval %+v to be found", expectedApprovals[i])
		}
	}
}

func TestCurrentApprovalsFromReviews(t *testing.T) {
	gh := setupReviews()
	currentApprovals, err := gh.GetCurrentReviewerApprovals()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expectedApprovals := []*CurrentApproval{
		{CommitID: "commit1", Reviewers: []string{"@a", "@b"}},
		{CommitID: "commit3", Reviewers: []string{"@e"}},
	}

	if len(currentApprovals) != len(expectedApprovals) {
		t.Errorf("Expected %d current approval, got %d", len(expectedApprovals), len(currentApprovals))
	}

	seen := make(map[int]bool)
	for _, approval := range currentApprovals {
		for i, expected := range expectedApprovals {
			if approval.CommitID == expected.CommitID && SlicesItemsMatch(approval.Reviewers, expected.Reviewers) {
				seen[i] = true
			}
		}
	}
	for i, found := range seen {
		if !found {
			t.Errorf("Expected approval %+v to be found", expectedApprovals[i])
		}
	}
}

func TestAllApprovals(t *testing.T) {
	gh := setupReviews()
	currentApprovals, err := gh.AllApprovals()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedApprovals := []*CurrentApproval{
		{CommitID: "commit1", Reviewers: []string{"@a", "@b"}},
		{CommitID: "commit1", Reviewers: []string{"@a", "@d"}}, // reviewer3
		{CommitID: "commit3", Reviewers: []string{"@e"}},
	}

	if len(currentApprovals) != len(expectedApprovals) {
		t.Errorf("Expected %d current approval, got %d", len(expectedApprovals), len(currentApprovals))
	}

	seen := make(map[int]bool)
	for _, approval := range currentApprovals {
		for i, expected := range expectedApprovals {
			if approval.CommitID == expected.CommitID && SlicesItemsMatch(approval.Reviewers, expected.Reviewers) {
				seen[i] = true
			}
		}
	}
	for i, found := range seen {
		if !found {
			t.Errorf("Expected approval %+v to be found", expectedApprovals[i])
		}
	}
}

func TestFindUserApproval(t *testing.T) {
	gh := setupReviews()
	approval, err := gh.FindUserApproval("reviewer1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if approval.CommitID != "commit1" {
		t.Errorf("Expected approval commit ID to be commit1, got %s", approval.CommitID)
	}
	// Reviewers is always nil for FindUserApproval
	if approval.Reviewers != nil {
		t.Errorf("Expected reviewers to be nil, got %v", approval.Reviewers)
	}
}

func TestGetAlreadyReviewed(t *testing.T) {
	gh := setupReviews()
	alreadyReviewed, err := gh.GetAlreadyReviewed()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := []string{"@a", "@b", "@c", "@e"}
	if len(alreadyReviewed) != len(expected) {
		t.Errorf("Expected %d reviewers, got %d", len(expected), len(alreadyReviewed))
	}
	if !SlicesItemsMatch(alreadyReviewed, expected) {
		t.Errorf("Expected reviewers to be %v, got %v", expected, alreadyReviewed)
	}
}

func TestCurrentlyRequested(t *testing.T) {
	userReviewerMap := ghUserReviewerMap{
		"user1":     []string{"@a", "@b"},
		"user2":     []string{"@c"},
		"user3":     []string{"@d"},
		"org/team1": []string{"@e", "@a"},
		"org/team2": []string{"@f"},
	}
	pr := &github.PullRequest{
		RequestedReviewers: []*github.User{
			{Login: github.String("user1")},
			{Login: github.String("user2")},
		},
		RequestedTeams: []*github.Team{
			{Slug: github.String("team1")},
			{Slug: github.String("team2")},
		},
	}

	gh := &GitHubClient{
		PR:              pr,
		owner:           "org",
		userReviewerMap: userReviewerMap,
	}

	requested, err := gh.GetCurrentlyRequested()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := []string{"@a", "@b", "@c", "@e", "@f"}

	if len(requested) != len(expected) {
		t.Errorf("Expected %d requested reviewers, got %d", len(expected), len(requested))
	}
	if !SlicesItemsMatch(requested, expected) {
		t.Errorf("Expected requested reviewers to be %v, got %v", expected, requested)
	}
}

func TestSplitReviewers(t *testing.T) {
	reviewers := []string{"@user1", "@user2", "@user3", "@org/team1", "@org/team2"}
	individuals, teams := splitReviewers(reviewers)
	if !SlicesItemsMatch(individuals, []string{"user1", "user2", "user3"}) {
		t.Errorf("Expected individuals to be [user1, user2, user3], got %v", individuals)
	}
	if !SlicesItemsMatch(teams, []string{"team1", "team2"}) {
		t.Errorf("Expected teams to be [team1, team2], got %v", teams)
	}
}

func TestMakeGHUserReviewerMap(t *testing.T) {
	teamFetcher := func(org, team string) []*github.User {
		if org != "org" {
			return []*github.User{}
		}
		if team == "team1" {
			return []*github.User{
				{Login: github.String("user1")},
				{Login: github.String("user3")},
			}
		}
		return []*github.User{
			{Login: github.String("user1")},
		}
	}
	userReviewerMap := makeGHUserReviwerMap([]string{"@user1", "@user2", "@org/team1", "@org/team2", "@other/teamX"}, teamFetcher)
	expectedUserReviewerMap := ghUserReviewerMap{
		"user1":       []string{"@user1", "@org/team1", "@org/team2"},
		"user2":       []string{"@user2"},
		"user3":       []string{"@org/team1"},
		"org/team1":   []string{"@org/team1"},
		"org/team2":   []string{"@org/team2"},
		"other/teamX": []string{"@other/teamX"},
	}

	if !reflect.DeepEqual(userReviewerMap, expectedUserReviewerMap) {
		t.Errorf("Expected user reviewer map to be %v, got %v", expectedUserReviewerMap, userReviewerMap)
	}
}

func TestIsInComments(t *testing.T) {
	gh := &GitHubClient{
		PR: &github.PullRequest{Number: github.Int(1)},
		comments: []*github.IssueComment{
			{Body: github.String("comment1"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -2)}},
			{Body: github.String("comment2"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -1)}},
			{Body: github.String("comment3"), CreatedAt: &github.Timestamp{Time: time.Now()}},
		},
	}

	tt := []struct {
		string string
		since  *time.Time
		found  bool
	}{
		{"comment1", nil, true},
		{"comment2", nil, true},
		{"comment3", nil, true},
		{"comment4", nil, false},
		{"comment1", &gh.comments[1].CreatedAt.Time, false},
		{"comment2", &gh.comments[1].CreatedAt.Time, true},
		{"comment3", &gh.comments[1].CreatedAt.Time, true},
	}

	for i, tc := range tt {
		found, err := gh.IsInComments(tc.string, tc.since)
		if err != nil {
			t.Errorf("Case %d: Unexpected error: %v", i, err)
		}
		if found != tc.found {
			t.Errorf("Case %d: Expected %t, got %t", i, tc.found, found)
		}
	}
}

func TestIsSubstringInComments(t *testing.T) {
	gh := &GitHubClient{
		PR: &github.PullRequest{Number: github.Int(1)},
		comments: []*github.IssueComment{
			{Body: github.String("part1 part4"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -2)}},
			{Body: github.String("part2 part5"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -1)}},
			{Body: github.String("part3 part6"), CreatedAt: &github.Timestamp{Time: time.Now()}},
		},
	}

	tt := []struct {
		string string
		since  *time.Time
		found  bool
	}{
		{"part1", nil, true},
		{"part2", nil, true},
		{"part3", nil, true},
		{"part4", nil, true},
		{"part5", nil, true},
		{"part6", nil, true},
		{"part7", nil, false},
		{"part1", &gh.comments[1].CreatedAt.Time, false},
		{"part4", &gh.comments[1].CreatedAt.Time, false},
		{"part2", &gh.comments[1].CreatedAt.Time, true},
		{"part5", &gh.comments[1].CreatedAt.Time, true},
		{"part3", &gh.comments[1].CreatedAt.Time, true},
		{"part6", &gh.comments[1].CreatedAt.Time, true},
	}

	for i, tc := range tt {
		found, err := gh.IsSubstringInComments(tc.string, tc.since)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if found != tc.found {
			t.Errorf("Case %d: Expected found to be %t, got %t", i, tc.found, found)
		}
	}
}

func TestNewGithubClient(t *testing.T) {
	client := NewGithubClient("owner", "repo", "token")
	if client.owner != "owner" {
		t.Errorf("Expected owner to be owner, got %s", client.owner)
	}
	if client.repo != "repo" {
		t.Errorf("Expected repo to be repo, got %s", client.repo)
	}
	if client.client == nil {
		t.Error("Expected client to be non-nil")
	}
	if client.PR != nil {
		t.Error("Expected PR to be nil")
	}
	if client.userReviewerMap != nil {
		t.Error("Expected userReviewerMap to be nil")
	}
}

func TestNilPRErr(t *testing.T) {
	gh := &GitHubClient{}
	tt := []func() (string, error){
		func() (string, error) {
			_, err := gh.GetCurrentReviewerApprovals()
			return "GetCurrentApprovals", err
		},
		func() (string, error) {
			_, err := gh.AllApprovals()
			return "IsSubstringInComments", err
		},
		func() (string, error) {
			err := gh.ApprovePR()
			return "ApprovePR", err
		},
		func() (string, error) {
			_, err := gh.FindUserApproval("user")
			return "FindUserApproval", err
		},
		func() (string, error) {
			_, err := gh.GetCurrentlyRequested()
			return "GetCurrentlyRequested", err
		},
		func() (string, error) {
			err := gh.InitReviews()
			return "InitReviews", err
		},
		func() (string, error) {
			err := gh.InitComments()
			return "InitComments", err
		},
		func() (string, error) {
			err := gh.DismissStaleReviews([]*CurrentApproval{})
			return "DismissStaleReviews", err
		},
		func() (string, error) {
			err := gh.RequestReviewers([]string{})
			return "RequestReviewers", err
		},
		func() (string, error) {
			err := gh.AddComment("comment")
			return "AddComment", err
		},
		func() (string, error) {
			_, err := gh.IsInComments("comment", nil)
			return "IsInComments", err
		},
		func() (string, error) {
			_, err := gh.IsSubstringInComments("comment", nil)
			return "IsSubstringInComments", err
		},
	}
	for _, tc := range tt {
		s, err := tc()
		if err == nil {
			t.Errorf("%s: Expected error for nil user reviewer map", s)
		}
		if _, ok := err.(*NoPRError); !ok {
			t.Errorf("%s: Expected NoPRError, got %T", s, err)
		}
	}
}

func TestNilUserReviewerMapErr(t *testing.T) {
	gh := &GitHubClient{
		PR: &github.PullRequest{Number: github.Int(1)},
	}
	tt := []func() (string, error){
		func() (string, error) {
			_, err := gh.GetCurrentReviewerApprovals()
			return "GetCurrentApprovals", err
		},
		func() (string, error) {
			_, err := gh.GetCurrentlyRequested()
			return "GetCurrentlyRequested", err
		},
	}
	for _, tc := range tt {
		s, err := tc()
		if err == nil {
			t.Errorf("%s: Expected error for nil user reviewer map", s)
		}
		if _, ok := err.(*UserReviewerMapNotInitError); !ok {
			t.Errorf("%s: Expected NoUserReviewerMapError, got %T", s, err)
		}
	}
}

func TestNilReviewsErr(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(1)}
	gh.userReviewerMap = make(ghUserReviewerMap)

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	tt := []func() (string, error){
		func() (string, error) {
			_, err := gh.AllApprovals()
			return "AllApprovals", err
		},
		func() (string, error) {
			_, err := gh.GetCurrentReviewerApprovals()
			return "GetCurrentReviewerApprovals", err
		},
		func() (string, error) {
			_, err := gh.FindUserApproval("user")
			return "FindUserApproval", err
		},
	}
	for _, tc := range tt {
		gh.reviews = nil
		s, err := tc()
		if err == nil {
			t.Errorf("%s: Expected error for nil reviews", s)
		}
		if _, ok := err.(*github.ErrorResponse); !ok {
			t.Errorf("%s: Expected ErrorResponse, got %T", s, err)
		}
	}
}

func TestNilCommentsErr(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(1)}
	gh.userReviewerMap = make(ghUserReviewerMap)

	mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	tt := []func() (string, bool, error){
		func() (string, bool, error) {
			found, err := gh.IsInComments("", nil)
			return "IsInComments", found, err
		},
		func() (string, bool, error) {
			found, err := gh.IsSubstringInComments("", nil)
			return "IsSubstringInComments", found, err
		},
	}
	for _, tc := range tt {
		gh.comments = nil
		s, exists, err := tc()
		if err != nil {
			t.Errorf("%s: Expected error for nil comments", s)
		}
		if exists {
			t.Errorf("%s: Expected no comment found", s)
		}
	}
}

func mockServerAndClient(t *testing.T) (*http.ServeMux, *httptest.Server, *GitHubClient) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	client := github.NewClient(nil)
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client.BaseURL = baseURL
	gh := &GitHubClient{
		ctx:    context.Background(),
		owner:  "test-owner",
		repo:   "test-repo",
		client: client,
	}
	return mux, server, gh

}

func TestInitPRSuccess(t *testing.T) {
	// Mock server to handle requests
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	prID := 123
	mockPR := &github.PullRequest{Number: github.Int(prID)}

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockPR)
	})

	err := gh.InitPR(prID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if gh.PR == nil {
		t.Error("expected PR to be initialized, got nil")
	} else if gh.PR.GetNumber() != prID {
		t.Errorf("expected PR number %d, got %d", prID, gh.PR.GetNumber())
	}
}

func TestInitPRFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = nil // Reset PR

	prID := 999

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/999", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	})

	err := gh.InitPR(prID)
	if err == nil {
		t.Error("expected an error, got nil")
	}
	if gh.PR != nil {
		t.Errorf("expected PR to be nil, got %+v", gh.PR)
	}
}

func TestGetTokenUserSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	mockUser := &github.User{Login: github.String("test-user")}

	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockUser)
	})

	user, err := gh.GetTokenUser()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if user != "test-user" {
		t.Errorf("expected user 'test-user', got '%s'", user)
	}
}

func TestGetTokenUserFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	user, err := gh.GetTokenUser()
	if err == nil {
		t.Error("expected an error, got nil")
	}
	if user != "" {
		t.Errorf("expected empty user, got '%s'", user)
	}
}

func TestInitReviewsSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}
	mockReviews := []*github.PullRequestReview{
		{ID: github.Int64(1)},
		{ID: github.Int64(2)},
	}

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockReviews)
	})

	err := gh.InitReviews()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(gh.reviews) != 2 {
		t.Errorf("expected 2 reviews, got %d", len(gh.reviews))
	}
}

func TestInitReviewsFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	err := gh.InitReviews()
	if err == nil {
		t.Error("expected an error, got nil")
	}
	if gh.reviews != nil {
		t.Errorf("expected reviews to be nil, got %+v", gh.reviews)
	}
}

func TestDismissStaleReviewsSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	staleApprovals := []*CurrentApproval{
		{ReviewID: 1},
		{ReviewID: 2},
	}

	// Mock the GitHub API for dismissal
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews/1/dismissals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected method PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews/2/dismissals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected method PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	err := gh.DismissStaleReviews(staleApprovals)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDismissStaleReviewsFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}
	staleApprovals := []*CurrentApproval{
		{ReviewID: 1},
	}

	// Mock the GitHub API to simulate an error
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews/1/dismissals", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	err := gh.DismissStaleReviews(staleApprovals)
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestRequestReviewersSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	reviewers := []string{"@reviewer1", "@org/team1"}

	// Mock the GitHub API endpoint
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/requested_reviewers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected method POST, got %s", r.Method)
		}

		// Validate the request payload
		var req github.ReviewersRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if len(req.Reviewers) != 1 || req.Reviewers[0] != "reviewer1" {
			t.Errorf("expected reviewers [reviewer1], got %v", req.Reviewers)
		}
		if len(req.TeamReviewers) != 1 || req.TeamReviewers[0] != "team1" {
			t.Errorf("expected team reviewers [team1], got %v", req.TeamReviewers)
		}

		w.WriteHeader(http.StatusCreated)
	})

	err := gh.RequestReviewers(reviewers)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRequestReviewersFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	reviewers := []string{"@reviewer1", "@org/team1"}

	// Mock the GitHub API to simulate an error
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/requested_reviewers", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	err := gh.RequestReviewers(reviewers)
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestApprovePRSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	// Mock the GitHub API endpoint
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected method POST, got %s", r.Method)
		}

		// Validate the request payload
		var req github.PullRequestReviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if *req.Event != "APPROVE" {
			t.Errorf("expected Event 'APPROVE', got '%s'", *req.Event)
		}
		if *req.Body != "Codeowners reviews satisfied" {
			t.Errorf("expected Body 'Codeowners reviews satisfied', got '%s'", *req.Body)
		}

		w.WriteHeader(http.StatusCreated)
	})

	err := gh.ApprovePR()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApprovePRFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	// Mock the GitHub API to simulate an error
	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	err := gh.ApprovePR()
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestInitCommentsSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	mockComments := []*github.IssueComment{
		{ID: github.Int64(1), Body: github.String("Comment 1")},
		{ID: github.Int64(2), Body: github.String("Comment 2")},
	}

	mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockComments)
	})

	err := gh.InitComments()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(gh.comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(gh.comments))
	}
}

func TestInitCommentsFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	err := gh.InitComments()
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestAddCommentSuccess(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	// Mock the GitHub API endpoint
	mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected method POST, got %s", r.Method)
		}

		// Validate the request payload
		var req github.IssueComment
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if *req.Body != "test comment" {
			t.Errorf("expected Body 'test comment', got '%s'", *req.Body)
		}

		w.WriteHeader(http.StatusCreated)
	})

	err := gh.AddComment("test comment")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddCommentFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	// Mock the GitHub API to simulate an error
	mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	err := gh.AddComment("test comment")
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestInitUserReviewerMap(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.PR = &github.PullRequest{Number: github.Int(123)}

	// Mock the GitHub API endpoint
	reviewers := []string{"@org1/team1", "@user1", "@org2/team2"}

	// Mock team members for org1/team1
	mux.HandleFunc("/orgs/org1/teams/team1/members", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]*github.User{
			{Login: github.String("team_member1")},
			{Login: github.String("team_member2")},
		})
	})

	// Mock team members for org2/team2
	mux.HandleFunc("/orgs/org2/teams/team2/members", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]*github.User{
			{Login: github.String("team_member1")},
		})
	})

	err := gh.InitUserReviewerMap(reviewers)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Validate the userReviewerMap
	expectedMap := ghUserReviewerMap{
		"user1":        []string{"@user1"},
		"team_member1": []string{"@org1/team1", "@org2/team2"},
		"team_member2": []string{"@org1/team1"},
	}

	for user := range expectedMap {
		if !SlicesItemsMatch(gh.userReviewerMap[user], expectedMap[user]) {
			t.Errorf("expected user %s to be in userReviewerMap", user)
		}
	}
}
