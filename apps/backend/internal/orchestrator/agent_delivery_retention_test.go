package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type retentionPromptAgentManager struct {
	*durableDeliveryTestAgentManager
	submissionIDs []string
}

func (m *retentionPromptAgentManager) PromptAgentWithAdmissionCallbackAndSubmissionID(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	beforeAdmission func() error,
	onDispatched func(),
	submissionID string,
) (*executor.PromptResult, error) {
	m.submissionIDs = append(m.submissionIDs, submissionID)
	return m.PromptAgentWithAdmissionCallback(
		ctx, executionID, prompt, attachments, dispatchOnly, beforeAdmission, onDispatched,
	)
}

func (m *retentionPromptAgentManager) PromptAgentWithDispatchCallbackAndSubmissionID(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
	submissionID string,
) (*executor.PromptResult, error) {
	m.submissionIDs = append(m.submissionIDs, submissionID)
	return m.PromptAgentWithDispatchCallback(
		ctx, executionID, prompt, attachments, dispatchOnly, onDispatched,
	)
}

func TestDeliveryRetentionNoResend(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "retention-no-resend-task"
		sessionID = "retention-no-resend-session"
		execution = "retention-no-resend-execution"
		oldID     = "retention-no-resend-old"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, execution)
	baseManager := &mockAgentManager{isAgentRunning: true}
	baseManager.repoForExecutionLookup = repo
	var initializeCalls int
	baseManager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		initializeCalls++
		return &executor.LaunchAgentResponse{AgentExecutionID: execution}, nil
	}
	manager := &retentionPromptAgentManager{
		durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
			mockAgentManager: baseManager,
			advertised:       true,
			capability: agentruntime.DurableDeliveryCapability{
				Version: journal.CurrentVersion, Durable: true,
			},
		},
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	service.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	// The distinct prompt is a positive control: the same live session still
	// reaches the harness when the submission identity has not been consumed.
	if _, err := service.PromptTaskWithDeliverySubmissionID(
		ctx, taskID, sessionID, "fresh control prompt", "", false, nil, false, "retention-control",
	); err != nil {
		t.Fatalf("dispatch fresh control prompt: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("settle control prompt session: %v", err)
	}
	if _, err := service.PromptTaskWithDeliverySubmissionID(
		ctx, taskID, sessionID, "previous accepted prompt", "", false, nil, false, oldID,
	); err != nil {
		t.Fatalf("dispatch original prompt: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("settle original prompt session: %v", err)
	}
	oldSubmission, err := repo.GetAgentDeliverySubmission(ctx, "prompt:"+oldID)
	if err != nil || oldSubmission.State != models.DeliverySubmissionCompleted {
		t.Fatalf("original submission = %+v, err %v; want completed", oldSubmission, err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE agent_delivery_submissions
		SET payload = ?, payload_pruned_at = ?, payload_bytes = ?
		WHERE id = ?`, []byte{}, time.Now().UTC(), oldSubmission.PayloadBytes, oldSubmission.ID); err != nil {
		t.Fatalf("prune completed prompt payload: %v", err)
	}
	messagesBefore, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessionBefore, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	taskBefore, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.PromptTaskWithDeliverySubmissionID(
		ctx, taskID, sessionID, "previous accepted prompt", "", false, nil, false, oldID,
	)
	if !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("replay pruned completed prompt error = %v, want reconciliation block", err)
	}
	if got := len(baseManager.capturedPrompts); got != 2 {
		t.Fatalf("harness prompt sends = %d, want 2 before and after replay", got)
	}
	if len(manager.submissionIDs) != 2 || manager.submissionIDs[0] != "prompt:retention-control" || manager.submissionIDs[1] != "prompt:"+oldID {
		t.Fatalf("dispatched submission identities = %v, want only the two original dispatches", manager.submissionIDs)
	}
	if initializeCalls != 0 {
		t.Fatalf("ACP initialization calls = %d, want 0 for an already-live execution", initializeCalls)
	}
	messagesAfter, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessionAfter, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	taskAfter, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messagesAfter) != len(messagesBefore) {
		t.Fatalf("canonical message count changed from %d to %d after replay", len(messagesBefore), len(messagesAfter))
	}
	if sessionAfter.State != sessionBefore.State || taskAfter.WorkflowStepID != taskBefore.WorkflowStepID {
		t.Fatalf("replay changed session/workflow state: session %q -> %q, step %q -> %q",
			sessionBefore.State, sessionAfter.State, taskBefore.WorkflowStepID, taskAfter.WorkflowStepID)
	}
	if _, err := repo.GetAgentDeliverySubmission(ctx, oldSubmission.ID); !errors.Is(err, repoerrors.ErrAgentDeliveryPayloadPruned) {
		t.Fatalf("full prompt read error = %v, want pruned-payload error", err)
	}
	if current, err := repo.GetAgentDeliverySubmissionMetadata(ctx, oldSubmission.ID); err != nil || current.State != models.DeliverySubmissionCompleted {
		t.Fatalf("completed prompt metadata = %+v, err %v", current, err)
	}
}
