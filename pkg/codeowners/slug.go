package codeowners

import (
	"encoding/json"
	"strings"
)

// Slug represents a GitHub username or team name (handle) with case-insensitive semantics.
// It stores both the original representation (for display/API calls) and normalized
// form (for comparisons). The zero value is not valid - use NewSlug to construct.
//
// GitHub uses "slug" as the canonical term for handles that work for both users and teams.
type Slug struct {
	original   string
	normalized string
}

// NewSlug creates a Slug from a string, normalizing it for comparison.
// The original casing is preserved for display purposes.
func NewSlug(name string) Slug {
	return Slug{
		original:   name,
		normalized: strings.ToLower(name),
	}
}

// Equals performs case-insensitive comparison with another Slug.
func (s Slug) Equals(other Slug) bool {
	return s.normalized == other.normalized
}

// EqualsString performs case-insensitive comparison with a string.
// Provided for convenience during migration and at boundaries.
func (s Slug) EqualsString(str string) bool {
	return s.normalized == strings.ToLower(str)
}

// Original returns the original string representation (preserves case).
// Use this for display, comments, API calls to GitHub.
func (s Slug) Original() string {
	return s.original
}

// Normalized returns the lowercase normalized form.
// Use this for map keys and comparisons with legacy code.
func (s Slug) Normalized() string {
	return s.normalized
}

// String returns the original representation for fmt.Printf, logging, etc.
func (s Slug) String() string {
	return s.original
}

// MarshalJSON implements json.Marshaler for GitHub API compatibility.
// Serializes as the original string to preserve case.
func (s Slug) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.original)
}

// NewSlugs converts a slice of strings to Slugs.
func NewSlugs(names []string) []Slug {
	if names == nil {
		return nil
	}
	slugs := make([]Slug, len(names))
	for i, name := range names {
		slugs[i] = NewSlug(name)
	}
	return slugs
}

// OriginalStrings extracts original strings from Slug slice.
// Use at boundaries: API calls, comments, display.
func OriginalStrings(slugs []Slug) []string {
	result := make([]string, len(slugs))
	for i, s := range slugs {
		result[i] = s.Original()
	}
	return result
}

// NormalizedStrings extracts normalized strings from Slug slice.
// Use for comparisons with legacy code, generic functions (Intersection, Filtered).
func NormalizedStrings(slugs []Slug) []string {
	result := make([]string, len(slugs))
	for i, s := range slugs {
		result[i] = s.Normalized()
	}
	return result
}

// ContainsSlug checks if a Slug is in a slice (case-insensitive).
func ContainsSlug(slugs []Slug, target Slug) bool {
	for _, s := range slugs {
		if s.Equals(target) {
			return true
		}
	}
	return false
}
