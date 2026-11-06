package handlers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// capacityBlockedOrchestrator reports a full ceiling and records the accepted
// queue head without dispatching.
type capacityBlockedOrchestrator struct {
	*switchingTurnStartOrchestrator

	mu                sync.Mutex
	registeredTaskID  string
	registeredSession string
	registeredEntryID string
	notifyCalls       int
}

func (*capacityBlockedOrchestrator) ReserveMessageTurn(
	context.Context, string, string, string,
) (*orchestrator.MessageTurnReservation, bool) {
	return nil, false
}

func (o *capacityBlockedOrchestrator) RegisterMessageCapacityWait(
	_ context.Context, taskID, sessionID, entryID string,
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.registeredTaskID = taskID
	o.registeredSession = sessionID
	o.registeredEntryID = entryID
}

func (o *capacityBlockedOrchestrator) NotifyQueuedUserPrompt(context.Context, string, string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.notifyCalls++
}

func (*capacityBlockedOrchestrator) ProcessOnTurnStart(
	context.Context, string, string,
) (orchestrator.ProcessOnTurnStartResult, error) {
	return orchestrator.ProcessOnTurnStartResult{}, nil
}

func (r *messageAddSwitchRepo) CreateMessageAndQueueOnce(
	_ context.Context, message *models.Message, queued *messagequeue.QueuedMessage, _ int,
) (bool, error) {
	r.messagesMu.Lock()
	defer r.messagesMu.Unlock()
	r.messages = append(r.messages, message)
	clone := *queued
	r.queuedMessage = &clone
	return true, nil
}

// TestMessageCapacityAdmission_QueuesOrdinaryMessageAtFullCapacity verifies
// the direct request acceptance boundary: the handler reserves or persists
// before returning success, and no asynchronous prompt is dispatched.
func TestMessageCapacityAdmission_QueuesOrdinaryMessageAtFullCapacity(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: v1.TaskStateInProgress, UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {
				ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput,
				AgentProfileID: "profile-1", QueueIncarnationID: "inc-1", UpdatedAt: now,
			},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &capacityBlockedOrchestrator{
		switchingTurnStartOrchestrator: &switchingTurnStartOrchestrator{repo: repo},
	}
	h := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-cap-1", ws.ActionMessageAdd, map[string]interface{}{
		"task_id":           "t1",
		"session_id":        "s1",
		"client_message_id": "client-cap-1",
		"content":           "wait for capacity",
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Equal(t, 1, repo.messageCount())
	require.NotNil(t, repo.queuedMessage, "accepted input must persist a queue receipt before success")
	require.Equal(t, "client-cap-1", repo.queuedMessage.ID)
	require.Equal(t, "wait for capacity", repo.queuedMessage.Content)
	require.Equal(t, true, repo.queuedMessage.Metadata["user_message_recorded"])
	require.Equal(t, true, repo.queuedMessage.Metadata[orchestrator.MetaKeyTurnStartAlreadyProcessed])

	orch.mu.Lock()
	require.Equal(t, "t1", orch.registeredTaskID)
	require.Equal(t, "s1", orch.registeredSession)
	require.Equal(t, "client-cap-1", orch.registeredEntryID)
	require.Zero(t, orch.notifyCalls, "a full ceiling must not immediately start the queued prompt")
	orch.mu.Unlock()
	require.Empty(t, orch.getForwardedSession(), "no prompt may dispatch after a capacity refusal")
}

type steerCapacityOrchestrator struct {
	*switchingTurnStartOrchestrator

	mu                  sync.Mutex
	promptCalls         int
	queueCalls          int
	queuedUserRecorded  bool
	queuedMetadata      map[string]interface{}
	registeredTaskID    string
	registeredSessionID string
	registeredEntryID   string
}

func (o *steerCapacityOrchestrator) SteerTask(
	context.Context, string, string, string, string, bool, []v1.MessageAttachment,
) (*orchestrator.PromptResult, error) {
	return nil, orchestrator.ErrSteerNotEligible
}

func (o *steerCapacityOrchestrator) PromptTask(
	context.Context, string, string, string, string, bool, []v1.MessageAttachment, bool,
) (*orchestrator.PromptResult, error) {
	o.mu.Lock()
	o.promptCalls++
	o.mu.Unlock()
	return nil, orchestrator.ErrMessageTurnCapacityExceeded
}

func (o *steerCapacityOrchestrator) QueueUserPrompt(
	_ context.Context, _, _, _, _ string, _ bool, _ []v1.MessageAttachment,
	metadata map[string]interface{}, userMessageRecorded bool,
) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.queueCalls++
	o.queuedUserRecorded = userMessageRecorded
	o.queuedMetadata = metadata
	return nil
}

func (o *steerCapacityOrchestrator) RegisterMessageCapacityWait(
	_ context.Context, taskID, sessionID, entryID string,
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.registeredTaskID = taskID
	o.registeredSessionID = sessionID
	o.registeredEntryID = entryID
}

func TestMessageCapacityAdmission_SteerFallbackQueuesInsteadOfFailing(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: v1.TaskStateInProgress, UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {
				ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput,
				AgentProfileID: "profile-1", QueueIncarnationID: "inc-1", UpdatedAt: now,
			},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &steerCapacityOrchestrator{
		switchingTurnStartOrchestrator: &switchingTurnStartOrchestrator{repo: repo},
	}
	h := NewMessageHandlers(svc, orch, log)

	h.forwardMessageAsSteer(context.Background(), "t1", "s1", "steer fallback", "", false, nil, "message-1")

	orch.mu.Lock()
	defer orch.mu.Unlock()
	require.Equal(t, 1, orch.promptCalls)
	require.Equal(t, 1, orch.queueCalls, "the capacity refusal must become durable pending input")
	require.True(t, orch.queuedUserRecorded)
	require.Equal(t, true, orch.queuedMetadata["user_message_recorded"])
	require.Equal(t, true, orch.queuedMetadata[orchestrator.MetaKeyTurnStartAlreadyProcessed])
	require.Equal(t, "message-1", orch.queuedMetadata[messagequeue.MetadataDurableTranscriptMessageID])
	require.Equal(t, "t1", orch.registeredTaskID)
	require.Equal(t, "s1", orch.registeredSessionID)
	require.Zero(t, repo.messageCount(), "the fallback must not create a duplicate transcript or failure message")
}
