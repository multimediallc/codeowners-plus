package f

import (
	"reflect"
	"testing"
)

func TestSlicesItemsMatch(t *testing.T) {
	tt := []struct {
		name     string
		s1       []int
		s2       []int
		expected bool
	}{
		{
			name:     "empty slices",
			s1:       []int{},
			s2:       []int{},
			expected: true,
		},
		{
			name:     "nil slices",
			s1:       nil,
			s2:       nil,
			expected: true,
		},
		{
			name:     "nil and empty slice",
			s1:       nil,
			s2:       []int{},
			expected: true,
		},
		{
			name:     "single element slices",
			s1:       []int{1},
			s2:       []int{1},
			expected: true,
		},
		{
			name:     "different single element slices",
			s1:       []int{1},
			s2:       []int{2},
			expected: false,
		},
		{
			name:     "different size slices",
			s1:       []int{1, 2, 3, 4},
			s2:       []int{1, 2, 3},
			expected: false,
		},
		{
			name:     "different size slices with same items",
			s1:       []int{1, 2, 3, 3},
			s2:       []int{1, 2, 3},
			expected: false,
		},
		{
			name:     "same order same items",
			s1:       []int{1, 2, 3},
			s2:       []int{1, 2, 3},
			expected: true,
		},
		{
			name:     "different order same items",
			s1:       []int{1, 2, 3},
			s2:       []int{2, 1, 3},
			expected: true,
		},
		{
			name:     "different items",
			s1:       []int{1, 2, 3},
			s2:       []int{1, 2, 4},
			expected: false,
		},
		{
			name:     "missing items",
			s1:       []int{1, 2, 3},
			s2:       []int{1, 1, 3},
			expected: false,
		},
		{
			name:     "missing items reversed",
			s1:       []int{1, 1, 3},
			s2:       []int{1, 2, 3},
			expected: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if SlicesItemsMatch(tc.s1, tc.s2) != tc.expected {
				if tc.expected {
					t.Error("Expected match")
				} else {
					t.Error("Expected not to match")
				}
			}
		})
	}
}

func TestSet(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
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
	})

	t.Run("empty set operations", func(t *testing.T) {
		s := NewSet[int]()
		if len(s.Items()) != 0 {
			t.Error("New set should be empty")
		}
		s.Remove(1) // Should not panic
		if s.Contains(1) {
			t.Error("Empty set should not contain any items")
		}
	})

	t.Run("multiple operations", func(t *testing.T) {
		s := NewSet[string]()
		s.Add("a")
		s.Add("b")
		s.Add("a") // Duplicate add
		if len(s.Items()) != 2 {
			t.Error("Set should have 2 unique items")
		}
		s.Remove("a")
		if s.Contains("a") {
			t.Error("Set should not contain removed item")
		}
		if !s.Contains("b") {
			t.Error("Set should still contain non-removed item")
		}
		s.Remove("b")
		if len(s.Items()) != 0 {
			t.Error("Set should be empty after removing all items")
		}
	})

	t.Run("complex type", func(t *testing.T) {
		type testStruct struct {
			id   int
			name string
		}
		s := NewSet[testStruct]()
		item1 := testStruct{1, "test1"}
		item2 := testStruct{2, "test2"}
		s.Add(item1)
		s.Add(item2)
		if !s.Contains(item1) {
			t.Error("Set should contain added struct")
		}
		s.Remove(item1)
		if s.Contains(item1) {
			t.Error("Set should not contain removed struct")
		}
		if !s.Contains(item2) {
			t.Error("Set should still contain non-removed struct")
		}
	})
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
		name        string
		ts1         []int
		ts2         []int
		expected    []int
		failMessage string
	}{
		{
			name:        "simple case",
			ts1:         []int{1, 2, 3},
			ts2:         []int{2, 3, 4},
			expected:    []int{2, 3},
			failMessage: "Should work for simple case",
		},
		{
			name:        "duplicates in first slice",
			ts1:         []int{1, 2, 2, 3},
			ts2:         []int{2, 3, 4},
			expected:    []int{2, 3},
			failMessage: "Should not include duplicates if only in one slice",
		},
		{
			name:        "duplicates in second slice",
			ts1:         []int{1, 2, 3},
			ts2:         []int{2, 2, 3, 4},
			expected:    []int{2, 3},
			failMessage: "Should not include duplicates if only in one slice reversed",
		},
		{
			name:        "duplicates in both slices",
			ts1:         []int{1, 2, 2, 3},
			ts2:         []int{2, 2, 3, 4},
			expected:    []int{2, 2, 3},
			failMessage: "Should include duplicates if in both slices",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if !SlicesItemsMatch(Intersection(tc.ts1, tc.ts2), tc.expected) {
				t.Error(tc.failMessage)
			}
		})
	}
}

