package app

import (
	"testing"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/multimediallc/codeowners-plus/pkg/inlineowners"
)

func TestComputeOverrides_RemovedInlineOwner(t *testing.T) {
	diffFile := codeowners.DiffFile{FileName: "file.py", Hunks: []codeowners.HunkRange{{Start: 10, End: 12}}}

	rgm := codeowners.NewReviewerGroupMemo()
	baseGrp := rgm.ToReviewerGroup("@file")

	base := map[string]codeowners.ReviewerGroups{"file.py": {baseGrp}}

	oldOracle := inlineowners.Oracle{"file.py": {
		{Owners: []string{"@old"}, Start: 10, End: 12},
	}}
	newOracle := inlineowners.Oracle{} // block deleted

	overrides := computeOverrides([]codeowners.DiffFile{diffFile}, base, newOracle, oldOracle)

	grps := overrides["file.py"].Flatten()
	if len(grps) != 1 || grps[0] != "@old" {
		t.Fatalf("expected @old owner, got %v", grps)
	}
}
