package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func seedParentQuestionScenario(t *testing.T, svc *service.Service, repo seedRepo) (*models.Task, *models.Task, *models.TaskSession, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-parent-question", Name: "Parent questions"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-parent-question", WorkspaceID: "ws-parent-question", Name: "Board"}))
	parentResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-parent-question",
		WorkflowID:  "wf-parent-question",
		Title:       "Parent task",
	})
	require.NoError(t, err)
	parent := parentResult.Task
	childResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-parent-question",
		WorkflowID:  "wf-parent-question",
		ParentID:    parent.ID,
		Title:       "Autopilot child",
		Autopilot:   true,
	})
	require.NoError(t, err)
	child := childResult.Task
	parentSession := &models.TaskSession{ID: "parent-question-parent-session", TaskID: parent.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	childSession := &models.TaskSession{ID: "parent-question-child-session", TaskID: child.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(ctx, parentSession))
	require.NoError(t, repo.CreateTaskSession(ctx, childSession))
	return parent, child, parentSession, childSession
}

func parentQuestionMessage(t *testing.T, taskID, sessionID string) *ws.Message {
	t.Helper()
	return makeWSMessage(t, ws.ActionMCPAskParentQuestion, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"questions": []map[string]interface{}{{
			"id":     "database",
			"prompt": "Which database should I use?",
			"options": []map[string]interface{}{
				{"label": "SQLite", "description": "Use the embedded database"},
				{"label": "Postgres", "description": "Use the hosted database"},
			},
		}},
		"context": "The migration needs a database choice.",
	})
}

func TestHandleAskParentQuestion_PersistsRoutesAndPauses(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, child, parentSession, childSession := seedParentQuestionScenario(t, svc, repo)
	h, orch := newMessageTaskHandler(t, svc, repo)
	pauser := &recordingClarificationInputPauser{}
	h.inputPauser = pauser

	resp, err := h.handleAskParentQuestion(context.Background(), parentQuestionMessage(t, child.ID, childSession.ID))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	questionID, ok := payload["question_id"].(string)
	require.True(t, ok)
	require.Equal(t, "waiting_for_parent", payload["status"])
	require.Equal(t, parent.ID, payload["parent_task_id"])
	require.Equal(t, []string{childSession.ID}, pauser.sessions)

	question, err := svc.GetMessage(context.Background(), questionID)
	require.NoError(t, err)
	require.Equal(t, models.MessageTypeClarificationRequest, question.Type)
	require.True(t, question.RequestsInput)
	require.Equal(t, true, question.Metadata[models.MetaKeyParentQuestion])
	require.Equal(t, "pending", question.Metadata[models.MetaKeyParentQuestionStatus])
	require.Equal(t, parent.ID, question.Metadata[models.MetaKeyParentQuestionParentID])
	require.Equal(t, child.ID, question.Metadata[models.MetaKeyParentQuestionChildID])

	childSessionAfter, err := repo.GetTaskSession(context.Background(), childSession.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, childSessionAfter.State)
	childTaskAfter, err := svc.GetTask(context.Background(), child.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, childTaskAfter.State)

	status := orch.queue.GetStatus(context.Background(), parentSession.ID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, questionID, status.Entries[0].Metadata[models.MetaKeyParentQuestionID])
	require.Contains(t, status.Entries[0].Content, questionID)
	require.Contains(t, status.Entries[0].Content, "Which database should I use?")
}

func TestHandleMessageTask_AnswersParentQuestionIdempotently(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, child, _, childSession := seedParentQuestionScenario(t, svc, repo)
	h, _ := newMessageTaskHandler(t, svc, repo)
	h.inputPauser = &recordingClarificationInputPauser{}

	questionResp, err := h.handleAskParentQuestion(context.Background(), parentQuestionMessage(t, child.ID, childSession.ID))
	require.NoError(t, err)
	var questionPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(questionResp.Payload, &questionPayload))
	questionID := questionPayload["question_id"].(string)

	answer := senderPayload(child.ID, "Use Postgres.", parent.ID)
	answer["reply_to_question_id"] = questionID
	answerResp, err := h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, answer))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, answerResp.Type)

	question, err := svc.GetMessage(context.Background(), questionID)
	require.NoError(t, err)
	require.Equal(t, "answered", question.Metadata[models.MetaKeyParentQuestionStatus])
	require.Equal(t, "Use Postgres.", question.Metadata[models.MetaKeyParentQuestionResponse])

	answerAgain, err := h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, answer))
	require.NoError(t, err)
	var answerAgainPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(answerAgain.Payload, &answerAgainPayload))
	require.Equal(t, "already_answered", answerAgainPayload["status"])
}

func TestHandleAskParentQuestion_RejectsRootTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, _, _, _ := seedParentQuestionScenario(t, svc, repo)
	rootSession := &models.TaskSession{ID: "parent-question-root-session", TaskID: parent.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(context.Background(), rootSession))
	h, _ := newMessageTaskHandler(t, svc, repo)

	resp, err := h.handleAskParentQuestion(context.Background(), parentQuestionMessage(t, parent.ID, rootSession.ID))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleAskParentQuestion_RejectsNonAutopilotTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, _, _, _ := seedParentQuestionScenario(t, svc, repo)
	normalChildResult, err := svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID: "ws-parent-question",
		WorkflowID:  "wf-parent-question",
		ParentID:    parent.ID,
		Title:       "Normal child",
	})
	require.NoError(t, err)
	normalChild := normalChildResult.Task
	normalSession := &models.TaskSession{ID: "parent-question-normal-session", TaskID: normalChild.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(context.Background(), normalSession))
	h, _ := newMessageTaskHandler(t, svc, repo)

	resp, err := h.handleAskParentQuestion(context.Background(), parentQuestionMessage(t, normalChild.ID, normalSession.ID))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMessageTask_RejectsParentQuestionReplyFromUnrelatedTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parent, child, _, childSession := seedParentQuestionScenario(t, svc, repo)
	strangerResult, err := svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID: "ws-parent-question",
		WorkflowID:  "wf-parent-question",
		Title:       "Unrelated task",
	})
	require.NoError(t, err)
	stranger := strangerResult.Task
	strangerSession := &models.TaskSession{ID: "parent-question-stranger-session", TaskID: stranger.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(context.Background(), strangerSession))
	h, _ := newMessageTaskHandler(t, svc, repo)
	h.inputPauser = &recordingClarificationInputPauser{}

	questionResp, err := h.handleAskParentQuestion(context.Background(), parentQuestionMessage(t, child.ID, childSession.ID))
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(questionResp.Payload, &payload))
	answer := senderPayload(child.ID, "Use SQLite.", stranger.ID)
	answer["reply_to_question_id"] = payload["question_id"]
	resp, err := h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, answer))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	_ = parent
}
