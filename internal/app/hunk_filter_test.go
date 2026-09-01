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
	a := &App{config: &Config{HunkFilter: path, WarningBuffer: warnings, InfoBuffer: &bytes.Buffer{}}}

	reviewed, err := a.hunkFilter("basesha", "headsha")("approvalsha", filterFiles())
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
				HunkFilter:    writeFilterHook(t, tc.body),
				WarningBuffer: warnings,
				InfoBuffer:    &bytes.Buffer{},
			}}

			reviewed, err := a.hunkFilter("basesha", "headsha")("approvalsha", filterFiles())
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
		HunkFilter:    filepath.Join(t.TempDir(), "absent"),
		WarningBuffer: warnings,
		InfoBuffer:    &bytes.Buffer{},
	}}

	reviewed, err := a.hunkFilter("basesha", "headsha")("approvalsha", filterFiles())
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
