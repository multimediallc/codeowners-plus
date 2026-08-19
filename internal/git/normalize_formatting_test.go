package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
)

// Several remove a different number of lines than they add, which is the point:
// only a whole-block comparison can pair the two sides up.
var formattingOnlyHunks = []hunkCase{
	{
		name: "single statement wrapped in braces",
		file: "widget.js",
		body: `-        if (!isOpen) return
+        if (!isOpen) {
+            return
+        }`,
	},
	{
		name: "call rewrapped across lines",
		file: "billing.py",
		body: `-    result = compute(alpha, beta, gamma)
+    result = compute(
+        alpha,
+        beta,
+        gamma,
+    )`,
	},
	{
		name: "nested selectors flattened",
		file: "theme.scss",
		body: `-.card {
-    .title {
-        color: red;
-    }
-}
+.card .title { color: red; }`,
	},
	{
		name: "trailing comma added to a list literal",
		file: "billing.py",
		body: `-values = [alpha, beta]
+values = [alpha, beta,]`,
	},
	{
		name: "space added after a separator",
		file: "billing.py",
		body: `-    label = concat(alpha,beta)
+    label = concat(alpha, beta)`,
	},
	{
		name: "statement semicolon dropped",
		file: "widget.js",
		body: `-const total = sum(items);
+const total = sum(items)`,
	},
	{
		name: "closing brace alone in its own hunk",
		file: "widget.js",
		body: `+}`,
	},
	{
		name: "empty body reflowed onto one line",
		file: "widget.js",
		body: `-    describe("cleanup", () => {
-    })
+    describe("cleanup", () => {})`,
	},
	{
		name: "empty body respaced",
		file: "billing.py",
		body: `-    render(template, { })
+    render(template, {})`,
	},
}

// An empty pair of braces is an argument, a body or a literal rather than block
// structure, so adding or removing one changes what the code says.
var emptyBracePairHunks = []hunkCase{
	{
		name: "empty object argument added",
		file: "widget.ts",
		body: `-    register(handler)
+    register(handler, {})`,
	},
	{
		name: "empty dict argument removed",
		file: "billing.py",
		body: `-    render(template, {})
+    render(template)`,
	},
	{
		name: "empty object added to a wrapped list",
		file: "widget.js",
		body: ` const rows = [
     first,
+    {},
 ]`,
	},
	{
		name: "empty options argument added before a semicolon",
		file: "widget.ts",
		body: `-    apply(config);
+    apply(config, {});`,
	},
}

func TestFormattingRetainsFormattingOnlyHunks(t *testing.T) {
	assertTrivial(t, newNormalizer(steps(formattingOn)), formattingOnlyHunks)
}

func TestFormattingAloneDismissesEverythingElse(t *testing.T) {
	n := newNormalizer(steps(formattingOn))

	assertSignificant(t, n, significantHunks)
}

// An empty pair of braces says something the code does not say without it, so
// formatting may not drop it the way it drops the braces around a body.
func TestFormattingKeepsEmptyBracePairs(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"formatting alone": steps(formattingOn),
		"every step":       steps(commentsOn, whitespaceOn, formattingOn, stringLiteralsOn, renamesOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertSignificant(t, newNormalizer(retention), emptyBracePairHunks)
		})
	}
}
