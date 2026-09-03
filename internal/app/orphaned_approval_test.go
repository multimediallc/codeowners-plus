package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v89/github"
	"github.com/multimediallc/codeowners-plus/internal/git"
	gh "github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

type realCheckApprovalsClient struct {
	*mockGitHubClient
	real      gh.Client
	dismissed []*gh.CurrentApproval
}

func (c *realCheckApprovalsClient) CheckApprovals(
	fileReviewerMap map[string][]string,
	approvals []*gh.CurrentApproval,
	originalDiff git.Diff,
) ([]codeowners.Slug, []*gh.CurrentApproval) {
	return c.real.CheckApprovals(fileReviewerMap, approvals, originalDiff)
}

func (c *realCheckApprovalsClient) DismissStaleReviews(approvals []*gh.CurrentApproval) error {
	c.dismissed = append(c.dismissed, approvals...)
	return c.mockGitHubClient.DismissStaleReviews(approvals)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.invalid")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func commitAll(t *testing.T, dir, message string) string {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", message)
	return runGit(t, dir, "rev-parse", "HEAD")
}

func runApp(t *testing.T, repoDir, baseSHA, headSHA, approvalSHA string) (*OutputData, []*gh.CurrentApproval, string) {
	t.Helper()

	warnings := &bytes.Buffer{}
	info := &bytes.Buffer{}

	realClient, err := gh.NewClient("test-owner", "test-repo", "test-token", "")
	if err != nil {
		t.Fatalf("failed to build the real client: %v", err)
	}
	realClient.SetWarningBuffer(warnings)
	realClient.SetInfoBuffer(info)

	client := &realCheckApprovalsClient{
		mockGitHubClient: &mockGitHubClient{
			pr: &github.PullRequest{
				Number: github.Ptr(1),
				Base:   &github.PullRequestBranch{SHA: github.Ptr(baseSHA)},
				Head:   &github.PullRequestBranch{SHA: github.Ptr(headSHA)},
				User:   &github.User{Login: github.Ptr("author")},
			},
			currentApprovals: []*gh.CurrentApproval{{
				GHLogin:   codeowners.NewSlug("@reviewer"),
				ReviewID:  1,
				Reviewers: []codeowners.Slug{codeowners.NewSlug("@owner")},
				CommitID:  approvalSHA,
			}},
		},
		real: realClient,
	}

	app := &App{
		config: &Config{
			RepoDir:       repoDir,
			PR:            1,
			Quiet:         true,
			InfoBuffer:    info,
			WarningBuffer: warnings,
		},
		client: client,
	}

	output, err := app.Run()
	if err != nil {
		t.Fatalf("app.Run failed: %v\nwarnings: %s", err, warnings)
	}
	return output, client.dismissed, warnings.String()
}

const orphanBaseSource = `package service

func Gamma() int {
	return 3
}
`

const orphanApprovedSource = `package service

func Gamma() int {
	return 30
}
`

func buildOrphanedApprovalRepo(t *testing.T, configBody string) (repoDir, baseSHA, headSHA, approvalSHA string) {
	t.Helper()
	repoDir = t.TempDir()
	initRepo(t, repoDir)

	writeRepoFile(t, repoDir, ".codeowners", "service.go @owner\n")
	writeRepoFile(t, repoDir, "codeowners.toml", configBody)
	writeRepoFile(t, repoDir, "service.go", orphanBaseSource)
	writeRepoFile(t, repoDir, "notes.md", "first note\n")
	baseSHA = commitAll(t, repoDir, "base")

	originDir := t.TempDir()
	runGit(t, repoDir, "clone", "-q", repoDir, originDir)
	initRepo(t, originDir)
	runGit(t, originDir, "config", "uploadpack.allowAnySHA1InWant", "true")
	writeRepoFile(t, originDir, "service.go", orphanApprovedSource)
	approvalSHA = commitAll(t, originDir, "approved change")

	writeRepoFile(t, repoDir, "service.go", orphanApprovedSource)
	writeRepoFile(t, repoDir, "notes.md", "first note\nsecond note\n")
	headSHA = commitAll(t, repoDir, "approved change plus an unowned edit")
	runGit(t, repoDir, "remote", "add", "origin", originDir)

	return repoDir, baseSHA, headSHA, approvalSHA
}

func TestRunFetchesOrphanedApproval(t *testing.T) {
	const fetchOff = `disable_review_status_comments = true
suppress_unowned_warning = true
`
	const fetchOn = `disable_review_status_comments = true
suppress_unowned_warning = true
fetch_orphaned_approval = true
`

	tt := []struct {
		name            string
		config          string
		expectDismissed bool
	}{
		{name: "fetch disabled, approval dismissed", config: fetchOff, expectDismissed: true},
		{name: "fetch enabled, approval recovered", config: fetchOn, expectDismissed: false},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			repoDir, baseSHA, headSHA, approvalSHA := buildOrphanedApprovalRepo(t, tc.config)

			cmd := exec.Command("git", "cat-file", "-e", approvalSHA)
			cmd.Dir = repoDir
			if err := cmd.Run(); err == nil {
				t.Fatalf("approval commit %s should not be present locally", approvalSHA)
			}

			output, dismissed, warnings := runApp(t, repoDir, baseSHA, headSHA, approvalSHA)

			if tc.expectDismissed {
				if len(dismissed) != 1 {
					t.Errorf("expected the approval to be dismissed, got %d dismissals", len(dismissed))
				}
				if !strings.Contains(warnings, "Error getting changes since") {
					t.Errorf("expected a warning about the unresolvable ref, got %q", warnings)
				}
				if output.Success {
					t.Error("expected the run to fail without the approval")
				}
				return
			}

			if len(dismissed) != 0 {
				t.Errorf("expected the approval to survive, got %d dismissals: %s", len(dismissed), warnings)
			}
			if !output.Success {
				t.Errorf("expected the run to succeed, got %q (warnings: %s)", output.Message, warnings)
			}
		})
	}
}
