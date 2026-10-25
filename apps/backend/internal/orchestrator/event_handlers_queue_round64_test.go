package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type lifecycleRetryRoutingRepository struct {
	messagequeue.Repository
	lifecycleRetries int
	ordinaryRetries  int
}

func (r *lifecycleRetryRoutingRepository) RequeuePreservingFIFO(ctx context.Context, msg *messagequeue.QueuedMessage) error {
	r.ordinaryRetries++
	return r.Repository.RequeuePreservingFIFO(ctx, msg)
}

func (r *lifecycleRetryRoutingRepository) InsertOrReplaceLifecycleByCoalesceKey(
	ctx context.Context,
	msg *messagequeue.QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*messagequeue.QueuedMessage, bool, error) {
	r.lifecycleRetries++
	return r.Repository.InsertOrReplaceLifecycleByCoalesceKey(ctx, msg, coalesceKey, maxPerSession, allowInsert)
}

func TestRequeueDurableLifecycleMarkerWithoutOriginUsesLifecyclePath(t *testing.T) {
	ctx := context.Background()
	repo := &lifecycleRetryRoutingRepository{Repository: messagequeue.NewMemoryRepository()}
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}

	queued, _, accepted, err := queue.QueueLifecycleMessageWithCoalesceKey(
		ctx,
		"session-round64",
		"task-round64",
		"durable prompt",
		"",
		messagequeue.QueuedByWorkflow,
		false,
		nil,
		map[string]interface{}{},
		"lifecycle:round64",
		true,
	)
	if err != nil || !accepted || queued == nil {
		t.Fatalf("queue durable lifecycle message: accepted=%t entry=%v err=%v", accepted, queued, err)
	}

	reserved, dispatched, autoRun := queue.ReserveQueuedWithAutoRun(ctx, "session-round64")
	if !autoRun || !dispatched || reserved == nil {
		t.Fatalf("reserve durable lifecycle message: auto_run=%t dispatched=%t message=%v", autoRun, dispatched, reserved)
	}
	if !reserved.IsReservedLifecycleDelivery() {
		t.Fatal("reserved lifecycle message did not carry delivery reservation")
	}

	// Legacy durable rows can have the durable marker without a provider origin.
	// Their retry must retain lifecycle generation and inactive-task fencing,
	// rather than taking the ordinary user FIFO retry path.
	repo.lifecycleRetries = 0
	repo.ordinaryRetries = 0
	svc.requeueMessage(ctx, reserved, reserved.QueuedBy)

	if repo.lifecycleRetries != 1 {
		t.Fatalf("lifecycle retry repository calls = %d, want 1", repo.lifecycleRetries)
	}
	if repo.ordinaryRetries != 0 {
		t.Fatalf("ordinary retry repository calls = %d, want 0", repo.ordinaryRetries)
	}
}

func TestLifecycleManualRecoveryReplacesReservedRow(t *testing.T) {
	ctx := context.Background()
	repo := messagequeue.NewMemoryRepository()
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}

	queued, _, accepted, err := queue.QueueLifecycleMessageWithCoalesceKey(
		ctx,
		"session-round64-recovery",
		"task-round64-recovery",
		"durable prompt",
		"",
		messagequeue.QueuedByWorkflow,
		false,
		nil,
		map[string]interface{}{"origin": githubPRAutomationOrigin},
		"lifecycle:round64:recovery",
		true,
	)
	if err != nil || !accepted || queued == nil {
		t.Fatalf("queue durable lifecycle message: accepted=%t entry=%v err=%v", accepted, queued, err)
	}
	reserved, dispatched, autoRun := queue.ReserveQueuedWithAutoRun(ctx, "session-round64-recovery")
	if !autoRun || !dispatched || reserved == nil {
		t.Fatalf("reserve durable lifecycle message: auto_run=%t dispatched=%t message=%v", autoRun, dispatched, reserved)
	}

	svc.handleQueuedMessageExecutionError(
		ctx,
		"session-round64-recovery",
		reserved,
		nil,
		true,
		false,
		errors.New("UsageLimitExceeded"),
	)

	entries, err := repo.ListBySession(ctx, "session-round64-recovery")
	if err != nil {
		t.Fatalf("list recovered lifecycle message: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("recovered lifecycle entries = %d, want exactly one", len(entries))
	}
	if entries[0].IsReservedInFlight() {
		t.Fatal("recovered lifecycle message remained reserved")
	}
}

