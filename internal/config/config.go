package owners

import (
	"strings"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	MaxReviews            *int         `toml:"max_reviews"`
	MinReviews            *int         `toml:"min_reviews"`
	UnskippableReviewers  []string     `toml:"unskippable_reviewers"`
	Ignore                []string     `toml:"ignore"`
	Enforcement           *Enforcement `toml:"enforcement"`
	HighPriorityLabels    []string     `toml:"high_priority_labels"`
	AdminBypass           *AdminBypass `toml:"admin_bypass"`
	DetailedReviewers          bool         `toml:"detailed_reviewers"`
	DisableSmartDismissal      bool         `toml:"disable_smart_dismissal"`
	RequireBothBranchReviewers bool         `toml:"require_both_branch_reviewers"`
}

type Enforcement struct {
	Approval  bool `toml:"approval"`
	FailCheck bool `toml:"fail_check"`
}

type AdminBypass struct {
	Enabled      bool     `toml:"enabled"`
	AllowedUsers []string `toml:"allowed_users"`
}

func ReadConfig(path string, fileReader codeowners.FileReader) (*Config, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	defaultConfig := &Config{
		MaxReviews:            nil,
		MinReviews:            nil,
		UnskippableReviewers:  []string{},
		Ignore:                []string{},
		Enforcement:           &Enforcement{Approval: false, FailCheck: true},
		HighPriorityLabels:    []string{},
		AdminBypass:           &AdminBypass{Enabled: false, AllowedUsers: []string{}},
		DetailedReviewers:          false,
		DisableSmartDismissal:      false,
		RequireBothBranchReviewers: false,
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
	return config, nil
}
