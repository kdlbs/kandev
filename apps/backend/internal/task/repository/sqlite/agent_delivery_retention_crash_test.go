package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionCrash(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			for _, point := range []string{
				"before_hash", "after_hash", "before_payload", "after_payload",
				"before_generic_effect", "after_generic_effect", "before_retired_cursor",
				"after_retired_cursor", "before_progress", "after_progress",
			} {
				t.Run(point, func(t *testing.T) {
					repo, event := seedRetentionCrashEvent(t, engine)
					if point == "before_hash" || point == "after_hash" {
						if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
							UPDATE agent_delivery_inbox
							SET content_hash = '', hash_version = 0, payload_bytes = 0
							WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence); err != nil {
							t.Fatalf("reset event hash for backfill: %v", err)
						}
					}
					injected := errors.New("injected retention failure")
					repo.agentDeliveryRetentionFailpoint = func(got string) error {
						if got == point {
							return injected
						}
						return nil
					}
					if _, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{}); !errors.Is(err, injected) {
						t.Fatalf("retention failure at %s = %v, want injected error", point, err)
					}
					assertRetentionCrashEvidence(t, repo, event, point == "before_hash" || point == "after_hash")

					repo.agentDeliveryRetentionFailpoint = nil
					result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
					if err != nil || result.EventRowsRetired != 1 {
						t.Fatalf("retention retry = %+v, err %v", result, err)
					}
				})
			}

			t.Run("hash_corruption", func(t *testing.T) {
				repo, event := seedRetentionCrashEvent(t, engine)
				if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
					UPDATE agent_delivery_inbox SET content_hash = 'tampered'
					WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence); err != nil {
					t.Fatalf("tamper event hash: %v", err)
				}
				if _, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{}); !errors.Is(err, repoerrors.ErrAgentDeliveryReceiptCorrupt) {
					t.Fatalf("corrupt event hash error = %v, want receipt corruption", err)
				}
				var payload []byte
				var prunedAt sql.NullTime
				if err := repo.db.QueryRowx(repo.db.Rebind(`
				SELECT payload, payload_pruned_at FROM agent_delivery_inbox
				WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence).Scan(&payload, &prunedAt); err != nil {
					t.Fatalf("read corrupt event after failed cleanup: %v", err)
				}
				if len(payload) == 0 || prunedAt.Valid {
					t.Fatalf("corrupt event was pruned: payload=%d bytes pruned=%v", len(payload), prunedAt)
				}
			})

			for _, point := range []string{"before_format_marker", "after_format_marker"} {
				t.Run(point, func(t *testing.T) {
					legacyDB := openDeliveryRetentionTestDB(t, engine)
					seedPreRetentionDeliverySchema(t, legacyDB)
					injected := errors.New("injected format marker failure")
					repo := &Repository{db: legacyDB, ro: legacyDB}
					repo.agentDeliveryRetentionFailpoint = func(got string) error {
						if got == point {
							return injected
						}
						return nil
					}
					if err := repo.ensureAgentDeliveryRetentionFormat(); !errors.Is(err, injected) {
						t.Fatalf("format marker failure at %s = %v, want injected error", point, err)
					}
					exists, err := internaldb.TableExistsContext(context.Background(), legacyDB, "agent_delivery_retention_state")
					if err != nil || exists {
						t.Fatalf("format marker survived failed transaction: exists=%v, err=%v", exists, err)
					}
					repo.agentDeliveryRetentionFailpoint = nil
					if err := repo.ensureAgentDeliveryRetentionFormat(); err != nil {
						t.Fatalf("retry format marker: %v", err)
					}
				})
			}

			t.Run("cancel_during_batch", func(t *testing.T) {
				repo, event := seedRetentionCrashEvent(t, engine)
				var postgresSchema string
				if engine == "Postgres" {
					if err := repo.db.QueryRowx(`SELECT current_schema()`).Scan(&postgresSchema); err != nil {
						t.Fatalf("read postgres test schema: %v", err)
					}
				}
				if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
					UPDATE agent_delivery_inbox
					SET content_hash = '', hash_version = 0, payload_bytes = 0
					WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence); err != nil {
					t.Fatalf("reset event hash for cancellation: %v", err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				repo.agentDeliveryRetentionFailpoint = func(point string) error {
					if point == "before_hash" {
						cancel()
					}
					return nil
				}
				if _, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{}); !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled batch error = %v, want context canceled", err)
				}
				if postgresSchema != "" {
					if _, err := repo.db.ExecContext(context.Background(), `SET search_path TO "`+postgresSchema+`"`); err != nil {
						t.Fatalf("restore postgres test schema: %v", err)
					}
				}
				assertRetentionCrashEvidence(t, repo, event, true)
			})
		})
	}
}

func TestDeliveryRetentionCrashReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "retention-crash-reopen.db")
	repo := openRetentionSQLiteRepo(t, dbPath)
	event := seedRetentionCrashEventInRepo(t, repo)
	injected := errors.New("crash after progress write")
	repo.agentDeliveryRetentionFailpoint = func(point string) error {
		if point == "after_progress" {
			return injected
		}
		return nil
	}
	if _, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{}); !errors.Is(err, injected) {
		t.Fatalf("injected batch error = %v", err)
	}
	if err := repo.db.Close(); err != nil {
		t.Fatalf("close database after rollback: %v", err)
	}

	reopened := openRetentionSQLiteRepo(t, dbPath)
	assertRetentionCrashEvidence(t, reopened, event, false)
	reopened.agentDeliveryRetentionFailpoint = nil
	result, err := reopened.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
	if err != nil || result.EventRowsRetired != 1 {
		t.Fatalf("reopened retention retry = %+v, err %v", result, err)
	}
	if err := reopened.db.Close(); err != nil {
		t.Fatalf("close database after retry: %v", err)
	}

	verified := openRetentionSQLiteRepo(t, dbPath)
	cursor, err := verified.GetAgentDeliveryCursor(context.Background(), event.StreamID)
	if err != nil || cursor.RetiredSequence != 1 {
		t.Fatalf("retired cursor after reopen = %+v, err %v", cursor, err)
	}
	assertRetentionRowCount(t, verified, "agent_delivery_inbox", "stream_id = ?", event.StreamID, 0)
	assertRetentionRowCount(t, verified, "agent_delivery_effects", "effect_key = ?", "agent_delivery.event:"+event.StreamID+":1", 0)
}

func openRetentionSQLiteRepo(t *testing.T, path string) *Repository {
	t.Helper()
	raw, err := internaldb.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open SQLite retention database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("initialize SQLite retention repository: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return repo
}

func seedRetentionCrashEvent(t *testing.T, engine string) (*Repository, *models.AgentDeliveryEvent) {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	return repo, seedRetentionCrashEventInRepo(t, repo)
}

func seedRetentionCrashEventInRepo(t *testing.T, repo *Repository) *models.AgentDeliveryEvent {
	t.Helper()
	ctx := context.Background()
	const (
		taskID    = "retention-crash-task"
		sessionID = "retention-crash-session"
		streamID  = "retention-crash-stream"
	)
	baseTime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	currentTime := baseTime
	repo.clockNow = func() time.Time { return currentTime }
	workspaceID := retentionTestWorkspaceID(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Retention crash"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-" + sessionID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "crash-safe output"})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	event := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: "incarnation-" + sessionID,
		HarnessGeneration: 1, StreamID: streamID, Sequence: 1,
		EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: baseTime,
	}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		t.Fatalf("receive event = %v, err %v", inserted, err)
	}
	if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event)); err != nil {
		t.Fatalf("project event: %v", err)
	}
	currentTime = baseTime.Add(26 * time.Hour)
	return event
}

func assertRetentionCrashEvidence(t *testing.T, repo *Repository, event *models.AgentDeliveryEvent, hashBackfillExpected bool) {
	t.Helper()
	var payload []byte
	var hash string
	var hashVersion int
	var payloadBytes int64
	var prunedAt sql.NullTime
	if err := repo.db.QueryRowx(repo.db.Rebind(`
		SELECT payload, content_hash, hash_version, payload_bytes, payload_pruned_at
		FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence).Scan(
		&payload, &hash, &hashVersion, &payloadBytes, &prunedAt,
	); err != nil {
		t.Fatalf("read inbox after rollback: %v", err)
	}
	if len(payload) == 0 || prunedAt.Valid {
		t.Errorf("payload mutation survived rollback: payload=%d bytes, pruned=%v", len(payload), prunedAt)
	}
	if hashBackfillExpected {
		if hash != "" || hashVersion != 0 || payloadBytes != 0 {
			t.Errorf("hash mutation survived rollback: hash=%q version=%d payload bytes=%d", hash, hashVersion, payloadBytes)
		}
	} else if hash != event.ContentHash || hashVersion != agentDeliveryPayloadHashVersion || payloadBytes != int64(len(event.Payload)) {
		t.Errorf("event hash changed after rollback: hash=%q version=%d payload bytes=%d", hash, hashVersion, payloadBytes)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key = ?", "agent_delivery.event:"+event.StreamID+":1", 1)
	cursor, err := repo.GetAgentDeliveryCursor(context.Background(), event.StreamID)
	if err != nil {
		t.Fatalf("read cursor after rollback: %v", err)
	}
	if cursor.RetiredSequence != 0 {
		t.Errorf("retired cursor after rollback = %d, want 0", cursor.RetiredSequence)
	}
	var lastStreamID string
	var pruningStarted sql.NullTime
	if err := repo.db.QueryRowx(`SELECT last_stream_id, pruning_started_at FROM agent_delivery_retention_state WHERE id = 1`).Scan(&lastStreamID, &pruningStarted); err != nil {
		t.Fatalf("read progress after rollback: %v", err)
	}
	if lastStreamID != "" || pruningStarted.Valid {
		t.Errorf("progress survived rollback: stream=%q pruning_started=%v", lastStreamID, pruningStarted)
	}
}
