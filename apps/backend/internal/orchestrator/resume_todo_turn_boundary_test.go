package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestResumeTodosPersistOutOfTurnWithoutOpenTurn(t *testing.T) {
	ctx := context.Background()
	fixture := newResumeTodoFixture(t)
	entries := []streams.PlanEntry{{Description: "Review the changes", Status: "in_progress"}}

	fixture.svc.handleSessionTodosEvent(ctx, resumeTodosPayload(fixture, entries))
	fixture.svc.handleSessionStatusEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    fixture.taskID,
		SessionID: fixture.workerID,
		Data: &lifecycle.AgentStreamEventData{
			Type:          "session_status",
			SessionStatus: streams.SessionStatusResumed,
		},
	})

	require.Zero(t, openTurnCount(t, fixture.repo, fixture.workerID),
		"recovery metadata must not create a promptless worker turn")
	messages, err := fixture.repo.ListMessages(ctx, fixture.workerID)
	require.NoError(t, err)
	require.Len(t, messages, 2, "the changed recovery snapshot must be durable alongside history")
	require.Equal(t, fixture.previousTodoID, messages[0].ID)
	require.Equal(t, fixture.previousTurn.ID, messages[0].TurnID)
	replayed := messages[1]
	require.NotEqual(t, fixture.previousTodoID, replayed.ID)
	require.Equal(t, models.MessageTypeTodo, replayed.Type)
	requireTodoEntries(t, replayed, entries)
	replayTurn, err := fixture.repo.GetTurn(ctx, replayed.TurnID)
	require.NoError(t, err)
	require.NotNil(t, replayTurn.CompletedAt)
	require.Equal(t, true, replayTurn.Metadata[models.TurnMetaKeyLifecycleOnly])

	require.Len(t, fixture.eventBus.events, 1, "the live todo indicator must still update")
	require.Equal(t, events.BuildSessionTodosSubject(fixture.workerID), fixture.eventBus.events[0].subject)
	require.Equal(t, events.SessionTodosUpdated, fixture.eventBus.events[0].event.Type)
	updated, ok := fixture.eventBus.events[0].event.Data.(lifecycle.SessionTodosEventPayload)
	require.True(t, ok)
	require.Equal(t, entries, updated.Entries)
}

