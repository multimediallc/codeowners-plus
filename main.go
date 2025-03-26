package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/multimediallc/codeowners-plus/internal/git"
	gh "github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

// AppConfig holds the application configuration
type AppConfig struct {
	Token       string
	RepoDir     string
	PR          int
	Repo        string
	Verbose     bool
	AddComments bool
}

// App represents the application with its dependencies
type App struct {
	config     AppConfig
	client     gh.Client
	codeowners codeowners.CodeOwners
	gitDiff    git.Diff
	conf       *owners.Config
}

// Flags holds the command line flags
type Flags struct {
	Token       *string
	RepoDir     *string
	PR          *int
	Repo        *string
	Verbose     *bool
	AddComments *bool
}

var (
	flags = &Flags{
		Token:       flag.String("token", getEnv("INPUT_GITHUB-TOKEN", ""), "GitHub authentication token"),
		RepoDir:     flag.String("dir", getEnv("GITHUB_WORKSPACE", "/"), "Path to local Git repo"),
		PR:          flag.Int("pr", ignoreError(strconv.Atoi(getEnv("INPUT_PR", ""))), "Pull Request number"),
		Repo:        flag.String("repo", getEnv("INPUT_REPOSITORY", ""), "GitHub repo name"),
		Verbose:     flag.Bool("v", ignoreError(strconv.ParseBool(getEnv("INPUT_VERBOSE", "0"))), "Verbose output"),
		AddComments: flag.Bool("comments", ignoreError(strconv.ParseBool(getEnv("INPUT_ADD_COMMENTS", "1"))), "Add comments to Pull Request"),
	}
	WarningBuffer   = bytes.NewBuffer([]byte{})
	InfoBuffer      = bytes.NewBuffer([]byte{})
	QUIET_MODE_TEST = true
)

// initFlags initializes and parses command line flags
func initFlags(flags *Flags) error {
	// Only parse flags if we're not testing
	if !testing.Testing() {
		flag.Parse()
	}

	// Validate required flags
	badFlags := make([]string, 0, 4)
	if *flags.Token == "" {
		badFlags = append(badFlags, "token")
	}
	if *flags.PR == 0 {
		badFlags = append(badFlags, "pr")
	}
	if *flags.Repo == "" {
		badFlags = append(badFlags, "repo")
	}
	if len(badFlags) > 0 {
		return fmt.Errorf("Required flags or environment variables not set: %s", badFlags)
	}

	return nil
}

// NewApp creates a new App instance with the given configuration
func NewApp(cfg AppConfig) (*App, error) {
	repoSplit := strings.Split(cfg.Repo, "/")
	if len(repoSplit) != 2 {
		return nil, fmt.Errorf("invalid repo name: %s", cfg.Repo)
	}
	owner := repoSplit[0]
	repo := repoSplit[1]

	client := gh.NewClient(owner, repo, cfg.Token)
	app := &App{
		config: cfg,
		client: client,
	}

	return app, nil
}

// Run executes the application logic
func (a *App) Run() (bool, string, error) {
	// Initialize PR
	if err := a.client.InitPR(a.config.PR); err != nil {
		return false, "", fmt.Errorf("InitPR Error: %v", err)
	}
	printDebug("PR: %d\n", a.client.PR().GetNumber())

	// Read config
	conf, err := owners.ReadConfig(a.config.RepoDir)
	if err != nil {
		printWarning("Error reading codeowners.toml - using default config\n")
	}
	a.conf = conf

	// Setup diff context
	diffContext := git.DiffContext{
		Base:       a.client.PR().Base.GetSHA(),
		Head:       a.client.PR().Head.GetSHA(),
		Dir:        a.config.RepoDir,
		IgnoreDirs: conf.Ignore,
	}

	// Get the diff of the PR
	printDebug("Getting diff for %s...%s\n", diffContext.Base, diffContext.Head)
	gitDiff, err := git.NewDiff(diffContext)
	if err != nil {
		return false, "", fmt.Errorf("NewGitDiff Error: %v", err)
	}
	a.gitDiff = gitDiff

	// Initialize codeowners
	codeOwners, err := codeowners.New(a.config.RepoDir, gitDiff.AllChanges(), WarningBuffer)
	if err != nil {
		return false, "", fmt.Errorf("NewCodeOwners Error: %v", err)
	}
	a.codeowners = codeOwners

	// Set author
	author := fmt.Sprintf("@%s", a.client.PR().User.GetLogin())
	codeOwners.SetAuthor(author)

	// Warn about unowned files
	for _, uFile := range codeOwners.UnownedFiles() {
		printWarning("WARNING: Unowned File: %s\n", uFile)
	}

	// Print file owners if verbose
	if a.config.Verbose {
		printFileOwners(codeOwners)
	}

	// Process approvals and reviewers
	return a.processApprovalsAndReviewers()
}

