package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
)

// Hunks the whitespace step is meant to recognize: the two sides hold the same
// tokens in the same order and disagree only about the gaps between them.
var whitespaceOnlyHunks = []hunkCase{
	{
		name: "reindented statement",
		file: "billing.go",
		body: `-    total := price * quantity
+        total := price * quantity`,
	},
	{
		name: "tabs replaced by spaces",
		file: "billing.py",
		body: "-\ttotal = price * quantity\n+    total = price * quantity",
	},
	{
		name: "operands realigned",
		file: "billing.py",
		body: `-alpha = 1
-beta  = 2
+alpha  = 1
+beta   = 2`,
	},
	{
		name: "trailing whitespace stripped",
		file: "cache.py",
		body: "-    value = lookup(key)   \n+    value = lookup(key)",
	},
	{
		name: "opening brace moved to its own line",
		file: "server.go",
		body: `-func handle(w Writer) {
+func handle(w Writer)
+{`,
	},
	{
		name: "argument list wrapped at an existing space",
		file: "billing.py",
		body: `-    result = compute(alpha, beta)
+    result = compute(alpha,
+        beta)`,
	},
	{
		name: "separating blank line removed",
		file: "billing.py",
		body: `-alpha = 1
-
+alpha = 1`,
	},
}

// Two sides already the same text before any step ran: the lines moved, and only
// a flag which reads where a line sits has looked at anything here.
var identicalSideHunks = []hunkCase{
	{
		name: "line deleted and added back",
		file: "billing.py",
		body: `-    total = price * quantity
+    total = price * quantity`,
	},
	{
		name: "blank line added",
		file: "billing.py",
		body: `+`,
	},
	{
		name: "blank line moved",
		file: "billing.py",
		body: `-
+`,
	},
	{
		name: "comment line deleted and added back",
		file: "connection.js",
		body: `-    // reuse the pooled connection
+    // reuse the pooled connection`,
	},
}

// Formatting rewrites the whitespace around the punctuation it drops, so either
// step alone has to leave the whitespace-only hunks trivial.
var whitespaceAwareRetentions = map[string]*owners.ApprovalRetention{
	"whitespace alone": steps(whitespaceOn),
	"formatting alone": steps(formattingOn),
	"both":             steps(whitespaceOn, formattingOn),
}

func TestWhitespaceRetainsWhitespaceOnlyHunks(t *testing.T) {
	for name, retention := range whitespaceAwareRetentions {
		t.Run(name, func(t *testing.T) {
			assertTrivial(t, newNormalizer(retention), whitespaceOnlyHunks)
		})
	}
}

func TestWhitespaceAloneDismissesEverythingElse(t *testing.T) {
	n := newNormalizer(steps(whitespaceOn))

	assertSignificant(t, n, significantHunks)
}

// The layout flags' business and only theirs: a flag which rewrites something
// else read nothing here, so it may not hand the approval back.
func TestOnlyLayoutFlagsRetainIdenticalSides(t *testing.T) {
	retained := map[string]*owners.ApprovalRetention{
		"whitespace alone":  steps(whitespaceOn),
		"formatting alone":  steps(formattingOn),
		"both layout flags": steps(whitespaceOn, formattingOn),
	}
	for name, retention := range retained {
		t.Run(name, func(t *testing.T) {
			assertTrivial(t, newNormalizer(retention), identicalSideHunks)
		})
	}

	dismissed := map[string]*owners.ApprovalRetention{
		"comments alone":        steps(commentsOn),
		"string literals alone": steps(stringLiteralsOn),
		"renames alone":         steps(renamesOn),
		"section on, no flag":   {Enabled: true},
	}
	for name, retention := range dismissed {
		t.Run(name, func(t *testing.T) {
			assertSignificant(t, newNormalizer(retention), identicalSideHunks)
		})
	}
}
