package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-github/v89/github"
	"github.com/multimediallc/codeowners-plus/internal/git"
	gh "github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

// The shared mock approves everything without reading the diff, which is the
// decision under test, so the real staleness check is spliced back in.
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

	realClient, err := gh.NewClient("test-owner", "test-repo", "test-token")
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

const retentionBaseSource = `package service

func Alpha() int {
	return 1
}

func Beta() int {
	return 2
}
`

// retentionApprovedSource is the change the reviewer approved.
const retentionApprovedSource = `package service

func Alpha() int {
	return 1
}

func Beta() int {
	return 20
}
`

// Adds a comment and nothing else, so a comment is all the reviewer has not seen.
const retentionHeadSource = `package service

// Alpha is the first step.
func Alpha() int {
	return 1
}

func Beta() int {
	return 20
}
`

// configBody is committed as codeowners.toml on the base ref, which is where the
// application reads its configuration from.
func buildCommentOnlyRepo(t *testing.T, configBody string) (repoDir, baseSHA, headSHA, approvalSHA string) {
	t.Helper()
	repoDir = t.TempDir()
	initRepo(t, repoDir)

	writeRepoFile(t, repoDir, ".codeowners", "* @owner\n")
	writeRepoFile(t, repoDir, "codeowners.toml", configBody)
	writeRepoFile(t, repoDir, "service.go", retentionBaseSource)
	baseSHA = commitAll(t, repoDir, "base")

	writeRepoFile(t, repoDir, "service.go", retentionApprovedSource)
	approvalSHA = commitAll(t, repoDir, "approved change")

	writeRepoFile(t, repoDir, "service.go", retentionHeadSource)
	headSHA = commitAll(t, repoDir, "comment on top of the approved change")

	return repoDir, baseSHA, headSHA, approvalSHA
}

const retentionOffConfig = `disable_review_status_comments = true
`

// The feature is inert until asked for: no section and an all-off section have to
// produce the same bytes.
func TestRunWithoutRetentionSectionIsUnchanged(t *testing.T) {
	const explicitlyOff = `disable_review_status_comments = true

[approval_retention]
enabled = false
whitespace = false
comments = false
formatting = false
string_literals = false
renames = false
fetch_orphaned_approval = false
`

	repoDir, baseSHA, headSHA, approvalSHA := buildCommentOnlyRepo(t, retentionOffConfig)
	absentOutput, absentDismissed, absentWarnings := runApp(t, repoDir, baseSHA, headSHA, approvalSHA)

	repoDir, baseSHA, headSHA, approvalSHA = buildCommentOnlyRepo(t, explicitlyOff)
	offOutput, offDismissed, offWarnings := runApp(t, repoDir, baseSHA, headSHA, approvalSHA)

	if absentOutput.Message != offOutput.Message || absentOutput.Success != offOutput.Success {
		t.Errorf("expected identical results, got %+v and %+v", absentOutput, offOutput)
	}
	if !slices.Equal(absentOutput.StillRequired, offOutput.StillRequired) {
		t.Errorf("expected identical still required, got %v and %v", absentOutput.StillRequired, offOutput.StillRequired)
	}
	if len(absentDismissed) != len(offDismissed) {
		t.Errorf("expected identical dismissals, got %d and %d", len(absentDismissed), len(offDismissed))
	}
	if absentWarnings != offWarnings {
		t.Errorf("expected identical warnings, got %q and %q", absentWarnings, offWarnings)
	}
	// Both are the pre-feature behavior, not merely equal to each other.
	if len(absentDismissed) != 1 || absentOutput.Success {
		t.Errorf("expected the approval to be dismissed as it always was, got %d dismissals, success %t",
			len(absentDismissed), absentOutput.Success)
	}
}
