package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionPartialTerminal(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, event, _ := seedRetentionPromptSubmission(t, engine, "partial")
			completed, err := repo.CompleteAgentDeliveryTerminalSettlement(
				context.Background(), terminalRetentionEffectKey(event), time.Time{},
			)
			if err != nil {
				t.Fatalf("complete terminal without workflow outcome: %v", err)
			}
			if completed {
				t.Fatal("terminal completed without an exact workflow outcome receipt")
			}

			submission, err := repo.GetAgentDeliverySubmissionMetadata(context.Background(), event.SubmissionID)
			if err != nil {
				t.Fatalf("read partial submission: %v", err)
			}
			if submission.FullySettledAt != nil || submission.PayloadPrunedAt != nil || len(submission.Payload) != 0 {
				t.Fatalf("partial submission became retention-eligible: %+v", submission)
			}
			terminal, err := repo.GetAgentDeliveryEffect(context.Background(), terminalRetentionEffectKey(event))
			if err != nil || terminal.State != models.DeliveryEffectBlockResolved {
				t.Fatalf("terminal effect after incomplete settlement = %+v, %v", terminal, err)
			}
		})
	}
}

func TestDeliveryRetentionPrompt(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, event, advance := seedRetentionPromptSubmission(t, engine, "prompt")
			putRetentionWorkflowOutcome(t, repo, event, "not_required", retentionPromptTurnID(t, event))
			completed, err := repo.CompleteAgentDeliveryTerminalSettlement(
				context.Background(), terminalRetentionEffectKey(event), time.Time{},
			)
			if err != nil || !completed {
				t.Fatalf("complete fully proved terminal = %v, %v", completed, err)
			}
			advance(26 * time.Hour)
			limited, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{
				MaxCandidates: 1, MaxPayloadBytes: int64(len("composed prompt") - 1),
			})
			if err != nil {
				t.Fatalf("run byte-bounded settled-prompt retention: %v", err)
			}
			if limited.Candidates > 1 || limited.PromptPayloadsPruned != 0 || limited.PayloadBytesPruned != 0 {
				t.Fatalf("over-budget prompt batch exceeded its bounds: %+v", limited)
			}
			var lastBatch models.AgentDeliveryRetentionBatchResult
			for attempt := 0; attempt < 2; attempt++ {
				lastBatch, err = repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{
					MaxCandidates: 1,
				})
				if err != nil {
					t.Fatalf("run fair settled-prompt retention pass %d: %v", attempt+1, err)
				}
				if lastBatch.Candidates > 1 {
					t.Fatalf("prompt sweep exceeded its candidate bound: %+v", lastBatch)
				}
			}
			if lastBatch.PromptPayloadsPruned != 1 || lastBatch.PromptPayloadBytesPruned != int64(len("composed prompt")) ||
				lastBatch.PayloadBytesPruned != int64(len("composed prompt")) || lastBatch.PayloadBytesRead != 0 {
				t.Fatalf("settled prompt retention accounting = %+v", lastBatch)
			}
			var payload []byte
			var payloadPrunedAt sql.NullTime
			if err := repo.db.QueryRowx(repo.db.Rebind(`
				SELECT payload, payload_pruned_at FROM agent_delivery_submissions WHERE id = ?`), event.SubmissionID).Scan(
				&payload, &payloadPrunedAt,
			); err != nil {
				t.Fatalf("read compacted prompt payload: %v", err)
			}
			if len(payload) != 0 || !payloadPrunedAt.Valid {
				t.Fatalf("settled prompt payload still present after fair sweep: %d bytes, pruned=%v", len(payload), payloadPrunedAt)
			}

			submission, err := repo.GetAgentDeliverySubmissionMetadata(context.Background(), event.SubmissionID)
			if err != nil {
				t.Fatalf("read retained submission metadata: %v", err)
			}
			if submission.PayloadPrunedAt == nil || len(submission.Payload) != 0 || submission.PayloadBytes != int64(len([]byte("composed prompt"))) {
				t.Fatalf("settled prompt payload metadata = %+v", submission)
			}
			if submission.State != models.DeliverySubmissionCompleted || submission.Outcome != string(models.DeliverySubmissionCompleted) ||
				submission.TerminalStreamID != event.StreamID || submission.TerminalSequence != event.Sequence || submission.TerminalTurnID != retentionPromptTurnID(t, event) {
				t.Fatalf("prompt summary lost terminal identity: %+v", submission)
			}
		})
	}
}

