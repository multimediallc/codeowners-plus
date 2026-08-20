package oracle

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

func TestParse(t *testing.T) {
	tt := []struct {
		name          string
		input         string
		expectedRules int
		expectedErr   string
	}{
		{
			name:          "valid rule set",
			input:         `{"rules": [{"files": ["a/**"], "owners": ["@team-a"], "reason": "test"}]}`,
			expectedRules: 1,
		},
		{
			name:          "empty rules",
			input:         `{"rules": []}`,
			expectedRules: 0,
		},
		{
			name:          "no rules key",
			input:         `{}`,
			expectedRules: 0,
		},
		{
			name:        "invalid JSON",
			input:       `{"rules": [`,
			expectedErr: "invalid oracle JSON",
		},
		{
			name:        "rule without files",
			input:       `{"rules": [{"files": [], "owners": ["@team-a"]}]}`,
			expectedErr: "rule 0 has no files",
		},
		{
			name:        "rule with invalid pattern",
			input:       `{"rules": [{"files": ["[invalid"], "owners": ["@team-a"]}]}`,
			expectedErr: `rule 0 has an invalid pattern "[invalid"`,
		},
		{
			name:        "rule with empty pattern",
			input:       `{"rules": [{"files": [""], "owners": ["@team-a"]}]}`,
			expectedErr: `rule 0 has an invalid pattern ""`,
		},
		{
			name:        "rule without owners",
			input:       `{"rules": [{"files": ["a.go"], "owners": []}]}`,
			expectedErr: "rule 0 has no owners",
		},
		{
			name:        "rule with empty owner",
			input:       `{"rules": [{"files": ["a.go"], "owners": [""]}]}`,
			expectedErr: "rule 0 has an empty owner",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ruleSet, err := Parse([]byte(tc.input))
			if tc.expectedErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectedErr) {
					t.Fatalf("expected error containing %q, got %v", tc.expectedErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ruleSet.Rules) != tc.expectedRules {
				t.Errorf("expected %d rules, got %d", tc.expectedRules, len(ruleSet.Rules))
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oracle.json")
	content := `{"rules": [{"files": ["src/**"], "owners": ["@org/data-platform"], "reason": "telemetry"}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ruleSet, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ruleSet.Rules) != 1 || ruleSet.Rules[0].Owners[0] != "@org/data-platform" {
		t.Errorf("unexpected rule set: %+v", ruleSet)
	}

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("expected error for missing file")
	}

	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(badPath); err == nil || !strings.Contains(err.Error(), badPath) {
		t.Errorf("expected error naming the file, got %v", err)
	}
}

func requiredNames(co codeowners.CodeOwners, file string) []string {
	groups, ok := co.FileRequired()[file]
	if !ok {
		return nil
	}
	return codeowners.OriginalStrings(groups.Flatten())
}

func TestToCodeOwners(t *testing.T) {
	ruleSet := &RuleSet{Rules: []Rule{
		{Files: []string{"src/telemetry/**"}, Owners: []string{"@org/data-platform"}, Reason: "telemetry"},
		{Files: []string{"src/telemetry/events.ts"}, Owners: []string{"@org/frontend"}},
		{Files: []string{"docs/**"}, Owners: []string{"@org/docs"}, Optional: true},
	}}
	changed := []string{
		"src/telemetry/events.ts",
		"src/telemetry/nested/deep.py",
		"docs/readme.md",
		"unrelated/main.go",
	}

	warnings := &bytes.Buffer{}
	co := ruleSet.ToCodeOwners(changed, warnings)

	// Two rules match events.ts: their groups AND together
	names := requiredNames(co, "src/telemetry/events.ts")
	if !slices.Contains(names, "@org/data-platform") || !slices.Contains(names, "@org/frontend") {
		t.Errorf("expected both oracle owners for events.ts, got %v", names)
	}
	if len(co.FileRequired()["src/telemetry/events.ts"]) != 2 {
		t.Errorf("expected 2 AND groups for events.ts, got %d", len(co.FileRequired()["src/telemetry/events.ts"]))
	}

	// Globstar matches nested files
	if names := requiredNames(co, "src/telemetry/nested/deep.py"); !slices.Contains(names, "@org/data-platform") {
		t.Errorf("expected data-platform for nested file, got %v", names)
	}

	// Optional rules go to FileOptional, not FileRequired
	if _, ok := co.FileRequired()["docs/readme.md"]; ok {
		t.Error("optional rule should not create required reviewers")
	}
	optional := co.FileOptional()["docs/readme.md"]
	if len(optional) != 1 || !slices.Contains(codeowners.OriginalStrings(optional.Flatten()), "@org/docs") {
		t.Errorf("expected optional docs owner, got %v", optional)
	}

	// Unmatched files are not tracked and not unowned
	if _, ok := co.FileRequired()["unrelated/main.go"]; ok {
		t.Error("unmatched file should not be tracked")
	}
	if len(co.UnownedFiles()) != 0 {
		t.Errorf("oracle CodeOwners should report no unowned files, got %v", co.UnownedFiles())
	}

	// Approvals satisfy oracle groups, case-insensitively
	co.ApplyApprovals([]codeowners.Slug{codeowners.NewSlug("@ORG/Data-Platform")})
	if names := requiredNames(co, "src/telemetry/nested/deep.py"); len(names) != 0 {
		t.Errorf("expected no remaining required reviewers after approval, got %v", names)
	}
}

func TestToCodeOwnersNilWarningWriter(t *testing.T) {
	ruleSet := &RuleSet{Rules: []Rule{
		{Files: []string{"[invalid"}, Owners: []string{"@org/data-platform"}},
	}}
	// Must not panic: the bad pattern's warning goes to io.Discard.
	co := ruleSet.ToCodeOwners([]string{"a.go"}, nil)
	if len(co.FileRequired()) != 0 {
		t.Errorf("bad pattern should match nothing, got %v", co.FileRequired())
	}
}

func TestToCodeOwnersBadPattern(t *testing.T) {
	// Parse rejects invalid patterns, but a RuleSet constructed directly
	// can still carry one; matching warns and skips the pattern.
	ruleSet := &RuleSet{Rules: []Rule{
		{Files: []string{"[invalid"}, Owners: []string{"@org/data-platform"}},
	}}
	warnings := &bytes.Buffer{}
	co := ruleSet.ToCodeOwners([]string{"a.go"}, warnings)
	if len(co.FileRequired()) != 0 {
		t.Errorf("bad pattern should match nothing, got %v", co.FileRequired())
	}
	if !strings.Contains(warnings.String(), "PatternError") {
		t.Errorf("expected pattern warning, got %q", warnings.String())
	}
}
