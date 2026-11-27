package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// The opt-in harness owns two entire disposable databases because pg_restore
// and a separately running backend cannot share connection-local search paths.
func TestDeliveryRetentionCompatibilityPostgresBackupRollback(t *testing.T) {
	dsn := os.Getenv("KANDEV_RETENTION_ROLLBACK_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KANDEV_RETENTION_ROLLBACK_POSTGRES_DSN for the disposable database binary/backup gate")
	}
	harness := newRetentionPostgresBinaryHarness(t, dsn)
	stop := harness.start(t, os.Getenv("KANDEV_RETENTION_CURRENT_BINARY"))
	raw, err := db.OpenPostgres(harness.source.String(), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(raw, "pgx")
	defer func() { _ = database.Close() }()
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := messagequeue.NewSQLiteRepository(database, database); err != nil {
		t.Fatal(err)
	}
	assertRetentionStorageToolPolicy(t, repo)
	session := seedRetentionPostgresCompatibility(t, repo)
	seedRetentionPostgresSettledPrompt(t, repo, session.TaskID)
	harness.requireRealPromptAndEventWrites(t, database, retentionTestWorkspaceID(t, repo))
	harness.ageSettledLivePrompt(t, database)
	harness.captureProtectedNativePrefix(t, database)
	stop()
	before := retentionPostgresEvidence(t, database)
	harness.backupAndRestore(t, database, before)
	var retired, compacted, prompts int
	for attempt := 0; attempt < 3; attempt++ {
		result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		retired += result.EventRowsRetired
		compacted += result.EventPayloadsPruned
		prompts += result.PromptPayloadsPruned
	}
	if retired < 2 || compacted < 4 || prompts != 2 {
		t.Fatalf("real pruning: retired=%d compacted=%d prompts=%d", retired, compacted, prompts)
	}
	var hash string
	if err := database.Get(&hash, `SELECT content_hash FROM agent_delivery_inbox WHERE sequence = 3 AND stream_id = 'postgres-rollback-stream'`); err != nil || hash == "" {
		t.Fatalf("compact hash missing: %q %v", hash, err)
	}
	var manualRetired int64
	if err := database.Get(&manualRetired, `SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = 'postgres-settled-stream'`); err != nil || manualRetired != 1 {
		t.Fatalf("actual manual terminal retirement checkpoint: %d %v", manualRetired, err)
	}
	if _, err := repo.RunAgentDeliveryOrphanBatch(context.Background(), models.AgentDeliveryOrphanBatchOptions{MaxCandidates: 1}); err != nil {
		t.Fatal(err)
	}
	harness.capturePrunedLiveEvidence(t, database)
	pruned := retentionPostgresEvidence(t, database)
	stop = harness.start(t, os.Getenv("KANDEV_RETENTION_COMPAT_BINARY"))
	harness.requireHistory(t, session.ID)
	harness.requireNewSessionAndRetiredIDFence(t, session.TaskID)
	harness.requireRealPromptAndEventWrites(t, database, retentionTestWorkspaceID(t, repo))
	harness.assertPrunedLiveEvidence(t, database)
	stop()
	assertRetentionPostgresBinaryDeletion(t, database)
	if after := retentionPostgresEvidence(t, database); after != pruned {
		t.Fatalf("Release N altered retained history/checkpoints/hashes/pending job/guards\nbefore: %s\nafter: %s", pruned, after)
	}
	forward := os.Getenv("KANDEV_RETENTION_CURRENT_BINARY")
	if forward == "" {
		t.Fatal("set KANDEV_RETENTION_CURRENT_BINARY for the forward upgrade gate")
	}
	harness.preserveAgents = false
	stop = harness.start(t, forward)
	harness.requireRealPromptAndEventWrites(t, database, retentionTestWorkspaceID(t, repo))
	harness.requireHistory(t, session.ID)
	harness.assertPrunedLiveEvidence(t, database)
	stop()
	assertRetentionPostgresBinaryDeletion(t, database)
	if after := retentionPostgresEvidence(t, database); after != pruned {
		t.Fatalf("forward upgrade altered retained evidence\nbefore: %s\nafter: %s", pruned, after)
	}
	t.Log("verified populated pre-pruning pg_dump/restore; frozen N read compacted history, completed a real mock prompt with new event writes, journaled deletion and refused retired identity; forward reader preserved history")
}