func TestRemoveValue(t *testing.T) {
	tt := []struct {
		name     string
		slice    []int
		value    int
		expected []int
	}{
		{
			name:     "value in slice",
			slice:    []int{1, 2, 3},
			value:    2,
			expected: []int{1, 3},
		},
		{
			name:     "value not in slice",
			slice:    []int{1, 2, 3},
			value:    4,
			expected: []int{1, 2, 3},
		},
		{
			name:     "multiple occurrences",
			slice:    []int{1, 2, 3, 2, 2},
			value:    2,
			expected: []int{1, 3},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			removeResult := RemoveValue(tc.slice, tc.value)
			if !SlicesItemsMatch(removeResult, tc.expected) {
				t.Errorf("Expected items %+v, got %+v", tc.expected, removeResult)
			}
		})
	}
}

func TestGetZero(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		if getZero[int]() != 0 {
			t.Error("Expected zero value for int to be 0")
		}
	})

	t.Run("string", func(t *testing.T) {
		if getZero[string]() != "" {
			t.Error("Expected zero value for string to be empty string")
		}
	})

	t.Run("bool", func(t *testing.T) {
		if getZero[bool]() != false {
			t.Error("Expected zero value for bool to be false")
		}
	})

	t.Run("struct", func(t *testing.T) {
		type testStruct struct {
			a int
			b string
		}
		zero := getZero[testStruct]()
		if zero.a != 0 || zero.b != "" {
			t.Error("Expected zero value for struct to have zero values for all fields")
		}
	})
}

func TestFind(t *testing.T) {
	tt := []struct {
		name        string
		slice       []int
		findFunc    func(int) bool
		expected    int
		shouldFind  bool
		failMessage string
	}{
		{
			name:  "find existing value",
			slice: []int{1, 2, 3, 4, 5},
			findFunc: func(i int) bool {
				return i == 3
			},
			expected:    3,
			shouldFind:  true,
			failMessage: "Should find value 3 in slice",
		},
		{
			name:  "find first even number",
			slice: []int{1, 2, 3, 4, 5},
			findFunc: func(i int) bool {
				return i%2 == 0
			},
			expected:    2,
			shouldFind:  true,
			failMessage: "Should find first even number (2) in slice",
		},
		{
			name:  "value not found",
			slice: []int{1, 2, 3, 4, 5},
			findFunc: func(i int) bool {
				return i > 10
			},
			expected:    0,
			shouldFind:  false,
			failMessage: "Should not find value greater than 10",
		},
		{
			name:  "empty slice",
			slice: []int{},
			findFunc: func(i int) bool {
				return true
			},
			expected:    0,
			shouldFind:  false,
			failMessage: "Should not find anything in empty slice",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			found, ok := Find(tc.slice, tc.findFunc)
			if ok != tc.shouldFind {
				t.Errorf("%s: Expected shouldFind=%v, got %v", tc.name, tc.shouldFind, ok)
			}
			if found != tc.expected {
				t.Errorf("%s: Expected %v, got %v", tc.name, tc.expected, found)
			}
		})
	}
}
