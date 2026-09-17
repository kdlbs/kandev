package backendapp

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type attentionTestPermissions struct {
	rows []streams.PendingAgentPermission
	err  error
}

func (p *attentionTestPermissions) ListPendingAgentPermissions(context.Context, string, string) ([]streams.PendingAgentPermission, error) {
	return p.rows, p.err
}

func TestAssistantAttentionNativeAllSessions(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	task := &taskmodels.Task{ID: "worker", WorkspaceID: "ws-1", Title: "Synthetic task", State: "IN_PROGRESS"}
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, task))
	for _, session := range []*taskmodels.TaskSession{{ID: "older", TaskID: task.ID, State: taskmodels.TaskSessionStateWaitingForInput}, {ID: "newer", TaskID: task.ID, State: taskmodels.TaskSessionStateRunning}} {
		require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, session))
		require.NoError(t, adapter.taskRepo.CreateTurn(ctx, &taskmodels.Turn{ID: session.ID + "-turn", TaskSessionID: session.ID, TaskID: task.ID}))
	}
	question := &taskmodels.Message{ID: "question-row", TaskID: task.ID, TaskSessionID: "older", TurnID: "older-turn", Type: taskmodels.MessageTypeClarificationRequest, AuthorType: taskmodels.MessageAuthorAgent, Content: "Choose a color", Metadata: map[string]any{"pending_id": "question", "question_id": "color", "status": "pending", "question": map[string]any{"id": "color", "prompt": "Choose a sample color"}}}
	permission := &taskmodels.Message{ID: "permission-row", TaskID: task.ID, TaskSessionID: "newer", TurnID: "newer-turn", Type: taskmodels.MessageTypePermissionRequest, AuthorType: taskmodels.MessageAuthorAgent, Content: "SYNTHETIC_PRIVATE_TOOL_ARGUMENTS", Metadata: map[string]any{"pending_id": "permission", "request_id": "generation"}}
	require.NoError(t, adapter.taskRepo.CreateMessage(ctx, question))
	require.NoError(t, adapter.taskRepo.CreateMessage(ctx, permission))
	questions := clarification.NewStore(time.Minute)
	questions.CreateRequest(&clarification.Request{PendingID: "question", SessionID: "older", TaskID: task.ID, Questions: []clarification.Question{{ID: "color", Prompt: "Choose a sample color"}}})
	permissions := &attentionTestPermissions{rows: []streams.PendingAgentPermission{{TaskID: task.ID, SessionID: "newer", PendingID: "permission", RequestID: "generation"}}}
	reader := &assistantAttentionReader{tasks: svc, permissions: permissions, questions: questions}
	b := &shared.AssistantBinding{OwnerUserID: "owner", WorkspaceID: "ws-1"}
	rows, err := reader.ReadAttention(ctx, b, task.ID, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "pending", row.State)
	}
	raw, _ := json.Marshal(rows)
	require.NotContains(t, string(raw), "SYNTHETIC_PRIVATE_TOOL_ARGUMENTS")
	questions.CancelRequest("question")
	permissions.rows = nil
	expired, err := reader.ReadAttention(ctx, b, task.ID, nil)
	require.NoError(t, err)
	for _, row := range expired {
		require.Equal(t, "expired", row.State)
	}
	b.WorkspaceID = "foreign"
	_, err = reader.ReadAttention(ctx, b, task.ID, nil)
	require.Error(t, err)
	b.WorkspaceID = "ws-1"
	_, err = reader.ReadAttention(context.Background(), b, task.ID, nil)
	require.Error(t, err)
}
func TestAssistantAttentionNativeErrorsAndNoIdleQuestion(t *testing.T) {
	task := &taskmodels.Task{ID: "worker", State: "IN_PROGRESS"}
	idle := &taskmodels.TaskSession{ID: "idle", TaskID: task.ID, State: taskmodels.TaskSessionStateWaitingForInput}
	require.Empty(t, taskAttentionSources(task, []*taskmodels.TaskSession{idle}))
	idle.Metadata = map[string]any{taskmodels.SessionMetaKeyLastAgentError: taskmodels.LastAgentError{Message: "SYNTHETIC_PRIVATE_ERROR", Code: "auth_required", OccurredAt: time.Now()}}
	rows := taskAttentionSources(task, []*taskmodels.TaskSession{idle})
	require.Len(t, rows, 1)
	require.Equal(t, "authentication", rows[0].Kind)
	raw, _ := json.Marshal(rows)
	require.NotContains(t, string(raw), "SYNTHETIC_PRIVATE_ERROR")
	task.State = "REVIEW"
	rows = taskAttentionSources(task, []*taskmodels.TaskSession{idle})
	require.Len(t, rows, 2)
	require.Equal(t, "review", rows[0].Kind)
}
