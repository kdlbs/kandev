package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAgentDeliveryRecoveryRevisionFencesPhaseOwnerAndSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-recovery-state", Title: "Recovery state"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery-state", TaskID: "task-recovery-state",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-recovery-state", SessionID: "session-recovery-state", TaskID: "task-recovery-state",
		ExecutorID: "executor-recovery-state", AgentExecutionID: "exec-recovery-state", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "session-recovery-state")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSessionMetadataKey(ctx, session.ID, "last_agent_error", map[string]any{"message": "keep this"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "submission-recovery-state", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 4, OwnerGeneration: 1, PayloadHash: "hash-recovery-state", Payload: []byte("prompt"),
		State: models.DeliverySubmissionInterruptedUnknown,
	}); err != nil {
		t.Fatal(err)
	}

	reconnecting := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryReconnecting, SessionID: session.ID,
		AgentExecutionID: "exec-recovery-state", SubmissionID: "submission-recovery-state",
		StreamID: "stream-recovery-state", IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 4, PromptGeneration: 9, UpdatedAt: time.Now().UTC(),
	}
	block := deliveryRecoveryBlock(reconnecting)
	stored, err := repo.UpsertAgentDeliveryRecovery(ctx, &reconnecting, block)
	if err != nil || !stored {
		t.Fatalf("UpsertAgentDeliveryRecovery(reconnecting) = %v, %v; want stored", stored, err)
	}
	if reconnecting.Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", reconnecting.Revision)
	}
	uncertain := reconnecting
	uncertain.Phase = models.AgentDeliveryRecoveryUncertain
	uncertain.Revision = 0
	stored, err = repo.UpsertAgentDeliveryRecovery(ctx, &uncertain, block)
	if err != nil || !stored {
		t.Fatalf("UpsertAgentDeliveryRecovery(uncertain) = %v, %v; want stored", stored, err)
	}
	if uncertain.Revision != 2 {
		t.Fatalf("uncertain revision = %d, want 2", uncertain.Revision)
	}

	stalePhase := reconnecting
	stalePhase.Revision = 0
	stored, err = repo.UpsertAgentDeliveryRecovery(ctx, &stalePhase, block)
	if err != nil || stored {
		t.Fatalf("stale reconnecting phase = %v, %v; want rejected", stored, err)
	}
	staleOwner := uncertain
	staleOwner.AgentExecutionID = "exec-replaced"
	stored, err = repo.UpsertAgentDeliveryRecovery(ctx, &staleOwner, block)
	if err != nil || stored {
		t.Fatalf("stale execution owner = %v, %v; want rejected", stored, err)
	}
	staleSubmission := uncertain
	staleSubmission.SubmissionID = "submission-old"
	staleSubmission.StreamID = "stream-old"
	staleBlock := deliveryRecoveryBlock(staleSubmission)
	stored, err = repo.UpsertAgentDeliveryRecovery(ctx, &staleSubmission, staleBlock)
	if err != nil || stored {
		t.Fatalf("different submission at same generation = %v, %v; want rejected", stored, err)
	}

	stored, err = repo.ClearAgentDeliveryRecovery(ctx, session.ID, session.QueueIncarnationID,
		"submission-old", 4)
	if err != nil || stored {
		t.Fatalf("clear for another submission = %v, %v; want unchanged", stored, err)
	}
	stored, err = repo.ClearAgentDeliveryRecovery(ctx, session.ID, session.QueueIncarnationID,
		"submission-recovery-state", 4)
	if err != nil || !stored {
		t.Fatalf("clear exact submission = %v, %v; want cleared", stored, err)
	}

	final, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	settled, ok := models.LoadAgentDeliveryRecovery(final.Metadata)
	if !ok || settled.Phase != models.AgentDeliveryRecoverySettled || settled.Revision != 3 {
		t.Fatalf("recovery metadata is not a settled revisioned tombstone: %+v, ok=%v", settled, ok)
	}
	if final.Metadata["last_agent_error"] == nil {
		t.Fatal("recovery clearing removed the independent last_agent_error")
	}
}

func TestAgentDeliveryRecoveryRejectsReplacedSessionAndOlderPrompt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-recovery-order", Title: "Recovery order"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery-order", TaskID: "task-recovery-order",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-recovery-order", SessionID: "session-recovery-order", TaskID: "task-recovery-order",
		ExecutorID: "executor-recovery-order", AgentExecutionID: "exec-current", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "session-recovery-order")
	if err != nil {
		t.Fatal(err)
	}
	for _, submissionID := range []string{"submission-new", "submission-old"} {
		if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
			ID: submissionID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
			HarnessGeneration: 3, OwnerGeneration: 1, PayloadHash: "hash-" + submissionID,
			Payload: []byte("prompt"), State: models.DeliverySubmissionInterruptedUnknown,
		}); err != nil {
			t.Fatal(err)
		}
	}
	newer := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryReconnecting, SessionID: session.ID,
		AgentExecutionID: "exec-current", SubmissionID: "submission-new", StreamID: "stream-new",
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: 3, PromptGeneration: 8,
	}
	stored, err := repo.UpsertAgentDeliveryRecovery(ctx, &newer, deliveryRecoveryBlock(newer))
	if err != nil || !stored {
		t.Fatalf("store newer snapshot = %v, %v", stored, err)
	}
	older := newer
	older.SubmissionID = "submission-old"
	older.StreamID = "stream-old"
	older.PromptGeneration = 7
	stored, err = repo.UpsertAgentDeliveryRecovery(ctx, &older, deliveryRecoveryBlock(older))
	if err != nil || stored {
		t.Fatalf("store older prompt = %v, %v; want rejected", stored, err)
	}
	wrongIncarnation := newer
	wrongIncarnation.IncarnationID = "old-incarnation"
	stored, err = repo.UpsertAgentDeliveryRecovery(ctx, &wrongIncarnation, deliveryRecoveryBlock(wrongIncarnation))
	if err != nil || stored {
		t.Fatalf("store old incarnation = %v, %v; want rejected", stored, err)
	}
}

func deliveryRecoveryBlock(recovery models.AgentDeliveryRecovery) *models.SessionRecoveryBlock {
	return &models.SessionRecoveryBlock{
		SessionID: recovery.SessionID, IncarnationID: recovery.IncarnationID,
		ExpectedGeneration: recovery.HarnessGeneration, Reason: "unknown_prompt_outcome",
		State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID,
	}
}
