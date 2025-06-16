package app

import (
	"os"
	"path/filepath"
	"testing"

	ownersConfig "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	"github.com/multimediallc/codeowners-plus/pkg/inlineowners"
)

func TestInlinePrecedence_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	// create .codeowners with file-level owner
	if err := os.WriteFile(filepath.Join(tmp, ".codeowners"), []byte("a.go @file\n"), 0644); err != nil {
		t.Fatalf("write codeowners: %v", err)
	}
	// create a.go with inline block lines 1-3
	src := `// <CO-inline={@inline}>
line
// </CO-inline>
`
	if err := os.WriteFile(filepath.Join(tmp, "a.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}

	diffFile := codeowners.DiffFile{
		FileName: "a.go",
		Hunks:    []codeowners.HunkRange{{Start: 2, End: 2}}, // line inside block
	}

	// Build base codeowners
	baseCO, err := codeowners.New(tmp, []codeowners.DiffFile{diffFile}, os.Stdout)
	if err != nil {
		t.Fatalf("codeowners.New err: %v", err)
	}

	// Build oracle
	blocks, _ := inlineowners.Parse(src, os.Stdout)
	oracle := inlineowners.Oracle{"a.go": {
		{Owners: blocks[0].Owners, Start: blocks[0].StartLine, End: blocks[0].EndLine},
	}}

	// Build overrides like app logic
	lists := oracle.OwnersForRange("a.go", 2, 2)
	overrides := map[string]codeowners.ReviewerGroups{}
	rgs := codeowners.ReviewerGroups{}
	for _, lst := range lists {
		rgs = append(rgs, &codeowners.ReviewerGroup{Names: lst})
	}
	overrides["a.go"] = rgs

	ov := newOverlayOwners(baseCO, overrides)

	conf := &ownersConfig.Config{InlineOwnershipEnabled: true}
	_ = conf // unused but indicates enable flag

	req := ov.AllRequired().Flatten()
	if len(req) != 1 || req[0] != "@inline" {
		t.Fatalf("expected only @inline, got %v", req)
	}
}
