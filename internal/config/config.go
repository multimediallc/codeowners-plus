package owners

import (
	"strings"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	MaxReviews                  *int               `toml:"max_reviews"`
	MinReviews                  *int               `toml:"min_reviews"`
	UnskippableReviewers        []string           `toml:"unskippable_reviewers"`
	Ignore                      []string           `toml:"ignore"`
	Enforcement                 *Enforcement       `toml:"enforcement"`
	HighPriorityLabels          []string           `toml:"high_priority_labels"`
	AdminBypass                 *AdminBypass       `toml:"admin_bypass"`
	ApprovalRetention           *ApprovalRetention `toml:"approval_retention"`
	DetailedReviewers           bool               `toml:"detailed_reviewers"`
	DisableSmartDismissal       bool               `toml:"disable_smart_dismissal"`
	RequireBothBranchReviewers  bool               `toml:"require_both_branch_reviewers"`
	SuppressUnownedWarning      bool               `toml:"suppress_unowned_warning"`
	AllowSelfApproval           bool               `toml:"allow_self_approval"`
	SelfApprovalViaTeams        bool               `toml:"self_approval_via_teams"`
	DisableReviewStatusComments bool               `toml:"disable_review_status_comments"`
}

type Enforcement struct {
	Approval  bool `toml:"approval"`
	FailCheck bool `toml:"fail_check"`
}

type AdminBypass struct {
	Enabled      bool     `toml:"enabled"`
	AllowedUsers []string `toml:"allowed_users"`
}

// ApprovalRetention lists the kinds of diff change which may retain an approval.
// Every flag is opt-in, including Enabled, so upgrading never changes how a
// repository's approvals behave.
type ApprovalRetention struct {
	Enabled               bool  `toml:"enabled"`
	Whitespace            *bool `toml:"whitespace"`
	Comments              *bool `toml:"comments"`
	Formatting            *bool `toml:"formatting"`
	StringLiterals        *bool `toml:"string_literals"`
	Renames               *bool `toml:"renames"`
	FetchOrphanedApproval *bool `toml:"fetch_orphaned_approval"`
}

func (r *ApprovalRetention) WhitespaceEnabled() bool {
	if r == nil {
		return false
	}
	return r.enabled(r.Whitespace)
}

func (r *ApprovalRetention) CommentsEnabled() bool {
	if r == nil {
		return false
	}
	return r.enabled(r.Comments)
}

func (r *ApprovalRetention) FormattingEnabled() bool {
	if r == nil {
		return false
	}
	return r.enabled(r.Formatting)
}

func (r *ApprovalRetention) StringLiteralsEnabled() bool {
	if r == nil {
		return false
	}
	return r.enabled(r.StringLiterals)
}

func (r *ApprovalRetention) RenamesEnabled() bool {
	if r == nil {
		return false
	}
	return r.enabled(r.Renames)
}

func (r *ApprovalRetention) FetchOrphanedApprovalEnabled() bool {
	if r == nil {
		return false
	}
	return r.enabled(r.FetchOrphanedApproval)
}

// Enabled is a kill switch, not a default: turning it on retains nothing on its own.
func (r *ApprovalRetention) enabled(flag *bool) bool {
	return r.Enabled && flag != nil && *flag
}

func ReadConfig(path string, fileReader codeowners.FileReader) (*Config, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	defaultConfig := &Config{
		MaxReviews:                  nil,
		MinReviews:                  nil,
		UnskippableReviewers:        []string{},
		Ignore:                      []string{},
		Enforcement:                 &Enforcement{Approval: false, FailCheck: true},
		HighPriorityLabels:          []string{},
		AdminBypass:                 &AdminBypass{Enabled: false, AllowedUsers: []string{}},
		ApprovalRetention:           &ApprovalRetention{Enabled: false},
		DetailedReviewers:           false,
		SelfApprovalViaTeams:        false,
		DisableSmartDismissal:       false,
		RequireBothBranchReviewers:  false,
		DisableReviewStatusComments: false,
	}

	// Use filesystem reader if none provided
	if fileReader == nil {
		fileReader = &codeowners.FilesystemReader{}
	}

	fileName := path + "codeowners.toml"

	if !fileReader.PathExists(fileName) {
		return defaultConfig, nil
	}
	file, err := fileReader.ReadFile(fileName)
	if err != nil {
		return defaultConfig, err
	}
	config := defaultConfig
	err = toml.Unmarshal(file, &config)
	if err != nil {
		return defaultConfig, err
	}
	if config.Enforcement == nil {
		config.Enforcement = defaultConfig.Enforcement
	}
	if config.AdminBypass == nil {
		config.AdminBypass = defaultConfig.AdminBypass
	}
	if config.ApprovalRetention == nil {
		config.ApprovalRetention = defaultConfig.ApprovalRetention
	}
	return config, nil
}
