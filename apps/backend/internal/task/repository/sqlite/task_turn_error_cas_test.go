package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func turnErrorRecord(stamp, sessionID, message string, at time.Time) models.TaskTurnError {
	return models.TaskTurnError{
		StampValue:  stamp,
		SessionID:   sessionID,
		ExecutionID: "execution-1",
		TurnID:      "turn-1",
		MessageID:   stamp,
		OccurredAt:  at,
		Message:     message,
	}
}

func TestRecordTaskTurnErrorKeepsNewestAndPreservesUnrelatedMetadata(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})
	ctx := context.Background()

	first := turnErrorRecord("error-1", "session-a", "first failure", time.Date(2026, 10, 29, 20, 32, 0, 0, time.UTC))
	stored, err := repo.RecordTaskTurnError(ctx, casTaskID, first)
	if err != nil || !stored {
		t.Fatalf("record first attention = %v, %v", stored, err)
	}
	stored, err = repo.RecordTaskTurnError(ctx, casTaskID, first)
	if err != nil || stored {
		t.Fatalf("duplicate attention = %v, %v, want a no-op", stored, err)
	}
	older := turnErrorRecord("error-0", "session-a", "older failure", time.Date(2026, 10, 29, 19, 0, 0, 0, time.UTC))
	stored, err = repo.RecordTaskTurnError(ctx, casTaskID, older)
	if err != nil || stored {
		t.Fatalf("older attention = %v, %v, want a no-op", stored, err)
	}

	task, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	current, ok := models.LoadTaskTurnError(task.Metadata)
	if !ok || current.StampValue != "error-1" {
		t.Fatalf("attention = %+v, want error-1", current)
	}
	if task.Metadata["other_key"] != "keep me" {
		t.Fatalf("unrelated metadata = %#v, want preserved", task.Metadata["other_key"])
	}
}

func TestDismissTaskTurnErrorIfStamp(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{})
	ctx := context.Background()
	at := time.Date(2026, 10, 29, 20, 32, 0, 0, time.UTC)
	if _, err := repo.RecordTaskTurnError(ctx, casTaskID, turnErrorRecord("error-1", "session-a", "failure", at)); err != nil {
		t.Fatalf("record attention: %v", err)
	}

	result, err := repo.AcknowledgeTaskTurnErrorIfStamp(ctx, casTaskID, "stale-stamp", at.Add(time.Minute), models.TurnErrorAcknowledgementDismiss)
	if err != nil {
		t.Fatalf("stale dismissal: %v", err)
	}
	if result != models.TurnErrorAcknowledgementStale {
		t.Fatalf("stale dismissal = %v, want stale", result)
	}

	result, err = repo.AcknowledgeTaskTurnErrorIfStamp(ctx, casTaskID, "error-1", at.Add(time.Minute), models.TurnErrorAcknowledgementDismiss)
	if err != nil {
		t.Fatalf("authorized dismissal: %v", err)
	}
	if result != models.TurnErrorAcknowledgementStored {
		t.Fatalf("authorized dismissal = %v, want stored", result)
	}

	result, err = repo.AcknowledgeTaskTurnErrorIfStamp(ctx, casTaskID, "error-1", at.Add(2*time.Minute), models.TurnErrorAcknowledgementDismiss)
	if err != nil {
		t.Fatalf("repeat dismissal: %v", err)
	}
	if result != models.TurnErrorAcknowledgementNoOp {
		t.Fatalf("repeat dismissal = %v, want no-op", result)
	}

	task, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	current, ok := models.LoadTaskTurnError(task.Metadata)
	if !ok || !current.AcknowledgedAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("acknowledged record = %+v, want the first acknowledgement", current)
	}
}

