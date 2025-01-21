package owners

import (
	"testing"
)

func TestReadCodeownersConfig(t *testing.T) {
	conf, err := ReadCodeownersConfig("../test_project/")
	if err != nil {
		t.Errorf("Error reading codeowners config: %v", err)
		return
	}
	if conf == nil {
		t.Errorf("Error reading codeowners config")
		return
	}
	// if *conf.MaxReviews != 2 {
	// 	t.Errorf("Expected max reviews 2, got %d", *conf.MaxReviews)
	// }
	if conf.MaxReviews != nil {
		t.Errorf("Expected max reviews to be nil, got %d", *conf.MaxReviews)
	}
	if len(conf.UnskippableReviewers) != 2 {
		t.Errorf("Expected 2 unskippable reviewers, got %d", len(conf.UnskippableReviewers))
	}
	if conf.UnskippableReviewers[0] != "@user1" {
		t.Errorf("Expected unskippable reviewer @user1, got %s", conf.UnskippableReviewers[0])
	}
	if conf.UnskippableReviewers[1] != "@user2" {
		t.Errorf("Expected unskippable reviewer @user2, got %s", conf.UnskippableReviewers[0])
	}
	if len(conf.Ignore) != 1 {
		t.Errorf("Expected 1 ignore dir, got %d", len(conf.Ignore))
	}
	if conf.Ignore[0] != "ignored" {
		t.Errorf("Expected ignore dir ignored, got %s", conf.Ignore[0])
	}
	if conf.Enforcement == nil {
		t.Errorf("Expected enforcement to be set")
	}
	if conf.Enforcement.Approval != false {
		t.Errorf("Expected Approval to be false")
	}
	if conf.Enforcement.FailCheck != true {
		t.Errorf("Expected FailCheck to be true")
	}
}

func TestReadCodeowners(t *testing.T) {
	tt := []struct {
		path         string
		fallback     bool
		fallbackName string
		owners       int
		additional   int
		optional     int
	}{
		{"../test_project", true, "@base", 4, 2, 1},
		{"../test_project/frontend/", true, "@frontend", 2, 1, 0},
		{"../test_project/a.py", false, "", 0, 0, 0},  // test non-directory
		{"../test_project/empty", false, "", 0, 0, 0}, // test directory with no .codeowners file
	}

	rgMan := NewReviewerGroupMemo()
	for i, tc := range tt {
		rules := ReadCodeownersFile(tc.path, rgMan)

		if !tc.fallback && rules.Fallback != nil {
			t.Errorf("Case %d: Expected fallback to be nil, got %+v", i, rules.Fallback)
		}

		if tc.fallback && rules.Fallback.Names[0] != tc.fallbackName {
			t.Errorf("Case %d: Expected fallback to be %s, got %s", i, tc.fallbackName, rules.Fallback.Names[0])
		}

		if len(rules.OwnerTests) != tc.owners {
			t.Errorf("Case %d: Expected %d owner tests, got %d", i, tc.owners, len(rules.OwnerTests))
		}

		if len(rules.AdditionalReviewerTests) != tc.additional {
			t.Errorf("Case %d: Expected %d additional tests, got %d", i, tc.additional, len(rules.AdditionalReviewerTests))
		}

		if len(rules.OptionalReviewerTests) != tc.optional {
			t.Errorf("Case %d: Expected %d optional tests, got %d", i, tc.optional, len(rules.OptionalReviewerTests))
		}
	}
}
