package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func testEvent(sessionID string) *streams.AgentEvent {
	return &streams.AgentEvent{Type: streams.EventTypeMessageChunk, SessionID: sessionID, Text: "hello"}
}

// TestACPLog_PerSessionRouting verifies two sessions land in two distinct
// files, each filename carrying its own session id, and keeping the
// normalized-/raw- prefix + .jsonl suffix the debug reader requires.
func TestACPLog_PerSessionRouting(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 8})
	defer m.closeAll()

	m.writeNormalized(ProtocolACP, "claude-acp", "sess-A", testEvent("sess-A"))
	m.writeNormalized(ProtocolACP, "claude-acp", "sess-B", testEvent("sess-B"))
	m.writeRaw(ProtocolACP, "claude-acp", "sess-A", "session_notification", json.RawMessage(`{"a":1}`))
	m.flushAll()

	want := []string{
		"normalized-acp-claude-acp-sess-A.jsonl",
		"normalized-acp-claude-acp-sess-B.jsonl",
		"raw-acp-claude-acp-sess-A.jsonl",
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected file %s: %v", name, err)
		}
		if !strings.HasSuffix(name, jsonlSuffix) {
			t.Errorf("file %s missing .jsonl suffix", name)
		}
	}
}

// TestACPLog_SanitizesSessionID ensures path-unsafe characters in a session
// id can't escape the log dir or break Windows filenames.
func TestACPLog_SanitizesSessionID(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 8})
	defer m.closeAll()

	m.writeNormalized(ProtocolACP, "claude-acp", "../../etc/passwd:bad", testEvent("x"))
	m.flushAll()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 file in dir, got %d", len(entries))
	}
	name := entries[0].Name()
	if strings.ContainsAny(name, `/\:`) {
		t.Errorf("filename %q contains path-unsafe characters", name)
	}
}

func TestACPLog_EnforcesPrivateDirPerm(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod broad dir: %v", err)
	}
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 8})
	defer m.closeAll()

	m.writeNormalized(ProtocolACP, "claude-acp", "sess", testEvent("sess"))

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat log dir: %v", err)
	}
	if got := info.Mode().Perm(); got != acpDirPerm {
		t.Fatalf("log dir perms = %o, want %o", got, acpDirPerm)
	}
}

// TestACPLog_Rotation verifies a chatty session rolls its file once it passes
// the byte cap, capping a single file's size while preserving prior data in a
// rotated sibling.
func TestACPLog_Rotation(t *testing.T) {
	dir := t.TempDir()
	// Tiny cap so a handful of lines triggers a roll.
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 200, ringSize: 8})
	defer m.closeAll()

	big := strings.Repeat("x", 80)
	for range 10 {
		m.writeNormalized(ProtocolACP, "acp", "sess", &streams.AgentEvent{Type: streams.EventTypeMessageChunk, SessionID: "sess", Text: big})
	}
	m.flushAll()

	active := filepath.Join(dir, "normalized-acp-acp-sess.jsonl")
	info, err := os.Stat(active)
	if err != nil {
		t.Fatalf("stat active: %v", err)
	}
	if info.Size() > 200+128 {
		t.Errorf("active file not capped: %d bytes", info.Size())
	}
	// A rotated sibling must exist and still match the reader's prefix/suffix.
	rotated, _ := filepath.Glob(filepath.Join(dir, "normalized-acp-acp-sess.*.jsonl"))
	if len(rotated) == 0 {
		t.Errorf("expected a rotated sibling file, found none")
	}
}

// TestACPLog_RingTail returns the most recent normalized events for a session
// without touching disk.
func TestACPLog_RingTail(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 3})
	defer m.closeAll()

	for i := range 5 {
		m.writeNormalized(ProtocolACP, "acp", "sess", &streams.AgentEvent{
			Type: streams.EventTypeMessageChunk, SessionID: "sess", Text: string(rune('a' + i)),
		})
	}
	tail := m.ringTail("sess", 10)
	if len(tail) != 3 {
		t.Fatalf("ring should cap at 3, got %d", len(tail))
	}
	// Oldest two (a,b) evicted; newest is 'e'. Assert the specific text field
	// so the check can't pass on an incidental 'e' in some JSON key.
	last := string(tail[len(tail)-1])
	if !strings.Contains(last, `"text":"e"`) {
		t.Errorf("expected newest entry text 'e', got %s", last)
	}
	if got := m.ringTail("nope", 10); got != nil {
		t.Errorf("unknown session should return nil tail, got %v", got)
	}
}

