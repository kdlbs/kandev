package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type recordingClarificationInputPauser struct {
	sessions []string
	count    int
	err      error
}

func (p *recordingClarificationInputPauser) PauseForClarificationInput(_ context.Context, sessionID string) (int, error) {
	p.sessions = append(p.sessions, sessionID)
	return p.count, p.err
}

type recordingSessionCanceller struct {
	sessions []string
	count    int
}

func (c *recordingSessionCanceller) DetachSessionAndNotify(_ context.Context, sessionID string) int {
	c.sessions = append(c.sessions, sessionID)
	return c.count
}

type immediateClarificationService struct {
	response *clarification.Response
	waitErr  error
}

func (s *immediateClarificationService) CreateRequest(*clarification.Request) (string, bool) {
	return "pending-race", true
}

func (s *immediateClarificationService) WaitForResponse(context.Context, string) (*clarification.Response, error) {
	return s.response, s.waitErr
}

func (s *immediateClarificationService) CancelRequest(string) bool { return true }

// clarificationCoordinatorStopRaceRepo simulates the coordinator committing a
// stop immediately before one clarification state write. The legacy
// unconditional method demonstrates the stale-writer bug by restoring the
// target state; the conditional method preserves CANCELLED.
type clarificationCoordinatorStopRaceRepo struct {
	SessionRepository
	taskRepo    TaskRepository
	targetState models.TaskSessionState
	stopped     bool
}

func (r *clarificationCoordinatorStopRaceRepo) stopBeforeTarget(
	ctx context.Context,
	sessionID string,
	state models.TaskSessionState,
) error {
	if r.stopped || state != r.targetState {
		return nil
	}
	r.stopped = true
	session, err := r.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := r.SessionRepository.UpdateTaskSessionState(
		ctx,
		sessionID,
		models.TaskSessionStateCancelled,
		"stopped by parent task via MCP",
	); err != nil {
		return err
	}
	return r.taskRepo.UpdateTaskState(ctx, session.TaskID, v1.TaskStateReview)
}

func (r *clarificationCoordinatorStopRaceRepo) UpdateTaskSessionState(
	ctx context.Context,
	sessionID string,
	state models.TaskSessionState,
	errorMessage string,
) error {
	if err := r.stopBeforeTarget(ctx, sessionID, state); err != nil {
		return err
	}
	return r.SessionRepository.UpdateTaskSessionState(ctx, sessionID, state, errorMessage)
}

func (r *clarificationCoordinatorStopRaceRepo) UpdateTaskSessionStateIfCurrent(
	ctx context.Context,
	sessionID string,
	expected, state models.TaskSessionState,
	errorMessage string,
) (bool, time.Time, error) {
	if err := r.stopBeforeTarget(ctx, sessionID, state); err != nil {
		return false, time.Time{}, err
	}
	updater := r.SessionRepository.(interface {
		UpdateTaskSessionStateIfCurrent(
			context.Context,
			string,
			models.TaskSessionState,
			models.TaskSessionState,
			string,
		) (bool, time.Time, error)
	})
	return updater.UpdateTaskSessionStateIfCurrent(ctx, sessionID, expected, state, errorMessage)
}

// clarificationTaskStateRaceRepo commits coordinator cancellation after the
// clarification session CAS has restored RUNNING but before its task-state
// write. The underlying session-owned task CAS must reject IN_PROGRESS.
type clarificationTaskStateRaceRepo struct {
	TaskRepository
	sessionRepo SessionRepository
	stopped     bool
}

func (r *clarificationTaskStateRaceRepo) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	if !r.stopped {
		r.stopped = true
		if err := r.sessionRepo.UpdateTaskSessionState(
			ctx,
			sessionID,
			models.TaskSessionStateCancelled,
			"stopped by parent task via MCP",
		); err != nil {
			return false, err
		}
		if err := r.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
			return false, err
		}
	}
	updater := r.TaskRepository.(sessionOwnedTaskStateUpdater)
	return updater.UpdateTaskStateIfSessionState(
		ctx,
		taskID,
		sessionID,
		expectedSessionState,
		state,
	)
}

func askUserQuestionRaceMessage(t *testing.T, taskID, sessionID string) *ws.Message {
	t.Helper()
	return makeWSMessage(t, ws.ActionMCPAskUserQuestion, map[string]interface{}{
		"session_id": sessionID,
		"task_id":    taskID,
		"questions": []map[string]interface{}{
			{
				"prompt": "What colour?",
				"options": []map[string]interface{}{
					{"label": "Red", "description": "R"},
					{"label": "Blue", "description": "B"},
				},
			},
		},
	})
}

func TestHandleAskUserQuestion_CoordinatorStopWinsWaitingTransition(t *testing.T) {
	_, repo := newTestTaskService(t)
	const taskID = "task-clarification-wait-race"
	const sessionID = "session-clarification-wait-race"
	seedMCPHandlerSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)

	racingRepo := &clarificationCoordinatorStopRaceRepo{
		SessionRepository: repo,
		taskRepo:          repo,
		targetState:       models.TaskSessionStateWaitingForInput,
	}
	eventBus := &mcpRecordingEventBus{}
	h := &Handlers{
		clarificationSvc: &immediateClarificationService{waitErr: errors.New("wait cancelled")},
		sessionRepo:      racingRepo,
		taskRepo:         repo,
		eventBus:         eventBus,
		logger:           testLogger(t).WithFields(),
	}

	_, err := h.handleAskUserQuestion(context.Background(), askUserQuestionRaceMessage(t, taskID, sessionID))
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Equal(t, "REVIEW", string(task.State))
	require.Empty(t, eventBus.events, "rejected WAITING_FOR_INPUT transition must not publish")
}