func TestDeliveryRetentionIndependentBlocks(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, event, _ := seedRetentionPromptSubmission(t, engine, "independent")
			if err := repo.UpsertSessionRecoveryBlock(context.Background(), &models.SessionRecoveryBlock{
				ID: "operator-retention-block", SessionID: event.SessionID, IncarnationID: event.IncarnationID,
				ExpectedGeneration: event.HarnessGeneration, Reason: "native_state_missing", State: models.RecoveryBlockOpen,
				ConsumerReference: "operator",
			}); err != nil {
				t.Fatalf("create independent recovery block: %v", err)
			}
			putRetentionWorkflowOutcome(t, repo, event, "not_required", retentionPromptTurnID(t, event))
			completed, err := repo.CompleteAgentDeliveryTerminalSettlement(
				context.Background(), terminalRetentionEffectKey(event), time.Time{},
			)
			if err != nil || !completed {
				t.Fatalf("complete exact delivery settlement with independent block: %v, %v", completed, err)
			}
			block, err := repo.GetOpenSessionRecoveryBlock(context.Background(), event.SessionID, event.IncarnationID, event.HarnessGeneration)
			if err != nil || block == nil || block.ID != "operator-retention-block" || block.Reason != "native_state_missing" {
				t.Fatalf("independent recovery cause = %+v, %v", block, err)
			}
		})
	}
}

func TestDeliveryRetentionWorkflowOutcome(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, event, _ := seedRetentionPromptSubmission(t, engine, "workflow")
			putRetentionWorkflowOutcome(t, repo, event, "no_automatic_transition", "successor-turn")
			completed, err := repo.CompleteAgentDeliveryTerminalSettlement(
				context.Background(), terminalRetentionEffectKey(event), time.Time{},
			)
			if err != nil {
				t.Fatalf("complete terminal with mismatched workflow turn: %v", err)
			}
			if completed {
				t.Fatal("mismatched workflow receipt settled the terminal")
			}

			exactRepo, exactEvent, _ := seedRetentionPromptSubmission(t, engine, "workflow-exact")
			putRetentionWorkflowOutcome(t, exactRepo, exactEvent, "applied", retentionPromptTurnID(t, exactEvent))
			completed, err = exactRepo.CompleteAgentDeliveryTerminalSettlement(
				context.Background(), terminalRetentionEffectKey(exactEvent), time.Time{},
			)
			if err != nil || !completed {
				t.Fatalf("complete terminal with exact workflow outcome: %v, %v", completed, err)
			}
			submission, err := exactRepo.GetAgentDeliverySubmissionMetadata(context.Background(), exactEvent.SubmissionID)
			if err != nil || submission.FullySettledAt == nil {
				t.Fatalf("fully settled submission = %+v, %v", submission, err)
			}

			guardedRepo, guardedEvent, _ := seedRetentionPromptSubmission(t, engine, "workflow-guarded")
			stepTime := time.Date(2025, 6, 7, 8, 10, 0, 0, time.UTC)
			seedRetentionWorkflowPolicy(t, guardedRepo, guardedEvent, "workflow-guarded", "step-current", stepTime)
			guarded := retentionWorkflowOutcome(
				guardedEvent, retentionPromptTurnID(t, guardedEvent), "no_automatic_transition", "wf-workflow-guarded", "step-current", stepTime,
			)
			committed, err := guardedRepo.RecordAgentDeliveryWorkflowOutcome(
				context.Background(), retentionTaskIDForEvent(guardedEvent), guarded,
			)
			if err != nil || !committed {
				t.Fatalf("record exact workflow outcome: committed=%v, err=%v", committed, err)
			}
			staleRepo, staleEvent, _ := seedRetentionPromptSubmission(t, engine, "workflow-stale")
			seedRetentionWorkflowPolicy(t, staleRepo, staleEvent, "workflow-stale", "step-current", stepTime)
			if _, err := staleRepo.db.ExecContext(context.Background(), staleRepo.db.Rebind(
				`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`), "step-successor", retentionTaskIDForEvent(staleEvent)); err != nil {
				t.Fatalf("move task to successor step: %v", err)
			}
			stale := retentionWorkflowOutcome(
				staleEvent, retentionPromptTurnID(t, staleEvent), "no_automatic_transition", "wf-workflow-stale", "step-current", stepTime,
			)
			committed, err = staleRepo.RecordAgentDeliveryWorkflowOutcome(
				context.Background(), retentionTaskIDForEvent(staleEvent), stale,
			)
			if err != nil || committed {
				t.Fatalf("stale workflow source result = committed %v, err %v; want guarded no-op", committed, err)
			}
			if _, err := staleRepo.GetAgentDeliveryEffect(context.Background(), stale.EffectKey); !errors.Is(err, repoerrors.ErrAgentDeliveryEffectNotFound) {
				t.Fatalf("stale workflow source receipt lookup = %v, want not found", err)
			}
		})
	}
}