// TestACPLog_RotationDoesNotClobberExisting verifies rotation picks a free
// sibling name instead of reusing ".1.jsonl" (seq resets per writer), so a
// rotated segment from a previous run survives.
func TestACPLog_RotationDoesNotClobberExisting(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 200, ringSize: 4})
	defer m.closeAll()

	// Pre-create the .1 rotated sibling as if from a previous process run.
	pre := filepath.Join(dir, "normalized-acp-acp-sess.1.jsonl")
	if err := os.WriteFile(pre, []byte("SENTINEL\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	big := strings.Repeat("x", 80)
	for range 6 {
		m.writeNormalized(ProtocolACP, "acp", "sess", &streams.AgentEvent{
			Type: streams.EventTypeMessageChunk, SessionID: "sess", Text: big,
		})
	}
	m.flushAll()

	got, err := os.ReadFile(pre)
	if err != nil || string(got) != "SENTINEL\n" {
		t.Errorf("pre-existing rotated file clobbered: got %q err %v", string(got), err)
	}
	rotated, _ := filepath.Glob(filepath.Join(dir, "normalized-acp-acp-sess.*.jsonl"))
	if len(rotated) < 2 {
		t.Errorf("expected >=2 rotated siblings (pre-existing + new), got %d: %v", len(rotated), rotated)
	}
}

// TestACPLog_RingEviction confirms idle ring buffers are dropped so memory
// doesn't accumulate one per historical session.
func TestACPLog_RingEviction(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 4})
	defer m.closeAll()

	m.writeNormalized(ProtocolACP, "acp", "sess", testEvent("sess"))
	if m.ringTail("sess", 10) == nil {
		t.Fatal("expected ring tail before eviction")
	}
	r := m.ring("sess")
	r.mu.Lock()
	r.lastWrite = time.Now().Add(-time.Hour)
	r.mu.Unlock()

	m.closeIdle(time.Minute)

	if got := m.ringTail("sess", 10); got != nil {
		t.Errorf("expected ring evicted after idle sweep, got %d entries", len(got))
	}
}

func TestACPLog_CloseAllClearsRings(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 4})

	m.writeNormalized(ProtocolACP, "acp", "sess", testEvent("sess"))
	if m.ringTail("sess", 10) == nil {
		t.Fatal("expected ring tail before closeAll")
	}

	m.closeAll()

	if got := m.ringTail("sess", 10); got != nil {
		t.Errorf("expected closeAll to clear ring buffers, got %d entries", len(got))
	}
}

// TestACPRingTail_GlobalWrapper exercises the exported ACPRingTail over the
// process-wide registry: n cap, n<=0 returns all, unknown session is nil, and
// disabled mode records nothing.
func TestACPRingTail_GlobalWrapper(t *testing.T) {
	prevMgr, prevDebug := acpLog, debugMode
	mgr := newACPLogManager(acpLogConfig{dir: t.TempDir(), maxFileBytes: 1 << 20, ringSize: 8})
	acpLog = mgr
	debugMode = true
	// Close the temporary manager's writers (not just restore the globals) so
	// no file handles leak.
	t.Cleanup(func() { mgr.closeAll(); acpLog = prevMgr; debugMode = prevDebug })

	for range 3 {
		LogNormalizedEvent(ProtocolACP, "acp", "sess", testEvent("sess"))
	}
	if got := ACPRingTail("sess", 2); len(got) != 2 {
		t.Errorf("expected n=2 cap, got %d", len(got))
	}
	if got := ACPRingTail("sess", 0); len(got) != 3 {
		t.Errorf("expected all 3 with n<=0, got %d", len(got))
	}
	if got := ACPRingTail("unknown", 10); got != nil {
		t.Errorf("unknown session should be nil, got %v", got)
	}

	debugMode = false
	LogNormalizedEvent(ProtocolACP, "acp", "sess2", testEvent("sess2"))
	if ACPRingTail("sess2", 10) != nil {
		t.Errorf("disabled mode should not record events")
	}
}

// TestACPLog_ConcurrentSessions exercises many goroutines writing distinct
// sessions concurrently; run with -race.
func TestACPLog_ConcurrentSessions(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFileBytes: 1 << 20, ringSize: 16})
	defer m.closeAll()

	var wg sync.WaitGroup
	for s := range 16 {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			sess := "sess-" + string(rune('A'+s))
			for range 50 {
				m.writeNormalized(ProtocolACP, "acp", sess, testEvent(sess))
				m.writeRaw(ProtocolACP, "acp", sess, "n", json.RawMessage(`{}`))
			}
		}(s)
	}
	wg.Wait()
	m.flushAll()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	// 16 sessions × {normalized,raw} = 32 files.
	if len(entries) != 32 {
		t.Errorf("expected 32 files, got %d", len(entries))
	}
}
