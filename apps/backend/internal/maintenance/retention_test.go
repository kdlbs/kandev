package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionMaintenance(t *testing.T) {
	t.Run("current_schema_dry_run_is_bounded_and_read_only", func(t *testing.T) {
		fixture := newTestFixture(t)
		seedMaintenanceRetentionEvents(fixture, 3)
		before, err := os.ReadFile(fixture.dbPath)
		if err != nil {
			t.Fatalf("read database before dry-run: %v", err)
		}

		outcome, err := Run(context.Background(), t.TempDir(), "sqlite", fixture.dbPath,
			RunOptions{CandidateLimit: 1}, nil)
		if err != nil {
			t.Fatalf("Run dry-run: %v", err)
		}
		if !outcome.Report.DeliveryRetentionAvailable {
			t.Fatalf("delivery retention unavailable: %s", outcome.Report.DeliveryRetentionUnavailableReason)
		}
		if got := outcome.Report.ProjectedDeliveryPayloads.RowCount; got != 1 {
			t.Fatalf("projected payload sample = %d, want 1", got)
		}
		if !outcome.Report.ProjectedDeliveryPayloads.SampleLimitReached {
			t.Fatal("projected payload sample did not report its candidate limit")
		}
		if outcome.Report.RetiredDeliveryEventReceipts.RowCount != 1 {
			t.Fatalf("retired receipt sample = %d, want 1", outcome.Report.RetiredDeliveryEventReceipts.RowCount)
		}
		after, err := os.ReadFile(fixture.dbPath)
		if err != nil {
			t.Fatalf("read database after dry-run: %v", err)
		}
		if string(after) != string(before) {
			t.Fatal("dry-run changed database bytes")
		}
	})

	t.Run("legacy_retention_schema_is_reported_without_migration", func(t *testing.T) {
		fixture := newTestFixture(t)
		if _, err := fixture.repo.DB().Exec(`DROP TABLE agent_delivery_retention_state`); err != nil {
			t.Fatalf("remove retention schema marker: %v", err)
		}
		before, err := os.ReadFile(fixture.dbPath)
		if err != nil {
			t.Fatalf("read database before legacy dry-run: %v", err)
		}

		outcome, err := Run(context.Background(), t.TempDir(), "sqlite", fixture.dbPath, RunOptions{}, nil)
		if err != nil {
			t.Fatalf("Run legacy dry-run: %v", err)
		}
		if outcome.Report.DeliveryRetentionAvailable || outcome.Report.DeliveryRetentionUnavailableReason != "schema_unavailable" {
			t.Fatalf("legacy retention report = %+v", outcome.Report)
		}
		after, err := os.ReadFile(fixture.dbPath)
		if err != nil {
			t.Fatalf("read database after legacy dry-run: %v", err)
		}
		if string(after) != string(before) {
			t.Fatal("legacy dry-run changed or migrated the database")
		}
	})

	t.Run("partially_settled_terminal_stays_protected", func(t *testing.T) {
		fixture := newTestFixture(t)
		event := seedMaintenancePartialTerminal(fixture)

		outcome, err := Run(context.Background(), t.TempDir(), "sqlite", fixture.dbPath, RunOptions{}, nil)
		if err != nil {
			t.Fatalf("Run partial-terminal dry-run: %v", err)
		}
		if got := outcome.Report.SettledDeliveryPrompts.RowCount; got != 0 {
			t.Fatalf("partially settled prompt candidates = %d, want 0", got)
		}

		executed, err := Run(context.Background(), t.TempDir(), "sqlite", fixture.dbPath,
			RunOptions{Execute: true}, nil)
		if err != nil {
			t.Fatalf("Run partial-terminal execute: %v", err)
		}
		if executed.Execution.DeletedSettledDeliveryPrompts != 0 {
			t.Fatalf("partially settled prompt payloads deleted = %d", executed.Execution.DeletedSettledDeliveryPrompts)
		}
		var payload []byte
		if err := fixture.repo.DB().QueryRow(`SELECT payload FROM agent_delivery_submissions WHERE id = ?`, event.SubmissionID).Scan(&payload); err != nil {
			t.Fatalf("read protected prompt payload: %v", err)
		}
		if len(payload) == 0 {
			t.Fatal("partially settled prompt payload was deleted")
		}
	})

	t.Run("cancelled_execute_keeps_delivery_payload", func(t *testing.T) {
		fixture := newTestFixture(t)
		seedMaintenanceRetentionEvents(fixture, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Run(ctx, t.TempDir(), "sqlite", fixture.dbPath, RunOptions{Execute: true}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled execute error = %v, want context canceled", err)
		}
		var payload []byte
		if err := fixture.repo.DB().QueryRow(`SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = 1`, "maintenance-retention-stream").Scan(&payload); err != nil {
			t.Fatalf("read delivery payload after cancellation: %v", err)
		}
		if len(payload) == 0 {
			t.Fatal("cancelled execute removed a delivery payload")
		}
	})

	t.Run("execute_uses_bounded_repository_batches_and_reports_partial_progress", func(t *testing.T) {
		fixture := newTestFixture(t)
		seedMaintenanceRetentionEvents(fixture, 3)
		homeDir := t.TempDir()

		first, err := Run(context.Background(), homeDir, "sqlite", fixture.dbPath,
			RunOptions{Execute: true, CandidateLimit: 1}, nil)
		if err != nil {
			t.Fatalf("first execute: %v", err)
		}
		if first.Execution.DeletedDeliveryEventPayloads != 1 || first.Execution.RetiredDeliveryEventReceipts != 1 {
			t.Fatalf("first bounded execution = %+v", first.Execution)
		}
		if !first.Execution.Partial {
			t.Fatal("first bounded execution did not report partial progress")
		}
		if first.BackupPath == "" {
			t.Fatal("execute omitted verified backup path")
		}

		for remaining := 2; remaining > 0; remaining-- {
			outcome, err := Run(context.Background(), homeDir, "sqlite", fixture.dbPath,
				RunOptions{Execute: true, CandidateLimit: 1}, nil)
			if err != nil {
				t.Fatalf("continue bounded execute: %v", err)
			}
			if outcome.Execution.DeletedDeliveryEventPayloads != 1 || outcome.Execution.RetiredDeliveryEventReceipts != 1 {
				t.Fatalf("continued bounded execution = %+v", outcome.Execution)
			}
		}
		var retired int64
		if err := fixture.repo.DB().QueryRow(`SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = ?`, "maintenance-retention-stream").Scan(&retired); err != nil {
			t.Fatalf("read retired cursor: %v", err)
		}
		if retired != 3 {
			t.Fatalf("retired cursor = %d, want 3", retired)
		}
		// Leave room past the retained live cursor to prove scan exhaustion.
		noop, err := Run(context.Background(), homeDir, "sqlite", fixture.dbPath,
			RunOptions{Execute: true, CandidateLimit: 2}, nil)
		if err != nil {
			t.Fatalf("repeat execute after cleanup: %v", err)
		}
		if noop.Execution.TotalDeleted() != 0 || noop.Execution.Partial {
			t.Fatalf("repeat execute was not an empty complete pass: %+v", noop.Execution)
		}
	})

	t.Run("compaction_failure_reports_committed_delivery_deletes", func(t *testing.T) {
		fixture := newTestFixture(t)
		seedMaintenanceRetentionEvents(fixture, 2)
		backupPath, err := createVerifiedBackup(fixture.sqlxDB, filepath.Join(t.TempDir(), "backups"))
		if err != nil {
			t.Fatalf("create verified backup: %v", err)
		}
		compactionErr := errors.New("injected compaction failure")
		outcome, err := runExecuteAndCompact(
			context.Background(), fixture.sqlxDB, fixture.closeForSwap, fixture.dbPath, backupPath,
			fixture.repo, Report{ProjectedDeliveryPayloads: CandidateGroup{SampleLimitReached: true}}, candidateSet{},
			models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1}, 1, true,
			func() (CompactionResult, error) { return CompactionResult{}, compactionErr },
		)
		if !errors.Is(err, compactionErr) {
			t.Fatalf("compaction error = %v, want injected error", err)
		}
		if !outcome.Executed || outcome.BackupPath != backupPath || outcome.Execution.DeletedDeliveryEventPayloads != 1 || !outcome.Execution.Partial {
			t.Fatalf("partial outcome after compaction failure = %+v", outcome)
		}
		if err := verifySQLiteIntegrity(backupPath); err != nil {
			t.Fatalf("verified pre-execution backup became invalid: %v", err)
		}
		var retired int64
		if err := fixture.repo.DB().QueryRow(`
			SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = ?`,
			"maintenance-retention-stream").Scan(&retired); err != nil {
			t.Fatalf("read committed checkpoint after compaction failure: %v", err)
		}
		var remaining int
		if err := fixture.repo.DB().QueryRow(`
			SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = 1`,
			"maintenance-retention-stream").Scan(&remaining); err != nil {
			t.Fatalf("read retired event count: %v", err)
		}
		if retired != 1 || remaining != 0 {
			t.Fatalf("cleanup did not remain committed: checkpoint=%d remaining=%d", retired, remaining)
		}
	})
}

