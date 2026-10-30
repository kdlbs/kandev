package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type asyncQuestionFixture struct {
	handlers *Handlers
	service  *questions.Service
	taskSvc  *service.Service
	repo     *sqliterepo.Repository
	store    *clarification.Store
}

func newAsyncQuestionFixture(t *testing.T) *asyncQuestionFixture {
	t.Helper()
	taskSvc, repo := newTestTaskService(t)
	store := clarification.NewStore(time.Minute)
	h := NewHandlers(taskSvc, nil, store, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	questionSvc := questions.NewService(repo, nil, testLogger(t).Zap())
	h.SetAsyncQuestionService(questionSvc)
	return &asyncQuestionFixture{handlers: h, service: questionSvc, taskSvc: taskSvc, repo: repo, store: store}
}

func (f *asyncQuestionFixture) seedTask(t *testing.T, ctx context.Context, sessionID string, autopilot bool, parentID string) (*models.Task, *models.TaskSession) {
	t.Helper()
	workspaceID := "ws-" + sessionID
	require.NoError(t, f.repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}))
	require.NoError(t, f.repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + sessionID, WorkspaceID: workspaceID, Name: "Board"}))
	task := &models.Task{
		ID: "task-" + sessionID, WorkspaceID: workspaceID, WorkflowID: "wf-" + sessionID,
		Title: "Task " + sessionID, State: "TODO", ParentID: parentID, Autopilot: autopilot,
	}
	require.NoError(t, f.repo.CreateTask(ctx, task))
	session := &models.TaskSession{
		ID: sessionID, TaskID: task.ID, IsPrimary: true, State: models.TaskSessionStateRunning,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, f.repo.CreateTaskSession(ctx, session))
	return task, session
}

func askPayload(sessionID, taskID string, async bool, requestID string) map[string]interface{} {
	payload := map[string]interface{}{
		"session_id": sessionID,
		"task_id":    taskID,
		"questions": []map[string]interface{}{
			{"prompt": "Which value?", "options": []map[string]interface{}{
				{"label": "Default", "description": "Use the default"},
				{"label": "Empty", "description": "Leave empty"},
			}},
		},
		"context": "API integration",
	}
	if async {
		payload["async"] = true
	}
	if requestID != "" {
		payload["request_id"] = requestID
	}
	return payload
}

func responsePayload(t *testing.T, msg *ws.Message) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	return payload
}

func TestAsyncQuestionsOperatorAskKeepsTurnRunning(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	task, session := fixture.seedTask(t, ctx, "session-async-operator", false, "")

	response, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-async-1")))
	require.NoError(t, err)
	payload := responsePayload(t, response)
	require.Equal(t, "async", payload["mode"])
	require.NotEmpty(t, payload["question_id"])
	require.Equal(t, "pending", payload["status"])

	// The receipt arrives before any answer and the generation is untouched.
	current, err := fixture.repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, current.State)
	require.Empty(t, fixture.store.ListPending(), "async creation must not register a blocking waiter")

	// A second distinct async request creates a second independent request.
	second, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-async-2")))
	require.NoError(t, err)
	require.NotEqual(t, payload["question_id"], responsePayload(t, second)["question_id"])

	// Discoverability is immediate and does not depend on the turn ending.
	list, err := fixture.handlers.handleListQuestionAnswers(ctx, makeWSMessage(t, ws.ActionMCPListQuestionAnswers,
		map[string]interface{}{"session_id": session.ID, "task_id": task.ID, "scope": "own"}))
	require.NoError(t, err)
	listPayload := responsePayload(t, list)
	require.Equal(t, float64(2), listPayload["count"])
}

func TestAsyncQuestionsRetryReturnsOriginalRequest(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	task, session := fixture.seedTask(t, ctx, "session-async-retry", false, "")

	first, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-async-retry")))
	require.NoError(t, err)
	second, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-async-retry")))
	require.NoError(t, err)
	require.Equal(t, responsePayload(t, first)["question_id"], responsePayload(t, second)["question_id"])
}

func TestAsyncQuestionsRequestIDRequiresAsyncMode(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	task, session := fixture.seedTask(t, ctx, "session-async-mode", false, "")

	response, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, false, "orphan-request")))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
}

func TestAsyncQuestionsParentKeepsChildRunning(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	parentTask, parentSession := fixture.seedTask(t, ctx, "session-parent", false, "")
	childTask, childSession := fixture.seedTask(t, ctx, "session-child", true, parentTask.ID)

	response, err := fixture.handlers.handleAskParentQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskParentQuestion,
		askPayload(childSession.ID, childTask.ID, true, "parent-request-1")))
	require.NoError(t, err)
	payload := responsePayload(t, response)
	require.Equal(t, "async", payload["mode"])
	require.Equal(t, parentTask.ID, payload["parent_task_id"])
	require.Equal(t, "parent", payload["recipient"])

	current, err := fixture.repo.GetTaskSession(ctx, childSession.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, current.State, "async parent creation must not pause the child")

	// The parent receives an immediate durable queue intent, not a pause.
	deliveries, err := fixture.repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, models.AsyncQuestionDeliveryParentQuestion, deliveries[0].Kind)
	require.Equal(t, parentSession.ID, deliveries[0].DestinationSession)
}

