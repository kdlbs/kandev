package shared

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

// seedLogFiles writes the given debug-log files with mtimes staggered one
// second apart (oldest first) so retention's mtime sort is deterministic.
func seedLogFiles(t *testing.T, dir string, names []string) {
	t.Helper()
	for i, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		mt := time.Now().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	return len(entries)
}

// TestACPPrune_KeepsNewestN mirrors the persistence.pruneBackups test: only the
// newest N files survive, oldest-by-mtime deleted first.
func TestACPPrune_KeepsNewestN(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFiles: 2, maxFileBytes: 1 << 20, ringSize: 4})

	names := []string{
		"normalized-acp-x-s1.jsonl", // oldest
		"normalized-acp-x-s2.jsonl",
		"raw-acp-x-s3.jsonl", // newest
	}
	seedLogFiles(t, dir, names)

	m.prune(time.Now())

	if got := countFiles(t, dir); got != 2 {
		t.Fatalf("expected 2 files after prune, got %d", got)
	}
	if _, err := os.Stat(filepath.Join(dir, names[0])); !os.IsNotExist(err) {
		t.Errorf("oldest file %q should have been pruned", names[0])
	}
}

// TestACPPrune_AgeCap deletes files older than the retention window and leaves
// non-log files untouched.
func TestACPPrune_AgeCap(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFiles: 100, retention: time.Hour, maxFileBytes: 1 << 20, ringSize: 4})

	fresh := filepath.Join(dir, "normalized-acp-x-fresh.jsonl")
	stale := filepath.Join(dir, "normalized-acp-x-stale.jsonl")
	other := filepath.Join(dir, "keepme.txt")
	for _, p := range []string{fresh, stale, other} {
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	m.prune(time.Now())

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file should have been pruned by age")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file should survive: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("non-log file should be left untouched: %v", err)
	}
}

// TestACPPrune_SkipsActiveWriter verifies retention won't delete a file whose
// session is still being written, even when buffering leaves the on-disk mtime
// looking stale (the retain tick racing ahead of the flush tick).
func TestACPPrune_SkipsActiveWriter(t *testing.T) {
	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFiles: 100, retention: time.Hour, maxFileBytes: 1 << 20, ringSize: 4})
	defer m.closeAll()

	// Buffered write: opens a live writer (fresh lastWrite); data isn't flushed.
	m.writeNormalized(ProtocolACP, "acp", "sess", testEvent("sess"))
	path := filepath.Join(dir, "normalized-acp-acp-sess.jsonl")
	// Force a stale on-disk mtime to simulate the unflushed-but-recent case.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	m.prune(time.Now())

	if _, err := os.Stat(path); err != nil {
		t.Errorf("active-writer file pruned despite recent buffered write: %v", err)
	}
}

// TestJanitor_StartStop verifies the background loop's retention ticker fires
// (pruning files seeded after Start) and that Stop drains the goroutine
// cleanly — leak detection comes from goleak in TestMain. Uses synctest so the
// tickers advance instantly without real sleeps.
func TestJanitor_StartStop(t *testing.T) {
	prev := debugMode
	debugMode = true
	defer func() { debugMode = prev }()

	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		m := newACPLogManager(acpLogConfig{dir: dir, maxFiles: 1, maxFileBytes: 1 << 20, ringSize: 4})
		j := newJanitor(m, 5*time.Millisecond, 20*time.Millisecond, time.Minute)

		j.Start(context.Background())
		// Seed AFTER Start so only the background retain tick can prune them.
		seedLogFiles(t, dir, []string{
			"normalized-acp-x-a.jsonl",
			"normalized-acp-x-b.jsonl",
			"raw-acp-x-c.jsonl",
		})
		// Advance fake time well past several retain ticks.
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		if got := countFiles(t, dir); got != 1 {
			t.Errorf("expected janitor to prune to 1 file, got %d", got)
		}
		j.Stop()
	})
}

// TestJanitor_StartStopIdempotent ensures double Start/Stop is safe.
func TestJanitor_StartStopIdempotent(t *testing.T) {
	prev := debugMode
	debugMode = true
	defer func() { debugMode = prev }()

	dir := t.TempDir()
	m := newACPLogManager(acpLogConfig{dir: dir, maxFiles: 10, maxFileBytes: 1 << 20, ringSize: 4})
	j := newJanitor(m, time.Hour, time.Hour, time.Hour)
	j.Start(context.Background())
	j.Start(context.Background()) // no-op
	j.Stop()
	j.Stop() // no-op

	j.Start(context.Background())
	j.Stop()
}
