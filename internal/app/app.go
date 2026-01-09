package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/multimediallc/codeowners-plus/internal/git"
	gh "github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

// OutputData holds the data that will be written to GITHUB_OUTPUT
type OutputData struct {
	FileOwners    map[string][]string `json:"file_owners"`
	FileOptional  map[string][]string `json:"file_optional"`
	UnownedFiles  []string            `json:"unowned_files"`
	StillRequired []string            `json:"still_required"`
	Success       bool                `json:"success"`
	Message       string              `json:"message"`
}

func NewOutputData(co codeowners.CodeOwners) *OutputData {
	fileOwners := make(map[string][]string)
	fileOptional := make(map[string][]string)
	for file, reviewers := range co.FileRequired() {
		fileOwners[file] = codeowners.OriginalStrings(reviewers.Flatten())
	}
	for file, reviewers := range co.FileOptional() {
		fileOptional[file] = codeowners.OriginalStrings(reviewers.Flatten())
	}
	return &OutputData{
		FileOwners:    fileOwners,
		FileOptional:  fileOptional,
		UnownedFiles:  co.UnownedFiles(),
		StillRequired: nil,
		Success:       false,
		Message:       "",
	}
}

func (od *OutputData) UpdateOutputData(success bool, message string, stillRequired []string) {
	od.Success = success
	od.Message = message
	od.StillRequired = stillRequired
}

// Config holds the application configuration
type Config struct {
	Token         string
	RepoDir       string
	PR            int
	Repo          string
	Verbose       bool
	Quiet         bool
	InfoBuffer    io.Writer
	WarningBuffer io.Writer
}

// App represents the application with its dependencies
type App struct {
	Conf       *owners.Config
	config     *Config
	client     gh.Client
	codeowners codeowners.CodeOwners
	gitDiff    git.Diff
}

// New creates a new App instance with the given configuration
func New(cfg Config) (*App, error) {
	repoSplit := strings.Split(cfg.Repo, "/")
	if len(repoSplit) != 2 {
		return nil, fmt.Errorf("invalid repo name: %s", cfg.Repo)
	}
	owner := repoSplit[0]
	repo := repoSplit[1]

	client := gh.NewClient(owner, repo, cfg.Token)
	app := &App{
		config: &cfg,
		client: client,
	}

	return app, nil
}

func (a *App) printDebug(format string, args ...interface{}) {
	if a.config.Verbose {
		_, _ = fmt.Fprintf(a.config.InfoBuffer, format, args...)
	}
}

func (a *App) printWarn(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(a.config.WarningBuffer, format, args...)
}

