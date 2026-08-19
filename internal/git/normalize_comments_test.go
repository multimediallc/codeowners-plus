package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
)

// Either the hunk only moves comment lines, or it leaves the code byte for byte
// identical and appends an annotation to it.
var commentOnlyHunks = []hunkCase{
	{
		name: "whole-line comment added",
		file: "connection.js",
		body: `-    client = connect(host)
+    // reuse the pooled connection
+    client = connect(host)`,
	},
	{
		name: "commented-out line removed",
		file: "connection.js",
		body: ` value = lookup(key)
-    // audit.log(value)`,
	},
	{
		name: "trailing block comment appended",
		file: "connection.js",
		body: `-    retries = 3
+    retries = 3  /* tuned by hand */`,
	},
	{
		name: "docstring rewritten",
		file: "billing.py",
		body: `-    """Return the total."""
+    """Return the running total."""`,
	},
	{
		name: "block comment reflowed",
		file: "connection.js",
		body: `-    /* pool the connection */
+    /*
+     * pool the connection
+     */`,
	},
	{
		name: "markup comment added",
		file: "index.html",
		body: ` <div>
+  <!-- draft copy -->`,
	},
	{
		name: "query comment added",
		file: "report.sql",
		body: ` SELECT id FROM users
+-- only the active rows`,
	},
	{
		name: "banner comment widened",
		file: "deploy.sh",
		body: `-# section
+### section`,
	},
	// The three the language table earns: each marker means what it says here and
	// spells something else in its pair under languageMistakenMarkerHunks.
	{
		name: "trailing line comment reworded in javascript",
		file: "widget.js",
		body: `-    share = total // 2
+    share = total // 3`,
	},
	{
		name: "trailing line comment reworded in scss",
		file: "theme.scss",
		body: `-  margin: 0 // 2px of slack
+  margin: 0 // 4px of slack`,
	},
	{
		name: "trailing query comment reworded",
		file: "report.sql",
		body: `-SELECT id FROM users WHERE active -- only the active rows
+SELECT id FROM users WHERE active -- active rows only`,
	},
}

// The same markers in a file whose language spells something else with them.
// Read as comments, each hands back an approval over a real change.
var languageMistakenMarkerHunks = []hunkCase{
	{
		name: "floor division mistaken for a line comment",
		file: "billing.py",
		body: `-    share = total // 2
+    share = total // 3`,
	},
	{
		name: "line comment marker in plain css",
		file: "theme.css",
		body: `-  margin: 0 // 2px of slack
+  margin: 0 // 4px of slack`,
	},
	// The spacing around the decrement is what puts the marker on its own, so
	// this is the shape a SQL comment marker would swallow the rest of.
	{
		name: "decrement mistaken for a query comment",
		file: "counter.go",
		body: `-	i -- ; total = 2
+	i -- ; total = 3`,
	},
	{
		name: "heading mistaken for a hash comment",
		file: "notes.md",
		body: `-# section
+### section`,
	},
	{
		name: "docstring quotes in a language without them",
		file: "widget.js",
		body: `-    """Return the total."""
+    """Return the running total."""`,
	},
}

