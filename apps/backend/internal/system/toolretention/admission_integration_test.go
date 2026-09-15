package toolretention

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3, 002.5
func TestAdmissionForegroundCommitAheadOfCleanupProtectsPayload(t *testing.T) {
	s := testService(t)
	original := seedAnalysis(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	revision := approveTestPolicy(t, s)
	_, err := s.Run(ctx, revision)
	require.NoError(t, err)
	readStarted := make(chan struct{})
	var observed sync.Once
	installAdmissionHook(t, s.pool.Reader(), func(c *sqlite3.SQLiteConn) {
		c.RegisterAuthorizer(func(op int, table, _, _ string) int {
			if op == sqlite3.SQLITE_READ && table == "settings" {
				observed.Do(func() { close(readStarted) })
			}
			return sqlite3.SQLITE_OK
		})
	})
	foreground, err := s.pool.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = foreground.Rollback() }()
	_, err = foreground.ExecContext(ctx, `UPDATE task_sessions SET state='RUNNING',updated_at='2099-01-01' WHERE id='session'`)
	require.NoError(t, err)
	cleanup := admissionCall(t, func() error { return s.stepScan(ctx) })
	awaitAdmissionBarrier(t, ctx, readStarted)
	// Cleanup has begun reading while the activity update is still uncommitted.
	require.NoError(t, foreground.Commit())
	awaitAdmissionResult(t, ctx, cleanup)
	finishAdmissionCleanup(t, ctx, s)
	require.Equal(t, original, admissionMetadata(t, s))
	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Zero(t, status.Operation.RemovedMessages)
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3, 002.5
func TestAdmissionCleanupCommitAheadOfForegroundPreservesMarker(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	revision := approveTestPolicy(t, s)
	_, err := s.Run(ctx, revision)
	require.NoError(t, err)
	written, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var observed sync.Once
	installAdmissionHook(t, s.pool.Writer(), func(c *sqlite3.SQLiteConn) {
		c.RegisterUpdateHook(func(op int, _, table string, _ int64) {
			if op == sqlite3.SQLITE_UPDATE && table == "task_session_messages" {
				observed.Do(func() {
					close(written)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}
		})
	})
	cleanup := admissionCall(t, func() error { return s.stepScan(ctx) })
	awaitAdmissionBarrier(t, ctx, written)
	foregroundStarted := make(chan struct{})
	foreground := admissionCall(t, func() error {
		close(foregroundStarted)
		return writeAdmissionActivity(ctx, s, true, 1)
	})
	awaitAdmissionBarrier(t, ctx, foregroundStarted)
	unblock()
	awaitAdmissionResult(t, ctx, cleanup)
	awaitAdmissionResult(t, ctx, foreground)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(admissionMetadata(t, s)), &metadata))
	require.Contains(t, metadata, "payload_retention")
	var state string
	require.NoError(t, s.pool.Reader().Get(&state, `SELECT state FROM task_sessions WHERE id='session'`))
	require.Equal(t, "RUNNING", state)
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.5, 003.2, 003.3
func TestAdmissionMultiBatchWritesAndExplicitCompactionEvidence(t *testing.T) {
	s := testService(t)
	original := seedAnalysis(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const messages = batchRows*3 + 7
	seedAdmissionBatches(t, ctx, s, original, messages)
	revision := approveTestPolicy(t, s)
	_, err := s.Run(ctx, revision)
	require.NoError(t, err)
	before := measureAdmissionDatabase(t, s)
	requests, results := startAdmissionBatches(t, ctx, s)
	writes, batches := 0, 0
	for batches < messages+10 {
		select {
		case requests <- struct{}{}:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		awaitAdmissionResult(t, ctx, results)
		batches++
		status, getErr := s.Get(ctx)
		require.NoError(t, getErr)
		if status.Operation.State != stateRunning {
			require.Equal(t, stateSucceeded, status.Operation.State)
			break
		}
		// The worker cannot begin the next batch until this foreground commit completes.
		foreground := admissionCall(t, func() error { return writeAdmissionActivity(ctx, s, false, writes+1) })
		awaitAdmissionResult(t, ctx, foreground)
		writes++
	}
	require.GreaterOrEqual(t, batches, 4)
	require.Equal(t, batches-1, writes)
	var updated string
	require.NoError(t, s.pool.Reader().Get(&updated, `SELECT CAST(updated_at AS TEXT) FROM task_sessions WHERE id='foreground-session'`))
	require.Equal(t, admissionActivityTime(writes), updated)
	afterCleanup := measureAdmissionDatabase(t, s)
	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, stateSucceeded, status.Operation.State)
	require.EqualValues(t, messages, status.Operation.RemovedMessages)
	require.Equal(t, before.PayloadBytes-afterCleanup.PayloadBytes, status.Operation.PayloadBytes)
	require.Greater(t, status.Operation.PayloadBytes, int64(0))
	require.Greater(t, afterCleanup.FreePages, before.FreePages)
	require.GreaterOrEqual(t, afterCleanup.FileBytes, before.FileBytes)
	_, err = s.pool.Writer().ExecContext(ctx, "VACUUM")
	require.NoError(t, err)
	afterVacuum := measureAdmissionDatabase(t, s)
	require.Less(t, afterVacuum.FileBytes, afterCleanup.FileBytes)
	require.Zero(t, afterVacuum.FreePages)
	require.Equal(t, afterCleanup.PayloadBytes, afterVacuum.PayloadBytes)
	t.Logf("batches=%d foreground_commits=%d logical_removed_bytes=%d page_size_bytes=%d freelist_pages_before=%d after_cleanup=%d after_vacuum=%d main_file_bytes_before=%d after_cleanup=%d after_vacuum=%d compacted_bytes=%d",
		batches, writes, status.Operation.PayloadBytes, before.PageSize, before.FreePages, afterCleanup.FreePages, afterVacuum.FreePages, before.FileBytes, afterCleanup.FileBytes, afterVacuum.FileBytes, afterCleanup.FileBytes-afterVacuum.FileBytes)
}

func installAdmissionHook(t *testing.T, pool *sqlx.DB, install func(*sqlite3.SQLiteConn)) {
	t.Helper()
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	conn, err := pool.Conn(context.Background())
	require.NoError(t, err)
	err = conn.Raw(func(raw any) error { install(raw.(*sqlite3.SQLiteConn)); return nil })
	closeErr := conn.Close()
	require.NoError(t, err)
	require.NoError(t, closeErr)
}

func admissionCall(t *testing.T, fn func() error) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn(); close(done) }()
	t.Cleanup(func() { <-done })
	return done
}

func awaitAdmissionBarrier(t *testing.T, ctx context.Context, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func awaitAdmissionResult(t *testing.T, ctx context.Context, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func admissionMetadata(t *testing.T, s *Service) string {
	t.Helper()
	var raw string
	require.NoError(t, s.pool.Reader().Get(&raw, `SELECT metadata FROM task_session_messages WHERE id='message'`))
	return raw
}

func finishAdmissionCleanup(t *testing.T, ctx context.Context, s *Service) {
	t.Helper()
	for i := 0; i < 10; i++ {
		status, err := s.Get(ctx)
		require.NoError(t, err)
		if status.Operation.State != stateRunning {
			require.Equal(t, stateSucceeded, status.Operation.State)
			return
		}
		require.NoError(t, s.stepScan(ctx))
	}
	t.Fatal("cleanup did not finish")
}

func writeAdmissionActivity(ctx context.Context, s *Service, checkMarker bool, sequence int) error {
	tx, err := s.pool.Writer().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	session := "foreground-session"
	if checkMarker {
		session = "session"
		var raw string
		if err := tx.GetContext(ctx, &raw, `SELECT metadata FROM task_session_messages WHERE id='message'`); err != nil {
			return err
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return err
		}
		if _, ok := metadata["payload_retention"]; !ok {
			return fmt.Errorf("foreground admitted before removal marker committed")
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE task_sessions SET state='RUNNING',updated_at=? WHERE id=?`, admissionActivityTime(sequence), session)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("foreground session update affected %d rows", changed)
	}
	return tx.Commit()
}

func admissionActivityTime(sequence int) string {
	return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(sequence) * time.Second).Format("2006-01-02 15:04:05")
}

func seedAdmissionBatches(t *testing.T, ctx context.Context, s *Service, raw string, count int) {
	t.Helper()
	tx, err := s.pool.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for i := 1; i < count; i++ {
		_, err = tx.ExecContext(ctx, `INSERT INTO task_session_messages(id,task_session_id,task_id,created_at,updated_at,type,metadata,requests_input) VALUES(?,'session','task','2020-01-01','2020-01-01','tool_execute',?,0)`, fmt.Sprintf("generated-%04d", i), raw)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks VALUES('foreground-task','IN_PROGRESS','',NULL,'2099-01-01','2099-01-01');
 INSERT INTO task_sessions VALUES('foreground-session','foreground-task','RUNNING','2099-01-01',NULL,'2099-01-01')`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func startAdmissionBatches(t *testing.T, ctx context.Context, s *Service) (chan<- struct{}, <-chan error) {
	t.Helper()
	requests, results := make(chan struct{}), make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-requests:
				if !ok {
					return
				}
				results <- s.stepScan(ctx)
			}
		}
	}()
	t.Cleanup(func() { close(requests); <-done })
	return requests, results
}

type admissionDatabaseMeasurement struct{ FileBytes, PayloadBytes, FreePages, PageSize int64 }

func measureAdmissionDatabase(t *testing.T, s *Service) admissionDatabaseMeasurement {
	t.Helper()
	var busy, frames, checkpointed int
	require.NoError(t, s.pool.Writer().QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &checkpointed))
	require.Zero(t, busy)
	var sequence int
	var schema, path string
	require.NoError(t, s.pool.Reader().QueryRow("PRAGMA database_list").Scan(&sequence, &schema, &path))
	info, err := os.Stat(path)
	require.NoError(t, err)
	result := admissionDatabaseMeasurement{FileBytes: info.Size()}
	require.NoError(t, s.pool.Reader().Get(&result.FreePages, "PRAGMA freelist_count"))
	require.NoError(t, s.pool.Reader().Get(&result.PageSize, "PRAGMA page_size"))
	require.NoError(t, s.pool.Reader().Get(&result.PayloadBytes, `SELECT COALESCE(SUM(length(CAST(metadata AS BLOB))),0) FROM task_session_messages`))
	return result
}
