// Package oracle loads reviewer requirements computed by external tooling
// (JSON format documented in the README under "Ownership Oracles"). Oracle
// rules can only add reviewer requirements, never weaken .codeowners ones.
package oracle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

// Rule is a single oracle reviewer requirement.
type Rule struct {
	// Files are doublestar globs matched against repo-relative changed paths.
	Files []string `json:"files"`
	// Owners is an OR group: any one listed reviewer satisfies the rule.
	Owners []string `json:"owners"`
	// Optional owners are CC'd instead of required.
	Optional bool `json:"optional"`
	// Reason is surfaced in verbose output.
	Reason string `json:"reason"`
}

// RuleSet is the parsed contents of an oracle file.
type RuleSet struct {
	Rules []Rule `json:"rules"`
}

// Parse decodes and validates oracle JSON. A rule that cannot take effect
// is an error, since silently skipping it would drop required reviews.
func Parse(data []byte) (*RuleSet, error) {
	var ruleSet RuleSet
	if err := json.Unmarshal(data, &ruleSet); err != nil {
		return nil, fmt.Errorf("invalid oracle JSON: %w", err)
	}
	for i, rule := range ruleSet.Rules {
		if len(rule.Files) == 0 {
			return nil, fmt.Errorf("oracle rule %d has no files", i)
		}
		for _, pattern := range rule.Files {
			// Empty patterns are "valid" per doublestar but match nothing.
			if pattern == "" || !doublestar.ValidatePattern(pattern) {
				return nil, fmt.Errorf("oracle rule %d has an invalid pattern %q", i, pattern)
			}
			// Doublestar globs are root-anchored; a leading slash never matches.
			if strings.HasPrefix(pattern, "/") {
				return nil, fmt.Errorf("oracle rule %d has a leading-slash pattern %q (patterns are repo-root-relative; drop the leading slash)", i, pattern)
			}
		}
		if len(rule.Owners) == 0 {
			return nil, fmt.Errorf("oracle rule %d has no owners", i)
		}
		for _, owner := range rule.Owners {
			if strings.TrimSpace(owner) == "" {
				return nil, fmt.Errorf("oracle rule %d has an empty owner", i)
			}
		}
	}
	return &ruleSet, nil
}

// Load reads and parses an oracle file.
func Load(path string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading oracle file: %w", err)
	}
	ruleSet, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("oracle file %s: %w", path, err)
	}
	return ruleSet, nil
}

// ToCodeOwners builds the rule set's requirements for the changed files,
// suitable for AND-merging via codeowners.MergeCodeOwners.
func (rs *RuleSet) ToCodeOwners(changedFiles []string, warningWriter io.Writer) codeowners.CodeOwners {
	if warningWriter == nil {
		warningWriter = io.Discard
	}
	rgm := codeowners.NewReviewerGroupMemo()
	required := make(map[string]codeowners.ReviewerGroups)
	optional := make(map[string]codeowners.ReviewerGroups)
	for _, rule := range rs.Rules {
		group := rgm.ToReviewerGroup(rule.Owners...)
		target := required
		if rule.Optional {
			target = optional
		}
		for _, file := range changedFiles {
			if !rule.matches(file, warningWriter) {
				continue
			}
			target[file] = append(target[file], group)
		}
	}
	return codeowners.NewFromFileOwners(required, optional)
}

func (r *Rule) matches(file string, warningWriter io.Writer) bool {
	for _, pattern := range r.Files {
		// A directly-constructed RuleSet can carry patterns Parse would reject.
		match, err := doublestar.Match(pattern, file)
		if err != nil {
			_, _ = fmt.Fprintf(warningWriter, "WARNING: PatternError for oracle pattern '%s': %s\n", pattern, err)
			continue
		}
		if match {
			return true
		}
	}
	return false
}