type recordingPauser struct {
	pauses  int
	session string
}

func (p *recordingPauser) PauseForClarificationInput(context.Context, string) (int, error) {
	p.pauses++
	return 0, nil
}

func (p *recordingPauser) PauseForClarificationInputWithOptions(_ context.Context, sessionID string, _ orchestrator.ClarificationPauseOptions) (int, error) {
	p.pauses++
	p.session = sessionID
	return 0, nil
}

type stalePauser struct {
	recordingPauser
	err error
}

func (p *stalePauser) CaptureActiveTurnID(context.Context, string) (string, error) {
	return "captured-turn", nil
}

func (p *stalePauser) PauseForClarificationInputWithOptions(context.Context, string, orchestrator.ClarificationPauseOptions) (int, error) {
	p.pauses++
	return 0, p.err
}

func TestAsyncQuestionsWaitStalePauseDoesNotBlockSuccessor(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	task, session := fixture.seedTask(t, ctx, "session-stale-wait", false, "")
	fixture.handlers.SetClarificationInputPauser(&stalePauser{err: orchestrator.ErrStaleQuestionWaitPause})

	response, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-stale-wait")))
	require.NoError(t, err)
	questionID, _ := responsePayload(t, response)["question_id"].(string)

	waitResponse, err := fixture.handlers.handleWaitForQuestionAnswers(ctx, makeWSMessage(t, ws.ActionMCPWaitForQuestionAnswers,
		map[string]interface{}{
			"session_id": session.ID, "task_id": task.ID,
			"question_ids": []string{questionID}, "reason": "dependency",
		}))
	require.NoError(t, err)
	require.Equal(t, "cancelled", responsePayload(t, waitResponse)["status"])
	require.False(t, fixture.service.HasParkedWait(ctx, session.ID))
}

func TestAsyncQuestionsWaitPauseFailurePropagates(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	task, session := fixture.seedTask(t, ctx, "session-failed-wait", false, "")
	fixture.handlers.SetClarificationInputPauser(&stalePauser{err: errors.New("pause failed")})

	response, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-failed-wait")))
	require.NoError(t, err)
	questionID, _ := responsePayload(t, response)["question_id"].(string)

	waitResponse, err := fixture.handlers.handleWaitForQuestionAnswers(ctx, makeWSMessage(t, ws.ActionMCPWaitForQuestionAnswers,
		map[string]interface{}{
			"session_id": session.ID, "task_id": task.ID,
			"question_ids": []string{questionID}, "reason": "dependency",
		}))
	require.NoError(t, err)
	assertWSError(t, waitResponse, ws.ErrorCodeInternalError)
	require.False(t, fixture.service.HasParkedWait(ctx, session.ID))
}

func TestAsyncQuestionsWaitPausesOnlyWhenNamed(t *testing.T) {
	fixture := newAsyncQuestionFixture(t)
	ctx := context.Background()
	task, session := fixture.seedTask(t, ctx, "session-async-wait", false, "")
	pauser := &recordingPauser{}
	fixture.handlers.SetClarificationInputPauser(pauser)

	response, err := fixture.handlers.handleAskUserQuestion(ctx, makeWSMessage(t, ws.ActionMCPAskUserQuestion,
		askPayload(session.ID, task.ID, true, "request-wait-1")))
	require.NoError(t, err)
	questionID, _ := responsePayload(t, response)["question_id"].(string)
	require.NotEmpty(t, questionID)
	require.Equal(t, 0, pauser.pauses, "an open request alone must not pause")

	waitResponse, err := fixture.handlers.handleWaitForQuestionAnswers(ctx, makeWSMessage(t, ws.ActionMCPWaitForQuestionAnswers,
		map[string]interface{}{
			"session_id": session.ID, "task_id": task.ID,
			"question_ids": []string{questionID}, "reason": "remaining work depends on it",
		}))
	require.NoError(t, err)
	waitPayload := responsePayload(t, waitResponse)
	require.Equal(t, "waiting", waitPayload["status"])
	require.Equal(t, 1, pauser.pauses)
	require.True(t, fixture.service.HasParkedWait(ctx, session.ID))

	result, _, err := fixture.service.ResolveOperator(ctx, questionID, questions.AnswerInput{
		Answers: []questions.Answer{{
			QuestionID:      "q1",
			SelectedOptions: []string{"q1_opt1"},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, models.AsyncQuestionStatusAnswered, result.Status)
	require.False(t, fixture.service.HasParkedWait(ctx, session.ID), "an answer releases the explicit gate")
}
