package inlineowners

import "testing"

func makeOracle() Oracle {
	return Oracle{
		"file.go": {
			{Owners: []string{"a"}, Start: 10, End: 20},
			{Owners: []string{"b"}, Start: 30, End: 40},
		},
	}
}

func TestOwnersForRange_FullInside(t *testing.T) {
	o := makeOracle()
	owners := o.OwnersForRange("file.go", 12, 18)
	if len(owners) != 1 || owners[0][0] != "a" {
		t.Fatalf("expected owner a, got %+v", owners)
	}
}

func TestOwnersForRange_PartialOverlap(t *testing.T) {
	o := makeOracle()
	owners := o.OwnersForRange("file.go", 25, 32)
	if len(owners) != 1 || owners[0][0] != "b" {
		t.Fatalf("expected owner b, got %+v", owners)
	}
}

func TestOwnersForRange_Outside(t *testing.T) {
	o := makeOracle()
	owners := o.OwnersForRange("file.go", 1, 5)
	if owners != nil {
		t.Fatalf("expected nil owners, got %+v", owners)
	}
}
