package codeowners

import (
	"encoding/json"
	"testing"
)

func TestNewSlug(t *testing.T) {
	tt := []struct {
		name     string
		input    string
		wantOrig string
		wantNorm string
	}{
		{
			name:     "lowercase username",
			input:    "@user",
			wantOrig: "@user",
			wantNorm: "@user",
		},
		{
			name:     "uppercase username",
			input:    "@USER",
			wantOrig: "@USER",
			wantNorm: "@user",
		},
		{
			name:     "mixed case username",
			input:    "@UsEr",
			wantOrig: "@UsEr",
			wantNorm: "@user",
		},
		{
			name:     "team name",
			input:    "@org/team",
			wantOrig: "@org/team",
			wantNorm: "@org/team",
		},
		{
			name:     "mixed case team",
			input:    "@Org/Team",
			wantOrig: "@Org/Team",
			wantNorm: "@org/team",
		},
		{
			name:     "without @ prefix",
			input:    "user",
			wantOrig: "user",
			wantNorm: "user",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slug := NewSlug(tc.input)
			if slug.Original() != tc.wantOrig {
				t.Errorf("Original() = %q, want %q", slug.Original(), tc.wantOrig)
			}
			if slug.Normalized() != tc.wantNorm {
				t.Errorf("Normalized() = %q, want %q", slug.Normalized(), tc.wantNorm)
			}
		})
	}
}

func TestSlugEquals(t *testing.T) {
	tt := []struct {
		name  string
		slug1 string
		slug2 string
		want  bool
	}{
		{
			name:  "exact match",
			slug1: "@user",
			slug2: "@user",
			want:  true,
		},
		{
			name:  "case insensitive match",
			slug1: "@user",
			slug2: "@USER",
			want:  true,
		},
		{
			name:  "mixed case match",
			slug1: "@UsEr",
			slug2: "@uSeR",
			want:  true,
		},
		{
			name:  "different users",
			slug1: "@user1",
			slug2: "@user2",
			want:  false,
		},
		{
			name:  "team case insensitive",
			slug1: "@org/team",
			slug2: "@Org/Team",
			want:  true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s1 := NewSlug(tc.slug1)
			s2 := NewSlug(tc.slug2)
			if got := s1.Equals(s2); got != tc.want {
				t.Errorf("Equals() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSlugEqualsString(t *testing.T) {
	tt := []struct {
		name string
		slug string
		str  string
		want bool
	}{
		{
			name: "exact match",
			slug: "@user",
			str:  "@user",
			want: true,
		},
		{
			name: "case insensitive match",
			slug: "@user",
			str:  "@USER",
			want: true,
		},
		{
			name: "mixed case match",
			slug: "@UsEr",
			str:  "@uSeR",
			want: true,
		},
		{
			name: "different users",
			slug: "@user1",
			str:  "@user2",
			want: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSlug(tc.slug)
			if got := s.EqualsString(tc.str); got != tc.want {
				t.Errorf("EqualsString() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSlugString(t *testing.T) {
	tt := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "preserves lowercase",
			input: "@user",
			want:  "@user",
		},
		{
			name:  "preserves uppercase",
			input: "@USER",
			want:  "@USER",
		},
		{
			name:  "preserves mixed case",
			input: "@UsEr",
			want:  "@UsEr",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slug := NewSlug(tc.input)
			if got := slug.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSlugMarshalJSON(t *testing.T) {
	tt := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercase",
			input: "@user",
			want:  `"@user"`,
		},
		{
			name:  "uppercase",
			input: "@USER",
			want:  `"@USER"`,
		},
		{
			name:  "mixed case",
			input: "@UsEr",
			want:  `"@UsEr"`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slug := NewSlug(tc.input)
			data, err := json.Marshal(slug)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(data) != tc.want {
				t.Errorf("Marshal() = %s, want %s", data, tc.want)
			}
		})
	}
}

func TestNewSlugs(t *testing.T) {
	tt := []struct {
		name  string
		input []string
		want  []string // original strings
	}{
		{
			name:  "mixed case",
			input: []string{"@user1", "@USER2", "@UsEr3"},
			want:  []string{"@user1", "@USER2", "@UsEr3"},
		},
		{
			name:  "empty slice",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "nil slice",
			input: nil,
			want:  nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slugs := NewSlugs(tc.input)
			if tc.want == nil {
				if slugs != nil {
					t.Errorf("NewSlugs() = %v, want nil", slugs)
				}
				return
			}
			if len(slugs) != len(tc.want) {
				t.Fatalf("NewSlugs() length = %d, want %d", len(slugs), len(tc.want))
			}
			for i, slug := range slugs {
				if slug.Original() != tc.want[i] {
					t.Errorf("NewSlugs()[%d].Original() = %q, want %q", i, slug.Original(), tc.want[i])
				}
			}
		})
	}
}

func TestOriginalStrings(t *testing.T) {
	tt := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "preserves case",
			input: []string{"@user1", "@USER2", "@UsEr3"},
			want:  []string{"@user1", "@USER2", "@UsEr3"},
		},
		{
			name:  "empty slice",
			input: []string{},
			want:  []string{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slugs := NewSlugs(tc.input)
			got := OriginalStrings(slugs)
			if len(got) != len(tc.want) {
				t.Fatalf("OriginalStrings() length = %d, want %d", len(got), len(tc.want))
			}
			for i, s := range got {
				if s != tc.want[i] {
					t.Errorf("OriginalStrings()[%d] = %q, want %q", i, s, tc.want[i])
				}
			}
		})
	}
}

func TestNormalizedStrings(t *testing.T) {
	tt := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "normalizes to lowercase",
			input: []string{"@user1", "@USER2", "@UsEr3"},
			want:  []string{"@user1", "@user2", "@user3"},
		},
		{
			name:  "empty slice",
			input: []string{},
			want:  []string{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slugs := NewSlugs(tc.input)
			got := NormalizedStrings(slugs)
			if len(got) != len(tc.want) {
				t.Fatalf("NormalizedStrings() length = %d, want %d", len(got), len(tc.want))
			}
			for i, s := range got {
				if s != tc.want[i] {
					t.Errorf("NormalizedStrings()[%d] = %q, want %q", i, s, tc.want[i])
				}
			}
		})
	}
}

func TestContainsSlug(t *testing.T) {
	tt := []struct {
		name   string
		slugs  []string
		target string
		want   bool
	}{
		{
			name:   "contains exact match",
			slugs:  []string{"@user1", "@user2", "@user3"},
			target: "@user2",
			want:   true,
		},
		{
			name:   "contains case insensitive",
			slugs:  []string{"@user1", "@user2", "@user3"},
			target: "@USER2",
			want:   true,
		},
		{
			name:   "not contains",
			slugs:  []string{"@user1", "@user2", "@user3"},
			target: "@user4",
			want:   false,
		},
		{
			name:   "empty slice",
			slugs:  []string{},
			target: "@user1",
			want:   false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			slugs := NewSlugs(tc.slugs)
			target := NewSlug(tc.target)
			if got := ContainsSlug(slugs, target); got != tc.want {
				t.Errorf("ContainsSlug() = %v, want %v", got, tc.want)
			}
		})
	}
}
