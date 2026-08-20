// Package oracle implements ownership oracles: reviewer requirements
// produced by tooling outside the .codeowners file tree (for example a
// semantic diff analyzer) and fed into codeowners-plus as data.
//
// An oracle file is JSON:
//
//	{
//	  "rules": [
//	    {
//	      "files": ["src/telemetry/**", "src/events.py"],
//	      "owners": ["@org/data-platform"],
//	      "optional": false,
//	      "reason": "telemetry event schema changed"
//	    }
//	  ]
//	}
//
// Each rule's owners form a single OR group (any listed owner satisfies the
// rule), matching .codeowners semantics. Distinct rules matching the same
// file are AND-ed together. Oracle rules can only ADD reviewer requirements.
package oracle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

// Rule is a single oracle reviewer requirement.
type Rule struct {
	// Files are doublestar glob patterns matched against full paths of
	// files changed in the PR, relative to the repository root.
	Files []string `json:"files"`
	// Owners is an OR group: any one of these reviewers satisfies the rule.
	Owners []string `json:"owners"`
	// Optional marks the owners as non-blocking (CC'd instead of required).
	Optional bool `json:"optional"`
	// Reason is a human-readable explanation of why the rule exists.
	// It is surfaced in verbose output only.
	Reason string `json:"reason"`
}

// RuleSet is the parsed contents of an oracle file.
type RuleSet struct {
	Rules []Rule `json:"rules"`
}

// Parse decodes and validates oracle file contents. Validation is strict:
// a rule that cannot take effect (no files, no owners, or an invalid glob
// pattern) is an error, since skipping it would drop required reviews.
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
			// An empty pattern is "valid" per doublestar but matches
			// nothing, which would silently disable the rule.
			if pattern == "" || !doublestar.ValidatePattern(pattern) {
				return nil, fmt.Errorf("oracle rule %d has an invalid pattern %q", i, pattern)
			}
		}
		if len(rule.Owners) == 0 {
			return nil, fmt.Errorf("oracle rule %d has no owners", i)
		}
		for _, owner := range rule.Owners {
			if owner == "" {
				return nil, fmt.Errorf("oracle rule %d has an empty owner", i)
			}
		}
	}
	return &ruleSet, nil
}

// Load reads and parses an oracle file from disk.
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

// ToCodeOwners builds a codeowners.CodeOwners containing the rule set's
// requirements for the given changed files. Files matching no rule are
// omitted entirely, so the result is suitable for AND-merging into the
// .codeowners-derived ownership via codeowners.MergeCodeOwners.
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
		// Parse rejects invalid patterns, but a RuleSet can also be
		// constructed directly, so pattern errors are still handled.
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
