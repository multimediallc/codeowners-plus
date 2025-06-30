package owners

import (
	"errors"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	MaxReviews           *int         `toml:"max_reviews"`
	MinReviews           *int         `toml:"min_reviews"`
	UnskippableReviewers []string     `toml:"unskippable_reviewers"`
	Ignore               []string     `toml:"ignore"`
	Enforcement          *Enforcement `toml:"enforcement"`
	HighPriorityLabels   []string     `toml:"high_priority_labels"`
	AdminBypass          *AdminBypass `toml:"admin_bypass"`
}

type Enforcement struct {
	Approval  bool `toml:"approval"`
	FailCheck bool `toml:"fail_check"`
}

type AdminBypass struct {
	Enabled       bool     `toml:"enabled"`
	AllowedUsers  []string `toml:"allowed_users"`
}

func ReadConfig(path string) (*Config, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	defaultConfig := &Config{
		MaxReviews:           nil,
		MinReviews:           nil,
		UnskippableReviewers: []string{},
		Ignore:               []string{},
		Enforcement:          &Enforcement{Approval: false, FailCheck: true},
		HighPriorityLabels:   []string{},
		AdminBypass:          &AdminBypass{Enabled: false, AllowedUsers: []string{}},
	}

	fileName := path + "codeowners.toml"
	if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
		return defaultConfig, nil
	}
	file, err := os.ReadFile(fileName)
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
