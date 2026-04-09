// Package acpdbg is a headless ACP JSON-RPC debugging library used by the
// `acpdbg` CLI. It speaks raw line-delimited JSON-RPC 2.0 to an agent
// subprocess, captures every frame to a JSONL file, and never depends on the
// acp-go-sdk — so the frames recorded are authoritative wire bytes, not
// SDK-reparsed events.
package acpdbg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Frame is a single JSON-RPC 2.0 message — request, response, or notification.
// We keep it as a generic map because the whole point of this tool is to
// capture whatever the agent actually sends, including fields the SDK would
// drop or normalize.
type Frame map[string]any

// ID returns the request/response id as-is (may be nil for notifications,
// float64 for numeric ids, or string for string ids — ACP uses integers).
func (f Frame) ID() any {
	return f["id"]
}

// Method returns the method name if this is a request or notification, or "".
func (f Frame) Method() string {
	m, _ := f["method"].(string)
	return m
}

// IsResponse reports whether the frame has a result or error field (and
// therefore a matching request id).
func (f Frame) IsResponse() bool {
	if _, ok := f["result"]; ok {
		return true
	}
	if _, ok := f["error"]; ok {
		return true
	}
	return false
}

// Framer writes JSON-RPC frames to a child's stdin and reads frames from the
// child's stdout. It is NOT thread-safe for concurrent writes on the same
// stdin — callers should serialize writes (the CLI does this naturally
// because it's strictly request-response).
type Framer struct {
	stdin  io.Writer
	stdout *bufio.Scanner

	mu     sync.Mutex
	nextID int
}

// NewFramer wraps a child's stdin/stdout with a line-delimited JSON-RPC
// framer. The scanner buffer is sized for large `session/new` responses that
// can include many models and modes.
func NewFramer(stdin io.Writer, stdout io.Reader) *Framer {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // up to 10 MiB per line
	return &Framer{
		stdin:  stdin,
		stdout: sc,
		nextID: 0,
	}
}

// NextID returns a fresh JSON-RPC request id. Monotonic starting at 1.
func (f *Framer) NextID() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	return f.nextID
}

// Write serializes the given frame as a single JSON object followed by a
// newline and writes it to the child's stdin.
func (f *Framer) Write(frame Frame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	data = append(data, '\n')
	if _, err := f.stdin.Write(data); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}

// Read returns the next frame from the child's stdout. Returns io.EOF when
// the child closes stdout. Skips blank lines between frames.
func (f *Framer) Read() (Frame, error) {
	for f.stdout.Scan() {
		line := f.stdout.Bytes()
		if len(line) == 0 {
			continue
		}
		var frame Frame
		if err := json.Unmarshal(line, &frame); err != nil {
			return nil, fmt.Errorf("parse frame %q: %w", shortLine(line), err)
		}
		return frame, nil
	}
	if err := f.stdout.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// NewRequest builds a JSON-RPC request frame with the next id.
func (f *Framer) NewRequest(method string, params any) (Frame, int) {
	id := f.NextID()
	fr := Frame{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		fr["params"] = params
	}
	return fr, id
}

// NewMethodNotFound builds an error response for an incoming agent-initiated
// request whose id we don't plan to service. We still reply so the agent
// doesn't hang waiting for us.
func NewMethodNotFound(id any, method string) Frame {
	return Frame{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32601,
			"message": "method not found: " + method,
		},
	}
}

func shortLine(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