// Run executes the application logic
func (a *App) Run() (*OutputData, error) {
	// Initialize PR
	if err := a.client.InitPR(a.config.PR); err != nil {
		return &OutputData{}, fmt.Errorf("InitPR Error: %v", err)
	}
	a.printDebug("PR: %d\n", a.client.PR().GetNumber())

	// Create file reader for base ref to prevent PR authors from modifying config or .codeowners
	// This ensures the security policy comes from the protected branch, not the PR branch
	baseFileReader := git.NewGitRefFileReader(a.client.PR().Base.GetSHA(), a.config.RepoDir)
	a.printDebug("Using base ref %s for codeowners.toml and .codeowners files\n", a.client.PR().Base.GetSHA())

	// Read config from base ref
	conf, err := owners.ReadConfig(a.config.RepoDir, baseFileReader)
	if err != nil {
		a.printWarn("Error reading codeowners.toml - using default config\n")
	}
	a.Conf = conf

	// Setup diff context
	diffContext := git.DiffContext{
		Base:       a.client.PR().Base.GetSHA(),
		Head:       a.client.PR().Head.GetSHA(),
		Dir:        a.config.RepoDir,
		IgnoreDirs: conf.Ignore,
	}

	// Get the diff of the PR
	a.printDebug("Getting diff for %s...%s\n", diffContext.Base, diffContext.Head)
	gitDiff, err := git.NewDiff(diffContext)
	if err != nil {
		return &OutputData{}, fmt.Errorf("NewGitDiff Error: %v", err)
	}
	a.gitDiff = gitDiff

	// Initialize codeowners
	var codeOwners codeowners.CodeOwners
	if conf.RequireBothBranchReviewers {
		// Require both branch reviewers mode: read .codeowners from BOTH base and head, then merge
		a.printDebug("Require both branch reviewers mode enabled - reading .codeowners from both base and head refs\n")

		// Create base codeowners from base ref
		baseCodeOwners, err := codeowners.New(a.config.RepoDir, gitDiff.AllChanges(), baseFileReader, a.config.WarningBuffer)
		if err != nil {
			return &OutputData{}, fmt.Errorf("NewCodeOwners (base) Error: %v", err)
		}

		// Create head file reader and codeowners from head ref
		headFileReader := git.NewGitRefFileReader(a.client.PR().Head.GetSHA(), a.config.RepoDir)
		headCodeOwners, err := codeowners.New(a.config.RepoDir, gitDiff.AllChanges(), headFileReader, a.config.WarningBuffer)
		if err != nil {
			return &OutputData{}, fmt.Errorf("NewCodeOwners (head) Error: %v", err)
		}

		// Merge base and head codeowners using AND logic
		codeOwners = codeowners.MergeCodeOwners(baseCodeOwners, headCodeOwners)
		a.printDebug("Merged ownership rules from base and head refs\n")
	} else {
		// Standard mode: read .codeowners only from base ref
		codeOwners, err = codeowners.New(a.config.RepoDir, gitDiff.AllChanges(), baseFileReader, a.config.WarningBuffer)
		if err != nil {
			return &OutputData{}, fmt.Errorf("NewCodeOwners Error: %v", err)
		}
	}
	a.codeowners = codeOwners

	// Set author
	author := fmt.Sprintf("@%s", a.client.PR().User.GetLogin())
	codeOwners.SetAuthor(author)

	// Warn about unowned files
	for _, uFile := range codeOwners.UnownedFiles() {
		a.printWarn("WARNING: Unowned File: %s\n", uFile)
	}

	// Print file owners if verbose
	if a.config.Verbose {
		a.printFileOwners(codeOwners)
	}
	outputData := NewOutputData(a.codeowners)

	// Process approvals and reviewers
	success, message, stillRequired, err := a.processApprovalsAndReviewers()
	if err != nil {
		return outputData, err
	}

	outputData.UpdateOutputData(success, message, stillRequired)

	return outputData, nil
}

