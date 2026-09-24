package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestMessageTaskReadiness_ReadyBeforeAdmission(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)

	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""))

	result, err := h.dispatchTaskMessage(
		ctx,
		target.ID,
		session,
		"wake after stale state",
		map[string]interface{}{"sender_task_id": sender.ID},
		false,
		false,
	)

	require.NoError(t, err)
	assert.Equal(t, taskMessageStatusQueued, result.status)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, session.ID).Count)
	require.Len(t, orch.readinessCalls, 1)
	assert.Equal(t, identity, orch.readinessCalls[0])
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestMessageTaskReadiness_PreparedBusySession(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)

	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	result, err := h.dispatchPreparedTaskMessage(
		ctx,
		target.ID,
		session,
		"prepared branch",
		map[string]interface{}{"sender_task_id": "sender"},
	)

	require.NoError(t, err)
	assert.Equal(t, taskMessageStatusQueued, result.status)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, session.ID).Count)
	require.Len(t, orch.readinessCalls, 1)
	assert.Equal(t, identity, orch.readinessCalls[0])
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestMessageTaskReadiness_RejectedAdmission(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.queue.SetMaxPerSession(1)
	orch.queue.SetMergeEnabled(false)

	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	_, err = orch.queue.QueueMessageWithMetadataForSession(
		ctx, identity, "existing work", "", messagequeue.QueuedByAgent, false, nil, nil,
	)
	require.NoError(t, err)

	_, err = h.dispatchTaskMessage(
		ctx,
		target.ID,
		session,
		"rejected work",
		map[string]interface{}{"sender_task_id": "sender"},
		false,
		false,
	)

	var queueErr *queueFullDispatchError
	require.ErrorAs(t, err, &queueErr)
	assert.Empty(t, orch.readinessCalls)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, session.ID).Count)
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestMessageTaskReadiness_UnrelatedSender(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "queued peer message", sender.ID))
	resp, err := h.handleMessageTask(context.Background(), msg)

	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, taskMessageStatusQueued, payload[stopTaskStatusKey])
	assert.Empty(t, orch.interruptCalls)
	require.Len(t, orch.readinessCalls, 1)
	assert.Equal(t, session.ID, orch.readinessCalls[0].SessionID)
	assert.Equal(t, target.ID, orch.readinessCalls[0].TaskID)
	entry := orch.queue.GetStatus(context.Background(), session.ID).Entries[0]
	assert.Equal(t, sender.ID, entry.Metadata["sender_task_id"])
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestMessageTaskReadiness_Delivery(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	require.NoError(t, repo.UpdateTaskState(ctx, target.ID, v1.TaskStateInProgress))

	accepted := make(chan mcpReadinessAcceptedPrompt, 1)
	completed := make(chan struct{})
	queueRepo := &mcpReadinessQueueRepository{
		Repository: messagequeue.NewMemoryRepositoryWithAuthority(
			func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
				current, err := repo.GetTaskSession(ctx, sessionID)
				if err != nil {
					return messagequeue.QueueSessionIdentity{}, err
				}
				if current == nil || current.TaskID != taskID || current.QueueIncarnationID == "" {
					return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
				}
				return messagequeue.QueueSessionIdentity{
					TaskID: taskID, SessionID: sessionID,
					SessionIncarnationID: current.QueueIncarnationID,
				}, nil
			},
		),
		beforeFirstResolve: func() {
			require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""))
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID:               "exec-row-" + session.ID,
				SessionID:        session.ID,
				TaskID:           target.ID,
				Status:           "running",
				Resumable:        true,
				AgentExecutionID: "exec-" + session.ID,
			}))
		},
		completed: completed,
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger(t))
	agentManager := &mcpReadinessAgentManager{accepted: accepted}
	log := testLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	realOrchestrator := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(),
		eventBus,
		agentManager,
		&mcpReadinessSchedulerTaskRepo{repo: repo},
		repo,
		nil,
		nil,
		queue,
		log,
	)
	h := &Handlers{
		taskSvc:         svc,
		sessionRepo:     repo,
		sessionLauncher: realOrchestrator,
		logger:          log.WithFields(),
	}

	resp, err := h.handleMessageTask(ctx, makeWSMessage(
		t,
		ws.ActionMCPMessageTask,
		senderPayload(target.ID, "deliver through the orchestrator", sender.ID),
	))
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, taskMessageStatusQueued, payload[stopTaskStatusKey])

	select {
	case prompt := <-accepted:
		assert.Equal(t, "exec-sess-1", prompt.ExecutionID)
		assert.Contains(t, prompt.Prompt, "deliver through the orchestrator")
		assert.Contains(t, prompt.Prompt, "<kandev-system>")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider acceptance")
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queued delivery cleanup")
	}
	assert.Equal(t, 0, queue.GetStatus(ctx, session.ID).Count)
}