func seedMaintenancePartialTerminal(fixture *testFixture) models.AgentDeliveryEvent {
	fixture.t.Helper()
	ctx := context.Background()
	const taskID, sessionID = "maintenance-partial-task", "maintenance-partial-session"
	fixture.seedTask(taskID)
	fixture.seedSession(taskID, sessionID)
	session, err := fixture.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		fixture.t.Fatalf("read seeded session: %v", err)
	}
	now := time.Now().UTC().Add(-48 * time.Hour)
	submission := &models.AgentDeliverySubmission{
		ID: "maintenance-partial-submission", SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "maintenance-partial-dispatch",
		PayloadHash: "maintenance-partial-hash", Payload: []byte("composed prompt"),
		State: models.DeliverySubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := fixture.repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
		fixture.t.Fatalf("prepare prompt submission: %v", err)
	}
	if err := fixture.repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "maintenance-partial-block", SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submission.ID,
	}); err != nil {
		fixture.t.Fatalf("create prompt recovery block: %v", err)
	}
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "maintenance-partial-turn"})
	if err != nil {
		fixture.t.Fatalf("marshal terminal event: %v", err)
	}
	event := models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: "maintenance-partial-stream", Sequence: 1, SubmissionID: submission.ID,
		EventType: streams.EventTypeComplete, Payload: payload, Terminal: true, ReceivedAt: now,
	}
	if inserted, err := fixture.repo.ReceiveAgentDeliveryEvent(ctx, &event, 1); err != nil || !inserted {
		fixture.t.Fatalf("receive terminal event = %v, %v", inserted, err)
	}
	if _, err := fixture.repo.ProjectAgentDeliveryEvent(ctx, &event, nil); err != nil {
		fixture.t.Fatalf("project terminal event: %v", err)
	}
	if settled, err := fixture.repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, now); err != nil || !settled {
		fixture.t.Fatalf("settle terminal outcome = %v, %v", settled, err)
	}
	if complete, err := fixture.repo.CompleteAgentDeliveryTerminalSettlement(ctx, "agent_delivery.terminal:"+event.StreamID+":1", now); err != nil || complete {
		fixture.t.Fatalf("complete terminal without workflow outcome = %v, %v; want incomplete", complete, err)
	}
	if _, err := fixture.repo.DB().ExecContext(ctx, `UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ?`, now, event.StreamID); err != nil {
		fixture.t.Fatalf("age partial terminal: %v", err)
	}
	return event
}

