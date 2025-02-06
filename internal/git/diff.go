package git

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/sourcegraph/go-diff/diff"
)

type Diff interface {
	AllChanges() []codeowners.DiffFile
	ChangesSince(ref string) ([]codeowners.DiffFile, error)
	Context() DiffContext
}

type GitDiff struct {
	context DiffContext
	diff    []*diff.FileDiff
	files   []codeowners.DiffFile
}

func NewDiff(context DiffContext) (Diff, error) {
	gitDiff, err := getGitDiff(context)
	if err != nil {
		return nil, err
	}
	diffFiles, err := toDiffFiles(gitDiff)
	if err != nil {
		return nil, err
	}

	return &GitDiff{
		context: context,
		diff:    gitDiff,
		files:   diffFiles,
	}, nil
}

func (gd *GitDiff) AllChanges() []codeowners.DiffFile {
	return gd.files
}

func (gd *GitDiff) ChangesSince(ref string) ([]codeowners.DiffFile, error) {
	olderDiffContext := DiffContext{
		Base:       gd.context.Base,
		Head:       ref,
		Dir:        gd.context.Dir,
		IgnoreDirs: gd.context.IgnoreDirs,
	}
	olderDiff, err := getGitDiff(olderDiffContext)
	if err != nil {
		return nil, err
	}
	changesContext := changesSinceContext{
		newerDiff: gd.diff,
		olderDiff: olderDiff,
	}
	diffFiles, err := changesSince(changesContext)
	if err != nil {
		return nil, err
	}
	return diffFiles, nil
}

func (gd *GitDiff) Context() DiffContext {
	return gd.context
}

type DiffContext struct {
	Base       string
	Head       string
	Dir        string
	IgnoreDirs []string
}

type changesSinceContext struct {
	newerDiff []*diff.FileDiff
	olderDiff []*diff.FileDiff
}

// Parse the diff output to get the file names and hunks
func toDiffFiles(fileDiffs []*diff.FileDiff) ([]codeowners.DiffFile, error) {
	diffFiles := make([]codeowners.DiffFile, 0, len(fileDiffs))

	for _, d := range fileDiffs {
		newDiffFile := codeowners.DiffFile{
			FileName: d.NewName[2:],
			Hunks:    make([]codeowners.HunkRange, 0, len(d.Hunks)),
		}
		for _, hunk := range d.Hunks {
			newHunkRange := codeowners.HunkRange{
				Start: int(hunk.NewStartLine),
				End:   int(hunk.NewStartLine + hunk.NewLines - 1),
			}
			newDiffFile.Hunks = append(newDiffFile.Hunks, newHunkRange)
		}
		diffFiles = append(diffFiles, newDiffFile)
	}
	return diffFiles, nil
}

// Get Changes between two diffs
func changesSince(context changesSinceContext) ([]codeowners.DiffFile, error) {
	// Get hash of hunks in both diffs
	// For each file, filter out hunks that are in oldDiff
	// if len(hunks) > 0, add to diffFiles
	oldHunkHashes := make(map[[32]byte]bool)
	for _, d := range context.olderDiff {
		for _, h := range d.Hunks {
			oldHunkHashes[hunkHash(h)] = true
		}
	}

	diffFiles := make([]codeowners.DiffFile, 0, len(context.newerDiff))

	for _, d := range context.newerDiff {
		newDiffFile := codeowners.DiffFile{
			FileName: d.NewName[2:],
			Hunks:    make([]codeowners.HunkRange, 0, len(d.Hunks)),
		}
		for _, hunk := range d.Hunks {
			if !oldHunkHashes[hunkHash(hunk)] {
				newHunkRange := codeowners.HunkRange{
					Start: int(hunk.NewStartLine),
					End:   int(hunk.NewStartLine + hunk.NewLines - 1),
				}
				newDiffFile.Hunks = append(newDiffFile.Hunks, newHunkRange)
			}
		}
		if len(newDiffFile.Hunks) > 0 {
			diffFiles = append(diffFiles, newDiffFile)
		}
	}
	return diffFiles, nil
}

func getGitDiff(data DiffContext) ([]*diff.FileDiff, error) {
	cmd := exec.Command("git", "diff", "-U0", fmt.Sprintf("%s...%s", data.Base, data.Head))
	cmd.Dir = data.Dir
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Diff Error: %s\n%s\n", err, cmdOutput)
	}
	gitDiff, err := diff.ParseMultiFileDiff(cmdOutput)
	if err != nil {
		return nil, err
	}
	gitDiff = slices.DeleteFunc(gitDiff, func(d *diff.FileDiff) bool {
		for _, dir := range data.IgnoreDirs {
			if strings.HasPrefix(d.NewName[2:], dir) {
				return true
			}
		}
		return false
	})
	return gitDiff, nil
}

func hunkHash(hunk *diff.Hunk) [32]byte {
	// Generate a hash for a hunk based on its added and removed lines.
	var lines []byte
	data := hunk.Body

	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+', '-':
			// Include the line type and content
			lines = append(lines, line...)
		default:
			// Skip context lines
		}
	}
	return sha256.Sum256(lines)
}
