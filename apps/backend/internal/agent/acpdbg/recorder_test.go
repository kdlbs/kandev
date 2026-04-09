package acpdbg

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorder_WritesValidJSONLPerLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "run.jsonl")

	r, err := NewRecorder(path)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	if err := r.Meta("start", map[string]any{"agent": "claude-acp", "command": []string{"npx", "claude-agent-acp"}}); err != nil {
		t.Fatalf("meta start: %v", err)
	}
	if err := r.Sent(Frame{"jsonrpc": "2.0", "id": 1, "method": "initialize"}); err != nil {
		t.Fatalf("sent: %v", err)
	}
	if err := r.Received(Frame{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"protocolVersion": 1}}); err != nil {
		t.Fatalf("received: %v", err)
	}
	if err := r.Stderr("some warning on stderr"); err != nil {
		t.Fatalf("stderr: %v", err)
	}
	if err := r.Meta("close", map[string]any{"exit_code": 0}); err != nil {
		t.Fatalf("meta close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			t.Errorf("unexpected blank line in JSONL")
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad line %q: %v", line, err)
		}
		entries = append(entries, e)
	}

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Order check.
	wantDir := []Direction{DirMeta, DirSent, DirReceived, DirStderr, DirMeta}
	for i, e := range entries {
		if e.Direction != wantDir[i] {
			t.Errorf("entry %d direction = %q, want %q", i, e.Direction, wantDir[i])
		}
		if e.TS == "" {
			t.Errorf("entry %d has empty ts", i)
		}
	}

	// Timestamps must be monotonically non-decreasing.
	for i := 1; i < len(entries); i++ {
		if entries[i].TS < entries[i-1].TS {
			t.Errorf("timestamps non-monotonic: %q < %q", entries[i].TS, entries[i-1].TS)
		}
	}

	// Meta start carries its payload.
	if entries[0].Meta["agent"] != "claude-acp" {
		t.Errorf("start meta missing agent: %+v", entries[0].Meta)
	}
	// Received frame retains its full payload.
	res, ok := entries[2].Frame["result"].(map[string]any)
	if !ok {
		t.Fatalf("received frame has no result: %+v", entries[2].Frame)
	}
	if res["protocolVersion"].(float64) != 1 {
		t.Errorf("protocolVersion = %v, want 1", res["protocolVersion"])
	}
}

func TestRecorder_ConcurrentWritesAreSerialized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.jsonl")
	r, err := NewRecorder(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = r.Close() }()

	const n = 200
	done := make(chan struct{}, 2)
	go func() {
		for i := range n {
			_ = r.Sent(Frame{"id": i, "method": "test/sent"})
		}
		done <- struct{}{}
	}()
	go func() {
		for i := range n {
			_ = r.Received(Frame{"id": i, "result": map[string]any{}})
		}
		done <- struct{}{}
	}()
	<-done
	<-done

	// Every line must parse cleanly — no interleaved writes.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2*n {
		t.Errorf("expected %d lines, got %d", 2*n, len(lines))
	}
	for i, line := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("line %d not valid JSON: %q (%v)", i, line, err)
		}
	}
}
