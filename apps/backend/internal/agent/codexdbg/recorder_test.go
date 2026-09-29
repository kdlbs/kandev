package codexdbg

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/pkg/codexappserver"
)

func TestCaptureIsExclusivePrivateAndChronological(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	recorder, err := NewRecorder(path, codexappserver.SupportedCodexVersion)
	if err != nil {
		t.Fatalf("create capture: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat capture: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("capture permissions = %04o, want 0600", info.Mode().Perm())
	}
	if _, err := NewRecorder(path, codexappserver.SupportedCodexVersion); err == nil {
		t.Fatal("capture creation overwrote an existing file")
	}

	if err := recorder.RecordFrame(codexappserver.FrameSent, json.RawMessage(`{"jsonrpc":"2.0","id":41,"method":"model/list"}`)); err != nil {
		t.Fatalf("record request: %v", err)
	}
	if err := recorder.RecordFrame(codexappserver.FrameReceived, json.RawMessage(`{"jsonrpc":"2.0","id":41,"result":{"data":[]}}`)); err != nil {
		t.Fatalf("record response: %v", err)
	}
	if err := recorder.Close("completed"); err != nil {
		t.Fatalf("close capture: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open capture: %v", err)
	}
	defer func() { _ = file.Close() }()
	var entries []CaptureEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry CaptureEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode capture entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read capture: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("capture entries = %d, want start, request, response, close", len(entries))
	}
	for index, entry := range entries {
		if entry.Sequence != uint64(index+1) {
			t.Errorf("entry %d sequence = %d", index, entry.Sequence)
		}
	}
	if entries[1].Direction != "sent" || entries[1].Method != "model/list" {
		t.Errorf("request capture = %#v", entries[1])
	}
	if entries[2].Direction != "received" || entries[2].ResponseTo != "model/list" || string(entries[2].RequestID) != "41" {
		t.Errorf("response correlation = %#v", entries[2])
	}
	if entries[3].Meta["reason"] != "completed" {
		t.Errorf("close reason = %#v", entries[3].Meta["reason"])
	}
}

func TestCaptureDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "capture.jsonl")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := NewRecorder(path, codexappserver.SupportedCodexVersion); err == nil {
		t.Fatal("capture creation followed an existing symlink")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "keep" {
		t.Fatalf("symlink target changed to %q", strings.TrimSpace(string(content)))
	}
}

func TestCaptureLimitMarksTruncationAndPreservesCloseReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded.jsonl")
	recorder, err := NewRecorderWithLimit(path, "codex-cli test", 2048)
	if err != nil {
		t.Fatalf("create bounded recorder: %v", err)
	}
	frame := json.RawMessage(`{"jsonrpc":"2.0","method":"test/event","params":{"value":"` + strings.Repeat("x", 1800) + `"}}`)
	if err := recorder.RecordFrame("received", frame); !errors.Is(err, ErrCaptureLimit) {
		t.Fatalf("RecordFrame error = %v, want ErrCaptureLimit", err)
	}
	if err := recorder.Close("timeout"); err != nil {
		t.Fatalf("close bounded capture: %v", err)
	}
	entries, err := ReadCapture(path)
	if err != nil {
		t.Fatalf("read bounded capture: %v", err)
	}
	if entries[len(entries)-1].Event != captureEventClose || entries[len(entries)-1].Meta["reason"] != "timeout" {
		t.Fatalf("final capture entry = %#v", entries[len(entries)-1])
	}
	truncated := false
	for _, entry := range entries {
		truncated = truncated || entry.Event == "capture_truncated"
	}
	if !truncated {
		t.Fatal("capture has no truncation marker")
	}
}
