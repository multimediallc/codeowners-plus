package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-github/v63/github"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

func setupReviews() *GHClient {
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
	gh := &GHClient{
		reviews:         reviews,
		userReviewerMap: userReviewerMap,
		pr:              &github.PullRequest{Number: github.Int(1)},
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
		{CommitID: "commit3", Reviewers: []string{}},
	}

	if len(currentApprovals) != len(expectedApprovals) {
		t.Errorf("Expected %d current approval, got %d", len(expectedApprovals), len(currentApprovals))
	}

	seen := make(map[int]bool)
	for _, approval := range currentApprovals {
		for i, expected := range expectedApprovals {
			if approval.CommitID == expected.CommitID && f.SlicesItemsMatch(approval.Reviewers, expected.Reviewers) {
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
			if approval.CommitID == expected.CommitID && f.SlicesItemsMatch(approval.Reviewers, expected.Reviewers) {
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
	if !f.SlicesItemsMatch(alreadyReviewed, expected) {
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

	gh := &GHClient{
		pr:              pr,
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
	if !f.SlicesItemsMatch(requested, expected) {
		t.Errorf("Expected requested reviewers to be %v, got %v", expected, requested)
	}
}

func TestSplitReviewers(t *testing.T) {
	reviewers := []string{"@user1", "@user2", "@user3", "@org/team1", "@org/team2"}
	individuals, teams := splitReviewers(reviewers)
	if !f.SlicesItemsMatch(individuals, []string{"user1", "user2", "user3"}) {
		t.Errorf("Expected individuals to be [user1, user2, user3], got %v", individuals)
	}
	if !f.SlicesItemsMatch(teams, []string{"team1", "team2"}) {
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
		"other/teamx": []string{"@other/teamX"}, // Key is normalized to lowercase
	}

	if !reflect.DeepEqual(userReviewerMap, expectedUserReviewerMap) {
		t.Errorf("Expected user reviewer map to be %v, got %v", expectedUserReviewerMap, userReviewerMap)
	}
}

func TestIsInComments(t *testing.T) {
	gh := &GHClient{
		pr: &github.PullRequest{Number: github.Int(1)},
		comments: []*github.IssueComment{
			{Body: github.String("comment1"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -2)}},
			{Body: github.String("comment2"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -1)}},
			{Body: github.String("comment3"), CreatedAt: &github.Timestamp{Time: time.Now()}},
		},
	}

	tt := []struct {
		name   string
		string string
		since  *time.Time
		found  bool
	}{
		{name: "find comment1 with no time filter", string: "comment1", since: nil, found: true},
		{name: "find comment2 with no time filter", string: "comment2", since: nil, found: true},
		{name: "find comment3 with no time filter", string: "comment3", since: nil, found: true},
		{name: "non-existent comment", string: "comment4", since: nil, found: false},
		{name: "comment1 filtered by time", string: "comment1", since: &gh.comments[1].CreatedAt.Time, found: false},
		{name: "comment2 filtered by time", string: "comment2", since: &gh.comments[1].CreatedAt.Time, found: true},
		{name: "comment3 filtered by time", string: "comment3", since: &gh.comments[1].CreatedAt.Time, found: true},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			found, err := gh.IsInComments(tc.string, tc.since)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if found != tc.found {
				t.Errorf("Expected found to be %t, got %t", tc.found, found)
			}
		})
	}
}

func TestIsSubstringInComments(t *testing.T) {
	gh := &GHClient{
		pr: &github.PullRequest{Number: github.Int(1)},
		comments: []*github.IssueComment{
			{Body: github.String("part1 part4"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -2)}},
			{Body: github.String("part2 part5"), CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -1)}},
			{Body: github.String("part3 part6"), CreatedAt: &github.Timestamp{Time: time.Now()}},
		},
	}

	tt := []struct {
		name   string
		string string
		since  *time.Time
		found  bool
	}{
		{name: "find part1 with no time filter", string: "part1", since: nil, found: true},
		{name: "find part2 with no time filter", string: "part2", since: nil, found: true},
		{name: "find part3 with no time filter", string: "part3", since: nil, found: true},
		{name: "find part4 with no time filter", string: "part4", since: nil, found: true},
		{name: "find part5 with no time filter", string: "part5", since: nil, found: true},
		{name: "find part6 with no time filter", string: "part6", since: nil, found: true},
		{name: "non-existent part", string: "part7", since: nil, found: false},
		{name: "part1 filtered by time", string: "part1", since: &gh.comments[1].CreatedAt.Time, found: false},
		{name: "part4 filtered by time", string: "part4", since: &gh.comments[1].CreatedAt.Time, found: false},
		{name: "part2 filtered by time", string: "part2", since: &gh.comments[1].CreatedAt.Time, found: true},
		{name: "part5 filtered by time", string: "part5", since: &gh.comments[1].CreatedAt.Time, found: true},
		{name: "part3 filtered by time", string: "part3", since: &gh.comments[1].CreatedAt.Time, found: true},
		{name: "part6 filtered by time", string: "part6", since: &gh.comments[1].CreatedAt.Time, found: true},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			found, err := gh.IsSubstringInComments(tc.string, tc.since)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if found != tc.found {
				t.Errorf("Expected found to be %t, got %t", tc.found, found)
			}
		})
	}
}

