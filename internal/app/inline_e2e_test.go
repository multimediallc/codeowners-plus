package app

// End-to-end tests for inline ownership: build a real git repository with
// inline-tagged source files and run the real diff parser, git-ref file
// readers, and applyInlineOwnership over each change shape.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/internal/git"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/multimediallc/codeowners-plus/pkg/inlineowners"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func inlineOwnersByFile(requirements []inlineowners.Requirement) map[string][]string {
	result := make(map[string][]string)
	for _, requirement := range requirements {
		for _, owner := range requirement.Owners {
			if !slices.Contains(result[requirement.File], owner) {
				result[requirement.File] = append(result[requirement.File], owner)
			}
		}
	}
	return result
}

const padding = "pad1\npad2\npad3\npad4\npad5\npad6\n"

func TestInlineOwnershipE2E(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")

	// Base revision
	writeRepoFile(t, dir, "frontend/a.ts", padding+
		"// <CO-inline={@frontend-inline}>\nconst a = 1;\n// </CO-inline>\n"+padding)
	writeRepoFile(t, dir, "frontend/b.ts",
		"// <CO-inline={@frontend-inline}>\nconst b = 1;\n// </CO-inline>\n")
	writeRepoFile(t, dir, "models.py",
		"# <CO-inline={@model-owner,@devops}>\nfield_one = 1\nfield_two = 2\n# </CO-inline>\n")
	writeRepoFile(t, dir, "deleted.go",
		"// <CO-inline={@keeper}>\nvar precious = true\n// </CO-inline>\n")
	writeRepoFile(t, dir, "appended.ts",
		"// <CO-inline={@append-guard}>\nconst guarded = 1;\n// </CO-inline>\n")
	writeRepoFile(t, dir, "renamed_src.ts", padding+padding+
		"// <CO-inline={@rename-guard}>\nconst secret = 1;\n// </CO-inline>\n"+padding+padding)
	writeRepoFile(t, dir, "plain.ts", padding+"const plain = 1;\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "base")
	baseSHA := gitRun(t, dir, "rev-parse", "HEAD")

	// Head revision
	writeRepoFile(t, dir, "frontend/a.ts", padding+
		"// <CO-inline={@frontend-inline}>\nconst a = 1;\nconst added = 2;\n// </CO-inline>\n"+padding)
	writeRepoFile(t, dir, "frontend/b.ts",
		"// <CO-inline={@attacker}>\nconst b = 1;\n// </CO-inline>\n")
	writeRepoFile(t, dir, "models.py",
		"# <CO-inline={@model-owner,@devops}>\nfield_one = 1\n# </CO-inline>\n")
	if err := os.Remove(filepath.Join(dir, "deleted.go")); err != nil {
		t.Fatal(err)
	}
	// Append a line AFTER the block's end tag: the insertion produces a
	// zero-length base-side hunk, which must not count as touching the block.
	writeRepoFile(t, dir, "appended.ts",
		"// <CO-inline={@append-guard}>\nconst guarded = 1;\n// </CO-inline>\nconst afterBlock = 2;\n")
	// Rename with no content change: base blocks still require approval.
	gitRun(t, dir, "mv", "renamed_src.ts", "renamed_dst.ts")
	writeRepoFile(t, dir, "plain.ts", padding+"const plainChanged = 2;\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "head")
	headSHA := gitRun(t, dir, "rev-parse", "HEAD")

	gitDiff, err := git.NewDiff(git.DiffContext{Base: baseSHA, Head: headSHA, Dir: dir})
	if err != nil {
		t.Fatalf("NewDiff error: %v", err)
	}
	baseReader := git.NewGitRefFileReader(baseSHA, dir)
	headReader := git.NewGitRefFileReader(headSHA, dir)

	warn := &bytes.Buffer{}
	requirements, err := inlineowners.Requirements(gitDiff.AllChanges(), baseReader, headReader, warn)
	if err != nil {
		t.Fatalf("Requirements error: %v", err)
	}

	owners := inlineOwnersByFile(requirements)
	expected := map[string][]string{
		// edit inside a block requires the block owner
		"frontend/a.ts": {"@frontend-inline"},
		// tampering with the tag requires the base owner AND the new owner
		"frontend/b.ts": {"@frontend-inline", "@attacker"},
		// deleting a line inside a block requires the block owners
		"models.py": {"@model-owner", "@devops"},
		// deleting a whole owned file requires the owner recorded at base
		"deleted.go": {"@keeper"},
		// a pure rename requires the owners of all base blocks
		"renamed_dst.ts": {"@rename-guard"},
	}
	for file, expectedOwners := range expected {
		for _, owner := range expectedOwners {
			if !slices.Contains(owners[file], owner) {
				t.Errorf("expected %s to require %s, got %v", file, owner, owners[file])
			}
		}
	}
	// appended.ts (change after the block) and plain.ts require nothing
	for _, file := range []string{"appended.ts", "plain.ts"} {
		if len(owners[file]) != 0 {
			t.Errorf("expected no inline owners for %s, got %v", file, owners[file])
		}
	}

	// Push the requirements through the app merge path and confirm they
	// compose with .codeowners-derived ownership.
	rgm := codeowners.NewReviewerGroupMemo()
	base := codeowners.NewFromFileOwners(map[string]codeowners.ReviewerGroups{
		"frontend/a.ts": {rgm.ToReviewerGroup("@frontend-team")},
	}, nil)
	app := oracleTestApp(nil)
	merged, err := app.applyInlineOwnership(base, gitDiff, baseReader, headReader)
	if err != nil {
		t.Fatalf("applyInlineOwnership error: %v", err)
	}

	aOwners := codeowners.OriginalStrings(merged.FileRequired()["frontend/a.ts"].Flatten())
	if !slices.Contains(aOwners, "@frontend-team") || !slices.Contains(aOwners, "@frontend-inline") {
		t.Errorf("expected merged owners to include .codeowners and inline owners, got %v", aOwners)
	}
	bOwners := codeowners.OriginalStrings(merged.FileRequired()["frontend/b.ts"].Flatten())
	if !slices.Contains(bOwners, "@frontend-inline") || !slices.Contains(bOwners, "@attacker") {
		t.Errorf("expected merged owners for tampered tag, got %v", bOwners)
	}
}

func TestInlineOwnershipAdvancedBaseBranch(t *testing.T) {
	// Diffs are three-dot (merge-base...head), so base-side hunk line
	// numbers are relative to the merge base. When the base branch
	// advances after the PR branches (shifting the protected block's
	// lines at the base tip), blocks must be read at the merge base or
	// the overlap check misses them.
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")

	writeRepoFile(t, dir, "guarded.ts", padding+
		"// <CO-inline={@guard}>\nconst guarded = 1;\n// </CO-inline>\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "base")
	branchPoint := gitRun(t, dir, "rev-parse", "HEAD")

	// PR branch: edit inside the block
	gitRun(t, dir, "checkout", "-b", "feature")
	writeRepoFile(t, dir, "guarded.ts", padding+
		"// <CO-inline={@guard}>\nconst guarded = 2;\n// </CO-inline>\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "head")
	headSHA := gitRun(t, dir, "rev-parse", "HEAD")

	// Base branch advances: prepend lines, shifting the block down
	gitRun(t, dir, "checkout", "main")
	writeRepoFile(t, dir, "guarded.ts", padding+padding+padding+
		"// <CO-inline={@guard}>\nconst guarded = 1;\n// </CO-inline>\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "base advances")
	baseTipSHA := gitRun(t, dir, "rev-parse", "HEAD")

	mergeBase, err := git.MergeBase(dir, baseTipSHA, headSHA)
	if err != nil {
		t.Fatalf("MergeBase error: %v", err)
	}
	if mergeBase != branchPoint {
		t.Fatalf("expected merge base %s, got %s", branchPoint, mergeBase)
	}

	gitDiff, err := git.NewDiff(git.DiffContext{Base: baseTipSHA, Head: headSHA, Dir: dir})
	if err != nil {
		t.Fatalf("NewDiff error: %v", err)
	}
	headReader := git.NewGitRefFileReader(headSHA, dir)

	// Reading base blocks at the merge base finds the touched block.
	requirements, err := inlineowners.Requirements(gitDiff.AllChanges(), git.NewGitRefFileReader(mergeBase, dir), headReader, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Requirements error: %v", err)
	}
	if !slices.Contains(inlineOwnersByFile(requirements)["guarded.ts"], "@guard") {
		t.Errorf("expected @guard requirement with merge-base reader, got %+v", requirements)
	}
}

func TestApplyInlineOwnershipNoBlocks(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	writeRepoFile(t, dir, "plain.ts", "const plain = 1;\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "base")
	baseSHA := gitRun(t, dir, "rev-parse", "HEAD")
	writeRepoFile(t, dir, "plain.ts", "const plainChanged = 2;\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "head")
	headSHA := gitRun(t, dir, "rev-parse", "HEAD")

	gitDiff, err := git.NewDiff(git.DiffContext{Base: baseSHA, Head: headSHA, Dir: dir})
	if err != nil {
		t.Fatalf("NewDiff error: %v", err)
	}

	base := baseCodeOwnersForOracleTest()
	app := oracleTestApp(nil)
	merged, err := app.applyInlineOwnership(base, gitDiff, git.NewGitRefFileReader(baseSHA, dir), git.NewGitRefFileReader(headSHA, dir))
	if err != nil {
		t.Fatalf("applyInlineOwnership error: %v", err)
	}
	if merged != base {
		t.Error("expected base CodeOwners returned unchanged when no inline blocks are touched")
	}
}