func TestQueuedMessageExecutionError_RequeuesSeam3Refusal(t *testing.T) {
	ctx := context.Background()
	repo := messagequeue.NewMemoryRepository()
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}

	queued, err := queue.QueueMessage(
		ctx, "session-seam3-retry", "task-seam3-retry", "retry me", "", messagequeue.QueuedByAgent, false, nil,
	)
	if err != nil {
		t.Fatalf("queue message: %v", err)
	}
	reserved, dispatched, autoRun := queue.ReserveQueuedWithAutoRun(ctx, queued.SessionID)
	if !autoRun || !dispatched || reserved == nil {
		t.Fatalf("reserve message: auto_run=%t dispatched=%t message=%v", autoRun, dispatched, reserved)
	}

	svc.handleQueuedMessageExecutionError(
		ctx, queued.SessionID, reserved, nil, false, false,
		&seam3Refusal{reasonCode: ceilingReasonRefused},
	)

	entries, err := repo.ListBySession(ctx, queued.SessionID)
	if err != nil {
		t.Fatalf("list requeued messages: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != queued.ID || entries[0].IsReservedInFlight() {
		t.Fatalf("seam-3 refusal queue state = %#v, want one unreserved original message", entries)
	}
}

// reserveQueuedExecutionErrorMessage queues and reserves one agent message so a
// test can hand it to handleQueuedMessageExecutionError like a failed dispatch.
func reserveQueuedExecutionErrorMessage(
	t *testing.T, queue *messagequeue.Service, sessionID, taskID string,
) (*messagequeue.QueuedMessage, *messagequeue.QueuedMessage) {
	t.Helper()
	ctx := context.Background()
	queued, err := queue.QueueMessage(ctx, sessionID, taskID, "keep me", "", messagequeue.QueuedByAgent, false, nil)
	if err != nil {
		t.Fatalf("queue message: %v", err)
	}
	reserved, dispatched, autoRun := queue.ReserveQueuedWithAutoRun(ctx, queued.SessionID)
	if !autoRun || !dispatched || reserved == nil {
		t.Fatalf("reserve message: auto_run=%t dispatched=%t message=%v", autoRun, dispatched, reserved)
	}
	return queued, reserved
}

// AC-TASKS-RESUME-PROMPT-QUEUE-002.1: a dispatch that failed because a bounded
// wait for the session runtime expired keeps the message at the queue head.
func TestQueuedMessageExecutionError_RequeuesExpiredRuntimeWait(t *testing.T) {
	ctx := context.Background()
	repo := messagequeue.NewMemoryRepository()
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}
	queued, reserved := reserveQueuedExecutionErrorMessage(t, queue, "session-wait-expired", "task-wait-expired")

	// Drive the real readiness-wait boundary with an expired context.
	waitCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	expired := svc.waitForSessionReady(waitCtx, queued.SessionID)
	waitErr := svc.classifyEnsureSessionRunningFailureForPrompt(
		ctx, queued.TaskID, queued.SessionID, "keep me", "", false, nil, false,
		promptTaskOptions{}, expired,
	)
	svc.handleQueuedMessageExecutionError(ctx, queued.SessionID, reserved, nil, false, false, waitErr)

	entries, err := repo.ListBySession(ctx, queued.SessionID)
	if err != nil {
		t.Fatalf("list requeued messages: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != queued.ID || entries[0].IsReservedInFlight() {
		t.Fatalf("expired runtime wait left %d queue entries, want one unreserved original message", len(entries))
	}
	if entries[0].SessionID != queued.SessionID || entries[0].TaskID != queued.TaskID ||
		entries[0].Content != queued.Content || !entries[0].QueuedAt.Equal(queued.QueuedAt) {
		t.Fatal("runtime wait changed the original queue target, payload, or FIFO timestamp")
	}
}

// AC-TASKS-RESUME-PROMPT-QUEUE-002.3: a failure that is not retryable keeps its
// existing disposition and does not stay at the queue head.
func TestQueuedMessageExecutionError_DropsNonRetryableFailure(t *testing.T) {
	ctx := context.Background()
	repo := messagequeue.NewMemoryRepository()
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}
	queued, reserved := reserveQueuedExecutionErrorMessage(t, queue, "session-permanent", "task-permanent")

	svc.handleQueuedMessageExecutionError(
		ctx, queued.SessionID, reserved, nil, false, false, errors.New("provider rejected the prompt"),
	)

	entries, err := repo.ListBySession(ctx, queued.SessionID)
	if err != nil {
		t.Fatalf("list queued messages: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("non-retryable failure left %d queue entries, want none", len(entries))
	}
}

