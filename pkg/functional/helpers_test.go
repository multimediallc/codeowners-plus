package f

import (
	"reflect"
	"testing"
)

func TestSlicesItemsMatch(t *testing.T) {
	tt := []struct {
		s1          []int
		s2          []int
		result      bool
		failMessage string
	}{
		{[]int{1, 2, 3, 4}, []int{1, 2, 3}, false, "Different size Slices should not match"},
		{[]int{1, 2, 3, 3}, []int{1, 2, 3}, false, "Different size Slices should not match even with same items"},
		{[]int{1, 2, 3}, []int{1, 2, 3}, true, "Same order same items Slices should match"},
		{[]int{1, 2, 3}, []int{2, 1, 3}, true, "Different order same items Slices should match"},
		{[]int{1, 2, 3}, []int{1, 2, 4}, false, "Different items Slices should not match"},
		{[]int{1, 2, 3}, []int{1, 1, 3}, false, "Missing items Slices should not match"},
		{[]int{1, 1, 3}, []int{1, 2, 3}, false, "Missing items Slices should not match reversed"},
	}

	for _, tc := range tt {
		if SlicesItemsMatch(tc.s1, tc.s2) != tc.result {
			t.Error(tc.failMessage)
		}
	}
}

func TestSet(t *testing.T) {
	s := NewSet[int]()
	s.Add(1)
	if !s.Contains(1) {
		t.Error("Set should contain Added item")
	}
	s.Remove(1)
	if s.Contains(1) {
		t.Error("Set should not contain Removed item")
	}
	s.Add(1)
	s.Add(2)
	if !SlicesItemsMatch(s.Items(), []int{1, 2}) {
		t.Error("Items should return all items in the set")
	}
}

func TestMap(t *testing.T) {
	ts := []int{1, 2, 3}
	f := func(t int) int {
		return t * 2
	}
	if !SlicesItemsMatch(Map(ts, f), []int{2, 4, 6}) {
		t.Error("Should multiply each item by 2")
	}
}

func TestMapMap(t *testing.T) {
	tm := map[string]int{"a": 1, "b": 2}
	f := func(t int) int {
		return t * 2
	}
	if !reflect.DeepEqual(MapMap(tm, f), map[string]int{"a": 2, "b": 4}) {
		t.Error("Should multiply each item by 2")
	}
}

func TestFiltered(t *testing.T) {
	ts := []int{1, 2, 3, 4, 5, 6, 7}
	f := func(t int) bool {
		return t%2 == 0
	}
	if !SlicesItemsMatch(Filtered(ts, f), []int{2, 4, 6}) {
		t.Error("Should filter out odd numbers")
	}
}

func TestFilteredMap(t *testing.T) {
	ts := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7}
	f := func(t int) bool {
		return t%2 == 0
	}
	if !reflect.DeepEqual(FilteredMap(ts, f), map[string]int{"b": 2, "d": 4, "f": 6}) {
		t.Error("Should filter out odd numbers")
	}
}

func TestRemoveDuplicates(t *testing.T) {
	ts := []int{1, 2, 2, 3}
	if !SlicesItemsMatch(RemoveDuplicates(ts), []int{1, 2, 3}) {
		t.Error("Should remove duplicates")
	}
}

func TestIntersection(t *testing.T) {
	tt := []struct {
		ts1         []int
		ts2         []int
		result      []int
		failMessage string
	}{
		{[]int{1, 2, 3}, []int{2, 3, 4}, []int{2, 3}, "Should work for simple case"},
		{[]int{1, 2, 2, 3}, []int{2, 3, 4}, []int{2, 3}, "Should not include duplicates if only in one slice"},
		{[]int{1, 2, 3}, []int{2, 2, 3, 4}, []int{2, 3}, "Should not include duplicates if only in one slice reversed"},
		{[]int{1, 2, 2, 3}, []int{2, 2, 3, 4}, []int{2, 2, 3}, "Should include duplicates if in both slices"},
	}

	for _, tc := range tt {
		if !SlicesItemsMatch(Intersection(tc.ts1, tc.ts2), tc.result) {
			t.Error(tc.failMessage)
		}
	}
}

func TestRemoveValue(t *testing.T) {
	tt := []struct {
		slice       []int
		value       int
		result      []int
		failMessage string
	}{
		{[]int{1, 2, 3}, 2, []int{1, 3}, "Value in slice remove item"},
		{[]int{1, 2, 3}, 4, []int{1, 2, 3}, "Value not in slice should not remove any elements"},
		{[]int{1, 2, 3, 2, 2}, 2, []int{1, 3}, "Value in slice should be removed all occurrences"},
	}

	for _, tc := range tt {
		if !SlicesItemsMatch(RemoveValue(tc.slice, tc.value), tc.result) {
			t.Error(tc.failMessage)
		}
	}
}
