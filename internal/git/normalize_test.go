package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
	"github.com/sourcegraph/go-diff/diff"
)

// A hunkCase carries the file it changes: the same body classifies differently
// under a different extension, so the name is part of the fixture.
type hunkCase struct {
	name string
	file string
	body string
}

// Refiles a set of hunks to run the same bodies against another language.
func withFile(file string, cases []hunkCase) []hunkCase {
	refiled := make([]hunkCase, 0, len(cases))
	for _, tc := range cases {
		tc.file = file
		refiled = append(refiled, tc)
	}
	return refiled
}

func flag(value bool) *bool {
	return &value
}

type retentionOpt func(*owners.ApprovalRetention)

func whitespaceOn(r *owners.ApprovalRetention) { r.Whitespace = flag(true) }

func commentsOn(r *owners.ApprovalRetention) { r.Comments = flag(true) }

func formattingOn(r *owners.ApprovalRetention) { r.Formatting = flag(true) }

func stringLiteralsOn(r *owners.ApprovalRetention) { r.StringLiterals = flag(true) }

func renamesOn(r *owners.ApprovalRetention) { r.Renames = flag(true) }

// Enables exactly the named steps, so no test depends on what the umbrella covers.
func steps(opts ...retentionOpt) *owners.ApprovalRetention {
	retention := &owners.ApprovalRetention{
		Enabled:        true,
		Whitespace:     flag(false),
		Comments:       flag(false),
		Formatting:     flag(false),
		StringLiterals: flag(false),
		Renames:        flag(false),
	}
	for _, opt := range opts {
		opt(retention)
	}
	return retention
}

// Hunks which must keep dismissing approvals.  Each one either changes
// behavior outright or belongs to a step these flags do not turn on.
var significantHunks = []hunkCase{
	{
		name: "condition inverted",
		file: "widget.js",
		body: `-    if (isReady) {
+    if (!isReady) {`,
	},
	{
		name: "guard clause added",
		file: "widget.js",
		body: ` value = lookup(key)
+    if (value == null) {
+        return
+    }`,
	},
	{
		name: "reindented and edited at once",
		file: "billing.py",
		body: `-    total = price * quantity
+        total = price + quantity`,
	},
	{
		name: "parentheses moved between operands",
		file: "billing.py",
		body: `-    limit = (base + extra) * factor
+    limit = base + (extra * factor)`,
	},
	{
		name: "reference turned into a call",
		file: "widget.js",
		body: `-    button.onClick = submit
+    button.onClick = submit()`,
	},
	{
		name: "call turned into a bare token sequence",
		file: "widget.js",
		body: `-    render(items)
+    render items`,
	},
	{
		name: "space between two names removed",
		file: "index.html",
		body: `-<div class="card wide">
+<div class="cardwide">`,
	},
	{
		name: "space inside a keyword pair removed",
		file: "billing.py",
		body: `-    if not ready:
+    if notready:`,
	},
	{
		name: "two identifiers renamed at once",
		file: "views.py",
		body: `-def handler(request, session):
+def handle(req, session):`,
	},
	{
		name: "identifier renamed and an argument added",
		file: "billing.py",
		body: `-    total = compute(items)
+    sum = compute(items, rate)`,
	},
	{
		name: "identifier renamed and an operator changed",
		file: "billing.py",
		body: `-    total = price * quantity
+    amount = price + quantity`,
	},
	{
		name: "renamed name left standing at one of its uses",
		file: "views.py",
		body: `-    user = lookup(user)
+    account = lookup(user)`,
	},
	{
		name: "word operator swapped",
		file: "billing.py",
		body: `-    if ready and loaded:
+    if ready or loaded:`,
	},
	{
		name: "boolean literal flipped",
		file: "settings.py",
		body: `-    enabled = True
+    enabled = False`,
	},
	{
		name: "string literal edited",
		file: "settings.py",
		body: `-    label = "Allow calls"
+    label = "Allow all calls"`,
	},
	{
		name: "hash inside a string literal",
		file: "settings.py",
		body: `-    anchor = "/pricing#plans"
+    anchor = "/pricing"`,
	},
	{
		name: "spaced hash inside a string literal",
		file: "settings.py",
		body: `-    label = "Total # of items"
+    label = "Total # of rows"`,
	},
	{
		name: "slashes inside a string literal",
		file: "widget.js",
		body: `-    prefix = "path // separator"
+    prefix = "path // divider"`,
	},
	{
		name: "slashes inside an unquoted url",
		file: "package.yml",
		body: `-  homepage: https://example.com/docs
+  homepage: https://example.com/guides/docs`,
	},
	{
		name: "anchor glued to a url in a shell command",
		file: "deploy.sh",
		body: `-curl https://example.com/page#section-2
+curl https://example.com/page#section-3`,
	},
	{
		name: "preprocessor directive retuned",
		file: "limits.c",
		body: `-#define MAX_RETRIES 3
+#define MAX_RETRIES 5`,
	},
	{
		name: "interpreter line replaced",
		file: "deploy.sh",
		body: `-#!/bin/sh
+#!/usr/bin/env bash`,
	},
	{
		name: "id selector rule extended",
		file: "theme.css",
		body: `-#header { margin: 0; }
+#header { margin: 0 auto; }`,
	},
	{
		name: "splat argument copied",
		file: "billing.py",
		body: `-    **options,
+    **options.copy(),`,
	},
	{
		name: "command flag mistaken for a query comment",
		file: "pipeline.yml",
		body: `-  command: npm test --silent
+  command: npm test --silent --bail`,
	},
	{
		name: "code changed under an unchanged trailing comment",
		file: "connection.py",
		body: `-    retries = 3  # tuned by hand
+    retries = 5  # tuned by hand`,
	},
	{
		name: "code changed and its trailing comment dropped",
		file: "connection.py",
		body: `-    retries = 3  # tuned by hand
+    retries = 5`,
	},
	{
		name: "query string edited",
		file: "billing.py",
		body: `-    rows = fetch("SELECT id FROM users WHERE active")
+    rows = fetch("SELECT id FROM users WHERE enabled")`,
	},
	{
		name: "endpoint string edited",
		file: "settings.py",
		body: `-    endpoint = "https://example.com/v1/users"
+    endpoint = "https://example.com/v2/users"`,
	},
	{
		name: "translated endpoint string edited",
		file: "settings.py",
		body: `-    endpoint = _("https://example.com/v1 of the api")
+    endpoint = _("https://example.com/v2 of the api")`,
	},
	{
		name: "log message edited",
		file: "cache.py",
		body: `-    logger.warning("cache miss on lookup")
+    logger.warning("cache expired on lookup")`,
	},
	{
		name: "named placeholder changed in translated copy",
		file: "settings.py",
		body: `-    text = _("{count} items left")
+    text = _("{total} items left")`,
	},
	{
		name: "percent placeholder changed in translated copy",
		file: "settings.py",
		body: `-    text = _("%(count)d items left")
+    text = _("%(total)d items left")`,
	},
	{
		name: "translation key changed",
		file: "widget.js",
		body: `-    label = t("nav.home")
+    label = t("nav.dashboard")`,
	},
	{
		name: "copy passed to a helper ending in a translation name",
		file: "settings.py",
		body: `-    label = format_t("Old copy here")
+    label = format_t("New copy here")`,
	},
	{
		name: "translated copy reworded and an argument added",
		file: "settings.py",
		body: `-    text = ngettext("one file", "several files", count)
+    text = ngettext("a single file", "many files", count, locale)`,
	},
	{
		name: "lines added only",
		file: "billing.py",
		body: `+    audit.log(event)`,
	},
	{
		name: "lines removed only",
		file: "billing.py",
		body: `-    audit.log(event)`,
	},
	{
		name: "context lines only",
		file: "cache.py",
		body: ` value = lookup(key)`,
	},
}