type mcpReadinessAcceptedPrompt struct {
	ExecutionID string
	Prompt      string
}

type mcpReadinessAgentManager struct {
	executor.AgentManagerClient
	accepted chan<- mcpReadinessAcceptedPrompt
}

func (m *mcpReadinessAgentManager) IsAgentRunningForSession(context.Context, string) bool {
	return true
}

func (m *mcpReadinessAgentManager) IsAgentReadyForPrompt(context.Context, string) bool {
	return true
}

func (m *mcpReadinessAgentManager) IsPassthroughSession(context.Context, string) bool {
	return false
}

func (m *mcpReadinessAgentManager) GetExecutionIDForSession(context.Context, string) (string, error) {
	return "exec-sess-1", nil
}

func (m *mcpReadinessAgentManager) PromptAgent(
	_ context.Context,
	_ string,
	_ string,
	_ []v1.MessageAttachment,
	_ bool,
) (*executor.PromptResult, error) {
	return &executor.PromptResult{}, nil
}

func (m *mcpReadinessAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	_ bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	if onDispatched != nil {
		onDispatched()
	}
	select {
	case m.accepted <- mcpReadinessAcceptedPrompt{ExecutionID: executionID, Prompt: prompt}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &executor.PromptResult{}, nil
}

type mcpReadinessQueueRepository struct {
	messagequeue.Repository
	beforeFirstResolve func()
	beforeResolveOnce  sync.Once
	completed          chan<- struct{}
	completedOnce      sync.Once
}

func (r *mcpReadinessQueueRepository) ResolveSessionIdentity(
	ctx context.Context,
	taskID, sessionID string,
) (messagequeue.QueueSessionIdentity, error) {
	identity, err := r.Repository.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err == nil && r.beforeFirstResolve != nil {
		r.beforeResolveOnce.Do(r.beforeFirstResolve)
	}
	return identity, err
}

func (r *mcpReadinessQueueRepository) ListPendingQueueDispatches(
	context.Context,
) ([]messagequeue.PendingQueueDispatch, error) {
	return nil, nil
}

func (r *mcpReadinessQueueRepository) MarkPendingQueueDispatchAccepted(
	context.Context,
	*messagequeue.QueuedMessage,
) error {
	return nil
}

func (r *mcpReadinessQueueRepository) DeletePendingQueueDispatch(
	ctx context.Context,
	message *messagequeue.QueuedMessage,
) error {
	if delegate, ok := r.Repository.(interface {
		DeletePendingQueueDispatch(context.Context, *messagequeue.QueuedMessage) error
	}); ok {
		if err := delegate.DeletePendingQueueDispatch(ctx, message); err != nil {
			return err
		}
	}
	if r.completed != nil {
		r.completedOnce.Do(func() { close(r.completed) })
	}
	return nil
}

type mcpReadinessSchedulerTaskRepo struct {
	repo *sqliterepo.Repository
}

var _ scheduler.TaskRepository = (*mcpReadinessSchedulerTaskRepo)(nil)

func (r *mcpReadinessSchedulerTaskRepo) GetTask(
	ctx context.Context,
	taskID string,
) (*v1.Task, error) {
	task, err := r.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

func (r *mcpReadinessSchedulerTaskRepo) UpdateTaskState(
	ctx context.Context,
	taskID string,
	state v1.TaskState,
) error {
	return r.repo.UpdateTaskState(ctx, taskID, state)
}

func (r *mcpReadinessSchedulerTaskRepo) UpdateTaskStateIfCurrentIn(
	ctx context.Context,
	taskID string,
	state v1.TaskState,
	allowed []v1.TaskState,
) (bool, error) {
	_, updated, err := r.repo.UpdateTaskStateIfCurrentIn(ctx, taskID, state, allowed)
	return updated, err
}

func (r *mcpReadinessSchedulerTaskRepo) UpdateTaskStateIfNotArchived(
	ctx context.Context,
	taskID string,
	state v1.TaskState,
) (bool, error) {
	_, updated, err := r.repo.UpdateTaskStateIfNotArchived(ctx, taskID, state)
	return updated, err
}

func (r *mcpReadinessSchedulerTaskRepo) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	_, updated, err := r.repo.UpdateTaskStateIfSessionState(ctx, taskID, sessionID, expectedSessionState, state)
	return updated, err
}
