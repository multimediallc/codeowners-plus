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

// gitCommandExecutor defines the interface for executing git commands
type gitCommandExecutor interface {
	execute(command string, args ...string) ([]byte, error)
}

// realGitExecutor implements GitCommandExecutor using os/exec
type realGitExecutor struct {
	dir string
}

func newRealGitExecutor(dir string) *realGitExecutor {
	return &realGitExecutor{dir: dir}
}

func (e *realGitExecutor) execute(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = e.dir
	return cmd.CombinedOutput()
}

type Diff interface {
	AllChanges() []codeowners.DiffFile
	ChangesSince(ref string) ([]codeowners.DiffFile, error)
	Context() DiffContext
}

type GitDiff struct {
	context    DiffContext
	diff       []*diff.FileDiff
	files      []codeowners.DiffFile
	executor   gitCommandExecutor
	hunkFilter HunkFilter
}

type HunkText struct {
	Name          string
	HeadHunks     []string
	ApprovalHunks []string
}

// HunkFilter reports already-reviewed hunks as indexes into each file's HeadHunks; an unrecognised file, an out-of-range index or any error leaves every hunk in place.
type HunkFilter func(ref string, files []HunkText) (map[string][]int, error)

// DiffOption configures optional GitDiff behavior.
type DiffOption func(*GitDiff)

// WithHunkFilter routes surviving hunks through filter before they reach an approval.
func WithHunkFilter(filter HunkFilter) DiffOption {
	return func(gd *GitDiff) {
		gd.hunkFilter = filter
	}
}

func NewDiff(context DiffContext, opts ...DiffOption) (Diff, error) {
	executor := newRealGitExecutor(context.Dir)
	return NewDiffWithExecutor(context, executor, opts...)
}