func hunkOf(body string) *diff.Hunk {
	return &diff.Hunk{Body: []byte(body)}
}

func assertTrivial(t *testing.T, n normalizer, cases []hunkCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !n.isTrivial(tc.file, hunkOf(tc.body)) {
				t.Errorf("expected hunk to be trivial, got non-trivial in %s:\n%s", tc.file, tc.body)
			}
		})
	}
}

func assertSignificant(t *testing.T, n normalizer, cases []hunkCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if n.isTrivial(tc.file, hunkOf(tc.body)) {
				t.Errorf("expected hunk to be non-trivial, got trivial in %s:\n%s", tc.file, tc.body)
			}
		})
	}
}

func TestChangesSinceSkipsTrivialHunks(t *testing.T) {
	const currentDiff = `diff --git a/style.css b/style.css
index abc..def 100644
--- a/style.css
+++ b/style.css
@@ -4,5 +4 @@
-.card {
-    .title {
-        color: red;
-    }
-}
+.card .title { color: red; }
diff --git a/app.js b/app.js
index ghi..jkl 100644
--- a/app.js
+++ b/app.js
@@ -12 +12 @@
-const total = sum(items)
+const total = sum(items) * rate`

	const olderDiff = `diff --git a/notes.txt b/notes.txt
index abc..def 100644
--- a/notes.txt
+++ b/notes.txt
@@ -1,0 +2 @@
+unrelated`

	tt := []struct {
		name          string
		retention     *owners.ApprovalRetention
		expectedFiles []string
	}{
		{
			name:          "retention off keeps the formatting-only file",
			retention:     nil,
			expectedFiles: []string{"style.css", "app.js"},
		},
		{
			name:          "whitespace alone keeps the formatting-only file",
			retention:     steps(whitespaceOn),
			expectedFiles: []string{"style.css", "app.js"},
		},
		{
			name:          "formatting on drops the formatting-only file",
			retention:     steps(formattingOn),
			expectedFiles: []string{"app.js"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			context := DiffContext{
				Base:              "main",
				Head:              "feature",
				Dir:               ".",
				ApprovalRetention: tc.retention,
			}

			gitDiff, err := NewDiffWithExecutor(context, NewMockGitExecutor(currentDiff, nil))
			if err != nil {
				t.Fatalf("failed to create initial diff: %v", err)
			}
			gitDiff.(*GitDiff).executor = NewMockGitExecutor(olderDiff, nil)

			changes, err := gitDiff.ChangesSince("old-ref")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(changes) != len(tc.expectedFiles) {
				t.Fatalf("expected %d files, got %d (%v)", len(tc.expectedFiles), len(changes), changes)
			}
			for i, expected := range tc.expectedFiles {
				if changes[i].FileName != expected {
					t.Errorf("file %d: expected %s, got %s", i, expected, changes[i].FileName)
				}
			}
		})
	}
}
