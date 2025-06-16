package codeowners

import "testing"

func TestAddInlineOwners_MergeAndDedup(t *testing.T) {
	rgm := NewReviewerGroupMemo()
	rgA := rgm.ToReviewerGroup("@a")
	rgB := rgm.ToReviewerGroup("@b")

	fo := newFileOwners()
	fo.requiredReviewers = ReviewerGroups{rgA}

	om := &ownersMap{
		fileToOwner:     map[string]fileOwners{"file.go": *fo},
		nameReviewerMap: map[string]ReviewerGroups{"@a": {rgA}},
	}

	// Add B and duplicate A
	om.AddInlineOwners("file.go", ReviewerGroups{rgB, rgA})

	fr := om.FileRequired()
	groups := fr["file.go"]
	if len(groups) != 2 {
		t.Fatalf("expected 2 reviewer groups, got %d", len(groups))
	}
	flattened := groups.Flatten()
	if !(len(flattened) == 2 && flattened[0] == "@a" && flattened[1] == "@b") && !(flattened[0] == "@b" && flattened[1] == "@a") {
		t.Errorf("unexpected names: %v", flattened)
	}

	// Ensure reverse lookup updated
	if _, ok := om.nameReviewerMap["@b"]; !ok {
		t.Errorf("nameReviewerMap not updated for @b")
	}
}

func TestAddInlineOwners_FileNotPresent(t *testing.T) {
	rgm := NewReviewerGroupMemo()
	rgC := rgm.ToReviewerGroup("@c")

	om := &ownersMap{
		fileToOwner:     make(map[string]fileOwners),
		nameReviewerMap: make(map[string]ReviewerGroups),
	}

	om.AddInlineOwners("new.go", ReviewerGroups{rgC})

	fr := om.FileRequired()
	if len(fr) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(fr))
	}
	if _, ok := fr["new.go"]; !ok {
		t.Fatalf("new.go not in FileRequired map")
	}
}
