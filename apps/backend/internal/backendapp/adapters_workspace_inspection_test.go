package backendapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceTaskDetailsDoesNotExportRuntimeSnapshots(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := &models.Task{ID: "sample-task", WorkspaceID: "ws-1", Title: "Inspect a sample result", Description: strings.Repeat("sample description ", 4000), Metadata: map[string]any{"private_runtime_marker": strings.Repeat("x", 80000)}}
	require.NoError(t, a.taskRepo.CreateTask(ctx, task))
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "sample-session", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput, Metadata: map[string]any{"private_runtime_marker": strings.Repeat("y", 80000)}}))
	result, err := a.WorkspaceTaskDetails(ctx, "ws-1", task.ID)
	require.NoError(t, err)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private_runtime_marker")
	require.Less(t, len(raw), 16000)
}

func TestWorkspaceTaskContentReadsLongResultInPages(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := &models.Task{ID: "sample-task", WorkspaceID: "ws-1", Title: "Sample result"}
	require.NoError(t, a.taskRepo.CreateTask(ctx, task))
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "sample-session", TaskID: task.ID}))
	require.NoError(t, a.taskRepo.CreateTurn(ctx, &models.Turn{ID: "sample-turn", TaskID: task.ID, TaskSessionID: "sample-session"}))
	content := strings.Repeat("Example result. ", 600) + "Remaining verification is blocked."
	require.NoError(t, a.taskRepo.CreateMessage(ctx, &models.Message{ID: "sample-message", TaskID: task.ID, TaskSessionID: "sample-session", TurnID: "sample-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: content}))
	query := shared.WorkspaceContentQuery{MessageID: "sample-message", Limit: 2000}
	var assembled strings.Builder
	for {
		result, err := a.WorkspaceTaskContent(ctx, "ws-1", task.ID, query)
		require.NoError(t, err)
		require.NotNil(t, result)
		page := result.(map[string]any)
		assembled.WriteString(page["content"].(string))
		if !page["has_more"].(bool) {
			break
		}
		query.Offset = page["next_offset"].(int)
	}
	require.Equal(t, content, assembled.String())
	_, err := a.WorkspaceTaskContent(ctx, "foreign", task.ID, query)
	require.Error(t, err)
	query.Offset = -1
	_, err = a.WorkspaceTaskContent(ctx, "ws-1", task.ID, query)
	require.Error(t, err)
}

func TestWorkspaceContentPreservesShellFailuresAndAnsweredQuestions(t *testing.T) {
	message := &models.Message{Content: "Run sample check", Metadata: map[string]any{"normalized": map[string]any{"shell_exec": map[string]any{"output": map[string]any{"stdout": "Permission denied: Blocked by classifier"}}}}}
	require.Contains(t, workspaceMessageText(message), "Blocked by classifier")
	message.Type = "clarification_request"
	message.Metadata = map[string]any{"question": map[string]any{"prompt": "Use the sample database?"}, "response": map[string]any{"custom_text": "Use the disposable copy."}, "status": "answered", "context": strings.Repeat("irrelevant context", 5000)}
	text := workspaceMessageText(message)
	require.Contains(t, text, "Use the disposable copy.")
	require.NotContains(t, text, "irrelevant context")
}

func TestWorkspaceContentRejectsForeignMessageAndSession(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	for _, id := range []string{"own", "other"} {
		require.NoError(t, a.taskRepo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "ws-1", Title: "Synthetic task", Description: "A sample description"}))
		require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: id, TaskID: id}))
		require.NoError(t, a.taskRepo.CreateTurn(ctx, &models.Turn{ID: id, TaskID: id, TaskSessionID: id}))
		require.NoError(t, a.taskRepo.CreateMessage(ctx, &models.Message{ID: id, TaskID: id, TaskSessionID: id, TurnID: id, Type: models.MessageTypeMessage, Content: "Example result"}))
	}
	for _, query := range []shared.WorkspaceContentQuery{{MessageID: "other"}, {SessionID: "other"}, {SessionID: "own", Before: "other"}, {MessageID: "own", SessionID: "other"}, {Limit: 9000}} {
		_, err := a.WorkspaceTaskContent(ctx, "ws-1", "own", query)
		require.Error(t, err)
	}
	result, err := a.WorkspaceTaskContent(ctx, "ws-1", "own", shared.WorkspaceContentQuery{Source: "description"})
	require.NoError(t, err)
	require.Equal(t, "A sample description", result.(map[string]any)["content"])
}
