package app

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
	"github.com/multimediallc/codeowners-plus/pkg/inlineowners"
	"github.com/sourcegraph/go-diff/diff"
)

// TestInlineRepoFixture_EndToEnd ensures that inline ownership blocks in the
// test_project_inline fixture correctly override file-level CODEOWNERS rules
// for the changed lines contained in the diff fixture.
func TestInlineRepoFixture_EndToEnd(t *testing.T) {
	// Location of the fixture relative to this test file (internal/app)
	repoDir := filepath.Join("..", "..", "test_project_inline")

	// 1. Load and parse the diff fixture -------------------------------------------------
	diffBytes, err := os.ReadFile(filepath.Join(repoDir, ".diff_changes"))
	if err != nil {
		t.Fatalf("read diff fixture: %v", err)
	}

	parsed, err := diff.ParseMultiFileDiff(diffBytes)
	if err != nil {
		t.Fatalf("parse diff: %v", err)
	}

	// Convert to []codeowners.DiffFile (simplified in-place variant of git.toDiffFiles)
	diffFiles := make([]codeowners.DiffFile, 0, len(parsed))
	for _, fd := range parsed {
		// Tolerate malformed diffs from the test fixture
		if fd.NewName == "/dev/null" || len(fd.NewName) < 3 {
			continue
		}
		df := codeowners.DiffFile{
			FileName: strings.TrimPrefix(fd.NewName[2:], "test_project_inline/"), // strip leading "b/" & repo dir
			Hunks:    make([]codeowners.HunkRange, 0, len(fd.Hunks)),
		}
		for _, h := range fd.Hunks {
			hr := codeowners.HunkRange{
				Start: int(h.NewStartLine),
				End:   int(h.NewStartLine + h.NewLines - 1),
			}
			df.Hunks = append(df.Hunks, hr)
		}
		diffFiles = append(diffFiles, df)
	}

	// 2. Build the base CODEOWNERS model -------------------------------------------------
	baseCO, err := codeowners.New(repoDir, diffFiles, io.Discard)
	if err != nil {
		t.Fatalf("codeowners.New err: %v", err)
	}

	// 3. Build an inline ownership oracle for all changed files --------------------------
	oracle := inlineowners.Oracle{}
	for _, df := range diffFiles {
		absPath := filepath.Join(repoDir, df.FileName)
		data, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("read file %s: %v", df.FileName, err)
		}
		blocks, _ := inlineowners.Parse(string(data), io.Discard)
		if len(blocks) == 0 {
			continue
		}
		b2 := make([]inlineowners.Block, 0, len(blocks))
		for _, b := range blocks {
			b2 = append(b2, inlineowners.Block{Owners: b.Owners, Start: b.StartLine, End: b.EndLine})
		}
		oracle[df.FileName] = b2
	}

	// 4. Compute per-file override reviewer groups based on inline blocks ---------------
	overrides := map[string]codeowners.ReviewerGroups{}
	for _, df := range diffFiles {
		rgs := codeowners.ReviewerGroups{}
		for _, h := range df.Hunks {
			lists := oracle.OwnersForRange(df.FileName, h.Start, h.End)
			for _, lst := range lists {
				rgs = append(rgs, &codeowners.ReviewerGroup{Names: lst})
			}
		}
		if len(rgs) == 0 {
			if base, ok := baseCO.FileRequired()[df.FileName]; ok {
				rgs = append(rgs, base...)
			}
		} else {
			rgs = f.RemoveDuplicates(rgs)
		}
		overrides[df.FileName] = rgs
	}

	// 5. Overlay inline overrides onto the base model ------------------------------------
	owners := newOverlayOwners(baseCO, overrides)

	// 6. Validate aggregate required owners ---------------------------------------------
	expectedAll := []string{
		"@base-test",
		"@devops",
		"@frontend-inline-misspellede",
		"@model-owner",
		"@qa-friend",
	}
	slices.Sort(expectedAll)
	gotAll := owners.AllRequired().Flatten()
	if !slices.Equal(gotAll, expectedAll) {
		t.Fatalf("AllRequired mismatch. expected %v, got %v", expectedAll, gotAll)
	}

	// 7. Validate per-file owners --------------------------------------------------------
	wantPerFile := map[string][]string{
		"frontend/a.test.ts": {"@qa-friend"},
		"frontend/b.ts":      {"@frontend-inline-misspellede"},
		"models.py":          {"@model-owner", "@devops"},
		"test_a.py":          {"@base-test"},
	}

	for file, want := range wantPerFile {
		gotRGs, ok := owners.FileRequired()[file]
		if !ok {
			t.Fatalf("expected file %s in owners map", file)
		}
		got := gotRGs.Flatten()
		slices.Sort(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("file %s: expected required %v, got %v", file, want, got)
		}
	}
}
