package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/multimediallc/codeowners-plus/internal/git"
)

func writeFilterHook(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell hooks are not portable to windows")
	}
	path := filepath.Join(t.TempDir(), "filter")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("writing hook: %v", err)
	}
	return path
}

func filterFiles() []git.HunkText {
	return []git.HunkText{{
		Name:          "service.go",
		HeadHunks:     []string{"+one\n", "+two\n"},
		ApprovalHunks: []string{"+approved\n"},
	}}
}

func TestAppHunkFilterAppliesAValidAnswer(t *testing.T) {
	path := writeFilterHook(t, `cat >&2
echo '{"reviewed":[{"name":"service.go","indexes":[1]}]}'
`)
	warnings := &bytes.Buffer{}
	a := &App{config: &Config{WarningBuffer: warnings, InfoBuffer: &bytes.Buffer{}}}

	reviewed, err := a.hunkFilter(path, "basesha", "headsha")("approvalsha", filterFiles())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reviewed["service.go"]) != 1 || reviewed["service.go"][0] != 1 {
		t.Errorf("expected service.go index 1, got %v", reviewed)
	}

	sent := warnings.String()
	for _, want := range []string{`"ref":"approvalsha"`, `"base":"basesha"`, `"head":"headsha"`, `"approval_hunks"`, `"version":1`} {
		if !strings.Contains(sent, want) {
			t.Errorf("expected %s in the request, got %s", want, sent)
		}
	}
}

func TestAppHunkFilterAnswersNoneOnFailure(t *testing.T) {
	tt := []struct {
		name string
		body string
	}{
		{"exits non-zero", "exit 1\n"},
		{"writes nothing", "true\n"},
		{"names an index that was not sent", `echo '{"reviewed":[{"name":"service.go","indexes":[7]}]}'` + "\n"},
		{"names a file that was not sent", `echo '{"reviewed":[{"name":"invented.go","indexes":[0]}]}'` + "\n"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			warnings := &bytes.Buffer{}
			a := &App{config: &Config{
				WarningBuffer: warnings,
				InfoBuffer:    &bytes.Buffer{},
			}}

			reviewed, err := a.hunkFilter(writeFilterHook(t, tc.body), "basesha", "headsha")("approvalsha", filterFiles())
			if err != nil {
				t.Errorf("expected the failure to be swallowed, got %v", err)
			}
			if len(reviewed) != 0 {
				t.Errorf("expected nothing reviewed, got %v", reviewed)
			}
			if !strings.Contains(warnings.String(), "hunk filter not applied") {
				t.Errorf("expected a warning, got %q", warnings.String())
			}
		})
	}
}

func TestAppHunkFilterMissingHookAnswersNone(t *testing.T) {
	warnings := &bytes.Buffer{}
	a := &App{config: &Config{
		WarningBuffer: warnings,
		InfoBuffer:    &bytes.Buffer{},
	}}

	reviewed, err := a.hunkFilter(filepath.Join(t.TempDir(), "absent"), "basesha", "headsha")("approvalsha", filterFiles())
	if err != nil {
		t.Errorf("expected the failure to be swallowed, got %v", err)
	}
	if len(reviewed) != 0 {
		t.Errorf("expected nothing reviewed, got %v", reviewed)
	}
	if !strings.Contains(warnings.String(), "hunk filter not applied") {
		t.Errorf("expected a warning, got %q", warnings.String())
	}
}

func TestResolveHookPathRejects(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()

	tt := []struct {
		name string
		path string
		want string
	}{
		{"a relative path", "tools/filter", "not an absolute path"},
		{"a bare name off PATH", "filter", "not an absolute path"},
		{"a path in the checkout", filepath.Join(workspace, "tools", "filter"), "inside the checkout"},
		{"the checkout itself", workspace, "inside the checkout"},
		{"a traversal back into the checkout", filepath.Join(outside, "..", filepath.Base(workspace), "f"), "inside the checkout"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveHookPath(tc.path, workspace); err == nil {
				t.Fatalf("expected %q to be refused", tc.path)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q in error, got %q", tc.want, err)
			}
		})
	}
}

func TestResolveHookPathAllowsASiblingWithASharedPrefix(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "work")
	sibling := filepath.Join(root, "work-tools", "filter")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveHookPath(sibling, workspace); err != nil {
		t.Errorf("expected %q to be allowed, got %v", sibling, err)
	}
}

// A symlink planted in the checkout cannot carry a hook out of it on paper.
func TestResolveHookPathFollowsASymlinkIntoTheCheckout(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "work")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "filter")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "innocent")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := resolveHookPath(link, workspace); err == nil {
		t.Error("expected a symlink into the checkout to be refused")
	}
}

// Local runs have no GITHUB_WORKSPACE, so an absolute path stands on its own.
func TestResolveHookPathWithoutAWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "filter")
	got, err := resolveHookPath(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Clean(path) {
		t.Errorf("expected %q, got %q", path, got)
	}
}
