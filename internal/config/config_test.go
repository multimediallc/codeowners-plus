package owners

import (
	"testing"
)

func TestReadCodeownersConfig(t *testing.T) {
	conf, err := ReadConfig("../../test_project/")
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
