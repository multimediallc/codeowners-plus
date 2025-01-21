package owners

import (
	"maps"
	"slices"
)

type Set[T comparable] map[T]struct{}

func NewSet[T comparable]() Set[T] {
	return make(map[T]struct{})
}

func (s Set[T]) Add(item T) {
	s[item] = struct{}{}
}

func (s Set[T]) Remove(item T) {
	delete(s, item)
}

func (s Set[T]) Contains(item T) bool {
	_, found := s[item]
	return found
}

func (s Set[T]) Items() []T {
	return slices.Collect(maps.Keys(s))
}

func Map[T, U any](ts []T, f func(T) U) []U {
	us := make([]U, len(ts))
	for i, t := range ts {
		us[i] = f(t)
	}
	return us
}

func MapMap[K comparable, T, U any](tm map[K]T, f func(T) U) map[K]U {
	um := make(map[K]U, len(tm))
	for k, t := range tm {
		um[k] = f(t)
	}
	return um
}

func Filtered[T any](ts []T, f func(T) bool) []T {
	filtered := make([]T, 0)
	for _, t := range ts {
		if f(t) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func FilteredMap[K comparable, T any](tm map[K]T, f func(T) bool) map[K]T {
	filtered := make(map[K]T)
	for k, t := range tm {
		if f(t) {
			filtered[k] = t
		}
	}
	return filtered
}

func RemoveDuplicates[T comparable](sliceList []T) []T {
	seen := NewSet[T]()
	return slices.DeleteFunc(sliceList, func(t T) bool {
		if seen.Contains(t) {
			return true
		}
		seen.Add(t)
		return false
	})
}

func Intersection[T comparable](slice1, slice2 []T) []T {
	slice1Items := make(map[T]int, len(slice1))
	for _, item := range slice1 {
		_, found := slice1Items[item]
		if !found {
			slice1Items[item] = 1
		} else {
			slice1Items[item]++
		}
	}
	return Filtered(slice2, func(t T) bool {
		if slice1Items[t] > 0 {
			slice1Items[t]--
			return true
		}
		return false
	})
}

func SlicesItemsMatch[T comparable](slice1, slice2 []T) bool {
	if len(slice1) != len(slice2) {
		return false
	}
	slice1Map := make(map[T]bool, len(slice1))
	for _, item := range slice1 {
		slice1Map[item] = false
	}
	matchMap := maps.Clone(slice1Map)
	for _, item := range slice2 {
		matchMap[item] = true
	}
	if len(matchMap) != len(slice1Map) {
		return false
	}
	for _, found := range matchMap {
		if !found {
			return false
		}
	}
	return true
}

func RemoveValue[T comparable](slice []T, value T) []T {
	return slices.DeleteFunc(slice, func(t T) bool {
		return t == value
	})
}

func getZero[T any]() T {
	var zero T
	return zero
}

func Find[T comparable](slice []T, findFunc func(T) bool) (T, bool) {
	for _, item := range slice {
		if findFunc(item) {
			return item, true
		}
	}
	return getZero[T](), false
}
