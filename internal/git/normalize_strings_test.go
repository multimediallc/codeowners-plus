package git

import (
	"testing"

	owners "github.com/multimediallc/codeowners-plus/internal/config"
)

// Hunks the string literals step is meant to recognize: the only thing which
// changed is the wording of a sentence handed to a translation function.
var translatedCopyHunks = []hunkCase{
	{
		name: "translated label reworded",
		file: "settings.py",
		body: `-    title = _("Allow calls")
+    title = _("Allow all calls")`,
	},
	{
		name: "gettext copy reworded",
		file: "settings.py",
		body: `-    label = gettext("Save this draft")
+    label = gettext("Save these changes")`,
	},
	{
		name: "both plural forms reworded",
		file: "settings.py",
		body: `-    text = ngettext("one file", "several files", count)
+    text = ngettext("a single file", "many files", count)`,
	},
	{
		name: "template helper copy reworded",
		file: "SignIn.tsx",
		body: `-  <span>{t("Sign in")}</span>
+  <span>{t("Log in here")}</span>`,
	},
	{
		name: "namespaced helper copy reworded",
		file: "Draft.vue",
		body: `-  <button>{$t("Save draft")}</button>
+  <button>{$t("Save changes")}</button>`,
	},
}

// Copy changes which also moved, so they need the formatting step as well.
var translatedCopyWithFormattingHunks = []hunkCase{
	{
		name: "copy reworded across a wrapped call",
		file: "settings.py",
		body: `-    message = trans("Your session expired")
+    message = trans(
+        "Your session has expired",
+    )`,
	},
	{
		name: "translated argument reindented",
		file: "settings.py",
		body: `-    title = _("Allow calls")
+        title = _("Allow all calls")`,
	},
}

func TestStringLiteralsRetainsTranslatedCopyHunks(t *testing.T) {
	assertTrivial(t, newNormalizer(steps(stringLiteralsOn)), translatedCopyHunks)

	n := newNormalizer(steps(stringLiteralsOn, whitespaceOn, formattingOn))
	assertTrivial(t, n, translatedCopyHunks)
	assertTrivial(t, n, translatedCopyWithFormattingHunks)
}

func TestStringLiteralsAloneDismissesEverythingElse(t *testing.T) {
	n := newNormalizer(steps(stringLiteralsOn))

	assertSignificant(t, n, significantHunks)
}

// The umbrella never turns string literals on by itself, so a translated copy
// change dismisses until the flag is set to true in as many words.
func TestStringLiteralsStayOffUntilAskedFor(t *testing.T) {
	retentions := map[string]*owners.ApprovalRetention{
		"umbrella alone":        {Enabled: true},
		"every other step on":   steps(commentsOn, whitespaceOn, formattingOn),
		"opted in but disabled": {Enabled: true, StringLiterals: flag(false)},
	}

	for name, retention := range retentions {
		t.Run(name, func(t *testing.T) {
			n := newNormalizer(retention)
			assertSignificant(t, n, translatedCopyHunks)
			assertSignificant(t, n, translatedCopyWithFormattingHunks)
		})
	}
}
