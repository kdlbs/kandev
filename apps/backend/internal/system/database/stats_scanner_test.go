package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/maintenance"
)

func TestScanLogicalStorageStatsReadsAllKeysetBatches(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	seedLogicalStatsTables(t, svc)

	got, err := scanLogicalStorageStats(context.Background(), svc.pool, 1, time.Second, 0, nil)
	if err != nil {
		t.Fatalf("scanLogicalStorageStats: %v", err)
	}
	want := logicalStorageStats{messageContent: 6, messageMetadata: 6, messagePayload: 5, gitSnapshot: 9}
	if got != want {
		t.Fatalf("logical stats = %#v, want %#v", got, want)
	}
}

func TestLogicalStatsCacheLeavesColdSnapshotUnavailableWhenRequiredTableIsMissing(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	cache := newLogicalStatsCache(context.Background(), svc.scanLogicalStats, logicalStatsCacheOptions{})
	t.Cleanup(cache.Close)

	if pending := cache.Read(); pending.state != logicalStatsStatePending {
		t.Fatalf("cold read = %#v, want pending while the scan starts", pending)
	}
	waitLogicalStatsCache(t, cache)
	unavailable := cache.Read()
	if unavailable.state != "unavailable" || unavailable.snapshot != nil || unavailable.error != logicalStatsErrorScanFailed {
		t.Fatalf("read after missing-table scan = %#v, want unavailable without a snapshot", unavailable)
	}
}

func TestLogicalStatsCachePreservesCompleteSnapshotWhenRequiredTableDisappears(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	for _, statement := range []string{
		`CREATE TABLE task_session_messages (id TEXT PRIMARY KEY, content TEXT, metadata TEXT)`,
		`CREATE TABLE task_message_payloads (digest TEXT PRIMARY KEY, compressed_content BLOB)`,
		`CREATE TABLE task_session_git_snapshots (id TEXT PRIMARY KEY, files TEXT, metadata TEXT)`,
		`INSERT INTO task_session_messages VALUES ('m-a', 'x', '{}')`,
		`INSERT INTO task_message_payloads VALUES ('p-a', x'0102')`,
		`INSERT INTO task_session_git_snapshots VALUES ('g-a', 'a', '{}')`,
	} {
		if _, err := svc.pool.Writer().Exec(statement); err != nil {
			t.Fatalf("seed scanner fixture with %q: %v", statement, err)
		}
	}
	cache := newLogicalStatsCache(context.Background(), svc.scanLogicalStats, logicalStatsCacheOptions{})
	t.Cleanup(cache.Close)
	cache.Read()
	waitLogicalStatsCache(t, cache)
	ready := cache.Read()
	want := logicalStorageStats{messageContent: 1, messageMetadata: 2, messagePayload: 2, gitSnapshot: 3}
	if ready.state != logicalStatsStateReady || ready.snapshot == nil || ready.snapshot.values != want {
		t.Fatalf("initial read = %#v, want complete ready snapshot %#v", ready, want)
	}

	if _, err := svc.pool.Writer().Exec(`DROP TABLE task_session_git_snapshots`); err != nil {
		t.Fatalf("drop required table: %v", err)
	}
	cache.Invalidate(false)
	refreshing := cache.Read()
	if refreshing.state != logicalStatsStateRefreshing || refreshing.snapshot == nil || refreshing.snapshot.values != want {
		t.Fatalf("read during missing-table refresh = %#v, want prior complete values", refreshing)
	}
	waitLogicalStatsCache(t, cache)
	stale := cache.Read()
	if stale.state != logicalStatsStateStale || stale.snapshot == nil || stale.snapshot.values != want ||
		!stale.snapshot.measuredAt.Equal(ready.snapshot.measuredAt) || stale.error != logicalStatsErrorScanFailed {
		t.Fatalf("read after missing-table refresh = %#v, want unchanged last-good snapshot marked stale", stale)
	}
}

func TestScanLogicalStorageStatsDefersDuringMaintenance(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	release, ok := maintenance.ForPool(svc.pool).TryAcquire()
	if !ok {
		t.Fatal("could not acquire maintenance admission for test")
	}
	defer release()
	_, err := scanLogicalStorageStats(context.Background(), svc.pool, 1, time.Second, 0, nil)
	if !errors.Is(err, errLogicalStatsDeferred) {
		t.Fatalf("scan error = %v, want deferred during maintenance", err)
	}
}

func TestScanLogicalStorageStatsObservesCancellation(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := scanLogicalStorageStats(ctx, svc.pool, 1, time.Second, 0, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("scan error = %v, want context canceled", err)
	}
}

func TestDatabaseMetadataUnavailableWithoutLastGoodSnapshot(t *testing.T) {
	svc := NewService(newFakePostgresStatsPool(t), filepath.Join(t.TempDir(), "kandev.db"), ResetDirs{}, nil, nil)
	svc.SetPersistenceHealthProbe(func() bool { return false })
	_, _, _, err := svc.readMetadata(context.Background())
	if err == nil || err.Error() != "database metadata unavailable" {
		t.Fatalf("metadata error = %v, want unavailable", err)
	}
}

func seedLogicalStatsTables(t *testing.T, svc *Service) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE task_session_messages (id TEXT PRIMARY KEY, content TEXT, metadata TEXT)`,
		`CREATE TABLE task_message_payloads (digest TEXT PRIMARY KEY, compressed_content BLOB)`,
		`CREATE TABLE task_session_git_snapshots (id TEXT PRIMARY KEY, files TEXT, metadata TEXT)`,
		`INSERT INTO task_session_messages VALUES ('m-a', 'x', '{}'), ('m-b', 'yy', 'null'), ('m-c', 'zzz', '')`,
		`INSERT INTO task_message_payloads VALUES ('p-a', x'0102'), ('p-b', x'030405')`,
		`INSERT INTO task_session_git_snapshots VALUES ('g-a', 'a', '{}'), ('g-b', 'bb', 'null')`,
	} {
		if _, err := svc.pool.Writer().Exec(statement); err != nil {
			t.Fatalf("seed scanner fixture with %q: %v", statement, err)
		}
	}
}