func (a *App) processApprovalsAndReviewers() (bool, string, []string, error) {
	message := ""

	// Get all required owners before filtering
	allRequiredOwners := a.codeowners.AllRequired()
	allRequiredOwnerNames := allRequiredOwners.Flatten()
	a.printDebug("All Required Owners: %s\n", codeowners.OriginalStrings(allRequiredOwnerNames))

	// Get optional reviewers
	allOptionalReviewerNames := a.codeowners.AllOptional().Flatten()
	allOptionalReviewerNames = codeowners.FilterOutNames(allOptionalReviewerNames, allRequiredOwnerNames)
	a.printDebug("All Optional Reviewers: %s\n", codeowners.OriginalStrings(allOptionalReviewerNames))

	// Initialize user reviewer map
	if err := a.client.InitUserReviewerMap(codeowners.OriginalStrings(allRequiredOwnerNames)); err != nil {
		return false, message, nil, fmt.Errorf("InitUserReviewerMap Error: %v", err)
	}

	// Get current approvals
	ghApprovals, err := a.client.GetCurrentReviewerApprovals()
	if err != nil {
		return false, message, nil, fmt.Errorf("GetCurrentApprovals Error: %v", err)
	}

	// Check for bypass approvals
	var allowedBypassUsers []string
	if a.Conf.AdminBypass != nil && a.Conf.AdminBypass.Enabled {
		allowedBypassUsers = a.Conf.AdminBypass.AllowedUsers
	}
	hasValidBypass, err := a.client.ContainsValidBypassApproval(allowedBypassUsers)
	if err != nil {
		return false, message, nil, fmt.Errorf("ContainsValidBypassApproval Error: %v", err)
	}

	a.printDebug("Current Approvals: %+v\n", ghApprovals)

	// Process token owner approval if enabled
	var tokenOwnerApproval *gh.CurrentApproval
	if a.Conf.Enforcement.Approval {
		tokenOwnerApproval, err = a.processTokenOwnerApproval()
		if err != nil {
			return false, message, nil, err
		}
	}

	// Process approvals and dismiss stale ones
	validApprovalCount, err := a.processApprovals(ghApprovals)
	if err != nil {
		return false, message, nil, err
	}

	// Request reviews from required owners
	err = a.requestReviews()
	if err != nil {
		return false, message, nil, err
	}

	unapprovedOwners := a.codeowners.AllRequired()
	maxReviewsMet := false
	if a.Conf.MaxReviews != nil && *a.Conf.MaxReviews > 0 {
		unskippableReviewerSlugs := codeowners.NewSlugs(a.Conf.UnskippableReviewers)
		if validApprovalCount >= *a.Conf.MaxReviews && !unapprovedOwners.ContainsAny(unskippableReviewerSlugs) {
			maxReviewsMet = true
		}
	}

	// Add comments to the PR if necessary
	err = a.addReviewStatusComment(allRequiredOwners, maxReviewsMet)
	if err != nil {
		return false, message, nil, fmt.Errorf("failed to add review status comment: %w", err)
	}
	err = a.addOptionalCcComment(codeowners.OriginalStrings(allOptionalReviewerNames))
	if err != nil {
		return false, message, nil, fmt.Errorf("failed to add optional CC comment: %w", err)
	}

	// If we have a valid bypass, approve with token owner if needed and return success
	if hasValidBypass {
		// If enforcement approval is enabled and token owner approval is missing,
		// approve the PR with the token owner. This is necessary because users
		// typically have the token owner as a GitHub CODEOWNER
		if a.Conf.Enforcement.Approval && tokenOwnerApproval == nil {
			err := a.client.ApprovePR()
			if err != nil {
				a.printWarn("Warning: Failed to approve PR with token owner during bypass: %v\n", err)
			}
		}
		message = "SUCCESS: Codeowners requirements bypassed by administrator"
		return true, message, []string{}, nil
	}

	// Collect still required data
	stillRequiredSlugs := unapprovedOwners.Flatten()
	stillRequired := codeowners.OriginalStrings(stillRequiredSlugs)

	// Exit if there are any unapproved codeowner teams
	if len(unapprovedOwners) > 0 && !maxReviewsMet {
		// Return failed status if any codeowner team has not approved the PR
		unapprovedCommentString := unapprovedOwners.ToCommentString(false)
		if a.Conf.Enforcement.Approval && tokenOwnerApproval != nil && !a.Conf.DisableSmartDismissal {
			_ = a.client.DismissStaleReviews([]*gh.CurrentApproval{tokenOwnerApproval})
		}
		message = fmt.Sprintf(
			"FAIL: Codeowners reviews not satisfied\nStill required:\n%s",
			unapprovedCommentString,
		)
		return false, message, stillRequired, nil
	}

	// Exit if there are not enough reviews
	if a.Conf.MinReviews != nil && *a.Conf.MinReviews > 0 {
		// Check if we need to re-request from a satisfied team when min_reviews is not met
		// Handles the case when there min_reviews is higher than the number of teams required.
		if validApprovalCount < *a.Conf.MinReviews && len(unapprovedOwners) == 0 {
			// All required teams have approved, but we need more reviews
			// Re-request review from the satisfied team(s)
			currentlyRequestedOwners, err := a.client.GetCurrentlyRequested()
			if err != nil {
				a.printWarn("WARNING: Error getting currently requested owners for re-request: %v\n", err)
			} else {
				currentlyRequestedSet := make(map[string]struct{}, len(currentlyRequestedOwners))
				for _, owner := range currentlyRequestedOwners {
					currentlyRequestedSet[owner.Normalized()] = struct{}{}
				}
				ownersToReRequest := f.Filtered(allRequiredOwnerNames, func(owner codeowners.Slug) bool {
					_, exists := currentlyRequestedSet[owner.Normalized()]
					return !exists
				})
				if len(ownersToReRequest) > 0 {
					a.printDebug("Re-requesting Reviews from satisfied team(s) to meet min_reviews: %s\n", codeowners.OriginalStrings(ownersToReRequest))
					if err := a.client.RequestReviewers(codeowners.OriginalStrings(ownersToReRequest)); err != nil {
						a.printWarn("WARNING: Error re-requesting reviewers: %v\n", err)
					}
				}
			}
		}
		if validApprovalCount < *a.Conf.MinReviews {
			message = fmt.Sprintf("FAIL: Min Reviews not satisfied. Need %d, found %d", *a.Conf.MinReviews, validApprovalCount)
			return false, message, stillRequired, nil
		}
	}

	message = "Codeowners reviews satisfied"
	if a.Conf.Enforcement.Approval && tokenOwnerApproval == nil {
		// Approve the PR since all codeowner teams have approved
		err = a.client.ApprovePR()
		if err != nil {
			return true, message, stillRequired, fmt.Errorf("ApprovePR Error: %v", err)
		}
	}

	return true, message, stillRequired, nil
}

