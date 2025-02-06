package codeowners

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
)

type Rules struct {
	Fallback                *ReviewerGroup
	OwnerTests              FileTestCases
	AdditionalReviewerTests FileTestCases
	OptionalReviewerTests   FileTestCases
}

// Read the .codeowners file and return the fallback owner, ownership tests, and additional ownership tests
func Read(path string, reviewerGroupManager ReviewerGroupManager, warningWriter io.Writer) Rules {
	rules := Rules{
		Fallback:                nil,
		OwnerTests:              FileTestCases{},
		AdditionalReviewerTests: FileTestCases{},
		OptionalReviewerTests:   FileTestCases{},
	}

	ls, error := os.Lstat(path)
	if error != nil || !ls.IsDir() {
		return rules
	}

	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	file, err := os.Open(path + ".codeowners")
	if err != nil {
		return rules
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}

		additional := false
		optional := false
		if strings.HasPrefix(line, "&") {
			additional = true
			line = line[1:]
		}
		if strings.HasPrefix(line, "?") {
			optional = true
			line = line[1:]
		}
		line = strings.TrimSpace(line)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			fmt.Fprintln(warningWriter, "WARNING: Invalid line in .codeowners file:", line)
			continue
		}
		match := parts[0]
		if strings.HasPrefix(match, "/") {
			fmt.Fprintln(warningWriter, "WARNING: Leading `/` ignored by `.codeowners`:", match)
			// strip leading slash - all matches are relative to the current directory
			match = match[1:]
		}
		if strings.HasSuffix(match, "/") {
			fmt.Fprintln(warningWriter, "WARNING: Trailing `/` not supported by `.codeowners` - replacing with `/**`:", match)
			match = match + "**"
		}
		owner := parts[1:]
		if match == "*" {
			if !additional && !optional {
				rules.Fallback = reviewerGroupManager.ToReviewerGroup(owner...)
				continue
			} else {
				match = "**/*"
			}
		}
		test := &reviewerTest{Match: match, Reviewer: reviewerGroupManager.ToReviewerGroup(owner...)}
		if additional {
			rules.AdditionalReviewerTests = append(rules.AdditionalReviewerTests, test)
		} else if optional {
			rules.OptionalReviewerTests = append(rules.OptionalReviewerTests, test)
		} else {
			rules.OwnerTests = append(rules.OwnerTests, test)
		}
	}

	slices.Reverse(rules.OwnerTests)
	sort.Sort(rules.OwnerTests)

	slices.Reverse(rules.AdditionalReviewerTests)
	sort.Sort(rules.AdditionalReviewerTests)

	slices.Reverse(rules.OptionalReviewerTests)
	sort.Sort(rules.OptionalReviewerTests)

	return rules
}
