package hook

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func sampleRequest() Request {
	return Request{
		Base: "basesha",
		Head: "headsha",
		Ref:  "approvalsha",
		Files: []File{
			{Name: "a.go", HeadHunks: []Hunk{{Body: "+one\n"}, {Body: "+two\n"}}},
			{Name: "b.go", HeadHunks: []Hunk{{Body: "+three\n"}}},
		},
	}
}

func TestIndexesAcceptsWhatWasSent(t *testing.T) {
	req := sampleRequest()
	res := Response{Reviewed: []ReviewedFile{
		{Name: "a.go", Indexes: []int{1, 1, 0}},
		{Name: "b.go", Indexes: []int{}},
	}}

	reviewed, err := res.Indexes(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(reviewed["a.go"], []int{1, 0}) {
		t.Errorf("expected a repeated index to collapse and keep its order, got %v", reviewed["a.go"])
	}
	if len(reviewed["b.go"]) != 0 {
		t.Errorf("expected b.go to name nothing, got %v", reviewed["b.go"])
	}
}

func TestIndexesRejectsAnswersOutsideTheRequest(t *testing.T) {
	tt := []struct {
		name string
		res  Response
	}{
		{"file never sent", Response{Reviewed: []ReviewedFile{{Name: "invented.go", Indexes: []int{0}}}}},
		{"index past the end", Response{Reviewed: []ReviewedFile{{Name: "b.go", Indexes: []int{1}}}}},
		{"negative index", Response{Reviewed: []ReviewedFile{{Name: "a.go", Indexes: []int{-1}}}}},
		{"file named twice", Response{Reviewed: []ReviewedFile{
			{Name: "a.go", Indexes: []int{0}},
			{Name: "a.go", Indexes: []int{1}},
		}}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.res.Indexes(sampleRequest()); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func writeHook(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell hooks are not portable to windows")
	}
	path := filepath.Join(t.TempDir(), "hook")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("writing hook: %v", err)
	}
	return path
}

func TestRunReadsTheResponse(t *testing.T) {
	path := writeHook(t, `cat >&2
echo '{"reviewed":[{"name":"a.go","indexes":[0]}]}'
`)
	var stderr strings.Builder
	res, err := Run(context.Background(), path, sampleRequest(), &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Reviewed) != 1 || res.Reviewed[0].Name != "a.go" {
		t.Fatalf("unexpected response: %+v", res.Reviewed)
	}

	sent := stderr.String()
	if !strings.Contains(sent, `"version":1`) {
		t.Errorf("expected the contract version to be sent, got %s", sent)
	}
	if !strings.Contains(sent, `"ref":"approvalsha"`) {
		t.Errorf("expected the approval ref to be sent, got %s", sent)
	}
	if !strings.Contains(sent, `"approval_hunks"`) {
		t.Errorf("expected the approval-time hunks to be sent, got %s", sent)
	}
}

func TestRunFailsLoudly(t *testing.T) {
	tt := []struct {
		name string
		body string
	}{
		{"non-zero exit", "exit 1\n"},
		{"no output", "true\n"},
		{"not json", "echo not json\n"},
		{"unknown field", `echo '{"reviewed":[],"verdict":"yes"}'` + "\n"},
		{"two values", `echo '{"reviewed":[]} {"reviewed":[]}'` + "\n"},
		{"trailing brace", `echo '{"reviewed":[]}}'` + "\n"},
		{"trailing bracket", `echo '{"reviewed":[]}]'` + "\n"},
		{"trailing junk", `echo '{"reviewed":[]} nope'` + "\n"},
		{"over the size limit", "head -c 9000000 /dev/zero | tr '\\0' 'x'\n"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			path := writeHook(t, tc.body)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := Run(ctx, path, sampleRequest(), io.Discard); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestRunRejectsARelativePath(t *testing.T) {
	for _, path := range []string{"filter", "./tools/filter", "tools/filter"} {
		if _, err := Run(context.Background(), path, sampleRequest(), io.Discard); err == nil {
			t.Errorf("expected %q to be rejected", path)
		}
	}
}

func TestRunDoesNotHandTheHookTheActionInputs(t *testing.T) {
	path := writeHook(t, `env >&2
echo '{"reviewed":[]}'
`)
	t.Setenv("INPUT_GITHUB-TOKEN", "ghs_secretvalue")
	t.Setenv("INPUT_PR", "1")
	t.Setenv("KEPT_FOR_THE_HOOK", "yes")

	var stderr strings.Builder
	if _, err := Run(context.Background(), path, sampleRequest(), &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stderr.String(), "ghs_secretvalue") {
		t.Error("the hook must not receive the action's token")
	}
	if strings.Contains(stderr.String(), "INPUT_PR") {
		t.Error("the hook must not receive the action's inputs")
	}
	if !strings.Contains(stderr.String(), "KEPT_FOR_THE_HOOK") {
		t.Error("the rest of the environment should still reach the hook")
	}
}

func TestRunBoundsHookStderr(t *testing.T) {
	path := writeHook(t, `head -c 2000000 /dev/zero | tr '\0' 'e' >&2
echo '{"reviewed":[]}'
`)
	var stderr strings.Builder
	if _, err := Run(context.Background(), path, sampleRequest(), &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > maxStderrBytes {
		t.Errorf("expected stderr capped at %d bytes, got %d", maxStderrBytes, stderr.Len())
	}
}

func TestRunMissingHookIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if _, err := Run(context.Background(), missing, sampleRequest(), io.Discard); err == nil {
		t.Error("expected an error for a hook that does not exist")
	}
}

func TestRunHonoursTheDeadline(t *testing.T) {
	path := writeHook(t, "sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Run(ctx, path, sampleRequest(), io.Discard)
	if err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("expected the deadline to cut the hook off, took %s", elapsed)
	}
	if !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("expected the deadline to be named as the cause, got %v", err)
	}
}
