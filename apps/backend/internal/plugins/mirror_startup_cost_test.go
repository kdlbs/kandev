package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// mirrorFixtureShape describes a populated primary-journal fixture. The two
// shapes hold the event count constant while varying the session count, so the
// measurement attributes startup mirror cost to per-session work or per-event
// work instead of inferring it.
type mirrorFixtureShape struct {
	name             string
	sessions         int
	eventsPerSession int
}

func seedPrimaryJournalFixture(t *testing.T, path string, shape mirrorFixtureShape) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL UNIQUE,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, sequence)
		);`)
	require.NoError(t, err)

	tx, err := database.Begin()
	require.NoError(t, err)
	streamStmt, err := tx.Prepare(`INSERT INTO conversation_session_streams(session_id, watermark) VALUES (?, ?)`)
	require.NoError(t, err)
	eventStmt, err := tx.Prepare(`INSERT INTO conversation_session_events(
		session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	require.NoError(t, err)
	for s := range shape.sessions {
		sessionID := fmt.Sprintf("session-%06d", s)
		taskID := fmt.Sprintf("task-%06d", s)
		_, err = streamStmt.Exec(sessionID, shape.eventsPerSession)
		require.NoError(t, err)
		for e := 1; e <= shape.eventsPerSession; e++ {
			payload := fmt.Sprintf(
				`{"type":"message.deleted","session_id":%q,"task_id":%q,"message_id":"message-%d"}`,
				sessionID, taskID, e,
			)
			_, err = eventStmt.Exec(
				sessionID, e, fmt.Sprintf("%s:%d", sessionID, e), 1, "message.deleted",
				taskID, payload, "2026-09-07T12:00:00Z",
			)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tx.Commit())
	return database
}

func newMirrorService(t *testing.T, journal *sqlx.DB, mirrorPath string) *Service {
	t.Helper()
	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(journal)
	events, err := NewSessionEventLog(mirrorPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = events.Close() })
	service.sessionEvents = events
	service.sessionDelivery = NewSessionDeliveryDispatcher(events)
	return service
}

// TestMirrorStartupCostPopulated measures the first-startup mirror sweep
// (provider.go syncAllCommittedSessionEvents) against populated fixtures. It
// records evidence for startup budgeting; it imposes no machine-specific
// duration threshold.
func TestMirrorStartupCostPopulated(t *testing.T) {
	shapes := []mirrorFixtureShape{
		{name: "many-sessions", sessions: 2945, eventsPerSession: 2},
		{name: "few-sessions", sessions: 295, eventsPerSession: 20},
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			dir := t.TempDir()
			journal := seedPrimaryJournalFixture(t, filepath.Join(dir, "journal.db"), shape)
			totalEvents := shape.sessions * shape.eventsPerSession

			// Production shape: file-backed mirror, one commit per event.
			fileService := newMirrorService(t, journal, filepath.Join(dir, "session-events.sqlite"))
			started := time.Now()
			mirrored, err := fileService.syncAllCommittedSessionEvents(context.Background())
			fileDuration := time.Since(started)
			require.NoError(t, err)
			require.Len(t, mirrored, totalEvents)

			// Same work with an in-memory mirror: persistAppendLocked is a
			// no-op, so this isolates query + scan + projection from commits.
			memService := newMirrorService(t, journal, "")
			started = time.Now()
			memMirrored, err := memService.syncAllCommittedSessionEvents(context.Background())
			memDuration := time.Since(started)
			require.NoError(t, err)
			require.Len(t, memMirrored, totalEvents)

			// Query-only: per-session seek and scan, no append at all.
			queryService := newMirrorService(t, journal, "")
			started = time.Now()
			queryOnlyMirrorSweep(t, queryService, shape.sessions)
			queryDuration := time.Since(started)

			// Warm resync against an already-populated file mirror: the
			// steady-state startup path after the first sync.
			started = time.Now()
			resynced, err := fileService.syncAllCommittedSessionEvents(context.Background())
			resyncDuration := time.Since(started)
			require.NoError(t, err)
			require.Empty(t, resynced)

			t.Logf(
				"shape=%s sessions=%d events=%d | file-mirror=%s (%.2f ms/event, %.2f ms/session) | in-memory=%s | query-only=%s | warm-resync=%s",
				shape.name, shape.sessions, totalEvents,
				fileDuration,
				float64(fileDuration.Microseconds())/1000/float64(totalEvents),
				float64(fileDuration.Microseconds())/1000/float64(shape.sessions),
				memDuration, queryDuration, resyncDuration,
			)
		})
	}
}

