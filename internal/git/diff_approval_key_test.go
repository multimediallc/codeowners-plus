package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/sourcegraph/go-diff/diff"
)

func parseDiffOrFail(t *testing.T, text string) []*diff.FileDiff {
	t.Helper()
	parsed, err := diff.ParseMultiFileDiff([]byte(text))
	if err != nil {
		t.Fatalf("parsing diff: %v", err)
	}
	return parsed
}

// Every flag is set explicitly so a default-on one cannot ride along unasked.
func retention(formatting bool) *owners.ApprovalRetention {
	no, yes := false, true
	f := &no
	if formatting {
		f = &yes
	}
	return &owners.ApprovalRetention{
		Enabled: true, Whitespace: &no, Comments: &no,
		Formatting: f, StringLiterals: &no, Renames: &no,
	}
}

// An edit that shifts a hunk's boundaries re-anchors it over already-approved
// lines, so the hunk survives the raw hash and is too substantial to be trivial.
func TestChangesSinceMatchesShiftedHunkAgainstApproval(t *testing.T) {
	// A whole new function, then the call rewrapped: the only change since approval.
	const approved = `diff --git a/svc.go b/svc.go
--- a/svc.go
+++ b/svc.go
@@ -10,0 +11,3 @@
+func Handle(r *Request) error {
+	return dispatch(r, opts, deadline)
+}`
	const current = `diff --git a/svc.go b/svc.go
--- a/svc.go
+++ b/svc.go
@@ -10,0 +11,7 @@
+func Handle(r *Request) error {
+	return dispatch(
+		r,
+		opts,
+		deadline,
+	)
+}`

	tt := []struct {
		name      string
		retention *owners.ApprovalRetention
		wantFiles int
	}{
		{"formatting enabled: nothing new to review", retention(true), 0},
		{"formatting disabled: the hunk stands", retention(false), 1},
		{"retention off entirely: the hunk stands", nil, 1},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := changesSince(changesSinceContext{
				newerDiff:  parseDiffOrFail(t, current),
				olderDiff:  parseDiffOrFail(t, approved),
				normalizer: newNormalizer(tc.retention),
			})
			if err != nil {
				t.Fatalf("changesSince: %v", err)
			}
			if len(got) != tc.wantFiles {
				t.Errorf("got %d changed files, want %d", len(got), tc.wantFiles)
			}
		})
	}
}

// A hunk carrying real change survives however aggressively it is normalized.
func TestChangesSinceKeepsRealChangeAgainstApproval(t *testing.T) {
	const approved = `diff --git a/svc.go b/svc.go
--- a/svc.go
+++ b/svc.go
@@ -10,0 +11,3 @@
+func Handle(r *Request) error {
+	return dispatch(r)
+}`
	const current = `diff --git a/svc.go b/svc.go
--- a/svc.go
+++ b/svc.go
@@ -10,0 +11,3 @@
+func Handle(r *Request) error {
+	return dispatchAsync(r)
+}`

	got, err := changesSince(changesSinceContext{
		newerDiff:  parseDiffOrFail(t, current),
		olderDiff:  parseDiffOrFail(t, approved),
		normalizer: newNormalizer(retention(true)),
	})
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("a changed call target must still be reviewable, got %d files", len(got))
	}
}

// The same text arriving in a second file is new code there.
func TestApprovalKeyIsPerFile(t *testing.T) {
	const approved = `diff --git a/one.go b/one.go
--- a/one.go
+++ b/one.go
@@ -1,0 +2 @@
+	audit(user)`
	const current = `diff --git a/two.go b/two.go
--- a/two.go
+++ b/two.go
@@ -1,0 +2 @@
+       audit(user)`

	got, err := changesSince(changesSinceContext{
		newerDiff:  parseDiffOrFail(t, current),
		olderDiff:  parseDiffOrFail(t, approved),
		normalizer: newNormalizer(retention(true)),
	})
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("the same line in a different file is new there, got %d files", len(got))
	}
}

// The raw hash reads the two sides interleaved, so a key built from them
// separately is looser and must not exist at all while every flag is off.
func TestNoApprovalKeyWhenNoFlagsEnabled(t *testing.T) {
	hunk := &diff.Hunk{Body: []byte("+foo()\n-bar()")}
	swapped := &diff.Hunk{Body: []byte("-bar()\n+foo()")}

	if hunkHash(hunk) == hunkHash(swapped) {
		t.Fatal("hunkHash is expected to read the two sides interleaved")
	}
	for _, r := range []*owners.ApprovalRetention{nil, {Enabled: false}, retention(false)} {
		n := newNormalizer(r)
		if _, ok := n.approvalKey("x.go", hunk); ok {
			t.Errorf("approvalKey offered a key with no steps enabled: %+v", r)
		}
	}
	if _, ok := newNormalizer(retention(true)).approvalKey("x.go", hunk); !ok {
		t.Error("approvalKey withheld a key with a step enabled")
	}
}
