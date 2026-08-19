package git

import "testing"

// Where indentation delimits a block, moving a statement changes what runs, and
// collapseWhitespace drops the indentation that says so.
func TestReindentInIndentDelimitedLanguageIsNotTrivial(t *testing.T) {
	tt := []struct {
		name, file, body string
		trivial          bool
	}{
		{
			name: "python statement dedented out of a block",
			file: "billing.py",
			body: "-        charge(user)\n+    charge(user)",
		},
		{
			name: "python statement indented into a block",
			file: "billing.py",
			body: "-    charge(user)\n+        charge(user)",
		},
		{
			name: "yaml key moved to another parent",
			file: "deploy.yaml",
			body: "-    replicas: 3\n+  replicas: 3",
		},
		{
			name:    "same depth, tabs converted to spaces",
			file:    "billing.py",
			body:    "-\tcharge(user)\n+    charge(user)",
			trivial: true,
		},
		{
			name:    "trailing whitespace only",
			file:    "billing.py",
			body:    "-    charge(user)   \n+    charge(user)",
			trivial: true,
		},
		{
			name:    "indentation is only layout in a braced language",
			file:    "billing.go",
			body:    "-    charge(user)\n+        charge(user)",
			trivial: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			n := newNormalizer(steps(formattingOn))
			got := n.isTrivial(tc.file, hunkOf(tc.body))
			if got != tc.trivial {
				t.Errorf("isTrivial = %v, want %v", got, tc.trivial)
			}
		})
	}
}

// The guard reads indentation, so it must not fire where nothing reads it.
func TestReindentGuardInertWithoutALayoutFlag(t *testing.T) {
	dedent := "-        charge(user)\n+    charge(user)"
	if newNormalizer(nil).isTrivial("billing.py", hunkOf(dedent)) {
		t.Error("no retention configured must retain nothing")
	}
	if !reindentsBlock("billing.py", "    charge(user)", "        charge(user)") {
		t.Error("reindentsBlock missed a pure re-indent")
	}
	if reindentsBlock("billing.go", "    charge(user)", "        charge(user)") {
		t.Error("reindentsBlock fired on a braced language")
	}
	// Rewrapping changes the line count, so the line-for-line match lets it past.
	rewrapped := "    charge(\n        user,\n        amount,\n    )"
	if reindentsBlock("billing.py", rewrapped, "    charge(user, amount)") {
		t.Error("reindentsBlock fired on a rewrap, which is what it must not do")
	}
}
