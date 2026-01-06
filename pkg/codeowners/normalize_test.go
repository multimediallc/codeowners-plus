package codeowners

import (
	"testing"
)

func TestNormalizeUsername(t *testing.T) {
	tt := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase username with @",
			input:    "@user",
			expected: "@user",
		},
		{
			name:     "uppercase username with @",
			input:    "@USER",
			expected: "@user",
		},
		{
			name:     "mixed case username with @",
			input:    "@JohnDoe",
			expected: "@johndoe",
		},
		{
			name:     "username without @",
			input:    "user",
			expected: "user",
		},
		{
			name:     "mixed case username without @",
			input:    "JohnDoe",
			expected: "johndoe",
		},
		{
			name:     "team name with slash",
			input:    "@org/TeamName",
			expected: "@org/teamname",
		},
		{
			name:     "uppercase team name",
			input:    "@ORG/TEAM",
			expected: "@org/team",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unicode characters",
			input:    "@Üser",
			expected: "@üser",
		},
		{
			name:     "special characters preserved",
			input:    "@user-name_123",
			expected: "@user-name_123",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizeUsername(tc.input)
			if result != tc.expected {
				t.Errorf("NormalizeUsername(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNormalizeUsernames(t *testing.T) {
	tt := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "mixed case usernames",
			input:    []string{"@User", "@ADMIN", "@john-doe"},
			expected: []string{"@user", "@admin", "@john-doe"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single username",
			input:    []string{"@JohnDoe"},
			expected: []string{"@johndoe"},
		},
		{
			name:     "team names",
			input:    []string{"@org/Team1", "@org/Team2"},
			expected: []string{"@org/team1", "@org/team2"},
		},
		{
			name:     "already lowercase",
			input:    []string{"@user1", "@user2"},
			expected: []string{"@user1", "@user2"},
		},
		{
			name:     "mixed teams and users",
			input:    []string{"@User", "@org/Team", "@ADMIN"},
			expected: []string{"@user", "@org/team", "@admin"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizeUsernames(tc.input)

			// Handle nil case
			if tc.expected == nil {
				if result != nil {
					t.Errorf("NormalizeUsernames(%v) = %v, expected nil", tc.input, result)
				}
				return
			}

			if len(result) != len(tc.expected) {
				t.Errorf("NormalizeUsernames(%v) returned %d items, expected %d", tc.input, len(result), len(tc.expected))
				return
			}

			for i := range result {
				if result[i] != tc.expected[i] {
					t.Errorf("NormalizeUsernames(%v)[%d] = %q, expected %q", tc.input, i, result[i], tc.expected[i])
				}
			}
		})
	}
}