func TestNewGithubClient(t *testing.T) {
	client, ok := NewClient("owner", "repo", "token").(*GHClient)
	if !ok {
		t.Fatalf("Expected client to be of type *GHClient, got %T", client)
	}
	if client.owner != "owner" {
		t.Errorf("Expected owner to be owner, got %s", client.owner)
	}
	if client.repo != "repo" {
		t.Errorf("Expected repo to be repo, got %s", client.repo)
	}
	if client.client == nil {
		t.Error("Expected client to be non-nil")
	}
	if client.pr != nil {
		t.Error("Expected PR to be nil")
	}
	if client.userReviewerMap != nil {
		t.Error("Expected userReviewerMap to be nil")
	}
}

func TestNilPRErr(t *testing.T) {
	gh := &GHClient{}
	tt := []struct {
		name   string
		testFn func() error
	}{
		{
			name: "GetCurrentApprovals",
			testFn: func() error {
				_, err := gh.GetCurrentReviewerApprovals()
				return err
			},
		},
		{
			name: "AllApprovals",
			testFn: func() error {
				_, err := gh.AllApprovals()
				return err
			},
		},
		{
			name: "ApprovePR",
			testFn: func() error {
				return gh.ApprovePR()
			},
		},
		{
			name: "FindUserApproval",
			testFn: func() error {
				_, err := gh.FindUserApproval("user")
				return err
			},
		},
		{
			name: "GetCurrentlyRequested",
			testFn: func() error {
				_, err := gh.GetCurrentlyRequested()
				return err
			},
		},
		{
			name: "InitReviews",
			testFn: func() error {
				return gh.InitReviews()
			},
		},
		{
			name: "InitComments",
			testFn: func() error {
				return gh.InitComments()
			},
		},
		{
			name: "DismissStaleReviews",
			testFn: func() error {
				return gh.DismissStaleReviews([]*CurrentApproval{})
			},
		},
		{
			name: "RequestReviewers",
			testFn: func() error {
				return gh.RequestReviewers([]string{})
			},
		},
		{
			name: "AddComment",
			testFn: func() error {
				return gh.AddComment("comment")
			},
		},
		{
			name: "IsInComments",
			testFn: func() error {
				_, err := gh.IsInComments("comment", nil)
				return err
			},
		},
		{
			name: "IsSubstringInComments",
			testFn: func() error {
				_, err := gh.IsSubstringInComments("comment", nil)
				return err
			},
		},
		{
			name: "IsInLabels",
			testFn: func() error {
				_, err := gh.IsInLabels([]string{"label"})
				return err
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.testFn()
			if err == nil {
				t.Error("Expected error for nil PR")
			}
			if _, ok := err.(*NoPRError); !ok {
				t.Errorf("Expected NoPRError, got %T", err)
			}
		})
	}
}