func seedRetentionPostgresCompatibility(t *testing.T, repo *Repository) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	task := &models.Task{ID: "postgres-rollback-task", WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Postgres rollback"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "postgres-rollback-session", TaskID: task.ID, QueueIncarnationID: "postgres-rollback-inc", State: models.TaskSessionStateCreated}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-48 * time.Hour)
	for i, eventType := range []string{streams.EventTypeMessageChunk, streams.EventTypePermissionRequest, streams.EventTypeMessageChunk} {
		payload, err := json.Marshal(streams.AgentEvent{Type: eventType, Text: "byte-exact PostgreSQL saved output"})
		if err != nil {
			t.Fatal(err)
		}
		event := &models.AgentDeliveryEvent{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, StreamID: "postgres-rollback-stream", Sequence: int64(i + 1), EventType: eventType, Payload: payload, ReceivedAt: now}
		if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 3); err != nil || !inserted {
			t.Fatalf("receive: %v %v", inserted, err)
		}
		if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.Exec(`UPDATE agent_delivery_inbox SET projected_at = CURRENT_TIMESTAMP - INTERVAL '48 hours' WHERE stream_id = 'postgres-rollback-stream'`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "postgres-uncertain-prompt", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "uncertain-prompt-hash", Payload: []byte("never resend this uncertain prompt"), State: models.DeliverySubmissionPrepared}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{ID: "postgres-guard", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "native_state_missing", State: models.RecoveryBlockOpen, ConsumerReference: "operator"}); err != nil {
		t.Fatal(err)
	}
	retired := createRetentionSessionForExistingTask(t, repo, ctx, task.ID, "postgres-retired-session", false, "")
	seedRetentionDeliveryEvidence(t, repo, ctx, retired)
	if err := repo.DeleteTaskSession(ctx, retired); err != nil {
		t.Fatal(err)
	}
	return session
}

func retentionPostgresEvidence(t *testing.T, database *sqlx.DB) string {
	t.Helper()
	var evidence string
	for _, table := range []string{"task_session_messages", "agent_delivery_inbox", "agent_delivery_cursors", "agent_delivery_effects", "agent_delivery_submissions", "session_recovery_blocks", "agent_delivery_cleanup_jobs"} {
		var rows string
		sessionColumn := "session_id"
		if table == "task_session_messages" {
			sessionColumn = "task_session_id"
		}
		query := fmt.Sprintf(`SELECT COALESCE(json_agg(row_to_json(record) ORDER BY row_to_json(record)::text)::text, '[]') FROM (SELECT * FROM %s WHERE %s IN ('postgres-rollback-session','postgres-retired-session','postgres-settled-session')) record`, table, sessionColumn)
		if table == "agent_delivery_cleanup_jobs" {
			query = `SELECT COALESCE(json_agg(row_to_json(record) ORDER BY row_to_json(record)::text)::text, '[]') FROM (SELECT * FROM agent_delivery_cleanup_jobs WHERE session_id IN ('postgres-rollback-session','postgres-retired-session','postgres-settled-session')) record`
		}
		if err := database.Get(&rows, query); err != nil {
			t.Fatalf("snapshot %s: %v", table, err)
		}
		evidence += table + ":" + rows + "\n"
	}
	var marker string
	if err := database.Get(&marker, `SELECT row_to_json(record)::text FROM (SELECT format_version, pruning_started_at, sweep_phase, last_session_id, last_stream_id, last_sequence, last_submission_id, last_effect_key, event_sweep_phase, last_compaction_at, last_compaction_stream_id, last_compaction_sequence FROM agent_delivery_retention_state WHERE id = 1) record`); err != nil {
		t.Fatal(err)
	}
	return evidence + "format-marker:" + marker
}

func assertRetentionPostgresBinaryDeletion(t *testing.T, database *sqlx.DB) {
	t.Helper()
	var phase string
	if err := database.Get(&phase, `SELECT phase FROM agent_delivery_cleanup_jobs WHERE session_id = 'postgres-new-session'`); err != nil || phase != "pending" {
		t.Fatalf("Release N deletion job not preserved: %q %v", phase, err)
	}
	var count int
	if err := database.Get(&count, `SELECT COUNT(*) FROM task_sessions WHERE id = 'postgres-new-session'`); err != nil || count != 0 {
		t.Fatalf("Release N deleted session still present: %d %v", count, err)
	}
}

func seedRetentionPostgresSettledPrompt(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	previousClock := repo.clockNow
	repo.clockNow = func() time.Time { return old }
	defer func() { repo.clockNow = previousClock }()
	session := &models.TaskSession{ID: "postgres-settled-session", TaskID: taskID, QueueIncarnationID: "postgres-settled-inc", State: models.TaskSessionStateCreated}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	submission := &models.AgentDeliverySubmission{ID: "postgres-settled-prompt", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "postgres-settled-dispatch", PayloadHash: "postgres-settled-hash", Payload: []byte("completed prompt whose raw payload must expire"), State: models.DeliverySubmissionDispatching}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "postgres-settled-turn"})
	if err != nil {
		t.Fatal(err)
	}
	event := models.AgentDeliveryEvent{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, StreamID: "postgres-settled-stream", Sequence: 1, SubmissionID: submission.ID, EventType: streams.EventTypeComplete, Payload: payload, Terminal: true, ReceivedAt: old}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 1); err != nil || !inserted {
		t.Fatalf("settled terminal receive: %t %v", inserted, err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, &event, completedRetentionEventEffect(&event)); err != nil {
		t.Fatal(err)
	}
	if settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, 1, models.DeliverySubmissionCompleted, old); err != nil || !settled {
		t.Fatalf("terminal settlement: %t %v", settled, err)
	}
	putRetentionWorkflowOutcome(t, repo, event, "not_required", "postgres-settled-turn")
	if settled, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx, terminalRetentionEffectKey(event), old); err != nil || !settled {
		t.Fatalf("full settlement: %t %v", settled, err)
	}
}
