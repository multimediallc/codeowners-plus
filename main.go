package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/multimediallc/codeowners-plus/internal/diff"
	"github.com/multimediallc/codeowners-plus/internal/github"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/multimediallc/codeowners-plus/pkg/functional"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func ignoreError[V any, E error](res V, _ E) V {
	return res
}

var (
	WarningBuffer = bytes.NewBuffer([]byte{})
	InfoBuffer    = bytes.NewBuffer([]byte{})
)

var (
	gh_token = flag.String("token", getEnv("INPUT_GITHUB-TOKEN", ""), "GitHub authentication token")
	repo_dir = flag.String("dir", getEnv("GITHUB_WORKSPACE", "/"), "Path to local Git repo")
	pr       = flag.Int("pr", ignoreError(strconv.Atoi(getEnv("INPUT_PR", ""))), "Pull Request number")
	gh_repo  = flag.String("repo", getEnv("INPUT_REPOSITORY", ""), "GitHub repo name")
	verbose  = flag.Bool("v", ignoreError(strconv.ParseBool(getEnv("INPUT_VERBOSE", "0"))), "Verbose output")
)

// shouldFail should always be true for errors that are not recoverable
func errorAndExit(shouldFail bool, format string, args ...interface{}) {
	_, err := WarningBuffer.WriteTo(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing warning buffer: %v\n", err)
	}
	if *verbose {
		_, err := InfoBuffer.WriteTo(os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing info buffer: %v\n", err)
		}
	}
	fmt.Fprintf(os.Stderr, format, args...)
	if shouldFail {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func printDebug(format string, args ...interface{}) {
	if *verbose {
		fmt.Fprintf(InfoBuffer, format, args...)
	}
}

func printWarning(format string, args ...interface{}) {
	fmt.Fprintf(WarningBuffer, format, args...)
}

func init() {
	flag.Parse()
	badFlags := make([]string, 0, 4)
	if *gh_token == "" {
		badFlags = append(badFlags, "token")
	}
	if *pr == 0 {
		badFlags = append(badFlags, "pr")
	}
	if *gh_repo == "" {
		badFlags = append(badFlags, "repo")
	}
	if len(badFlags) > 0 {
		errorAndExit(true, "Required flags or environment variables not set: %s\n", badFlags)
	}
}

func main() {
	repoSplit := strings.Split(*gh_repo, "/")
	if len(repoSplit) != 2 {
		errorAndExit(true, "Invalid repo name: %s\n", *gh_repo)
	}
	owner := repoSplit[0]
	repo := repoSplit[1]

	client := gh.NewClient(owner, repo, *gh_token)

	err := client.InitPR(*pr)
	if err != nil {
		errorAndExit(true, "InitPR Error: %v\n", err)
	}
	printDebug("PR: %d\n", client.PR.GetNumber())

	conf, err := owners.ReadConfig(*repo_dir)
	if err != nil {
		printWarning("Error reading codeowners.toml - using default config\n")
	}

	diffContext := diff.DiffContext{
		Base:       client.PR.Base.GetSHA(),
		Head:       client.PR.Head.GetSHA(),
		Dir:        *repo_dir,
		IgnoreDirs: conf.Ignore,
	}

	// Get the diff of the PR
	printDebug("Getting diff for %s...%s\n", diffContext.Base, diffContext.Head)
	gitDiff, err := diff.NewGitDiff(diffContext)
	if err != nil {
		errorAndExit(true, "NewGitDiff Error: %v\n", err)
	}
	changedFiles := f.Map(gitDiff.AllChanges(), func(s diff.DiffFile) string { return s.FileName })

	// Based on the diff, get the codeowner teams by traversing the directory tree upwards and map against Github users/teams
	codeOwners, err := codeowners.NewCodeOwners(*repo_dir, changedFiles, WarningBuffer)
	if err != nil {
		errorAndExit(true, "NewCodeOwners Error: %v\n", err)
	}
	for _, uFile := range codeOwners.UnownedFiles() {
		printWarning("WARNING: Unowned File: %s\n", uFile)
	}

	author := fmt.Sprintf("@%s", client.PR.User.GetLogin())
	codeOwners.SetAuthor(author)

	// Print the file owners
	if *verbose {
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

	// Get all required owners - before filtering by approvals
	allRequiredOwners := codeOwners.AllRequired()
	allRequiredOwnerNames := allRequiredOwners.Flatten()
	printDebug("All Required Owners: %s\n", allRequiredOwnerNames)

	// Get all optional reviewers - filter out required owners as its redundant
	allOptionalReviewerNames := codeOwners.AllOptional().Flatten()
	allOptionalReviewerNames = f.Filtered(allOptionalReviewerNames, func(name string) bool {
		return !slices.Contains(allRequiredOwnerNames, name)
	})
	printDebug("All Optional Reviewers: %s\n", allOptionalReviewerNames)

	err = client.InitUserReviewerMap(allRequiredOwnerNames)
	if err != nil {
		errorAndExit(true, "InitUserReviewerMap Error: %v\n", err)
	}

	// Get approvals from the PR and the diffs since each approval
	ghApprovals, err := client.GetCurrentReviewerApprovals()
	if err != nil {
		errorAndExit(true, "GetCurrentApprovals Error: %v\n", err)
	}
	printDebug("Current Approvals: %+v\n", ghApprovals)

	var tokenOwnerApproval *gh.CurrentApproval
	if conf.Enforcement.Approval {
		// Get the token owner login
		tokenOwner, err := client.GetTokenUser()
		if err != nil {
			printWarning("WARNING: You might be trying to use a bot as an Enforcement.Approval user," +
				" but this will not work due to GitHub CODEOWNERS not allowing bots as code owners." +
				" To use the Enforcement.Approval feature, the token must belong to a GitHub user account")

			conf.Enforcement.Approval = false
		} else {
			// Find the approval for the token owner
			tokenOwnerApproval, _ = client.FindUserApproval(tokenOwner)
		}
	}

	// Mark reviewers as approved if there have been no changes in owned files since the approval. Otherwise, dismiss as stale.
	fileReviewers := f.MapMap(codeOwners.FileRequired(), func(reviewers codeowners.ReviewerGroups) []string { return reviewers.Flatten() })
	approvers, approvalsToDismiss := client.CheckApprovals(fileReviewers, ghApprovals, gitDiff)
	codeOwners.ApplyApprovals(approvers)
	if len(approvalsToDismiss) > 0 {
		printDebug("Dismissing Stale Approvals: %+v\n", approvalsToDismiss)
		err = client.DismissStaleReviews(approvalsToDismiss)
		if err != nil {
			errorAndExit(true, "DismissStaleReviews Error: %v\n", err)
		}
	}
	validApprovalCount := len(ghApprovals) - len(approvalsToDismiss)

	// Get all required owners - after filtering by approvals
	unapprovedOwners := codeOwners.AllRequired()
	unapprovedOwnerNames := unapprovedOwners.Flatten()
	printDebug("Remaining Required Owners: %s\n", unapprovedOwnerNames)

	// Get currently requested owners
	currentlyRequestedOwners, err := client.GetCurrentlyRequested()
	if err != nil {
		errorAndExit(true, "GetCurrentlyRequested Error: %v\n", err)
	}
	printDebug("Currently Requested Owners: %s\n", currentlyRequestedOwners)

	// Get previously requested (review submitted) owners
	previousReviewers, err := client.GetAlreadyReviewed()
	if err != nil {
		errorAndExit(true, "GetAlreadyReviewed Error: %v\n", err)
	}

	// Request reviews from the required owners not already requested
	filteredOwners := unapprovedOwners.FilterOut(currentlyRequestedOwners...)
	filteredOwners = filteredOwners.FilterOut(previousReviewers...)
	filteredOwnerNames := filteredOwners.Flatten()
	if len(filteredOwners) > 0 {
		printDebug("Requesting Reviews from: %s\n", filteredOwnerNames)
		err = client.RequestReviewers(filteredOwnerNames)
		if err != nil {
			errorAndExit(true, "RequestReviewers Error: %v\n", err)
		}
	}

	maxReviewsMet := false
	if conf.MaxReviews != nil && *conf.MaxReviews > 0 {
		if validApprovalCount >= *conf.MaxReviews && len(f.Intersection(unapprovedOwners.Flatten(), conf.UnskippableReviewers)) == 0 {
			maxReviewsMet = true
		}
	}

	// Add comments to the PR if necessary
	if len(unapprovedOwners) > 0 {
		// Comment on the PR with the codeowner teams that have not approved the PR
		comment := allRequiredOwners.ToCommentString()
		if maxReviewsMet {
			comment += "\n\n"
			comment += "The PR has received the max number of required reviews.  No further action is required."
		}
		fiveDaysAgo := time.Now().AddDate(0, 0, -5)
		found, err := client.IsInComments(comment, &fiveDaysAgo)
		if err != nil {
			errorAndExit(true, "IsInComments Error: %v\n", err)
		}
		if !found {
			err = client.AddComment(comment)
			if err != nil {
				errorAndExit(true, "AddComment Error: %v\n", err)
			}
		}
	}
	if len(allOptionalReviewerNames) > 0 {
		// Add CC comment to the PR with the optional reviewers that have not already been mentioned in the PR comments
		viewersToPing := f.Filtered(allOptionalReviewerNames, func(name string) bool {
			found, err := client.IsSubstringInComments(name, nil)
			if err != nil {
				errorAndExit(true, "IsInComments Error: %v\n", err)
			}
			return !found
		})
		if len(viewersToPing) > 0 {
			comment := fmt.Sprintf("cc %s", strings.Join(viewersToPing, " "))
			err = client.AddComment(comment)
			if err != nil {
				errorAndExit(true, "AddComment Error: %v\n", err)
			}
		}
	}

	// Exit if there are any unapproved codeowner teams
	if len(unapprovedOwners) > 0 && !maxReviewsMet {
		// Return failed status if any codeowner team has not approved the PR
		unapprovedCommentStrings := f.Map(unapprovedOwners, func(s *codeowners.ReviewerGroup) string {
			return s.ToCommentString()
		})
		if conf.Enforcement.Approval && tokenOwnerApproval != nil {
			_ = client.DismissStaleReviews([]*gh.CurrentApproval{tokenOwnerApproval})
		}
		errorAndExit(
			conf.Enforcement.FailCheck,
			"FAIL: Codeowners reviews not satisfied\nStill required:\n - %s\n",
			strings.Join(unapprovedCommentStrings, "\n - "),
		)
	}

	// Exit if there are not enough reviews
	if conf.MinReviews != nil && *conf.MinReviews > 0 {
		allApprovals, _ := client.AllApprovals()
		approvalCount := len(allApprovals) - len(approvalsToDismiss)
		if approvalCount < *conf.MinReviews {
			errorAndExit(conf.Enforcement.FailCheck, "FAIL: Min Reviews not satisfied. Need %d, found %d\n", *conf.MinReviews, approvalCount)
		}
	}

	if conf.Enforcement.Approval && tokenOwnerApproval == nil {
		// Approve the PR since all codeowner teams have approved
		err = client.ApprovePR()
		if err != nil {
			errorAndExit(true, "ApprovePR Error: %v\n", err)
		}
	}

	_, err = WarningBuffer.WriteTo(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing warning buffer: %v\n", err)
	}
	if *verbose {
		_, err = InfoBuffer.WriteTo(os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing info buffer: %v\n", err)
		}
	}
	fmt.Println("Codeowners reviews satisfied")
}