func queryOnlyMirrorSweep(t *testing.T, service *Service, sessions int) {
	t.Helper()
	for s := range sessions {
		sessionID := fmt.Sprintf("session-%06d", s)
		rows, err := service.conversationJournal.QueryxContext(
			context.Background(),
			`SELECT session_id, event_id, sequence, protocol_version, event_type, task_id, payload, created_at
			 FROM conversation_session_events WHERE session_id = ? AND sequence > ? ORDER BY sequence ASC`,
			sessionID, 0,
		)
		require.NoError(t, err)
		for rows.Next() {
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
}

// TestMirrorStartupCostLinearity checks that the first-sync cost stays linear
// in event count at production scale, so the small-fixture rate can be
// extrapolated, and records the commit-batching floor on the same schema.
func TestMirrorStartupCostLinearity(t *testing.T) {
	if os.Getenv("KANDEV_MIRROR_COST_LINEARITY") != "1" {
		t.Skip("set KANDEV_MIRROR_COST_LINEARITY=1 to run the ~40s production-scale fixture")
	}
	shape := mirrorFixtureShape{name: "production-scale", sessions: 2945, eventsPerSession: 34}
	dir := t.TempDir()
	journal := seedPrimaryJournalFixture(t, filepath.Join(dir, "journal.db"), shape)
	totalEvents := shape.sessions * shape.eventsPerSession

	service := newMirrorService(t, journal, filepath.Join(dir, "session-events.sqlite"))
	started := time.Now()
	mirrored, err := service.syncAllCommittedSessionEvents(context.Background())
	duration := time.Since(started)
	require.NoError(t, err)
	require.Len(t, mirrored, totalEvents)

	t.Logf("linearity: sessions=%d events=%d first-sync=%s (%.3f ms/event) extrapolated-to-600k=%s",
		shape.sessions, totalEvents, duration,
		float64(duration.Microseconds())/1000/float64(totalEvents),
		time.Duration(float64(duration)/float64(totalEvents)*600000).Round(time.Second),
	)
}

// TestMirrorCommitBatchingFloor bounds the achievable win on the durable
// mirror schema: the same rows written one transaction per event (today), one
// transaction per session, and one transaction for the whole sweep.
func TestMirrorCommitBatchingFloor(t *testing.T) {
	const (
		sessions         = 2945
		eventsPerSession = 2
	)
	for _, mode := range []struct {
		name        string
		commitEvery int // 0 means one transaction for the whole sweep
	}{
		{name: "per-event(today)", commitEvery: 1},
		{name: "per-session", commitEvery: eventsPerSession},
		{name: "per-256-events", commitEvery: 256},
		{name: "whole-sweep", commitEvery: 0},
	} {
		t.Run(mode.name, func(t *testing.T) {
			dir := t.TempDir()
			log, err := NewSessionEventLog(filepath.Join(dir, "session-events.sqlite"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = log.Close() })

			started := time.Now()
			written := 0
			var tx *sql.Tx
			commit := func() {
				if tx != nil {
					require.NoError(t, tx.Commit())
					tx = nil
				}
			}
			for s := range sessions {
				sessionID := fmt.Sprintf("session-%06d", s)
				for e := 1; e <= eventsPerSession; e++ {
					if tx == nil {
						tx, err = log.db.Begin()
						require.NoError(t, err)
					}
					_, err = tx.Exec(
						`INSERT INTO session_event_partitions(session_id, watermark, terminal) VALUES (?, ?, ?)
						 ON CONFLICT(session_id) DO UPDATE SET watermark = excluded.watermark, terminal = excluded.terminal`,
						sessionID, e, false)
					require.NoError(t, err)
					_, err = tx.Exec(
						`INSERT INTO session_events(session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at)
						 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
						sessionID, e, fmt.Sprintf("%s:%d", sessionID, e), 1, "message.deleted",
						fmt.Sprintf("task-%06d", s), []byte(`{"type":"message.deleted"}`), "2026-09-07T12:00:00Z")
					require.NoError(t, err)
					written++
					if mode.commitEvery > 0 && written%mode.commitEvery == 0 {
						commit()
					}
				}
			}
			commit()
			duration := time.Since(started)
			t.Logf("batching floor: mode=%s events=%d total=%s (%.3f ms/event)",
				mode.name, written, duration, float64(duration.Microseconds())/1000/float64(written))
		})
	}
}