func TestNilUserReviewerMapErr(t *testing.T) {
	gh := &GHClient{
		pr: &github.PullRequest{Number: github.Int(1)},
	}
	tt := []struct {
		name   string
		testFn func() error
	}{
		{
			name: "GetCurrentApprovals",
			testFn: func() error {
				_, err := gh.GetCurrentReviewerApprovals()
				return err
			},
		},
		{
			name: "GetCurrentlyRequested",
			testFn: func() error {
				_, err := gh.GetCurrentlyRequested()
				return err
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.testFn()
			if err == nil {
				t.Error("Expected error for nil user reviewer map")
			}
			if _, ok := err.(*UserReviewerMapNotInitError); !ok {
				t.Errorf("Expected NoUserReviewerMapError, got %T", err)
			}
		})
	}
}

func TestNilReviewsErr(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.pr = &github.PullRequest{Number: github.Int(1)}
	gh.userReviewerMap = make(ghUserReviewerMap)

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/123/reviews", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	tt := []struct {
		name   string
		testFn func() error
	}{
		{
			name: "AllApprovals",
			testFn: func() error {
				_, err := gh.AllApprovals()
				return err
			},
		},
		{
			name: "GetCurrentReviewerApprovals",
			testFn: func() error {
				_, err := gh.GetCurrentReviewerApprovals()
				return err
			},
		},
		{
			name: "FindUserApproval",
			testFn: func() error {
				_, err := gh.FindUserApproval("user")
				return err
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			gh.reviews = nil
			err := tc.testFn()
			if err == nil {
				t.Error("Expected error for nil reviews")
			}
			if _, ok := err.(*github.ErrorResponse); !ok {
				t.Errorf("Expected ErrorResponse, got %T", err)
			}
		})
	}
}

func TestNilCommentsErr(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.pr = &github.PullRequest{Number: github.Int(1)}
	gh.userReviewerMap = make(ghUserReviewerMap)

	mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	tt := []struct {
		name   string
		testFn func() (bool, error)
	}{
		{
			name: "IsInComments",
			testFn: func() (bool, error) {
				return gh.IsInComments("", nil)
			},
		},
		{
			name: "IsSubstringInComments",
			testFn: func() (bool, error) {
				return gh.IsSubstringInComments("", nil)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			gh.comments = nil
			exists, err := tc.testFn()
			if err != nil {
				t.Errorf("Expected no error for nil comments, got: %v", err)
			}
			if exists {
				t.Error("Expected no comment found")
			}
		})
	}
}

func mockServerAndClient(t *testing.T) (*http.ServeMux, *httptest.Server, *GHClient) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	client := github.NewClient(nil)
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client.BaseURL = baseURL
	gh := &GHClient{
		ctx:           context.Background(),
		owner:         "test-owner",
		repo:          "test-repo",
		client:        client,
		infoBuffer:    io.Discard,
		warningBuffer: io.Discard,
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
	if gh.pr == nil {
		t.Error("expected PR to be initialized, got nil")
	} else if gh.pr.GetNumber() != prID {
		t.Errorf("expected PR number %d, got %d", prID, gh.pr.GetNumber())
	}
}

func TestInitPRFailure(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	gh.pr = nil // Reset PR

	prID := 999

	mux.HandleFunc("/repos/test-owner/test-repo/pulls/999", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	})

	err := gh.InitPR(prID)
	if err == nil {
		t.Error("expected an error, got nil")
	}
	if gh.pr != nil {
		t.Errorf("expected PR to be nil, got %+v", gh.pr)
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

	gh.pr = &github.PullRequest{Number: github.Int(123)}
	mockReviews := []*github.PullRequestReview{
		{User: &github.User{Login: github.String("test")}, ID: github.Int64(1)},
		{User: &github.User{Login: github.String("test")}, ID: github.Int64(2)},
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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}
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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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

	gh.pr = &github.PullRequest{Number: github.Int(123)}

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
		if !f.SlicesItemsMatch(gh.userReviewerMap[user], expectedMap[user]) {
			t.Errorf("expected user %s to be in userReviewerMap", user)
		}
	}
}