func NewDiffWithExecutor(context DiffContext, executor gitCommandExecutor, opts ...DiffOption) (Diff, error) {
	gitDiff, err := getGitDiff(context, executor)
	if err != nil {
		return nil, err
	}
	diffFiles, err := toDiffFiles(gitDiff)
	if err != nil {
		return nil, err
	}

	gd := &GitDiff{
		context:  context,
		diff:     gitDiff,
		files:    diffFiles,
		executor: executor,
	}
	for _, opt := range opts {
		opt(gd)
	}
	return gd, nil
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
	olderDiff, err := getGitDiff(olderDiffContext, gd.executor)
	if err != nil {
		return nil, fmt.Errorf("failed to get older diff: %w", err)
	}
	changesContext := changesSinceContext{
		newerDiff: gd.diff,
		olderDiff: olderDiff,
		ref:       ref,
		filter:    gd.hunkFilter,
	}
	diffFiles, err := changesSince(changesContext)
	if err != nil {
		return nil, fmt.Errorf("failed to compute changes since: %w", err)
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
	ref       string
	filter    HunkFilter
}

func diffToFilename(d *diff.FileDiff) string {
	// For regular diffs, NewName/OrigName come from the "--- a/<path>" /
	// "+++ b/<path>" headers and carry an "a/"/"b/" prefix to strip. For
	// deleted files NewName is "/dev/null". For binary files git emits no
	// "---"/"+++" lines (only "Binary files a/<path> and b/<path> differ"),
	// leaving both fields empty — recover the path from the "diff --git"
	// extended header instead.
	if len(d.NewName) > 2 && d.NewName != "/dev/null" {
		return d.NewName[2:]
	}
	if len(d.OrigName) > 2 && d.OrigName != "/dev/null" {
		return d.OrigName[2:]
	}
	return filenameFromExtendedHeader(d.Extended)
}

func filenameFromExtendedHeader(headers []string) string {
	for _, h := range headers {
		rest, ok := strings.CutPrefix(h, "diff --git ")
		if !ok {
			continue
		}
		// Quoted form: "a/<path>" "b/<path>" (paths needing escaping).
		if idx := strings.LastIndex(rest, ` "b/`); idx >= 0 {
			return strings.TrimSuffix(rest[idx+len(` "b/`):], `"`)
		}
		// Unquoted form: a/<path> b/<path>
		if idx := strings.LastIndex(rest, " b/"); idx >= 0 {
			return rest[idx+len(" b/"):]
		}
	}
	return ""
}

// Parse the diff output to get the file names and hunks
func toDiffFiles(fileDiffs []*diff.FileDiff) ([]codeowners.DiffFile, error) {
	diffFiles := make([]codeowners.DiffFile, 0, len(fileDiffs))

	for _, d := range fileDiffs {
		fileName := diffToFilename(d)

		newDiffFile := codeowners.DiffFile{
			FileName: fileName,
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

	survivors := make([]survivingHunks, 0, len(context.newerDiff))

	for _, d := range context.newerDiff {
		file := survivingHunks{name: diffToFilename(d)}
		for _, hunk := range d.Hunks {
			if !oldHunkHashes[hunkHash(hunk)] {
				file.hunks = append(file.hunks, survivingHunk{
					rng: codeowners.HunkRange{
						Start: int(hunk.NewStartLine),
						End:   int(hunk.NewStartLine + hunk.NewLines - 1),
					},
					body: string(hunk.Body),
				})
			}
		}
		survivors = append(survivors, file)
	}

	if context.filter != nil {
		survivors = applyHunkFilter(context, survivors)
	}

	diffFiles := make([]codeowners.DiffFile, 0, len(survivors))
	for _, file := range survivors {
		// Binary files have no hunks; staleness is intentionally not tracked
		// for them (there is no hunk content to hash against the older diff).
		if len(file.hunks) == 0 {
			continue
		}
		diffFiles = append(diffFiles, codeowners.DiffFile{
			FileName: file.name,
			Hunks:    file.ranges(),
		})
	}
	return diffFiles, nil
}

type survivingHunk struct {
	rng  codeowners.HunkRange
	body string
}

type survivingHunks struct {
	name  string
	hunks []survivingHunk
}

func (f survivingHunks) bodies() []string {
	out := make([]string, 0, len(f.hunks))
	for _, h := range f.hunks {
		out = append(out, h.body)
	}
	return out
}

func (f survivingHunks) ranges() []codeowners.HunkRange {
	out := make([]codeowners.HunkRange, 0, len(f.hunks))
	for _, h := range f.hunks {
		out = append(out, h.rng)
	}
	return out
}

// A path a filter cannot address is a path it must not be asked about: a
// typechange gives one path two entries and the answer is keyed by name.
func addressableNames(survivors []survivingHunks) map[string]bool {
	count := make(map[string]int, len(survivors))
	for _, file := range survivors {
		count[file.name]++
	}
	names := make(map[string]bool, len(survivors))
	for _, file := range survivors {
		if len(file.hunks) > 0 && count[file.name] == 1 {
			names[file.name] = true
		}
	}
	return names
}

// applyHunkFilter drops the hunks the filter reports as already reviewed; any error, or any answer about something unasked, changes nothing.
func applyHunkFilter(context changesSinceContext, survivors []survivingHunks) []survivingHunks {
	survivorNames := addressableNames(survivors)
	if len(survivorNames) == 0 {
		return survivors
	}

	// Only the surviving files can be asked about, and olderDiff covers the whole PR.
	approvalBodies := make(map[string][]string, len(survivorNames))
	for _, d := range context.olderDiff {
		name := diffToFilename(d)
		if !survivorNames[name] {
			continue
		}
		for _, hunk := range d.Hunks {
			approvalBodies[name] = append(approvalBodies[name], string(hunk.Body))
		}
	}

	files := make([]HunkText, 0, len(survivorNames))
	for _, file := range survivors {
		if !survivorNames[file.name] {
			continue
		}
		files = append(files, HunkText{
			Name:          file.name,
			HeadHunks:     file.bodies(),
			ApprovalHunks: approvalBodies[file.name],
		})
	}

	reviewed, err := context.filter(context.ref, files)
	if err != nil || len(reviewed) == 0 {
		return survivors
	}

	filtered := make([]survivingHunks, 0, len(survivors))
	for _, file := range survivors {
		indexes, ok := reviewed[file.name]
		if !ok || len(indexes) == 0 || !survivorNames[file.name] {
			filtered = append(filtered, file)
			continue
		}
		drop := make(map[int]bool, len(indexes))
		for _, index := range indexes {
			if index < 0 || index >= len(file.hunks) {
				// An answer about an unsent hunk voids the answer for this file.
				drop = nil
				break
			}
			drop[index] = true
		}
		if drop == nil {
			filtered = append(filtered, file)
			continue
		}
		kept := survivingHunks{name: file.name}
		for i, hunk := range file.hunks {
			if !drop[i] {
				kept.hunks = append(kept.hunks, hunk)
			}
		}
		filtered = append(filtered, kept)
	}
	return filtered
}

func getGitDiff(data DiffContext, executor gitCommandExecutor) ([]*diff.FileDiff, error) {
	cmdOutput, err := executor.execute("git", "diff", "-U0", fmt.Sprintf("%s...%s", data.Base, data.Head))
	if err != nil {
		return nil, fmt.Errorf("diff Error: %s\n%s", err, cmdOutput)
	}
	gitDiff, err := diff.ParseMultiFileDiff(cmdOutput)
	if err != nil {
		return nil, err
	}
	gitDiff = slices.DeleteFunc(gitDiff, func(d *diff.FileDiff) bool {
		fileName := diffToFilename(d)

		for _, dir := range data.IgnoreDirs {
			if strings.HasPrefix(fileName, dir) {
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

	if len(data) == 0 {
		return sha256.Sum256(nil)
	}

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