// Comments addressed to a tool.  Each leaves the runtime code identical and
// changes only which findings a checker may report, which is the whole change.
var directiveHunks = []hunkCase{
	{
		name: "security lint silenced over a disabled certificate check",
		file: "connection.py",
		body: `-    client = connect(verify=False)
+    client = connect(verify=False)  # noqa: S501`,
	},
	{
		name: "security scanner suppression appended",
		file: "connection.py",
		body: `-    client = connect(verify=False)
+    client = connect(verify=False)  # nosec B501`,
	},
	{
		name: "complexity budget raised in a lint directive",
		file: "billing.py",
		body: `-# pylint: max-branches=12
+# pylint: max-branches=20`,
	},
	{
		name: "complexity budget raised in a trailing lint directive",
		file: "billing.py",
		body: `-def settle(rows):  # flake8: max-complexity=10
+def settle(rows):  # flake8: max-complexity=18`,
	},
	{
		name: "type checker ignore added",
		file: "billing.py",
		body: `-    total = rows.sum()
+    total = rows.sum()  # type: ignore[attr-defined]`,
	},
	{
		name: "type checker rule silenced",
		file: "billing.py",
		body: ` def settle(rows):
+    # mypy: disable-error-code=arg-type`,
	},
	{
		name: "static analysis directive removed",
		file: "billing.py",
		body: ` def settle(rows):
-    # pyright: reportMissingImports=false`,
	},
	{
		name: "formatter directive pair toggled",
		file: "billing.py",
		body: `-# isort:skip_file
+# isort:off`,
	},
	{
		name: "linter directive widened",
		file: "cache.py",
		body: `-# ruff: noqa: E501
+# ruff: noqa: E501, F401`,
	},
	{
		name: "coverage exclusion appended",
		file: "cache.py",
		body: `-    return fallback()
+    return fallback()  # pragma: no cover`,
	},
	{
		name: "coverage directive added",
		file: "cache.py",
		body: ` def warm(keys):
+    # coverage: ignore`,
	},
	{
		name: "eslint suppression added",
		file: "widget.js",
		body: `-    console.log(state)
+    // eslint-disable-next-line no-console
+    console.log(state)`,
	},
	{
		name: "eslint block reopened",
		file: "widget.js",
		body: ` }
-/* eslint-enable no-console */`,
	},
	{
		name: "typescript error suppression swapped for an expectation",
		file: "widget.ts",
		body: `-// @ts-ignore
+// @ts-expect-error not typed upstream`,
	},
	{
		name: "whole-file type checking turned off",
		file: "widget.ts",
		body: ` import { render } from "./render"
+// @ts-nocheck`,
	},
	{
		name: "formatter directive added",
		file: "widget.js",
		body: ` const rows = [
+    // prettier-ignore
     first,
 ]`,
	},
	{
		name: "coverage instrumentation skipped",
		file: "widget.js",
		body: `-    return fallback()
+    /* istanbul ignore next */
+    return fallback()`,
	},
	{
		name: "coverage runner directive widened",
		file: "widget.js",
		body: `-/* c8 ignore next */
+/* c8 ignore next 4 */`,
	},
	{
		name: "newer coverage runner directive added",
		file: "widget.js",
		body: ` function fallback() {
+    /* v8 ignore start */`,
	},
	{
		name: "stylesheet lint suppression added",
		file: "theme.scss",
		body: ` .card {
+  /* stylelint-disable declaration-no-important */`,
	},
	{
		name: "go linter suppression appended",
		file: "server.go",
		body: `-	config := &tls.Config{InsecureSkipVerify: true}
+	config := &tls.Config{InsecureSkipVerify: true} //nolint:gosec`,
	},
	{
		name: "go security scanner suppression added",
		file: "server.go",
		body: ` func dial(addr string) {
+	//#nosec G402`,
	},
	{
		name: "golangci directive removed",
		file: "server.go",
		body: ` func dial(addr string) {
-	//golangci-lint:ignore staticcheck`,
	},
	{
		name: "generator line retargeted",
		file: "server.go",
		body: `-//go:generate stringer -type=State
+//go:generate stringer -type=Status`,
	},
	{
		name: "build constraint narrowed",
		file: "server.go",
		body: `-//go:build linux || darwin
+//go:build linux`,
	},
	{
		name: "legacy build constraint narrowed",
		file: "server.go",
		body: `-// +build linux darwin
+// +build linux`,
	},
	{
		name: "sonar suppression added",
		file: "Order.java",
		body: `-        run(cmd);
+        run(cmd); // NOSONAR trusted input`,
	},
	{
		name: "checkstyle turned off around a block",
		file: "Order.java",
		body: ` class Order {
+// CHECKSTYLE:OFF`,
	},
	{
		name: "warning suppression widened",
		file: "Order.java",
		body: `-// SuppressWarnings("unchecked")
+// SuppressWarnings("unchecked", "rawtypes")`,
	},
	{
		name: "query analysis suppression added",
		file: "widget.ts",
		body: `-    exec(command)
+    exec(command) // codeql[js/command-line-injection]`,
	},
	{
		name: "shell analysis suppression added",
		file: "deploy.sh",
		body: ` for f in $files; do
+# shellcheck disable=SC2086`,
	},
}