// AC-TASKS-RESUME-PROMPT-QUEUE-002.3: a deadline raised outside the bounded
// runtime wait (a provider or downstream RPC timeout) is not a wait expiry, so
// it keeps its disposition instead of blocking the head of the queue.
func TestQueuedMessageExecutionError_DropsForeignDeadline(t *testing.T) {
	ctx := context.Background()
	repo := messagequeue.NewMemoryRepository()
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}
	queued, reserved := reserveQueuedExecutionErrorMessage(t, queue, "session-provider-deadline", "task-provider-deadline")

	providerErr := fmt.Errorf("provider stream read: %w", context.DeadlineExceeded)
	svc.handleQueuedMessageExecutionError(ctx, queued.SessionID, reserved, nil, false, false, providerErr)

	entries, err := repo.ListBySession(ctx, queued.SessionID)
	if err != nil {
		t.Fatalf("list queued messages: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("a foreign deadline left %d queue entries, want none", len(entries))
	}
}

func TestReuseSessionRestoresPrimaryWhenQueueTransferFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-round64-reuse", "session-round64-current", "step-round64")
	current, err := repo.GetTaskSession(ctx, "session-round64-current")
	if err != nil {
		t.Fatal(err)
	}
	current.State = models.TaskSessionStateRunning
	current.IsPrimary = true
	if err := repo.UpdateTaskSession(ctx, current); err != nil {
		t.Fatal(err)
	}
	existing := &models.TaskSession{
		ID:             "session-round64-existing",
		TaskID:         current.TaskID,
		State:          models.TaskSessionStateWaitingForInput,
		AgentProfileID: "profile-round64",
		StartedAt:      current.StartedAt.Add(time.Second),
		UpdatedAt:      current.UpdatedAt.Add(time.Second),
	}
	if err := repo.CreateTaskSession(ctx, existing); err != nil {
		t.Fatal(err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	transferErr := errors.New("queue transfer failed")
	svc.messageQueue = messagequeue.NewService(
		&transferFailureQueueRepository{Repository: messagequeue.NewMemoryRepository(), err: transferErr},
		messagequeue.DefaultMaxPerSession,
		testLogger(),
	)
	if _, err := svc.messageQueue.QueueMessage(
		ctx,
		current.ID,
		current.TaskID,
		"handoff",
		"",
		messagequeue.QueuedByUser,
		false,
		nil,
	); err != nil {
		t.Fatal(err)
	}

	_, err = svc.reuseSessionForStepWithEndPolicy(
		ctx, current.TaskID, current, existing, models.WorkflowProfileSessionEndPolicyComplete,
	)
	if !errors.Is(err, transferErr) {
		t.Fatalf("reuseSessionForStep error = %v, want queue transfer failure", err)
	}

	sessions, err := repo.ListTaskSessions(ctx, current.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		switch session.ID {
		case current.ID:
			if !session.IsPrimary || session.State != models.TaskSessionStateRunning {
				t.Fatalf("current session after failed transfer = primary %t state %q, want primary running", session.IsPrimary, session.State)
			}
		case existing.ID:
			if session.IsPrimary {
				t.Fatal("reused session remained primary after failed queue transfer")
			}
		}
	}
}

func TestQueuedMessageExecutionErrorDropsClassifiedProviderDeadline(t *testing.T) {
	ctx := t.Context()
	repo := messagequeue.NewMemoryRepository()
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger())
	svc := &Service{messageQueue: queue, logger: testLogger()}
	queued, reserved := reserveQueuedExecutionErrorMessage(t, queue, "session-nested-deadline", "task-nested-deadline")
	providerErr := fmt.Errorf("failed to resume session: provider handshake: %w", context.DeadlineExceeded)
	classified := svc.classifyEnsureSessionRunningFailureForPrompt(
		ctx, queued.TaskID, queued.SessionID, "keep me", "", false, nil, false, promptTaskOptions{}, providerErr,
	)
	svc.handleQueuedMessageExecutionError(ctx, queued.SessionID, reserved, nil, false, false, classified)
	entries, err := repo.ListBySession(ctx, queued.SessionID)
	if err != nil {
		t.Fatalf("list queued messages: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("a classified provider deadline left %d queue entries, want none", len(entries))
	}
}
