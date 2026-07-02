package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
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
	pauser := &recordingClarificationInputPauser{}
	h := NewHandlers(svc, nil, store, nil, nil, repo, repo, nil, nil, nil, nil, testLogger(t))
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

	require.Eventually(t, func() bool {
		return len(store.ListPending()) == 1
	}, time.Second, 5*time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ask_user_question handler")
	}
	require.Equal(t, []string{sess.ID}, pauser.sessions)
}
