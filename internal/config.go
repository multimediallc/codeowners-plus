package owners

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type CodeownersConfig struct {
	MaxReviews           *int         `toml:"max_reviews"`
	MinReviews           *int         `toml:"min_reviews"`
	UnskippableReviewers []string     `toml:"unskippable_reviewers"`
	Ignore               []string     `toml:"ignore"`
	Enforcement          *Enforcement `toml:"enforcement"`
}

type Enforcement struct {
	Approval  bool `toml:"approval"`
	FailCheck bool `toml:"fail_check"`
}

func ReadCodeownersConfig(path string) (*CodeownersConfig, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	defaultConfig := &CodeownersConfig{
		MaxReviews:           nil,
		MinReviews:           nil,
		UnskippableReviewers: []string{},
		Ignore:               []string{},
		Enforcement:          &Enforcement{Approval: false, FailCheck: true},
	}

	fileName := path + "codeowners.toml"
	if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
		return defaultConfig, nil
	}
	file, err := os.ReadFile(fileName)
	if err != nil {
		return defaultConfig, err
	}
	config := defaultConfig
	err = toml.Unmarshal(file, &config)
	if err != nil {
		return defaultConfig, err
	}
	if config.Enforcement == nil {
		config.Enforcement = defaultConfig.Enforcement
	}
	return config, nil
}

type CodeownersRules struct {
	Fallback                *ReviewerGroup
	OwnerTests              FileTestCases
	AdditionalReviewerTests FileTestCases
	OptionalReviewerTests   FileTestCases
}

// Read the .codeowners file and return the fallback owner, ownership tests, and additional ownership tests
func ReadCodeownersFile(path string, reviewerGroupManager ReviewerGroupManager) CodeownersRules {
	rules := CodeownersRules{
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
			fmt.Fprintln(WarningBuffer, "WARNING: Invalid line in .codeowners file:", line)
			continue
		}
		match := parts[0]
		if strings.HasPrefix(match, "/") {
			fmt.Fprintln(WarningBuffer, "WARNING: Leading `/` ignored by `.codeowners`:", match)
			// strip leading slash - all matches are relative to the current directory
			match = match[1:]
		}
		if strings.HasSuffix(match, "/") {
			fmt.Fprintln(WarningBuffer, "WARNING: Trailing `/` not supported by `.codeowners` - replacing with `/**`:", match)
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
