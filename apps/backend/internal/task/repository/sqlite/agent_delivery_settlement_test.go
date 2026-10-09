package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCancelledCompleteEventSettlesMatchingBlockAsCancelled(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "cancel", "submission-cancel")
	event := terminalDeliveryEvent("cancel", "submission-cancel", "stream-cancel", 1, "complete")
	event.Payload = []byte(`{"type":"complete","data":{"stop_reason":"cancelled"},"turn_id":"turn-cancel"}`)
	projectDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCancelled, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("cancel settlement = %v, %v; want settled", settled, err)
	}
	submission, err := repo.GetAgentDeliverySubmission(ctx, event.SubmissionID)
	if err != nil || submission.State != models.DeliverySubmissionCancelled {
		t.Fatalf("cancel submission = %+v, %v", submission, err)
	}
}

func TestSQLiteDeliveryTerminalSettlesMatchingBlock(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "settle", "submission-settle")

	event := terminalDeliveryEvent("settle", "submission-settle", "stream-settle", 1, "complete")
	projectDeliveryEvent(t, repo, ctx, event)

	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("SettleAgentDeliveryTerminal() = %v, %v; want settled", settled, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-settle")
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock: %v", err)
	}
	if block.State != models.RecoveryBlockResolved || block.DeliverySubmissionID != event.SubmissionID || block.DeliveryStreamID != event.StreamID {
		t.Fatalf("settled block = %+v", block)
	}
	submission, err := repo.GetAgentDeliverySubmission(ctx, event.SubmissionID)
	if err != nil || submission.State != models.DeliverySubmissionCompleted {
		t.Fatalf("authoritative submission = %+v, %v", submission, err)
	}
}

func TestDeliveryTerminalBeforeRecoveryBlockResolvesExactSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-delivery-terminal-first", Title: "Terminal first"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-delivery-terminal-first", TaskID: "task-delivery-terminal-first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "submission-terminal-first", SessionID: "session-delivery-terminal-first",
		IncarnationID: "incarnation-delivery-terminal-first", HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "hash-terminal-first", Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	terminal := terminalDeliveryEvent("terminal-first", "submission-terminal-first", "stream-terminal-first", 1, "complete")
	projectDeliveryEvent(t, repo, ctx, terminal)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, terminal.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("settlement before recovery block = %v, %v", settled, err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-terminal-first", SessionID: terminal.SessionID, IncarnationID: terminal.IncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: terminal.SubmissionID,
	}); err != nil {
		t.Fatalf("UpsertSessionRecoveryBlock after terminal: %v", err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-terminal-first")
	if err != nil {
		t.Fatal(err)
	}
	if block.State != models.RecoveryBlockResolved || block.DeliveryStreamID != terminal.StreamID || block.DeliverySequence != terminal.Sequence {
		t.Fatalf("late recovery block = %+v, want exact terminal identity and resolved state", block)
	}
}

func TestDeliveryTerminalJournalEvidenceProjectsIntoSQLSettlement(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "journal-sql", "submission-journal-sql")
	path := t.TempDir() + "/delivery.db"
	deliveryJournal, err := journal.Open(journal.Config{Path: path})
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })
	if _, err := deliveryJournal.PutSubmission(ctx, journal.Submission{
		ID: "submission-journal-sql", StreamID: "stream-journal-sql", SessionID: "session-delivery-journal-sql",
		IncarnationID: "incarnation-delivery-journal-sql", HarnessGeneration: 1,
		Hash: "hash-journal-sql", Payload: []byte("prompt"), State: journal.SubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist journal submission: %v", err)
	}
	committed, err := deliveryJournal.Append(ctx, journal.Event{
		SessionID: "session-delivery-journal-sql", IncarnationID: "incarnation-delivery-journal-sql",
		HarnessGeneration: 1, StreamID: "stream-journal-sql", SubmissionID: "submission-journal-sql",
		Type: "complete", Payload: []byte(`{"type":"complete","turn_id":"turn-journal-sql"}`), Terminal: true,
	})
	if err != nil {
		t.Fatalf("append terminal to real journal: %v", err)
	}
	replayed, _, err := deliveryJournal.Replay(ctx, committed.StreamID, 0, 10)
	if err != nil || len(replayed) != 1 {
		t.Fatalf("replay terminal from real journal = %d events, %v", len(replayed), err)
	}
	event := models.AgentDeliveryEvent{
		SessionID: replayed[0].SessionID, IncarnationID: replayed[0].IncarnationID,
		HarnessGeneration: int64(replayed[0].HarnessGeneration), StreamID: replayed[0].StreamID,
		Sequence: int64(replayed[0].Sequence), SubmissionID: replayed[0].SubmissionID,
		EventType: replayed[0].Type, Payload: replayed[0].Payload, Terminal: replayed[0].Terminal,
		ReceivedAt: replayed[0].CreatedAt,
	}
	projectDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("settle terminal from real journal and SQL = %v, %v", settled, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-journal-sql")
	if err != nil || block.State != models.RecoveryBlockResolved {
		t.Fatalf("persisted SQL recovery block = %+v, %v", block, err)
	}
}

func TestDeliveryTerminalPreservesOtherRecoveryCause(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "mixed", "submission-mixed")
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-native-mixed", SessionID: "session-delivery-mixed", IncarnationID: "incarnation-delivery-mixed",
		ExpectedGeneration: 1, Reason: "native_state_missing", State: models.RecoveryBlockOpen,
		ConsumerReference: "operator", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create independent recovery block: %v", err)
	}

	event := terminalDeliveryEvent("mixed", "submission-mixed", "stream-mixed", 1, "complete")
	projectDeliveryEvent(t, repo, ctx, event)
	if _, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC()); err != nil {
		t.Fatalf("SettleAgentDeliveryTerminal: %v", err)
	}
	open, err := repo.GetOpenSessionRecoveryBlock(ctx, "session-delivery-mixed", "incarnation-delivery-mixed", 1)
	if err != nil {
		t.Fatalf("GetOpenSessionRecoveryBlock: %v", err)
	}
	if open.ID != "block-native-mixed" || open.Reason != "native_state_missing" {
		t.Fatalf("open block = %+v, want the independent native-state cause", open)
	}
}

