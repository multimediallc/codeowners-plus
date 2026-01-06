package codeowners

import "strings"

// NormalizeUsername converts a username or team name to lowercase for case-insensitive comparison.
// This ensures that usernames like @User, @user, and @USER are treated as identical,
// matching GitHub's case-insensitive username behavior.
// The @ prefix is preserved and normalized as part of the string.
func NormalizeUsername(name string) string {
	return strings.ToLower(name)
}

// NormalizeUsernames normalizes a slice of usernames to lowercase for case-insensitive comparison.
// Returns a new slice with all usernames converted to lowercase.
func NormalizeUsernames(names []string) []string {
	if names == nil {
		return nil
	}
	normalized := make([]string, len(names))
	for i, name := range names {
		normalized[i] = NormalizeUsername(name)
	}
	return normalized
}