func TestIsInLabels(t *testing.T) {
	tt := []struct {
		name        string
		pr          *github.PullRequest
		labels      []string
		expected    bool
		expectError bool
		failMessage string
	}{
		{
			name: "has matching label",
			pr: &github.PullRequest{
				Labels: []*github.Label{
					{Name: github.String("high-priority")},
				},
			},
			labels:      []string{"high-priority"},
			expected:    true,
			expectError: false,
			failMessage: "Should detect matching label",
		},
		{
			name: "has multiple labels but no match",
			pr: &github.PullRequest{
				Labels: []*github.Label{
					{Name: github.String("bug")},
					{Name: github.String("enhancement")},
				},
			},
			labels:      []string{"high-priority"},
			expected:    false,
			expectError: false,
			failMessage: "Should not detect label when not present",
		},
		{
			name: "empty labels list",
			pr: &github.PullRequest{
				Labels: []*github.Label{
					{Name: github.String("high-priority")},
				},
			},
			labels:      []string{},
			expected:    false,
			expectError: false,
			failMessage: "Should return false for empty labels list",
		},
		{
			name: "multiple target labels",
			pr: &github.PullRequest{
				Labels: []*github.Label{
					{Name: github.String("urgent")},
				},
			},
			labels:      []string{"high-priority", "urgent"},
			expected:    true,
			expectError: false,
			failMessage: "Should detect any of the target labels",
		},
		{
			name:        "nil PR",
			pr:          nil,
			labels:      []string{"high-priority"},
			expected:    false,
			expectError: true,
			failMessage: "Should return error for nil PR",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			client := &GHClient{pr: tc.pr}
			hasLabel, err := client.IsInLabels(tc.labels)
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if _, ok := err.(*NoPRError); !ok {
					t.Errorf("Expected NoPRError, got %T", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if hasLabel != tc.expected {
				t.Error(tc.failMessage)
			}
		})
	}
}

func TestFindExistingComment(t *testing.T) {
	tt := []struct {
		name          string
		comments      []*github.IssueComment
		prefix        string
		since         *time.Time
		expectedID    int64
		expectedFound bool
		expectedError bool
	}{
		{
			name: "comment found",
			comments: []*github.IssueComment{
				{
					ID:   github.Int64(1),
					Body: github.String("Codeowners approval required for this PR:\n- [ ] @user1"),
				},
			},
			prefix:        "Codeowners approval required for this PR:",
			expectedID:    1,
			expectedFound: true,
			expectedError: false,
		},
		{
			name: "comment not found",
			comments: []*github.IssueComment{
				{
					ID:   github.Int64(1),
					Body: github.String("Some other comment"),
				},
			},
			prefix:        "Codeowners approval required for this PR:",
			expectedID:    0,
			expectedFound: false,
			expectedError: false,
		},
		{
			name: "comment too old",
			comments: []*github.IssueComment{
				{
					ID:        github.Int64(1),
					Body:      github.String("Codeowners approval required for this PR:\n- [ ] @user1"),
					CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -6)}, // 6 days old
				},
			},
			prefix:        "Codeowners approval required for this PR:",
			since:         func() *time.Time { t := time.Now().AddDate(0, 0, -5); return &t }(), // 5 days ago
			expectedID:    0,
			expectedFound: false,
			expectedError: false,
		},
		{
			name: "comment within time range",
			comments: []*github.IssueComment{
				{
					ID:        github.Int64(1),
					Body:      github.String("Codeowners approval required for this PR:\n- [ ] @user1"),
					CreatedAt: &github.Timestamp{Time: time.Now().AddDate(0, 0, -4)}, // 4 days old
				},
			},
			prefix:        "Codeowners approval required for this PR:",
			since:         func() *time.Time { t := time.Now().AddDate(0, 0, -5); return &t }(), // 5 days ago
			expectedID:    1,
			expectedFound: true,
			expectedError: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, gh := mockServerAndClient(t)
			defer server.Close()

			gh.pr = &github.PullRequest{Number: github.Int(123)}

			// Mock the GitHub API endpoint
			mux.HandleFunc("/repos/test-owner/test-repo/issues/123/comments", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected method GET, got %s", r.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tc.comments)
			})

			id, found, err := gh.FindExistingComment(tc.prefix, tc.since)
			if tc.expectedError {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if found != tc.expectedFound {
				t.Errorf("expected found to be %t, got %t", tc.expectedFound, found)
			}

			if id != tc.expectedID {
				t.Errorf("expected ID to be %d, got %d", tc.expectedID, id)
			}
		})
	}
}