func TestDeliveryRetentionWorkflowOutcomeWriteFailure(t *testing.T) {
	repo, event, _ := seedRetentionPromptSubmission(t, "SQLite", "workflow-write-failure")
	stepTime := time.Date(2025, 6, 7, 8, 10, 0, 0, time.UTC)
	seedRetentionWorkflowPolicy(t, repo, event, "workflow-write-failure", "step-current", stepTime)
	if _, err := repo.db.ExecContext(context.Background(), `
		CREATE TRIGGER reject_delivery_workflow_outcome
		BEFORE INSERT ON agent_delivery_effects
		WHEN NEW.effect_type = 'workflow.on_turn_complete'
		BEGIN SELECT RAISE(ABORT, 'forced workflow receipt failure'); END`); err != nil {
		t.Fatalf("install workflow receipt failure trigger: %v", err)
	}
	effect := retentionWorkflowOutcome(
		event, retentionPromptTurnID(t, event), "not_required", "wf-workflow-write-failure", "step-current", stepTime,
	)
	committed, err := repo.RecordAgentDeliveryWorkflowOutcome(context.Background(), retentionTaskIDForEvent(event), effect)
	if err == nil || committed {
		t.Fatalf("failed workflow receipt write = committed %v, err %v; want error without commit", committed, err)
	}
	settled, err := repo.CompleteAgentDeliveryTerminalSettlement(context.Background(), terminalRetentionEffectKey(event), time.Time{})
	if err != nil || settled {
		t.Fatalf("terminal after failed workflow receipt = settled %v, err %v; want protected", settled, err)
	}
	submission, err := repo.GetAgentDeliverySubmissionMetadata(context.Background(), event.SubmissionID)
	if err != nil || submission.FullySettledAt != nil || submission.PayloadPrunedAt != nil {
		t.Fatalf("submission after failed workflow receipt = %+v, err %v; want retention protected", submission, err)
	}
}