func seedMaintenanceRetentionEvents(fixture *testFixture, count int) {
	fixture.t.Helper()
	ctx := context.Background()
	fixture.seedTask("maintenance-retention-task")
	fixture.seedSession("maintenance-retention-task", "maintenance-retention-session")
	session, err := fixture.repo.GetTaskSession(ctx, "maintenance-retention-session")
	if err != nil {
		fixture.t.Fatalf("read seeded session: %v", err)
	}
	now := time.Now().UTC()
	for sequence := 1; sequence <= count; sequence++ {
		payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "retention sample"})
		if err != nil {
			fixture.t.Fatalf("marshal event: %v", err)
		}
		receivedAt := now.Add(-48 * time.Hour)
		event := &models.AgentDeliveryEvent{
			SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
			StreamID: "maintenance-retention-stream", Sequence: int64(sequence),
			EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: receivedAt,
		}
		if inserted, err := fixture.repo.ReceiveAgentDeliveryEvent(ctx, event, int64(sequence)); err != nil || !inserted {
			fixture.t.Fatalf("receive event %d = %v, err %v", sequence, inserted, err)
		}
		completedAt := receivedAt
		effect := &models.AgentDeliveryEffect{
			EffectKey: "agent_delivery.event:" + event.StreamID + ":" + string(rune('0'+sequence)),
			StreamID:  event.StreamID, Sequence: event.Sequence, EffectType: "agent_delivery.event",
			State: models.DeliveryEffectCompleted, CreatedAt: receivedAt, CompletedAt: &completedAt,
		}
		if _, err := fixture.repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, effect); err != nil {
			fixture.t.Fatalf("project event %d: %v", sequence, err)
		}
	}
	if _, err := fixture.repo.DB().ExecContext(ctx, `
		UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ?`, now.Add(-48*time.Hour), "maintenance-retention-stream"); err != nil {
		fixture.t.Fatalf("age projected events: %v", err)
	}
}