func TestUpdateComment(t *testing.T) {
	tt := []struct {
		name          string
		commentID     int64
		body          string
		expectedError bool
	}{
		{
			name:          "successful update",
			commentID:     1,
			body:          "Updated comment",
			expectedError: false,
		},
		{
			name:          "zero comment ID",
			commentID:     0,
			body:          "Updated comment",
			expectedError: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, gh := mockServerAndClient(t)
			defer server.Close()

			gh.pr = &github.PullRequest{Number: github.Int(123)}

			if tc.commentID != 0 {
				// Mock the GitHub API endpoint
				mux.HandleFunc(fmt.Sprintf("/repos/test-owner/test-repo/issues/comments/%d", tc.commentID), func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPatch {
						t.Errorf("expected method PATCH, got %s", r.Method)
					}

					// Validate the request payload
					var req github.IssueComment
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if *req.Body != tc.body {
						t.Errorf("expected Body '%s', got '%s'", tc.body, *req.Body)
					}

					w.WriteHeader(http.StatusOK)
				})
			}

			err := gh.UpdateComment(tc.commentID, tc.body)
			if tc.expectedError {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestContainsValidBypassApproval(t *testing.T) {
	tt := []struct {
		name         string
		reviews      []*github.PullRequestReview
		allowedUsers []string
		adminUsers   []string
		expected     bool
		expectError  bool
	}{
		{
			name: "admin user with bypass text",
			reviews: []*github.PullRequestReview{
				{
					ID:   github.Int64(123),
					Body: github.String("🔓 Codeowners Bypass Approved by admin"),
					User: &github.User{Login: github.String("admin-user")},
				},
			},
			allowedUsers: []string{},
			adminUsers:   []string{"admin-user"},
			expected:     true,
			expectError:  false,
		},
		{
			name: "allowed user with bypass text",
			reviews: []*github.PullRequestReview{
				{
					ID:   github.Int64(124),
					Body: github.String("Emergency bypass - codeowners bypass"),
					User: &github.User{Login: github.String("emergency-user")},
				},
			},
			allowedUsers: []string{"emergency-user"},
			adminUsers:   []string{},
			expected:     true,
			expectError:  false,
		},
		{
			name: "non-admin user with bypass text",
			reviews: []*github.PullRequestReview{
				{
					ID:   github.Int64(125),
					Body: github.String("codeowners bypass"),
					User: &github.User{Login: github.String("regular-user")},
				},
			},
			allowedUsers: []string{},
			adminUsers:   []string{},
			expected:     false,
			expectError:  false,
		},
		{
			name: "admin user without bypass text",
			reviews: []*github.PullRequestReview{
				{
					ID:   github.Int64(126),
					Body: github.String("LGTM"),
					User: &github.User{Login: github.String("admin-user")},
				},
			},
			allowedUsers: []string{},
			adminUsers:   []string{"admin-user"},
			expected:     false,
			expectError:  false,
		},
		{
			name: "case insensitive bypass text",
			reviews: []*github.PullRequestReview{
				{
					ID:   github.Int64(127),
					Body: github.String("CODEOWNERS BYPASS"),
					User: &github.User{Login: github.String("admin-user")},
				},
			},
			allowedUsers: []string{},
			adminUsers:   []string{"admin-user"},
			expected:     true,
			expectError:  false,
		},
		{
			name: "multiple reviews with one bypass",
			reviews: []*github.PullRequestReview{
				{
					ID:   github.Int64(128),
					Body: github.String("LGTM"),
					User: &github.User{Login: github.String("regular-user")},
				},
				{
					ID:   github.Int64(129),
					Body: github.String("codeowners bypass"),
					User: &github.User{Login: github.String("admin-user")},
				},
			},
			allowedUsers: []string{},
			adminUsers:   []string{"admin-user"},
			expected:     true,
			expectError:  false,
		},
		{
			name:         "no reviews",
			reviews:      []*github.PullRequestReview{},
			allowedUsers: []string{},
			adminUsers:   []string{},
			expected:     false,
			expectError:  false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, gh := mockServerAndClient(t)
			defer server.Close()

			gh.pr = &github.PullRequest{Number: github.Int(123)}
			gh.reviews = tc.reviews

			// Mock the admin permission check
			for _, adminUser := range tc.adminUsers {
				mux.HandleFunc(fmt.Sprintf("/repos/test-owner/test-repo/collaborators/%s/permission", adminUser), func(w http.ResponseWriter, r *http.Request) {
					response := map[string]interface{}{
						"permission": "admin",
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				})
			}

			// Mock non-admin users
			for _, review := range tc.reviews {
				userName := review.User.GetLogin()
				isAdmin := false
				for _, adminUser := range tc.adminUsers {
					if userName == adminUser {
						isAdmin = true
						break
					}
				}
				if !isAdmin {
					mux.HandleFunc(fmt.Sprintf("/repos/test-owner/test-repo/collaborators/%s/permission", userName), func(w http.ResponseWriter, r *http.Request) {
						response := map[string]interface{}{
							"permission": "write",
						}
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(response)
					})
				}
			}

			result, err := gh.ContainsValidBypassApproval(tc.allowedUsers)

			if tc.expectError {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestContainsValidBypassApprovalNoPR(t *testing.T) {
	gh := NewClient("test-owner", "test-repo", "test-token").(*GHClient)

	result, err := gh.ContainsValidBypassApproval([]string{})

	if err == nil {
		t.Error("expected NoPRError, got nil")
	}

	if result {
		t.Error("expected false result when no PR is set")
	}

	var noPRErr *NoPRError
	if !errors.As(err, &noPRErr) {
		t.Errorf("expected NoPRError, got %T", err)
	}
}

func TestIsRepositoryAdmin(t *testing.T) {
	tt := []struct {
		name        string
		username    string
		permission  string
		expected    bool
		expectError bool
	}{
		{
			name:        "admin user",
			username:    "admin-user",
			permission:  "admin",
			expected:    true,
			expectError: false,
		},
		{
			name:        "write user",
			username:    "write-user",
			permission:  "write",
			expected:    false,
			expectError: false,
		},
		{
			name:        "read user",
			username:    "read-user",
			permission:  "read",
			expected:    false,
			expectError: false,
		},
		{
			name:        "maintain user",
			username:    "maintain-user",
			permission:  "maintain",
			expected:    false,
			expectError: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, gh := mockServerAndClient(t)
			defer server.Close()

			mux.HandleFunc(fmt.Sprintf("/repos/test-owner/test-repo/collaborators/%s/permission", tc.username), func(w http.ResponseWriter, r *http.Request) {
				if tc.expectError {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				response := map[string]interface{}{
					"permission": tc.permission,
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			})

			result, err := gh.IsRepositoryAdmin(tc.username)

			if tc.expectError {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestIsRepositoryAdminError(t *testing.T) {
	mux, server, gh := mockServerAndClient(t)
	defer server.Close()

	mux.HandleFunc("/repos/test-owner/test-repo/collaborators/error-user/permission", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	result, err := gh.IsRepositoryAdmin("error-user")

	if err == nil {
		t.Error("expected an error, got nil")
	}

	if result {
		t.Error("expected false result on error")
	}
}