func seedRetentionWorkflowPolicy(
	t *testing.T,
	repo *Repository,
	event models.AgentDeliveryEvent,
	suffix, stepID string,
	updatedAt time.Time,
) {
	t.Helper()
	ctx := context.Background()
	workflowID := "wf-" + suffix
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: workflowID, WorkspaceID: retentionTestWorkspaceID(t, repo), Name: "Retention workflow",
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("create retention workflow: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`), stepID, workflowID, "Current", 0, updatedAt, updatedAt); err != nil {
		t.Fatalf("create retention workflow step: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET workflow_id = ?, workflow_step_id = ? WHERE id = ?`),
		workflowID, stepID, retentionTaskIDForEvent(event)); err != nil {
		t.Fatalf("bind retention workflow source to task: %v", err)
	}
}

func retentionWorkflowOutcome(
	event models.AgentDeliveryEvent,
	turnID string,
	outcome, workflowID, stepID string,
	policyUpdatedAt time.Time,
) *models.AgentDeliveryEffect {
	completedAt := time.Date(2025, 6, 7, 8, 11, 0, 0, time.UTC)
	return &models.AgentDeliveryEffect{
		EffectKey:  "workflow.on_turn_complete:" + turnID,
		EffectType: "workflow.on_turn_complete", State: models.DeliveryEffectCompleted,
		TaskID: retentionTaskIDForEvent(event), WorkflowID: workflowID, WorkflowStepID: stepID,
		WorkflowPolicyUpdatedAt: &policyUpdatedAt,
		SessionID:               event.SessionID, TurnID: turnID,
		StreamID: event.StreamID, Sequence: event.Sequence,
		IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration,
		SubmissionID: event.SubmissionID, Outcome: outcome,
		CreatedAt: completedAt, CompletedAt: &completedAt,
	}
}

func retentionTaskIDForEvent(event models.AgentDeliveryEvent) string {
	return "retention-prompt-task-" + strings.TrimPrefix(event.SessionID, "retention-prompt-session-")
}

func seedRetentionPromptSubmission(
	t *testing.T,
	engine, suffix string,
) (*Repository, models.AgentDeliveryEvent, func(time.Duration)) {
	return seedRetentionPromptOutcome(t, engine, suffix, models.DeliverySubmissionCompleted)
}

func seedRetentionPromptOutcome(
	t *testing.T,
	engine, suffix string,
	outcome models.DeliverySubmissionState,
) (*Repository, models.AgentDeliveryEvent, func(time.Duration)) {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	ctx := context.Background()
	now := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)
	currentTime := now
	repo.clockNow = func() time.Time { return currentTime }
	taskID, sessionID := "retention-prompt-task-"+suffix, "retention-prompt-session-"+suffix
	incarnationID, submissionID := "retention-prompt-inc-"+suffix, "retention-prompt-sub-"+suffix
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Retention prompt",
	}); err != nil {
		t.Fatalf("create prompt task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
		State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("create prompt session: %v", err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "dispatch-" + suffix,
		PayloadHash: "prompt-hash-" + suffix, Payload: []byte("composed prompt"),
		State: models.DeliverySubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("prepare prompt submission: %v", err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "retention-delivery-block-" + suffix, SessionID: sessionID, IncarnationID: incarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("create delivery recovery block: %v", err)
	}
	data := map[string]any{}
	switch outcome {
	case models.DeliverySubmissionCancelled:
		data["stop_reason"] = "cancelled"
	case models.DeliverySubmissionFailed:
		data["is_error"] = true
	}
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "retention-turn-" + suffix, Data: data})
	if err != nil {
		t.Fatalf("marshal terminal event: %v", err)
	}
	event := models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 1,
		StreamID: "retention-prompt-stream-" + suffix, Sequence: 1, SubmissionID: submissionID,
		EventType: streams.EventTypeComplete, Payload: payload, Terminal: true, ReceivedAt: now,
	}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 1); err != nil || !inserted {
		t.Fatalf("receive terminal event = %v, %v", inserted, err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, &event, nil); err != nil {
		t.Fatalf("project terminal event: %v", err)
	}
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, outcome, now)
	if err != nil || !settled {
		t.Fatalf("settle terminal outcome = %v, %v", settled, err)
	}
	return repo, event, func(delta time.Duration) { currentTime = now.Add(delta) }
}

func putRetentionWorkflowOutcome(t *testing.T, repo *Repository, event models.AgentDeliveryEvent, outcome, turnID string) {
	t.Helper()
	completedAt := repo.nowUTC()
	if _, err := repo.PutAgentDeliveryEffect(context.Background(), &models.AgentDeliveryEffect{
		EffectKey: "workflow.on_turn_complete:" + retentionPromptTurnID(t, event),
		StreamID:  event.StreamID, Sequence: event.Sequence, EffectType: "workflow.on_turn_complete",
		State: models.DeliveryEffectCompleted, SessionID: event.SessionID, IncarnationID: event.IncarnationID,
		HarnessGeneration: event.HarnessGeneration, SubmissionID: event.SubmissionID,
		TurnID: turnID, Outcome: outcome, CreatedAt: completedAt, CompletedAt: &completedAt,
	}); err != nil {
		t.Fatalf("write workflow outcome: %v", err)
	}
}

func terminalRetentionEffectKey(event models.AgentDeliveryEvent) string {
	return "agent_delivery.terminal:" + event.StreamID + ":1"
}

func retentionPromptTurnID(t *testing.T, event models.AgentDeliveryEvent) string {
	t.Helper()
	var payload streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode terminal event turn identity: %v", err)
	}
	return payload.TurnID
}