func (a *App) processApprovalsAndReviewers() (bool, string, error) {
	message := ""
	// Get all required owners before filtering
	allRequiredOwners := a.codeowners.AllRequired()
	allRequiredOwnerNames := allRequiredOwners.Flatten()
	printDebug("All Required Owners: %s\n", allRequiredOwnerNames)

	// Get optional reviewers
	allOptionalReviewerNames := a.codeowners.AllOptional().Flatten()
	allOptionalReviewerNames = f.Filtered(allOptionalReviewerNames, func(name string) bool {
		return !slices.Contains(allRequiredOwnerNames, name)
	})
	printDebug("All Optional Reviewers: %s\n", allOptionalReviewerNames)

	// Initialize user reviewer map
	if err := a.client.InitUserReviewerMap(allRequiredOwnerNames); err != nil {
		return false, message, fmt.Errorf("InitUserReviewerMap Error: %v", err)
	}

	// Get current approvals
	ghApprovals, err := a.client.GetCurrentReviewerApprovals()
	if err != nil {
		return false, message, fmt.Errorf("GetCurrentApprovals Error: %v", err)
	}
	printDebug("Current Approvals: %+v\n", ghApprovals)

	// Process token owner approval if enabled
	var tokenOwnerApproval *gh.CurrentApproval
	if a.conf.Enforcement.Approval {
		tokenOwnerApproval, err = a.processTokenOwnerApproval()
		if err != nil {
			return false, message, err
		}
	}

	// Process approvals and dismiss stale ones
	validApprovalCount, err := a.processApprovals(ghApprovals)
	if err != nil {
		return false, message, err
	}

	// Request reviews from required owners
	err = a.requestReviews()
	if err != nil {
		return false, message, err
	}

	unapprovedOwners := a.codeowners.AllRequired()
	maxReviewsMet := false
	if a.conf.MaxReviews != nil && *a.conf.MaxReviews > 0 {
		if validApprovalCount >= *a.conf.MaxReviews && len(f.Intersection(unapprovedOwners.Flatten(), a.conf.UnskippableReviewers)) == 0 {
			maxReviewsMet = true
		}
	}

	// Add comments to the PR if necessary
	err = a.addReviewStatusComment(allRequiredOwners, unapprovedOwners, maxReviewsMet)
	if err != nil {
		return false, message, fmt.Errorf("failed to add review status comment: %w", err)
	}
	err = a.addOptionalCcComment(allOptionalReviewerNames)
	if err != nil {
		return false, message, fmt.Errorf("failed to add optional CC comment: %w", err)
	}

	// Exit if there are any unapproved codeowner teams
	if len(unapprovedOwners) > 0 && !maxReviewsMet {
		// Return failed status if any codeowner team has not approved the PR
		unapprovedCommentStrings := f.Map(unapprovedOwners, func(s *codeowners.ReviewerGroup) string {
			return s.ToCommentString()
		})
		if a.conf.Enforcement.Approval && tokenOwnerApproval != nil {
			_ = a.client.DismissStaleReviews([]*gh.CurrentApproval{tokenOwnerApproval})
		}
		message = fmt.Sprintf(
			"FAIL: Codeowners reviews not satisfied\nStill required:\n - %s",
			strings.Join(unapprovedCommentStrings, "\n - "),
		)
		return false, message, nil
	}

	// Exit if there are not enough reviews
	if a.conf.MinReviews != nil && *a.conf.MinReviews > 0 {
		if validApprovalCount < *a.conf.MinReviews {
			message = fmt.Sprintf("FAIL: Min Reviews not satisfied. Need %d, found %d", *a.conf.MinReviews, validApprovalCount)
			return false, message, nil
		}
	}

	message = "Codeowners reviews satisfied"
	if a.conf.Enforcement.Approval && tokenOwnerApproval == nil {
		// Approve the PR since all codeowner teams have approved
		err = a.client.ApprovePR()
		if err != nil {
			return true, message, fmt.Errorf("ApprovePR Error: %v\n", err)
		}
	}
	return true, message, nil
}