func (a *App) addReviewStatusComment(allRequiredOwners codeowners.ReviewerGroups, maxReviewsMet bool) error {
	// Comment on the PR with the codeowner teams required for review

	if a.config.Quiet {
		a.printDebug("Skipping review status comment (disabled or no unapproved owners).\n")
		return nil
	}

	var commentPrefix = "Codeowners approval required for this PR:\n"

	hasHighPriority, err := a.client.IsInLabels(a.Conf.HighPriorityLabels)
	if err != nil {
		a.printWarn("WARNING: Error checking high priority labels: %v\n", err)
	} else if hasHighPriority {
		commentPrefix = "❗High Prio❗\n\n" + commentPrefix
	}

	comment := commentPrefix + allRequiredOwners.ToCommentString(true)

	if maxReviewsMet {
		comment += "\n\nThe PR has received the max number of required reviews. No further action is required."
	}

	if a.Conf.DetailedReviewers {
		comment += fmt.Sprintf("\n\n<details><summary>Show detailed file reviewers</summary>\n\n%s\n</details>", a.getFileOwnersMapToString(a.codeowners.FileRequired()))
	}

	fiveDaysAgo := time.Now().AddDate(0, 0, -5)
	existingComment, existingFound, err := a.client.FindExistingComment(commentPrefix, &fiveDaysAgo)
	if err != nil {
		return fmt.Errorf("FindExistingComment Error: %v", err)
	}

	if existingFound {
		if found, _ := a.client.IsInComments(comment, &fiveDaysAgo); found {
			// we don't need to update the comment
			return nil
		}
		a.printDebug("Updating existing review status comment\n")
		err = a.client.UpdateComment(existingComment, comment)
		if err != nil {
			return fmt.Errorf("UpdateComment Error: %v", err)
		}
	} else if len(allRequiredOwners) > 0 {
		a.printDebug("Adding new review status comment: %q\n", comment)
		err = a.client.AddComment(comment)
		if err != nil {
			return fmt.Errorf("AddComment Error: %v", err)
		}
	}

	return nil
}

func (a *App) addOptionalCcComment(allOptionalReviewerNames []string) error {
	// Add CC comment to the PR with the optional reviewers that have not already been mentioned in the PR comments

	if a.config.Quiet || len(allOptionalReviewerNames) == 0 {
		return nil
	}

	var isInCommentsError error
	viewersToPing := f.Filtered(allOptionalReviewerNames, func(name string) bool {
		if isInCommentsError != nil {
			return false
		}
		found, err := a.client.IsSubstringInComments(name, nil)
		if err != nil {
			a.printWarn("WARNING: Error checking comments for substring '%s': %v\n", name, err)
			isInCommentsError = err
			return false
		}
		return !found
	})

	if isInCommentsError != nil {
		return fmt.Errorf("IsInComments Error: %v", isInCommentsError)
	}

	// Add the CC comment if there are any viewers to ping
	if len(viewersToPing) > 0 {
		comment := fmt.Sprintf("cc %s", strings.Join(viewersToPing, " "))
		a.printDebug("Adding CC comment: %q\n", comment)
		err := a.client.AddComment(comment)
		if err != nil {
			return fmt.Errorf("AddComment Error: %v", err)
		}
	} else {
		a.printDebug("No new optional reviewers to CC.\n")
	}

	return nil
}

