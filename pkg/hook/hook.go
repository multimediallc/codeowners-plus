// Package hook asks an external program which post-approval hunks are already reviewed (see "Hunk Filters" in the README).
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// RequestVersion is bumped when Request changes shape in a way a hook could misread.
const RequestVersion = 1

const maxResponseBytes = 8 << 20

// Killing a hook does not close pipes its own children inherited, so Wait would
// block on a surviving grandchild for as long as it lived.
const ioGrace = 2 * time.Second

// limitedBuffer keeps at most limit bytes, reporting every write as complete so the hook does not die on a short write.
type limitedBuffer struct {
	buf      bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); len(p) > room {
		b.overflow = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
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

// Run writes req to the hook's stdin and decodes a Response from its stdout; the path is executed directly, never through a shell.
func Run(ctx context.Context, path string, req Request, stderr io.Writer) (Response, error) {
	req.Version = RequestVersion

	input, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("encoding hook request: %w", err)
	}

	stdout := &limitedBuffer{limit: maxResponseBytes}
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
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

	decoder := json.NewDecoder(&stdout.buf)
	// An unknown field means the hook may believe it answered when it did not.
	decoder.DisallowUnknownFields()

	var res Response
	if err := decoder.Decode(&res); err != nil {
		return Response{}, fmt.Errorf("decoding hook response: %w", err)
	}
	if decoder.More() {
		return Response{}, fmt.Errorf("hook %s wrote more than one JSON value", path)
	}
	return res, nil
}