func TestHandleAskUserQuestion_CoordinatorStopWinsRunningTransition(t *testing.T) {
	_, repo := newTestTaskService(t)
	const taskID = "task-clarification-resume-race"
	const sessionID = "session-clarification-resume-race"
	seedMCPHandlerSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)

	racingRepo := &clarificationCoordinatorStopRaceRepo{
		SessionRepository: repo,
		taskRepo:          repo,
		targetState:       models.TaskSessionStateRunning,
	}
	eventBus := &mcpRecordingEventBus{}
	h := &Handlers{
		clarificationSvc: &immediateClarificationService{response: &clarification.Response{}},
		sessionRepo:      racingRepo,
		taskRepo:         repo,
		eventBus:         eventBus,
		logger:           testLogger(t).WithFields(),
	}

	_, err := h.handleAskUserQuestion(context.Background(), askUserQuestionRaceMessage(t, taskID, sessionID))
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Equal(t, "REVIEW", string(task.State))
	require.Len(t, eventBus.events, 1, "only accepted WAITING_FOR_INPUT transition may publish")
	eventData := eventBus.events[0].Data.(map[string]interface{})
	require.Equal(t, string(models.TaskSessionStateWaitingForInput), eventData["new_state"])
}

func TestHandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition(t *testing.T) {
	_, repo := newTestTaskService(t)
	const taskID = "task-clarification-task-race"
	const sessionID = "session-clarification-task-race"
	seedMCPHandlerSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)

	racingTaskRepo := &clarificationTaskStateRaceRepo{
		TaskRepository: repo,
		sessionRepo:    repo,
	}
	eventBus := &mcpRecordingEventBus{}
	h := &Handlers{
		clarificationSvc: &immediateClarificationService{response: &clarification.Response{}},
		sessionRepo:      repo,
		taskRepo:         racingTaskRepo,
		eventBus:         eventBus,
		logger:           testLogger(t).WithFields(),
	}

	_, err := h.handleAskUserQuestion(
		context.Background(),
		askUserQuestionRaceMessage(t, taskID, sessionID),
	)
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, task.State)
	require.Len(t, eventBus.events, 1, "stale RUNNING transition must not publish")
	eventData := eventBus.events[0].Data.(map[string]interface{})
	require.Equal(t, string(models.TaskSessionStateWaitingForInput), eventData["new_state"])
}

func TestHandleClarificationTimeout_UsesHardPauser(t *testing.T) {
	pauser := &recordingClarificationInputPauser{count: 2}
	h := &Handlers{logger: testLogger(t).WithFields()}
	h.SetClarificationInputPauser(pauser)

	msg := makeWSMessage(t, ws.ActionMCPClarificationTimeout, map[string]interface{}{"session_id": "s1"})
	resp, err := h.handleClarificationTimeout(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Equal(t, []string{"s1"}, pauser.sessions)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Equal(t, true, payload["ok"])
	require.Equal(t, true, payload["paused"])
	require.Equal(t, float64(2), payload["cancelled"])
}

func TestHandleClarificationTimeout_FallsBackWhenHardPauseFails(t *testing.T) {
	pauser := &recordingClarificationInputPauser{err: errors.New("db unavailable")}
	canceller := &recordingSessionCanceller{count: 3}
	h := &Handlers{logger: testLogger(t).WithFields(), sessionCanceller: canceller}
	h.SetClarificationInputPauser(pauser)

	msg := makeWSMessage(t, ws.ActionMCPClarificationTimeout, map[string]interface{}{"session_id": "s1"})
	resp, err := h.handleClarificationTimeout(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Equal(t, []string{"s1"}, pauser.sessions)
	require.Equal(t, []string{"s1"}, canceller.sessions)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Equal(t, true, payload["ok"])
	require.Equal(t, false, payload["paused"])
	require.Equal(t, float64(3), payload["cancelled"])
	require.NotContains(t, string(resp.Payload), "db unavailable")
}

func TestHandleAskUserQuestion_NoAnswerPausesSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Task",
	})
	require.NoError(t, err)

	sess := &models.TaskSession{
		ID:        "sess-no-answer",
		TaskID:    task.ID,
		IsPrimary: true,
		State:     models.TaskSessionStateRunning,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))

	store := clarification.NewStore(time.Minute)
	waitEntered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(_ string) {
		select {
		case waitEntered <- struct{}{}:
		default:
		}
	})
	pauser := &recordingClarificationInputPauser{}
	h := NewHandlers(svc, nil, store, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetClarificationInputPauser(pauser)

	payload := map[string]interface{}{
		"session_id": sess.ID,
		"task_id":    task.ID,
		"questions": []map[string]interface{}{
			{"prompt": "What colour?", "options": []map[string]interface{}{
				{"label": "Red", "description": "R"},
				{"label": "Blue", "description": "B"},
			}},
		},
	}
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		msg := makeWSMessage(t, ws.ActionMCPAskUserQuestion, payload)
		if _, err := h.handleAskUserQuestion(waitCtx, msg); err != nil {
			t.Errorf("handleAskUserQuestion returned unexpected error: %v", err)
		}
	}()

	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for clarification request to register")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ask_user_question handler")
	}
	require.Equal(t, []string{sess.ID}, pauser.sessions)
}
