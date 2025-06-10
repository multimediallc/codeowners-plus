package app

import (
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
	"github.com/multimediallc/codeowners-plus/pkg/inlineowners"
)

// computeOverrides returns per-file reviewer groups after applying inline precedence.
// It expects both "new" and "old" oracles so that removed blocks contribute their
// previous owners.
func computeOverrides(
	files []codeowners.DiffFile,
	baseRequired map[string]codeowners.ReviewerGroups,
	newOracle inlineowners.Oracle,
	oldOracle inlineowners.Oracle,
) map[string]codeowners.ReviewerGroups {
	overrides := make(map[string]codeowners.ReviewerGroups)
	for _, df := range files {
		agg := codeowners.ReviewerGroups{}
		for _, h := range df.Hunks {
			if lists := newOracle.OwnersForRange(df.FileName, h.Start, h.End); lists != nil {
				for _, lst := range lists {
					agg = append(agg, &codeowners.ReviewerGroup{Names: lst})
				}
			} else if lists := oldOracle.OwnersForRange(df.FileName, h.Start, h.End); lists != nil {
				// deleted or changed owners
				for _, lst := range lists {
					agg = append(agg, &codeowners.ReviewerGroup{Names: lst})
				}
			}
		}
		if len(agg) == 0 {
			if base, ok := baseRequired[df.FileName]; ok {
				agg = append(agg, base...)
			}
		} else {
			agg = f.RemoveDuplicates(agg)
		}
		overrides[df.FileName] = agg
	}
	return overrides
}