func (a *App) addReviewStatusComment(allRequiredOwners, unapprovedOwners codeowners.ReviewerGroups, maxReviewsMet bool) error {
	// Comment on the PR with the codeowner teams that have not approved the PR

	if !a.config.AddComments || len(unapprovedOwners) == 0 {
		printDebug("Skipping review status comment (disabled or no unapproved owners).\n")
		return nil
	}

	comment := allRequiredOwners.ToCommentString()
	hasHighPriority, err := a.client.IsInLabels(a.conf.HighPriorityLabels)
	if err != nil {
		printWarning("WARNING: Error checking high priority labels: %v\n", err)
	} else if hasHighPriority {
		comment = "❗High Prio❗\n\n" + comment
	}

	if maxReviewsMet {
		comment += "\n\nThe PR has received the max number of required reviews. No further action is required."
	}

	fiveDaysAgo := time.Now().AddDate(0, 0, -5)
	found, err := a.client.IsInComments(comment, &fiveDaysAgo)
	if err != nil {
		return fmt.Errorf("IsInComments Error: %v\n", err)
	}

	// Add the comment if it wasn't found recently
	if !found {
		printDebug("Adding review status comment: %q\n", comment)
		err = a.client.AddComment(comment)
		if err != nil {
			return fmt.Errorf("AddComment Error: %v\n", err)
		}
	} else {
		printDebug("Similar review status comment already exists.\n")
	}

	return nil
}

func (a *App) addOptionalCcComment(allOptionalReviewerNames []string) error {
	// Add CC comment to the PR with the optional reviewers that have not already been mentioned in the PR comments

	if !a.config.AddComments || len(allOptionalReviewerNames) == 0 {
		printDebug("Skipping optional CC comment (disabled or no optional reviewers).\n")
		return nil
	}

	var isInCommentsError error
	viewersToPing := f.Filtered(allOptionalReviewerNames, func(name string) bool {
		if isInCommentsError != nil {
			return false
		}
		found, err := a.client.IsSubstringInComments(name, nil)
		if err != nil {
			printWarning("WARNING: Error checking comments for substring '%s': %v\n", name, err)
			isInCommentsError = err
			return false
		}
		return !found
	})

	if isInCommentsError != nil {
		return fmt.Errorf("IsInComments Error: %v\n", isInCommentsError)
	}

	// Add the CC comment if there are any viewers to ping
	if len(viewersToPing) > 0 {
		comment := fmt.Sprintf("cc %s", strings.Join(viewersToPing, " "))
		printDebug("Adding CC comment: %q\n", comment)
		err := a.client.AddComment(comment)
		if err != nil {
			return fmt.Errorf("AddComment Error: %v\n", err)
		}
	} else {
		printDebug("No new optional reviewers to CC.\n")
	}

	return nil
}

func (a *App) processTokenOwnerApproval() (*gh.CurrentApproval, error) {
	tokenOwner, err := a.client.GetTokenUser()
	if err != nil {
		printWarning("WARNING: You might be trying to use a bot as an Enforcement.Approval user," +
			" but this will not work due to GitHub CODEOWNERS not allowing bots as code owners." +
			" To use the Enforcement.Approval feature, the token must belong to a GitHub user account")

		a.conf.Enforcement.Approval = false
		return nil, nil
	}

	tokenOwnerApproval, _ := a.client.FindUserApproval(tokenOwner)
	return tokenOwnerApproval, nil
}

