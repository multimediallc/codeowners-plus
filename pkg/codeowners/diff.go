package codeowners

type HunkRange struct {
	Start int
	End   int
}

type DiffFile struct {
	FileName string
	Hunks    []HunkRange
}