func TestResumeTodosPersistOutOfTurnClear(t *testing.T) {
	ctx := context.Background()
	fixture := newResumeTodoFixture(t)

	fixture.svc.handleSessionTodosEvent(ctx, resumeTodosPayload(fixture, []streams.PlanEntry{}))

	require.Zero(t, openTurnCount(t, fixture.repo, fixture.workerID))
	messages, err := fixture.repo.ListMessages(ctx, fixture.workerID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	requireTodoEntries(t, messages[1], []streams.PlanEntry{})
	turn, err := fixture.repo.GetTurn(ctx, messages[1].TurnID)
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.Equal(t, true, turn.Metadata[models.TurnMetaKeyLifecycleOnly])
}

func TestResumeTodosPreservePromptTurn(t *testing.T) {
	entries := []streams.PlanEntry{{Description: "Keep this list", Status: "in_progress"}}
	tests := []struct {
		name             string
		entries          []streams.PlanEntry
		workerTurn       bool
		reserved         bool
		failLookup       bool
		wantLifecycle    bool
		wantPersisted    bool
		wantEmptyEntries bool
	}{
		{name: "active turn", entries: entries, workerTurn: true, wantPersisted: true},
		{name: "reserved prompt turn wins before lookup", entries: entries, workerTurn: true, reserved: true, failLookup: true, wantPersisted: true},
		{name: "empty list clears active turn", entries: []streams.PlanEntry{}, workerTurn: true, wantPersisted: true, wantEmptyEntries: true},
		{name: "completed only", entries: entries, wantLifecycle: true, wantPersisted: true},
		{name: "failed active lookup", entries: entries, failLookup: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newResumeTodoFixture(t)
			creator := &recordingTodoMessageCreator{serviceBackedMessageCreator: fixture.messageCreator}
			fixture.svc.messageCreator = creator
			fixture.svc.activeTurns.Store(fixture.workerID, fixture.previousTurn.ID)

			var workerTurn *models.Turn
			if tc.workerTurn {
				var err error
				workerTurn, err = fixture.taskService.StartTurn(ctx, fixture.workerID)
				require.NoError(t, err)
			}
			if tc.reserved {
				fixture.svc.reservedPromptTurns.Store(fixture.workerID, newReservedPromptTurn(workerTurn.ID))
			}
			if tc.failLookup {
				fixture.svc.turnService = &failingActiveTurnLookup{
					TurnService: fixture.taskService,
					err:         errors.New("active turn lookup failed"),
				}
			}

			fixture.svc.handleSessionTodosEvent(ctx, resumeTodosPayload(fixture, tc.entries))

			if tc.wantPersisted {
				messages, err := fixture.repo.ListMessages(ctx, fixture.workerID)
				require.NoError(t, err)
				require.Len(t, messages, 2)
				latest := messages[1]
				requireTodoEntries(t, latest, tc.entries)
				if tc.wantLifecycle {
					require.Empty(t, creator.turnIDs)
					require.Equal(t, 1, creator.lifecycleWrites)
					turn, err := fixture.repo.GetTurn(ctx, latest.TurnID)
					require.NoError(t, err)
					require.NotNil(t, turn.CompletedAt)
					require.Equal(t, true, turn.Metadata[models.TurnMetaKeyLifecycleOnly])
					require.Zero(t, openTurnCount(t, fixture.repo, fixture.workerID))
				} else {
					require.Equal(t, []string{workerTurn.ID}, creator.turnIDs)
					require.Zero(t, creator.lifecycleWrites)
					require.Equal(t, 1, openTurnCount(t, fixture.repo, fixture.workerID))
					require.Equal(t, workerTurn.ID, latest.TurnID)
				}
			} else {
				require.Empty(t, creator.turnIDs)
				require.Zero(t, creator.lifecycleWrites)
				messages, err := fixture.repo.ListMessages(ctx, fixture.workerID)
				require.NoError(t, err)
				require.Len(t, messages, 1)
				require.Zero(t, openTurnCount(t, fixture.repo, fixture.workerID))
				if tc.failLookup && !tc.reserved {
					lookup := fixture.svc.turnService.(*failingActiveTurnLookup)
					require.Zero(t, lookup.startCalls, "lookup failure must not fall back to turn creation")
				}
			}
			if tc.wantEmptyEntries {
				require.Empty(t, tc.entries)
			}
			require.Len(t, fixture.eventBus.events, 1,
				"the active reviewer sibling must not suppress or substitute for the worker's todo state")
		})
	}
}

