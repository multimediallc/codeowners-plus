package codeowners

type HunkRange struct {
	Start int
	End   int
}

type DiffFile struct {
	FileName string
	// BaseFileName is the file's name in the base (original) revision when
	// it differs from FileName (renames). Empty when the name is unchanged.
	BaseFileName string
	// Hunks are the changed line ranges in the head (new) revision.
	Hunks []HunkRange
	// BaseHunks are the changed line ranges in the base (original)
	// revision. Needed by consumers that must see deleted content, such
	// as inline ownership (a block removed in the PR only exists at base).
	BaseHunks []HunkRange
}

// BaseName returns the file's name in the base revision, falling back to
// FileName when the file was not renamed.
func (d DiffFile) BaseName() string {
	if d.BaseFileName != "" {
		return d.BaseFileName
	}
	return d.FileName
}