func TestOpenDeliveryRecoveryBlockCannotBeReboundToAnotherSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-delivery-rebind", Title: "Rebind"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-delivery-rebind", TaskID: "task-delivery-rebind"}); err != nil {
		t.Fatal(err)
	}
	first := &models.SessionRecoveryBlock{
		ID: "block-delivery-rebind-first", SessionID: "session-delivery-rebind", IncarnationID: "incarnation-delivery-rebind",
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-first",
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, first); err != nil {
		t.Fatalf("upsert first delivery block: %v", err)
	}
	second := &models.SessionRecoveryBlock{
		ID: "block-delivery-rebind-second", SessionID: first.SessionID, IncarnationID: first.IncarnationID,
		ExpectedGeneration: first.ExpectedGeneration, Reason: first.Reason, State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-second",
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, second); err != nil {
		t.Fatalf("upsert second delivery block: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second block identity = %q, want canonical %q", second.ID, first.ID)
	}
	stored, err := repo.GetSessionRecoveryBlock(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.RecoveryBlockOpen || stored.DeliverySubmissionID != "submission-first" {
		t.Fatalf("canonical open delivery block = %+v; want original submission binding preserved", stored)
	}
}

func TestDeliverySettlementCrashBoundaries(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "crash", "submission-crash")
	terminal := terminalDeliveryEvent("crash", "submission-crash", "stream-crash", 1, "complete")
	projectDeliveryEvent(t, repo, ctx, terminal)

	// Simulate process loss after projection by advancing a later event before
	// the persisted settlement intent is consumed.
	later := terminalDeliveryEvent("crash", "submission-crash", "stream-crash", 2, "message")
	later.Terminal = false
	projectDeliveryEvent(t, repo, ctx, later)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, terminal.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("settlement after projected cursor advanced = %v, %v", settled, err)
	}
	settled, err = repo.SettleAgentDeliveryTerminal(ctx, terminal.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("duplicate settlement = %v, %v", settled, err)
	}
}

func TestDeliverySettlementRejectsOldSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "stale", "submission-current")
	old := terminalDeliveryEvent("stale", "submission-old", "stream-old", 1, "complete")
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, &old, 1); err != nil {
		t.Fatalf("ReceiveAgentDeliveryEvent: %v", err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, &old, nil); err != nil {
		t.Fatalf("ProjectAgentDeliveryEvent: %v", err)
	}
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, old.StreamID, old.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil {
		t.Fatalf("SettleAgentDeliveryTerminal: %v", err)
	}
	if settled {
		t.Fatal("old terminal event settled the current submission")
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-stale")
	if err != nil || block.State != models.RecoveryBlockOpen || block.DeliverySubmissionID != "submission-current" {
		t.Fatalf("current block after stale event = %+v, %v", block, err)
	}
}

func TestDeliverySettlementSupersedesOlderTerminalForSameSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "terminal-successor", "submission-terminal-successor")
	first := terminalDeliveryEvent("terminal-successor", "submission-terminal-successor", "stream-terminal-successor", 1, "complete")
	projectDeliveryEvent(t, repo, ctx, first)
	second := terminalDeliveryEvent("terminal-successor", "submission-terminal-successor", "stream-terminal-successor", 2, "error")
	projectDeliveryEvent(t, repo, ctx, second)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, first.StreamID, first.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	if err != nil || settled {
		t.Fatalf("older terminal settlement = %v, %v; want superseded", settled, err)
	}
	firstEffect, err := repo.GetAgentDeliveryEffect(ctx, "agent_delivery.terminal:stream-terminal-successor:1")
	if err != nil || firstEffect.State != models.DeliveryEffectCompleted {
		t.Fatalf("superseded terminal effect = %+v, %v; want completed", firstEffect, err)
	}
	settled, err = repo.SettleAgentDeliveryTerminal(ctx, second.StreamID, second.Sequence, models.DeliverySubmissionFailed, time.Now().UTC())
	if err != nil || !settled {
		t.Fatalf("newer terminal settlement = %v, %v; want settled", settled, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-terminal-successor")
	if err != nil || block.State != models.RecoveryBlockResolved || block.DeliverySequence != second.Sequence || block.DeliveryOutcome != string(models.DeliverySubmissionFailed) {
		t.Fatalf("recovery block after terminal successor = %+v, %v", block, err)
	}
}

func TestDeliveryBlockUpgradeUnboundRecord(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-delivery-upgrade", Title: "Upgrade"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-delivery-upgrade", TaskID: "task-delivery-upgrade"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`ALTER TABLE session_recovery_blocks DROP COLUMN delivery_submission_id`); err != nil {
		t.Fatalf("emulate pre-change recovery blocks: %v", err)
	}
	if _, err := repo.db.Exec(`ALTER TABLE session_recovery_blocks DROP COLUMN delivery_stream_id`); err != nil {
		t.Fatalf("emulate pre-change recovery blocks stream: %v", err)
	}
	if _, err := repo.db.Exec(`INSERT INTO session_recovery_blocks
		(id, session_id, incarnation_id, expected_generation, reason, state, created_at, updated_at)
		VALUES ('legacy-block', 'session-delivery-upgrade', 'incarnation-old', 1, 'unknown_prompt_outcome', 'open', ?, ?)`, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("insert pre-change block: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("run pre-change upgrade migrations: %v", err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "legacy-block")
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock after upgrade: %v", err)
	}
	if block.State != models.RecoveryBlockOpen || block.DeliverySubmissionID != "" || block.DeliveryStreamID != "" {
		t.Fatalf("legacy block = %+v, want open and unbound", block)
	}
}

func seedDeliverySettlement(t *testing.T, repo *Repository, ctx context.Context, suffix, submissionID string) {
	t.Helper()
	taskID := "task-delivery-" + suffix
	sessionID := "session-delivery-" + suffix
	incarnationID := "incarnation-delivery-" + suffix
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Delivery"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	submission := &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 1,
		DispatchAttemptID: "queue-attempt-" + suffix, PayloadHash: "hash-" + suffix,
		Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
		t.Fatalf("PrepareAgentDeliverySubmission: %v", err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-delivery-" + suffix, SessionID: sessionID, IncarnationID: incarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("UpsertSessionRecoveryBlock: %v", err)
	}
}

func terminalDeliveryEvent(suffix, submissionID, streamID string, sequence int64, eventType string) models.AgentDeliveryEvent {
	payload, _ := json.Marshal(map[string]string{"turn_id": "turn-" + suffix})
	return models.AgentDeliveryEvent{
		SessionID: "session-delivery-" + suffix, IncarnationID: "incarnation-delivery-" + suffix,
		HarnessGeneration: 1, StreamID: streamID, Sequence: sequence,
		SubmissionID: submissionID, EventType: eventType, Payload: payload,
		Terminal:   eventType == "complete" || eventType == "error" || eventType == "cancelled",
		ReceivedAt: time.Now().UTC(),
	}
}

func projectDeliveryEvent(t *testing.T, repo *Repository, ctx context.Context, event models.AgentDeliveryEvent) {
	t.Helper()
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, event.Sequence); err != nil {
		t.Fatalf("ReceiveAgentDeliveryEvent: %v", err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, &event, nil); err != nil {
		t.Fatalf("ProjectAgentDeliveryEvent: %v", err)
	}
}
