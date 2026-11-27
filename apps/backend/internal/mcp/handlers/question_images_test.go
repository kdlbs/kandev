package handlers

import (
	"context"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestQuestionImagesAdmission(t *testing.T) {
	testQuestionImagesAdmission(t, false)
}

func TestChoiceImagesAdmission(t *testing.T) {
	testQuestionImagesAdmission(t, true)
}

func TestChoiceImagesEmptyParentFieldRejected(t *testing.T) {
	msg := makeWSMessage(t, ws.ActionMCPAskParentQuestion, map[string]any{
		"session_id": "child", "questions": []map[string]any{{"prompt": "Choose", "options": []map[string]any{{"label": "A", "images": []any{}}, {"label": "B"}}}},
	})
	_, err := parentQuestionRequestFromMessage(msg)
	require.NotNil(t, err)
	require.Contains(t, err.message, "do not support images")
}

func testQuestionImagesAdmission(t *testing.T, choice bool) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	taskID, sessionID, _ := seedRetrySession(t, context.Background(), svc, repo, "images")
	h, _ := newMessageTaskHandler(t, svc, repo)
	h.sessionRepo = repo
	payload := map[string]interface{}{
		"session_id": sessionID, "task_id": taskID,
		"questions": []map[string]interface{}{{"id": "layout", "prompt": "Approve?", "options": []map[string]interface{}{{"label": "Yes"}, {"label": "No"}}, "images": []map[string]interface{}{{"id": "a", "path": "a.png", "alt": "A"}}}},
	}
	if choice {
		q := payload["questions"].([]map[string]interface{})[0]
		q["options"].([]map[string]interface{})[0]["images"] = q["images"]
		delete(q, "images")
	}
	msg := makeWSMessage(t, ws.ActionMCPAskUserQuestion, payload)
	decoded, response, err := decodeAskUserQuestion(msg)
	require.NoError(t, err)
	require.Nil(t, response)
	require.Len(t, decoded.Questions[0].ImageReferences(), 1)
	_, parentErr := parentQuestionRequestFromMessage(msg)
	require.NotNil(t, parentErr)
	payload["task_id"] = "foreign"
	response, err = h.handleAskUserQuestion(context.Background(), makeWSMessage(t, ws.ActionMCPAskUserQuestion, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Contains(t, string(response.Payload), "does not belong")
}