func (a *App) processApprovals(ghApprovals []*gh.CurrentApproval) (int, error) {
	fileReviewers := f.MapMap(a.codeowners.FileRequired(), func(reviewers codeowners.ReviewerGroups) []string { return reviewers.Flatten() })
	approvers, approvalsToDismiss := a.client.CheckApprovals(fileReviewers, ghApprovals, a.gitDiff)
	a.codeowners.ApplyApprovals(approvers)

	if len(approvalsToDismiss) > 0 {
		printDebug("Dismissing Stale Approvals: %+v\n", approvalsToDismiss)
		if err := a.client.DismissStaleReviews(approvalsToDismiss); err != nil {
			return 0, fmt.Errorf("DismissStaleReviews Error: %v", err)
		}
	}

	return len(ghApprovals) - len(approvalsToDismiss), nil
}

func (a *App) requestReviews() error {
	if !a.config.AddComments {
		printDebug("Skipping review requests (disabled in quiet mode).\n")
		return nil
	}

	unapprovedOwners := a.codeowners.AllRequired()
	unapprovedOwnerNames := unapprovedOwners.Flatten()
	printDebug("Remaining Required Owners: %s\n", unapprovedOwnerNames)

	currentlyRequestedOwners, err := a.client.GetCurrentlyRequested()
	if err != nil {
		return fmt.Errorf("GetCurrentlyRequested Error: %v", err)
	}
	printDebug("Currently Requested Owners: %s\n", currentlyRequestedOwners)

	previousReviewers, err := a.client.GetAlreadyReviewed()
	if err != nil {
		return fmt.Errorf("GetAlreadyReviewed Error: %v", err)
	}
	printDebug("Already Reviewed Owners: %s\n", previousReviewers)

	filteredOwners := unapprovedOwners.FilterOut(currentlyRequestedOwners...)
	filteredOwners = filteredOwners.FilterOut(previousReviewers...)
	filteredOwnerNames := filteredOwners.Flatten()

	if len(filteredOwners) > 0 {
		printDebug("Requesting Reviews from: %s\n", filteredOwnerNames)
		if err := a.client.RequestReviewers(filteredOwnerNames); err != nil {
			return fmt.Errorf("RequestReviewers Error: %v", err)
		}
	}

	return nil
}

func printFileOwners(codeOwners codeowners.CodeOwners) {
	fileRequired := codeOwners.FileRequired()
	printDebug("File Reviewers:\n")
	for file, reviewers := range fileRequired {
		printDebug("- %s: %+v\n", file, reviewers.Flatten())
	}
	fileOptional := codeOwners.FileOptional()
	printDebug("File Optional:\n")
	for file, reviewers := range fileOptional {
		printDebug("- %s: %+v\n", file, reviewers.Flatten())
	}
}

// Helper functions
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func ignoreError[V any, E error](res V, _ E) V {
	return res
}

func errorAndExit(shouldFail bool, format string, args ...interface{}) {
	_, err := WarningBuffer.WriteTo(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing warning buffer: %v\n", err)
	}
	if *flags.Verbose {
		_, err := InfoBuffer.WriteTo(os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing info buffer: %v\n", err)
		}
	}

	fmt.Fprintf(os.Stderr, format, args...)
	if testing.Testing() {
		return
	}
	if shouldFail {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func printDebug(format string, args ...interface{}) {
	if *flags.Verbose {
		fmt.Fprintf(InfoBuffer, format, args...)
	}
}

func printWarning(format string, args ...interface{}) {
	fmt.Fprintf(WarningBuffer, format, args...)
}

func main() {
	err := initFlags(flags)
	if err != nil {
		errorAndExit(true, "%v\n", err)
	}

	cfg := AppConfig{
		Token:       *flags.Token,
		RepoDir:     *flags.RepoDir,
		PR:          *flags.PR,
		Repo:        *flags.Repo,
		Verbose:     *flags.Verbose,
		AddComments: *flags.AddComments,
	}

	app, err := NewApp(cfg)
	if err != nil {
		errorAndExit(true, "Failed to initialize app: %v\n", err)
	}

	success, message, err := app.Run()
	if err != nil {
		errorAndExit(true, "%v\n", err)
	}
	_, err = WarningBuffer.WriteTo(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing warning buffer: %v\n", err)
	}
	if *flags.Verbose {
		_, err = InfoBuffer.WriteTo(os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing info buffer: %v\n", err)
		}
	}
	if success {
		fmt.Fprintln(os.Stdout, message)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	if !success && app.conf.Enforcement.FailCheck {
		os.Exit(1)
	}
}
