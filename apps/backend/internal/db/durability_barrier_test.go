package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openBarrierTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := OpenSQLite(filepath.Join(t.TempDir(), "barrier.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)
	return database
}

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1
func TestDurabilityBarrierCommitsWithFull(t *testing.T) {
	ctx := t.Context()
	database := openBarrierTestDB(t)
	barrier := newDurabilityBarrier(database, 0)
	var observed int
	barrier.observe = func(ctx context.Context, conn *sql.Conn) error {
		return conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&observed)
	}
	var before string
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT value FROM kandev_meta WHERE key = 'durability_barrier'), '')`).Scan(&before))

	require.NoError(t, barrier.Await(ctx))

	require.Equal(t, 2, observed, "the barrier commit runs at synchronous=FULL")
	var after int
	require.NoError(t, database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&after))
	require.Equal(t, 1, after, "the writer connection returns to NORMAL")
	var generation string
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT value FROM kandev_meta WHERE key = 'durability_barrier'`).Scan(&generation))
	require.NotEmpty(t, generation)
	require.NotEqual(t, before, generation)
}

func TestDurabilityBarrierCoalescesWaiters(t *testing.T) {
	ctx := t.Context()
	database := openBarrierTestDB(t)
	barrier := newDurabilityBarrier(database, 40*time.Millisecond)

	var mu sync.Mutex
	runs := 0
	started := make(chan struct{})
	release := make(chan struct{})
	barrier.observe = func(context.Context, *sql.Conn) error {
		mu.Lock()
		runs++
		current := runs
		mu.Unlock()
		if current == 1 {
			close(started)
			<-release
		}
		return nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 9)
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- barrier.Await(ctx)
		}()
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("barrier run did not start")
	}

	arrived := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(arrived)
		errs <- barrier.Await(ctx)
	}()
	<-arrived
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, runs, "waiters that arrive during a run wait for the next one")
}

func TestDurabilityBarrierFailureDiscardsConnection(t *testing.T) {
	ctx := t.Context()
	database := openBarrierTestDB(t)
	_, err := database.Exec(`DROP TABLE kandev_meta`)
	require.NoError(t, err)
	barrier := newDurabilityBarrier(database, 0)
	require.Error(t, barrier.Await(ctx))

	var level int
	require.NoError(t, database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&level))
	require.Equal(t, 1, level, "a failed run must not leave its connection at FULL")

	_, err = database.Exec(`CREATE TABLE kandev_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)
	require.NoError(t, barrier.Await(ctx))
	require.NoError(t, database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&level))
	require.Equal(t, 1, level)
}