func (a *App) processTokenOwnerApproval() (*gh.CurrentApproval, error) {
	tokenOwner, err := a.client.GetTokenUser()
	if err != nil {
		a.printWarn("WARNING: You might be trying to use a bot as an Enforcement.Approval user," +
			" but this will not work due to GitHub CODEOWNERS not allowing bots as code owners." +
			" To use the Enforcement.Approval feature, the token must belong to a GitHub user account")

		a.Conf.Enforcement.Approval = false
		return nil, nil
	}

	tokenOwnerApproval, _ := a.client.FindUserApproval(tokenOwner)
	return tokenOwnerApproval, nil
}

func (a *App) processApprovals(ghApprovals []*gh.CurrentApproval) (int, error) {
	// Create file reviewer map with normalized names for case-insensitive comparison
	fileReviewers := f.MapMap(a.codeowners.FileRequired(), func(reviewers codeowners.ReviewerGroups) []string {
		return codeowners.NormalizedStrings(reviewers.Flatten())
	})

	var approvers []codeowners.Slug
	var approvalsToDismiss []*gh.CurrentApproval

	if a.Conf.DisableSmartDismissal {
		// Smart dismissal is disabled - treat all approvals as valid
		a.printDebug("Smart dismissal disabled - keeping all approvals\n")
		for _, approval := range ghApprovals {
			approvers = append(approvers, approval.Reviewers...)
		}
		approvalsToDismiss = []*gh.CurrentApproval{}
	} else {
		// Normal smart dismissal logic
		approvers, approvalsToDismiss = a.client.CheckApprovals(fileReviewers, ghApprovals, a.gitDiff)
	}

	a.codeowners.ApplyApprovals(approvers)

	if len(approvalsToDismiss) > 0 {
		a.printDebug("Dismissing Stale Approvals: %+v\n", approvalsToDismiss)
		if err := a.client.DismissStaleReviews(approvalsToDismiss); err != nil {
			return 0, fmt.Errorf("DismissStaleReviews Error: %v", err)
		}
	}

	return len(ghApprovals) - len(approvalsToDismiss), nil
}

func (a *App) requestReviews() error {
	if a.config.Quiet {
		return nil
	}

	unapprovedOwners := a.codeowners.AllRequired()
	unapprovedOwnerNames := unapprovedOwners.Flatten()
	a.printDebug("Remaining Required Owners: %s\n", codeowners.OriginalStrings(unapprovedOwnerNames))

	currentlyRequestedOwners, err := a.client.GetCurrentlyRequested()
	if err != nil {
		return fmt.Errorf("GetCurrentlyRequested Error: %v", err)
	}
	a.printDebug("Currently Requested Owners: %s\n", codeowners.OriginalStrings(currentlyRequestedOwners))

	previousReviewers, err := a.client.GetAlreadyReviewed()
	if err != nil {
		return fmt.Errorf("GetAlreadyReviewed Error: %v", err)
	}
	a.printDebug("Already Reviewed Owners: %s\n", codeowners.OriginalStrings(previousReviewers))

	filteredOwners := unapprovedOwners.FilterOut(currentlyRequestedOwners...)
	filteredOwners = filteredOwners.FilterOut(previousReviewers...)
	filteredOwnerNames := filteredOwners.Flatten()

	if len(filteredOwners) > 0 {
		a.printDebug("Requesting Reviews from: %s\n", codeowners.OriginalStrings(filteredOwnerNames))
		if err := a.client.RequestReviewers(codeowners.OriginalStrings(filteredOwnerNames)); err != nil {
			return fmt.Errorf("RequestReviewers Error: %v", err)
		}
	}

	return nil
}

func (a *App) printFileOwners(codeOwners codeowners.CodeOwners) {
	a.printDebug("File Reviewers:\n")
	a.printDebug(a.getFileOwnersMapToString(codeOwners.FileRequired()))
	a.printDebug("File Optional:\n")
	a.printDebug(a.getFileOwnersMapToString(codeOwners.FileOptional()))
}

func (a *App) getFileOwnersMapToString(fileReviewers map[string]codeowners.ReviewerGroups) string {
	builder := strings.Builder{}

	files := make([]string, 0, len(fileReviewers))
	for file := range fileReviewers {
		files = append(files, file)
	}
	slices.Sort(files)

	for _, file := range files {
		reviewers := fileReviewers[file]
		// builder.WriteString error return is always nil
		_, _ = fmt.Fprintf(&builder, "- %s: %+v\n", file, codeowners.OriginalStrings(reviewers.Flatten()))
	}
	return builder.String()
}
