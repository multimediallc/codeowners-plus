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
		fileOwners[file] = reviewers.Flatten()
	}
	for file, reviewers := range co.FileOptional() {
		fileOptional[file] = reviewers.Flatten()
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

	// Read config
	conf, err := owners.ReadConfig(a.config.RepoDir)
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
	codeOwners, err := codeowners.New(a.config.RepoDir, gitDiff.AllChanges(), a.config.WarningBuffer)
	if err != nil {
		return &OutputData{}, fmt.Errorf("NewCodeOwners Error: %v", err)
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
	a.printDebug("All Required Owners: %s\n", allRequiredOwnerNames)

	// Get optional reviewers
	allOptionalReviewerNames := a.codeowners.AllOptional().Flatten()
	allOptionalReviewerNames = f.Filtered(allOptionalReviewerNames, func(name string) bool {
		return !slices.Contains(allRequiredOwnerNames, name)
	})
	a.printDebug("All Optional Reviewers: %s\n", allOptionalReviewerNames)

	// Initialize user reviewer map
	if err := a.client.InitUserReviewerMap(allRequiredOwnerNames); err != nil {
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

	// Determine if a valid bypass approval exists
	hasValidBypass, err := a.client.ContainsValidBypassApproval(allowedBypassUsers)
	if err != nil {
		return false, message, nil, fmt.Errorf("ContainsValidBypassApproval Error: %v", err)
	}

	a.printDebug("Current Approvals: %+v\n", ghApprovals)