func TestResumeTodosKeepResolvedTurnWhenItCompletesDuringPersistence(t *testing.T) {
	ctx := context.Background()
	fixture := newResumeTodoFixture(t)
	resolved, err := fixture.taskService.StartTurn(ctx, fixture.workerID)
	require.NoError(t, err)
	creator := &blockingTodoMessageCreator{
		serviceBackedMessageCreator: fixture.messageCreator,
		entered:                     make(chan string, 1),
		release:                     make(chan struct{}),
	}
	fixture.svc.messageCreator = creator
	fixture.svc.activeTurns.Store(fixture.workerID, resolved.ID)
	done := make(chan struct{})
	go func() {
		fixture.svc.persistTodoMessage(ctx, fixture.taskID, fixture.workerID,
			[]streams.PlanEntry{{Description: "Owned by the first turn", Status: "in_progress"}})
		close(done)
	}()

	var observedTurnID string
	select {
	case observedTurnID = <-creator.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("todo persistence did not reach the message boundary")
	}
	require.Equal(t, resolved.ID, observedTurnID)
	require.NoError(t, fixture.taskService.CompleteTurn(ctx, resolved.ID))
	successor, err := fixture.taskService.StartTurn(ctx, fixture.workerID)
	require.NoError(t, err)
	fixture.svc.activeTurns.Store(fixture.workerID, successor.ID)

	creator.releaseOnce.Do(func() { close(creator.release) })
	defer creator.releaseOnce.Do(func() { close(creator.release) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("todo persistence did not finish after its owner completed")
	}

	messages, err := fixture.repo.ListMessages(ctx, fixture.workerID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, resolved.ID, messages[1].TurnID,
		"persistence must keep the resolved explicit turn ID after a successor starts")
	require.NotEqual(t, successor.ID, messages[1].TurnID)
	require.Equal(t, 1, openTurnCount(t, fixture.repo, fixture.workerID))
}

func TestWorkflowAutoStartAfterResumeTodos(t *testing.T) {
	for _, reconstruct := range []bool{false, true} {
		name := "warm orchestrator"
		if reconstruct {
			name = "reconstructed orchestrator"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newResumeTodoFixture(t)
			fixture.svc.handleSessionTodosEvent(ctx, resumeTodosPayload(fixture,
				[]streams.PlanEntry{{Description: "Review complete", Status: agentEventCompleted}}))
			fixture.svc.handleSessionStatusEvent(ctx, &lifecycle.AgentStreamEventPayload{
				TaskID: fixture.taskID, SessionID: fixture.workerID,
				Data: &lifecycle.AgentStreamEventData{Type: "session_status", SessionStatus: streams.SessionStatusResumed},
			})
			require.Zero(t, openTurnCount(t, fixture.repo, fixture.workerID))
			worker, err := fixture.repo.GetTaskSession(ctx, fixture.workerID)
			require.NoError(t, err)
			worker.State = models.TaskSessionStateWaitingForInput
			require.NoError(t, fixture.repo.UpdateTaskSession(ctx, worker))

			task, err := fixture.repo.GetTask(ctx, fixture.taskID)
			require.NoError(t, err)
			task.WorkflowStepID = "step-resume-work"
			require.NoError(t, fixture.repo.UpdateTask(ctx, task))
			seedExecutorRunning(t, fixture.repo, fixture.workerID, fixture.taskID, "exec-resume-todo-work")

			stepGetter := newMockStepGetter()
			step := &wfmodels.WorkflowStep{
				ID: "step-resume-work", WorkflowID: "wf1", Name: "Work", Prompt: "Continue the Work step",
				Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
			}
			stepGetter.steps[step.ID] = step
			agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: fixture.repo}
			svc := fixture.svc
			workflowMessages := &serviceBackedWorkflowMessageCreator{serviceBackedMessageCreator: fixture.messageCreator}
			if reconstruct {
				svc = createTestServiceWithAgent(fixture.repo, stepGetter, newMockTaskRepo(), agent)
				svc.turnService = fixture.taskService
				svc.messageCreator = workflowMessages
				svc.eventBus = fixture.eventBus
			} else {
				svc.agentManager = agent
				svc.workflowStepGetter = stepGetter
				svc.messageCreator = workflowMessages
			}
			svc.executor = executor.NewExecutor(agent, fixture.repo, testLogger(), executor.ExecutorConfig{})
			session, err := fixture.repo.GetTaskSession(ctx, fixture.workerID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			require.True(t, svc.workflowEntryDispatchIsCurrentForSession(ctx, fixture.taskID, fixture.workerID, step))
			svc.launchAfterOnEnterDispatch(ctx, fixture.taskID, session, step, "Test Task", false, true, false)

			messages, err := fixture.repo.ListMessages(ctx, fixture.workerID)
			require.NoError(t, err)
			turns, err := fixture.repo.ListTurnsBySession(ctx, fixture.workerID)
			require.NoError(t, err)
			require.Len(t, agent.capturedPrompts, 1,
				"the current step prompt must be delivered exactly once; queue=%+v session=%+v",
				svc.messageQueue.GetStatus(ctx, fixture.workerID), session)
			require.Contains(t, agent.capturedPrompts[0], step.Prompt)
			var workTurn *models.Turn
			for _, turn := range turns {
				if turn.CompletedAt == nil {
					workTurn = turn
				}
			}
			require.NotNil(t, workTurn)
			require.NotEqual(t, fixture.previousTurn.ID, workTurn.ID)
			require.Equal(t, "step-resume-work", workTurn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
			var userMessages []*models.Message
			for _, message := range messages {
				if message.AuthorType == models.MessageAuthorUser {
					userMessages = append(userMessages, message)
				}
			}
			require.Len(t, userMessages, 1)
			require.Equal(t, workTurn.ID, userMessages[0].TurnID)
		})
	}
}

type resumeTodoFixture struct {
	repo           *sqliterepo.Repository
	taskService    *taskservice.Service
	messageCreator *serviceBackedMessageCreator
	eventBus       *recordingEventBus
	svc            *Service
	taskID         string
	workerID       string
	reviewerID     string
	previousTurn   *models.Turn
	previousTodoID string
}

type recordingTodoMessageCreator struct {
	*serviceBackedMessageCreator
	turnIDs         []string
	lifecycleWrites int
}

type serviceBackedWorkflowMessageCreator struct {
	*serviceBackedMessageCreator
}

func (m *serviceBackedWorkflowMessageCreator) CreateUserMessage(
	ctx context.Context,
	taskID, content, sessionID, turnID string,
	metadata map[string]interface{},
) error {
	_, err := m.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	return err
}

func (m *recordingTodoMessageCreator) CreateSessionMessage(
	ctx context.Context,
	taskID, content, sessionID, messageType, turnID string,
	metadata map[string]interface{}, requestsInput bool,
) error {
	m.turnIDs = append(m.turnIDs, turnID)
	return m.serviceBackedMessageCreator.CreateSessionMessage(
		ctx, taskID, content, sessionID, messageType, turnID, metadata, requestsInput,
	)
}

func (m *recordingTodoMessageCreator) CreateLifecycleSessionMessage(
	ctx context.Context,
	taskID, content, sessionID, messageType string,
	metadata map[string]interface{},
) error {
	m.lifecycleWrites++
	return m.serviceBackedMessageCreator.CreateLifecycleSessionMessage(
		ctx, taskID, content, sessionID, messageType, metadata,
	)
}

type blockingTodoMessageCreator struct {
	*serviceBackedMessageCreator
	entered     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func (m *blockingTodoMessageCreator) CreateSessionMessage(
	ctx context.Context,
	taskID, content, sessionID, messageType, turnID string,
	metadata map[string]interface{}, requestsInput bool,
) error {
	m.entered <- turnID
	<-m.release
	return m.serviceBackedMessageCreator.CreateSessionMessage(
		ctx, taskID, content, sessionID, messageType, turnID, metadata, requestsInput,
	)
}

func newResumeTodoFixture(t *testing.T) *resumeTodoFixture {
	t.Helper()
	ctx := context.Background()
	const (
		taskID     = "task-resume-todo-boundary"
		workerID   = "session-resume-todo-worker"
		reviewerID = "session-resume-todo-reviewer"
		workStepID = "step-resume-work"
		reviewID   = "step-resume-review"
	)
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, workerID, workStepID)
	messageCreator := newServiceBackedMessageCreator(repo)

	previousTurn, err := messageCreator.svc.StartTurn(ctx, workerID)
	require.NoError(t, err)
	previousTodoID := "message-resume-todo-history"
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID:            previousTodoID,
		TaskID:        taskID,
		TaskSessionID: workerID,
		TurnID:        previousTurn.ID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeTodo,
		Content:       "Updated Todos",
		Metadata:      map[string]interface{}{"todos": []map[string]interface{}{{"text": "Earlier work", "done": true}}},
	}))
	require.NoError(t, messageCreator.svc.CompleteTurn(ctx, previousTurn.ID))

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkflowStepID = reviewID
	require.NoError(t, repo.UpdateTask(ctx, task))

	worker, err := repo.GetTaskSession(ctx, workerID)
	require.NoError(t, err)
	worker.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, worker))

	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: reviewerID, TaskID: taskID, State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))
	_, err = messageCreator.svc.StartTurn(ctx, reviewerID)
	require.NoError(t, err)

	eventBus := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = messageCreator.svc
	svc.messageCreator = messageCreator
	svc.eventBus = eventBus

	return &resumeTodoFixture{
		repo: repo, taskService: messageCreator.svc, messageCreator: messageCreator,
		eventBus: eventBus, svc: svc, taskID: taskID, workerID: workerID,
		reviewerID: reviewerID, previousTurn: previousTurn, previousTodoID: previousTodoID,
	}
}

func resumeTodosPayload(fixture *resumeTodoFixture, entries []streams.PlanEntry) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID:    fixture.taskID,
		SessionID: fixture.workerID,
		Data: &lifecycle.AgentStreamEventData{
			Type:        "session_todos",
			PlanEntries: entries,
		},
	}
}

func requireTodoEntries(t *testing.T, message *models.Message, entries []streams.PlanEntry) {
	t.Helper()
	got, err := json.Marshal(message.Metadata["todos"])
	require.NoError(t, err)
	wantEntries := make([]map[string]interface{}, len(entries))
	for i, entry := range entries {
		wantEntries[i] = map[string]interface{}{
			"text": entry.Description, "status": entry.Status, "done": entry.Status == agentEventCompleted,
		}
	}
	want, err := json.Marshal(wantEntries)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}