func TestTaskTurnErrorDismissalPreservesNewerSiblingFailure(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{})
	ctx := context.Background()
	at := time.Date(2026, 10, 29, 20, 32, 0, 0, time.UTC)
	if _, err := repo.RecordTaskTurnError(ctx, casTaskID, turnErrorRecord("error-1", "session-a", "first", at)); err != nil {
		t.Fatalf("record first attention: %v", err)
	}
	if _, err := repo.RecordTaskTurnError(ctx, casTaskID, turnErrorRecord("error-2", "session-b", "sibling", at.Add(time.Second))); err != nil {
		t.Fatalf("record sibling attention: %v", err)
	}

	result, err := repo.AcknowledgeTaskTurnErrorIfStamp(ctx, casTaskID, "error-1", at.Add(time.Minute), models.TurnErrorAcknowledgementPrompt)
	if err != nil {
		t.Fatalf("late acknowledgement: %v", err)
	}
	if result != models.TurnErrorAcknowledgementStale {
		t.Fatalf("late acknowledgement = %v, want stale", result)
	}
	task, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	current, ok := models.LoadTaskTurnError(task.Metadata)
	if !ok || current.StampValue != "error-2" || current.Acknowledged() {
		t.Fatalf("attention = %+v, want the unacknowledged newer sibling failure", current)
	}
}

func TestStaleFullTaskUpdatesPreserveTurnErrorAttention(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{"ordinary": "keep"})
	ctx := context.Background()
	at := time.Date(2026, 10, 29, 20, 32, 0, 0, time.UTC)

	stale, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("load stale snapshot: %v", err)
	}
	if _, err := repo.RecordTaskTurnError(ctx, casTaskID, turnErrorRecord("error-1", "session-a", "failure", at)); err != nil {
		t.Fatalf("record attention: %v", err)
	}
	stale.Metadata = nil
	stale.Title = "stale full update"
	if err := repo.UpdateTask(ctx, stale); err != nil {
		t.Fatalf("stale UpdateTask: %v", err)
	}
	assertTurnErrorRecord(t, repo, "error-1", false)

	staleAck, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("load stale acknowledgement snapshot: %v", err)
	}
	if _, err := repo.AcknowledgeTaskTurnErrorIfStamp(
		ctx, casTaskID, "error-1", at.Add(time.Minute), models.TurnErrorAcknowledgementDismiss,
	); err != nil {
		t.Fatalf("acknowledge attention: %v", err)
	}
	staleAck.Metadata = nil
	staleAck.Title = "stale deferred update"
	if err := repo.UpdateTaskPreservingDeferredLaunch(ctx, staleAck); err != nil {
		t.Fatalf("stale UpdateTaskPreservingDeferredLaunch: %v", err)
	}
	current := assertTurnErrorRecord(t, repo, "error-1", true)
	if !current.AcknowledgedAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("acknowledged_at = %v, want the committed acknowledgement", current.AcknowledgedAt)
	}

	if err := repo.MergeTaskMetadata(ctx, casTaskID, map[string]interface{}{
		models.MetaKeyLastTurnError: map[string]interface{}{"stamp": "forged"},
		"ordinary":                  "merged",
	}); err != nil {
		t.Fatalf("merge metadata: %v", err)
	}
	assertTurnErrorRecord(t, repo, "error-1", true)
	task, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Metadata["ordinary"] != "merged" {
		t.Fatalf("ordinary metadata = %#v, want merged", task.Metadata["ordinary"])
	}
}

func assertTurnErrorRecord(t *testing.T, repo *Repository, stamp string, acknowledged bool) models.TaskTurnError {
	t.Helper()
	task, err := repo.GetTask(context.Background(), casTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	current, ok := models.LoadTaskTurnError(task.Metadata)
	if !ok || current.StampValue != stamp {
		t.Fatalf("turn error = %+v, want stamp %q", current, stamp)
	}
	if current.Acknowledged() != acknowledged {
		t.Fatalf("turn error acknowledged = %v, want %v", current.Acknowledged(), acknowledged)
	}
	return current
}
