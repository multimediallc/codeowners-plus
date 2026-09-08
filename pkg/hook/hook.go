// Package hook asks an external program which post-approval hunks are already reviewed (see "Hunk Filters" in the README).
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RequestVersion is bumped when Request changes shape in a way a hook could misread.
const RequestVersion = 1

const maxResponseBytes = 8 << 20

// Killing a hook does not close pipes its own children inherited, so Wait would
// block on a surviving grandchild for as long as it lived.
const ioGrace = 2 * time.Second

// maxStderrBytes bounds what a hook can add to the run log.
const maxStderrBytes = 256 << 10

// cappedWriter forwards at most limit bytes and drops the rest, remembering that
// it had to. Every write is reported as complete so the hook never sees a short
// write it cannot act on.
type cappedWriter struct {
	to       io.Writer
	limit    int
	written  int
	overflow bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	room := c.limit - c.written
	if len(p) > room {
		c.overflow = true
	} else {
		room = len(p)
	}
	if room > 0 {
		n, err := c.to.Write(p[:room])
		c.written += n
		if err != nil {
			return len(p), nil
		}
	}
	return len(p), nil
}

func withoutActionInputs(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "INPUT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

type Hunk struct {
	Body string `json:"body"`
}

type File struct {
	Name          string `json:"name"`
	HeadHunks     []Hunk `json:"head_hunks"`
	ApprovalHunks []Hunk `json:"approval_hunks"`
}

type Request struct {
	Version int    `json:"version"`
	Base    string `json:"base"`
	Head    string `json:"head"`
	Ref     string `json:"ref"`
	Files   []File `json:"files"`
}

// ReviewedFile names hunks by index into the same file's head_hunks in the request.
type ReviewedFile struct {
	Name    string `json:"name"`
	Indexes []int  `json:"indexes"`
}

// Anything not named in a Response stays in the diff.
type Response struct {
	Reviewed []ReviewedFile `json:"reviewed"`
}

// Indexes validates a response against the request it answers; an unsent name or out-of-range index fails the whole answer rather than being skipped.
func (r Response) Indexes(req Request) (map[string][]int, error) {
	sent := make(map[string]int, len(req.Files))
	for _, file := range req.Files {
		sent[file.Name] = len(file.HeadHunks)
	}

	reviewed := make(map[string][]int, len(r.Reviewed))
	for _, file := range r.Reviewed {
		count, ok := sent[file.Name]
		if !ok {
			return nil, fmt.Errorf("hook named file %q, which was not sent", file.Name)
		}
		if _, seen := reviewed[file.Name]; seen {
			return nil, fmt.Errorf("hook named file %q twice", file.Name)
		}
		seenIndex := make(map[int]bool, len(file.Indexes))
		indexes := make([]int, 0, len(file.Indexes))
		for _, index := range file.Indexes {
			if index < 0 || index >= count {
				return nil, fmt.Errorf("hook named index %d for %q, which has %d hunks", index, file.Name, count)
			}
			if seenIndex[index] {
				continue
			}
			seenIndex[index] = true
			indexes = append(indexes, index)
		}
		reviewed[file.Name] = indexes
	}
	return reviewed, nil
}

// Decoder.More reports false on a stray closing delimiter, so it cannot tell a
// clean end of stream from trailing junk; only another Decode can.
func atEndOfStream(d *json.Decoder) bool {
	var trailing json.RawMessage
	return d.Decode(&trailing) == io.EOF
}

// Run writes req to the hook's stdin and decodes a Response from its stdout; the path is executed directly, never through a shell.
func Run(ctx context.Context, path string, req Request, stderr io.Writer) (Response, error) {
	if !filepath.IsAbs(path) {
		return Response{}, fmt.Errorf("hook path %q must be absolute", path)
	}

	req.Version = RequestVersion

	input, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("encoding hook request: %w", err)
	}

	var body bytes.Buffer
	stdout := &cappedWriter{to: &body, limit: maxResponseBytes}
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = &cappedWriter{to: stderr, limit: maxStderrBytes}
	cmd.Env = withoutActionInputs(os.Environ())
	cmd.WaitDelay = ioGrace

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Response{}, fmt.Errorf("hook %s did not finish: %w", path, ctxErr)
		}
		return Response{}, fmt.Errorf("hook %s failed: %w", path, err)
	}
	if stdout.overflow {
		return Response{}, fmt.Errorf("hook %s wrote more than the %d byte limit", path, maxResponseBytes)
	}

	decoder := json.NewDecoder(&body)
	// An unknown field means the hook may believe it answered when it did not.
	decoder.DisallowUnknownFields()

	var res Response
	if err := decoder.Decode(&res); err != nil {
		return Response{}, fmt.Errorf("decoding hook response: %w", err)
	}
	if !atEndOfStream(decoder) {
		return Response{}, fmt.Errorf("hook %s wrote more than one JSON value", path)
	}
	return res, nil
}
