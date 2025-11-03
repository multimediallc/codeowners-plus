package git

import (
	"fmt"
	"strings"
)

// GitRefFileReader reads files from a specific git ref
type GitRefFileReader struct {
	ref      string
	dir      string
	executor gitCommandExecutor
}

// NewGitRefFileReader creates a new GitRefFileReader for reading files from a git ref
func NewGitRefFileReader(ref string, dir string) *GitRefFileReader {
	return &GitRefFileReader{
		ref:      ref,
		dir:      dir,
		executor: newRealGitExecutor(dir),
	}
}

// ReadFile reads a file from the git ref
func (r *GitRefFileReader) ReadFile(path string) ([]byte, error) {
	// Normalize path - make it relative to the repository root
	path = r.normalizePathForGit(path)

	// Use git show to read the file from the ref
	output, err := r.executor.execute("git", "show", fmt.Sprintf("%s:%s", r.ref, path))
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s from ref %s: %w", path, r.ref, err)
	}
	return output, nil
}

// PathExists checks if a file exists in the git ref
func (r *GitRefFileReader) PathExists(path string) bool {
	// Normalize path - make it relative to the repository root
	path = r.normalizePathForGit(path)

	// Use git cat-file to check if the file exists
	_, err := r.executor.execute("git", "cat-file", "-e", fmt.Sprintf("%s:%s", r.ref, path))
	return err == nil
}

// normalizePathForGit converts an absolute filesystem path to a path relative to the repository root
func (r *GitRefFileReader) normalizePathForGit(path string) string {
	// We want to remove the path to the dir Root
	dir_prefix := r.dir
	if !strings.HasSuffix(dir_prefix, "/") {
		dir_prefix = dir_prefix + "/"
	}
	path = strings.TrimPrefix(path, dir_prefix)
	if strings.HasPrefix(path, "/") {
		return path[1:]
	}
	return path
}
