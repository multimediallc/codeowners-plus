package git

import (
	"errors"
	"testing"

	"github.com/sourcegraph/go-diff/diff"
)

const filterHeadDiff = `diff --git a/service.go b/service.go
--- a/service.go
+++ b/service.go
@@ -1,0 +2,1 @@
+// a note the reviewer has seen
@@ -10,0 +12,1 @@
+realChange()
diff --git a/other.go b/other.go
--- a/other.go
+++ b/other.go
@@ -3,0 +4,1 @@
+untouchedByTheFilter()
`

const filterApprovalDiff = `diff --git a/service.go b/service.go
--- a/service.go
+++ b/service.go
@@ -20,0 +21,1 @@
+approvedEarlier()
`

func parseFilterDiff(t *testing.T, body string) []*diff.FileDiff {
	t.Helper()
	parsed, err := diff.ParseMultiFileDiff([]byte(body))
	if err != nil {
		t.Fatalf("parsing diff: %v", err)
	}
	return parsed
}

func hunkCounts(t *testing.T, filter HunkFilter) map[string]int {
	t.Helper()
	files, err := changesSince(changesSinceContext{
		newerDiff: parseFilterDiff(t, filterHeadDiff),
		olderDiff: parseFilterDiff(t, filterApprovalDiff),
		ref:       "approvalsha",
		filter:    filter,
	})
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	counts := make(map[string]int, len(files))
	for _, file := range files {
		counts[file.FileName] = len(file.Hunks)
	}
	return counts
}

func TestChangesSinceWithoutFilterIsUnchanged(t *testing.T) {
	counts := hunkCounts(t, nil)
	if counts["service.go"] != 2 || counts["other.go"] != 1 {
		t.Errorf("expected service.go:2 other.go:1, got %v", counts)
	}
}

func TestHunkFilterDropsOnlyWhatItNames(t *testing.T) {
	var gotRef string
	var gotApproval []string
	counts := hunkCounts(t, func(ref string, files []HunkText) (map[string][]int, error) {
		gotRef = ref
		for _, file := range files {
			if file.Name == "service.go" {
				gotApproval = file.ApprovalHunks
			}
		}
		return map[string][]int{"service.go": {0}}, nil
	})

	if gotRef != "approvalsha" {
		t.Errorf("expected the approval ref to be passed, got %q", gotRef)
	}
	if len(gotApproval) != 1 {
		t.Errorf("expected service.go to carry 1 approval hunk, got %d", len(gotApproval))
	}
	if counts["service.go"] != 1 {
		t.Errorf("expected 1 hunk left in service.go, got %d", counts["service.go"])
	}
	if counts["other.go"] != 1 {
		t.Errorf("expected other.go untouched, got %d", counts["other.go"])
	}
}

func TestHunkFilterDroppingEveryHunkRemovesTheFile(t *testing.T) {
	counts := hunkCounts(t, func(string, []HunkText) (map[string][]int, error) {
		return map[string][]int{"service.go": {0, 1}}, nil
	})
	if _, present := counts["service.go"]; present {
		t.Errorf("expected service.go to drop out entirely, got %v", counts)
	}
	if counts["other.go"] != 1 {
		t.Errorf("expected other.go untouched, got %d", counts["other.go"])
	}
}

func TestHunkFilterFailuresChangeNothing(t *testing.T) {
	tt := []struct {
		name   string
		filter HunkFilter
	}{
		{
			name: "error",
			filter: func(string, []HunkText) (map[string][]int, error) {
				return map[string][]int{"service.go": {0}}, errors.New("hook exploded")
			},
		},
		{
			name: "index past the end",
			filter: func(string, []HunkText) (map[string][]int, error) {
				return map[string][]int{"service.go": {0, 9}}, nil
			},
		},
		{
			name: "negative index",
			filter: func(string, []HunkText) (map[string][]int, error) {
				return map[string][]int{"service.go": {-1}}, nil
			},
		},
		{
			name: "file that was never sent",
			filter: func(string, []HunkText) (map[string][]int, error) {
				return map[string][]int{"invented.go": {0}}, nil
			},
		},
		{
			name: "nothing named",
			filter: func(string, []HunkText) (map[string][]int, error) {
				return nil, nil
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			counts := hunkCounts(t, tc.filter)
			if counts["service.go"] != 2 || counts["other.go"] != 1 {
				t.Errorf("expected service.go:2 other.go:1, got %v", counts)
			}
		})
	}
}

func TestHunkFilterOnlySeesSurvivingHunks(t *testing.T) {
	var sent []HunkText
	_, err := changesSince(changesSinceContext{
		newerDiff: parseFilterDiff(t, filterHeadDiff),
		olderDiff: parseFilterDiff(t, filterHeadDiff),
		ref:       "approvalsha",
		filter: func(_ string, files []HunkText) (map[string][]int, error) {
			sent = files
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(sent) != 0 {
		t.Errorf("expected the filter not to be called with nothing outstanding, got %v", sent)
	}
}
