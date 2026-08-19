package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
)

// One name substituted everywhere it appeared and nothing else.  Each names
// something spelled out where it is used, so the hunk holds the whole change.
var renameOnlyHunks = []hunkCase{
	{
		name: "parameter renamed",
		file: "views.py",
		body: `-def handler(request):
+def handler(req):`,
	},
	{
		name: "loop variable renamed at every use",
		file: "billing.py",
		body: `-    for item in items:
-        total += item.price
+    for entry in items:
+        total += entry.price`,
	},
	{
		name: "unused binding renamed",
		file: "views.py",
		body: `-    handler = lambda user: None
+    handler = lambda _: None`,
	},
	{
		name: "local renamed at every use",
		file: "billing.py",
		body: `-    total = price * quantity
-    return total
+    subtotal = price * quantity
+    return subtotal`,
	},
	{
		name: "keyword parameter renamed",
		file: "cache.py",
		body: `-def connect(host, retry_count=3):
+def connect(host, retries=3):`,
	},
	{
		name: "comprehension variable renamed",
		file: "billing.py",
		body: `-    names = [row.label for row in rows]
+    names = [entry.label for entry in rows]`,
	},
	{
		name: "constant typo fixed at every use",
		file: "cache.py",
		body: `-RETRY_LIMTI = 3
-    if count > RETRY_LIMTI:
+RETRY_LIMIT = 3
+    if count > RETRY_LIMIT:`,
	},
	{
		name: "test renamed to another test name",
		file: "test_billing.py",
		body: `-def test_totals_add_up(self):
+def test_totals_include_tax(self):`,
	},
	{
		name: "member the hunk itself assigns renamed",
		file: "cache.py",
		body: `-        self.retry_count = 0
-        return self.retry_count
+        self.retries = 0
+        return self.retries`,
	},
}

// Each wears the shape of a substitution while the other half of what it renames
// - a call's body, a framework's hook, an error handler - lives out of sight.
var renamedNameNotOwnedHunks = []hunkCase{
	{
		name: "predicate swapped at its call site",
		file: "billing.py",
		body: `-    if is_active(account):
+    if can_post(account):`,
	},
	{
		name: "helper renamed at its call site",
		file: "widget.js",
		body: `-    result = computeTotal(items)
+    result = calculateTotal(items)`,
	},
	{
		name: "decorator swapped",
		file: "test_views.py",
		body: `-@override_settings
+@modify_settings`,
	},
	{
		name: "boolean property swapped in a predicate",
		file: "widget.js",
		body: `-    if (account.isActive) {
+    if (account.canPost) {`,
	},
	{
		name: "enum member swapped",
		file: "billing.py",
		body: `-    state = Status.ACTIVE
+    state = Status.PENDING`,
	},
	{
		name: "raised error type swapped",
		file: "billing.py",
		body: `-        raise ValidationError
+        raise ConfigError`,
	},
	{
		name: "thrown error type swapped",
		file: "widget.js",
		body: `-        throw ValidationError
+        throw ConfigError`,
	},
	{
		name: "overridden hook renamed away from the name it is dispatched to",
		file: "views.py",
		body: `-def get_queryset(self):
+def build_queryset(self):`,
	},
	{
		name: "class renamed",
		file: "views.py",
		body: `-class OrderView:
+class OrderPage:`,
	},
	{
		name: "exported function renamed",
		file: "widget.js",
		body: `-function computeTotal(items) {
+function calculateTotal(items) {`,
	},
}

// The rule reads punctuation to decide how a name is used, and what punctuation
// means is a fact about a language, so an unnamed file's guards do not apply.
func TestRenamesNeedAKnownLanguage(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"renames alone": steps(renamesOn),
		"every step":    steps(renamesOn, commentsOn, whitespaceOn, formattingOn, stringLiteralsOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			n := newNormalizer(retention)
			assertTrivial(t, n, renameOnlyHunks)

			for _, file := range []string{
				"data.unknownext",
				"CHANGELOG",
				"",
				"vendor/blob",
			} {
				t.Run(file, func(t *testing.T) {
					assertSignificant(t, n, withFile(file, renameOnlyHunks))
				})
			}
		})
	}
}

func TestRenamesRetainsRenameOnlyHunks(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"renames alone": steps(renamesOn),
		"every step":    steps(renamesOn, commentsOn, whitespaceOn, formattingOn, stringLiteralsOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertTrivial(t, newNormalizer(retention), renameOnlyHunks)
		})
	}
}

func TestRenamesAloneDismissesEverythingElse(t *testing.T) {
	n := newNormalizer(steps(renamesOn))

	assertSignificant(t, n, significantHunks)
}

// With only renames enabled there is no substitution to point at, so a hunk which
// normalizes to nothing dismisses rather than riding in on a flag that never read it.
func TestRenamesAloneDismissesAnEmptiedHunk(t *testing.T) {
	if newNormalizer(steps(renamesOn)).isTrivial("billing.py", hunkOf("+")) {
		t.Error("expected an added blank line to be non-trivial with only renames enabled")
	}
}

// Each substitutes one identifier consistently and would read as a rename on that
// alone, while what answers to the old name lives outside the hunk.
func TestRenamesDismissANameTheChangeDoesNotOwn(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"renames alone": steps(renamesOn),
		"every step":    steps(renamesOn, commentsOn, whitespaceOn, formattingOn, stringLiteralsOn),
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertSignificant(t, newNormalizer(retention), renamedNameNotOwnedHunks)
		})
	}
}

// A rename dismisses until the flag is set to true in as many words, whatever
// else the section has been asked for.
func TestRenamesStayOffUntilAskedFor(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"section on, no flag": {Enabled: true},
		"every other step on": steps(commentsOn, whitespaceOn, formattingOn, stringLiteralsOn),
		"asked for but false": {Enabled: true, Renames: flag(false)},
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			assertSignificant(t, newNormalizer(retention), renameOnlyHunks)
		})
	}
}

// Two names changing is not one rename.  Each substitution on its own would
// read as one, so the rule has to hold the pair it found and refuse a second.
var twoNamesChangedHunks = []hunkCase{
	{
		name: "two locals renamed in one hunk",
		file: "billing.py",
		body: `-    total = price * quantity
+    subtotal = cost * quantity`,
	},
	{
		name: "parameter and its use renamed to different names",
		file: "views.py",
		body: `-def handler(request, session):
-    return request.user, session
+def handler(req, ctx):
+    return req.user, ctx`,
	},
	{
		name: "one rename plus one value changed",
		file: "cache.py",
		body: `-    retry_count = 3
-    return retry_count
+    retries = 5
+    return retries`,
	},
}

func TestRenamesDismissTwoNamesChanging(t *testing.T) {
	assertSignificant(t, newNormalizer(steps(renamesOn)), twoNamesChangedHunks)
}