// Prose opening with a word a directive is also spelled with.  Each would read as
// a directive if the match did not require the word to end where the directive does.
var directiveLookalikeHunks = []hunkCase{
	{
		name: "comment opening with a word a directive shortens to",
		file: "billing.py",
		body: `-# pragmatic until the rewrite lands
+# pragmatic for now`,
	},
	{
		name: "comment naming a linter config file",
		file: "widget.js",
		body: `-// eslintrc already covers this rule
+// eslintrc covers this rule`,
	},
	{
		name: "comment about suppressing a linter",
		file: "server.go",
		body: `-// nolinting was needed once the retry landed
+// nolinting is no longer needed here`,
	},
	{
		name: "comment mentioning a directive mid-sentence",
		file: "billing.py",
		body: `-    # we can ignore the cached total here
+    # ignore the cached total, it is refreshed below`,
	},
	{
		name: "comment describing what a value's type is",
		file: "billing.py",
		body: `-# type of the row is decided by the caller
+# type of the row comes from the caller`,
	},
	{
		name: "trailing comment opening with a directive word in prose",
		file: "cache.py",
		body: `-    retries = 3  # coverage of this branch is thin
+    retries = 3  # coverage here is thin`,
	},
	{
		name: "comment mentioning a tool without addressing it",
		file: "widget.js",
		body: `-    // typescript infers this one
+    // typescript already infers this one`,
	},
}

func TestCommentsRetainsCommentOnlyHunks(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"comments alone": steps(commentsOn),
		"every step":     steps(commentsOn, whitespaceOn, formattingOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertTrivial(t, newNormalizer(retention), commentOnlyHunks)
			assertTrivial(t, newNormalizer(retention), directiveLookalikeHunks)
		})
	}
}

func TestCommentsAloneDismissesEverythingElse(t *testing.T) {
	n := newNormalizer(steps(commentsOn))

	assertSignificant(t, n, significantHunks)
}

// Comment-only bodies moved to a file whose language reads those characters as
// code, where reading them as comments hands back an approval over a real change.
func TestCommentsAreLanguageAware(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"comments alone": steps(commentsOn),
		"every step":     steps(commentsOn, whitespaceOn, formattingOn, stringLiteralsOn, renamesOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertSignificant(t, newNormalizer(retention), languageMistakenMarkerHunks)
		})
	}
}

// A file whose language we cannot name gets no markers at all.  Guessing one
// costs a real change; recognizing no comments costs a review round.
func TestCommentsNeedAKnownLanguage(t *testing.T) {
	n := newNormalizer(steps(commentsOn, whitespaceOn, formattingOn))

	for _, file := range []string{
		"data.unknownext",
		"CHANGELOG",
		"",
		"vendor/blob",
	} {
		t.Run(file, func(t *testing.T) {
			assertSignificant(t, n, withFile(file, commentOnlyHunks))
		})
	}
}

// Silencing a security lint or widening a complexity budget leaves the runtime
// code identical while changing what is enforced, which the approver wants to see.
func TestCommentsDismissDirectives(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"comments alone": steps(commentsOn),
		"every step":     steps(commentsOn, whitespaceOn, formattingOn, stringLiteralsOn, renamesOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertSignificant(t, newNormalizer(retention), directiveHunks)
		})
	}
}

// A sentence merely opening with a directive's word is prose; reading it as a
// directive would cost a review round on every comment which starts that way.
func TestCommentsRetainDirectiveLookalikes(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"comments alone": steps(commentsOn),
		"every step":     steps(commentsOn, whitespaceOn, formattingOn, stringLiteralsOn, renamesOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertTrivial(t, newNormalizer(retention), directiveLookalikeHunks)
		})
	}
}
