package app

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
)

// NewFromFileOwners is covered here (rather than pkg/codeowners) because
// its consumers are the oracle and inline-ownership merge paths.
func TestNewFromFileOwners(t *testing.T) {
	rgm := codeowners.NewReviewerGroupMemo()
	co := codeowners.NewFromFileOwners(
		map[string]codeowners.ReviewerGroups{
			"both.go":     {rgm.ToReviewerGroup("@required")},
			"required.go": {rgm.ToReviewerGroup("@required"), rgm.ToReviewerGroup("@required")},
		},
		map[string]codeowners.ReviewerGroups{
			"both.go":     {rgm.ToReviewerGroup("@optional")},
			"optional.go": {rgm.ToReviewerGroup("@optional")},
		},
	)

	if len(co.FileRequired()["both.go"]) != 1 || len(co.FileOptional()["both.go"]) != 1 {
		t.Errorf("expected both.go to carry one required and one optional group, got %+v / %+v",
			co.FileRequired()["both.go"], co.FileOptional()["both.go"])
	}
	if len(co.FileRequired()["required.go"]) != 1 {
		t.Errorf("expected duplicate groups to be deduplicated, got %d", len(co.FileRequired()["required.go"]))
	}
	if _, ok := co.FileRequired()["optional.go"]; ok {
		t.Error("optional-only file should have no required reviewers")
	}
	if len(co.UnownedFiles()) != 0 {
		t.Errorf("expected no unowned files, got %v", co.UnownedFiles())
	}
}

func writeOracleFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oracle.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func oracleTestApp(oracleFiles []string) *App {
	return &App{
		config: &Config{
			OracleFiles:   oracleFiles,
			InfoBuffer:    &bytes.Buffer{},
			WarningBuffer: &bytes.Buffer{},
		},
	}
}

func baseCodeOwnersForOracleTest() codeowners.CodeOwners {
	rgm := codeowners.NewReviewerGroupMemo()
	return codeowners.NewFromFileOwners(map[string]codeowners.ReviewerGroups{
		"src/telemetry/events.ts": {rgm.ToReviewerGroup("@org/frontend")},
	}, nil)
}

func TestApplyOraclesNoFiles(t *testing.T) {
	app := oracleTestApp(nil)
	base := baseCodeOwnersForOracleTest()
	result, err := app.applyOracles(base, mockGitDiff{changes: []string{"src/telemetry/events.ts"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != base {
		t.Error("expected base CodeOwners to be returned unchanged when no oracle files are configured")
	}
}

func TestApplyOraclesMergesRequirements(t *testing.T) {
	path := writeOracleFile(t, `{"rules": [
		{"files": ["src/telemetry/**"], "owners": ["@org/data-platform"], "reason": "NR event change"}
	]}`)
	app := oracleTestApp([]string{path})
	base := baseCodeOwnersForOracleTest()

	result, err := app.applyOracles(base, mockGitDiff{changes: []string{"src/telemetry/events.ts", "other.go"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	groups := result.FileRequired()["src/telemetry/events.ts"]
	names := codeowners.OriginalStrings(groups.Flatten())
	if !slices.Contains(names, "@org/frontend") || !slices.Contains(names, "@org/data-platform") {
		t.Errorf("expected both base and oracle owners, got %v", names)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 AND groups, got %d", len(groups))
	}
}

func TestApplyOraclesMissingFile(t *testing.T) {
	app := oracleTestApp([]string{"/nonexistent/oracle.json"})
	_, err := app.applyOracles(baseCodeOwnersForOracleTest(), mockGitDiff{changes: []string{"a.go"}})
	if err == nil || !strings.Contains(err.Error(), "Oracle Error") {
		t.Errorf("expected hard error for missing oracle file, got %v", err)
	}
}

func TestApplyOraclesMultipleFiles(t *testing.T) {
	pathA := writeOracleFile(t, `{"rules": [
		{"files": ["src/telemetry/**"], "owners": ["@org/data-platform"]}
	]}`)
	pathB := writeOracleFile(t, `{"rules": [
		{"files": ["src/**"], "owners": ["@org/security"]}
	]}`)
	app := oracleTestApp([]string{pathA, pathB})
	base := baseCodeOwnersForOracleTest()

	result, err := app.applyOracles(base, mockGitDiff{changes: []string{"src/telemetry/events.ts"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	groups := result.FileRequired()["src/telemetry/events.ts"]
	names := codeowners.OriginalStrings(groups.Flatten())
	for _, expected := range []string{"@org/frontend", "@org/data-platform", "@org/security"} {
		if !slices.Contains(names, expected) {
			t.Errorf("expected owner %s from merged oracle files, got %v", expected, names)
		}
	}
	if len(groups) != 3 {
		t.Errorf("expected 3 AND groups (base + one per oracle file), got %d", len(groups))
	}
}

func TestApplyOraclesUnownedFileBecomesOwned(t *testing.T) {
	// A file .codeowners reports as unowned stops being reported once an
	// oracle rule matches it (documented behavior).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := mockGitDiff{changes: []string{"a.go"}}
	base, err := codeowners.New(dir, diff.AllChanges(), nil, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.UnownedFiles()) != 1 {
		t.Fatalf("expected a.go to start unowned, got %v", base.UnownedFiles())
	}

	path := writeOracleFile(t, `{"rules": [{"files": ["a.go"], "owners": ["@org/adopters"]}]}`)
	app := oracleTestApp([]string{path})
	result, err := app.applyOracles(base, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.UnownedFiles()) != 0 {
		t.Errorf("oracle-matched file should count as owned, got unowned: %v", result.UnownedFiles())
	}
}

func TestApplyOraclesEmptyRules(t *testing.T) {
	path := writeOracleFile(t, `{"rules": []}`)
	app := oracleTestApp([]string{path})
	base := baseCodeOwnersForOracleTest()
	result, err := app.applyOracles(base, mockGitDiff{changes: []string{"a.go"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != base {
		t.Error("expected base CodeOwners to be returned unchanged for empty oracle rules")
	}
}
