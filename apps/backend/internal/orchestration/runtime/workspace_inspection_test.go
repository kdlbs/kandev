package runtime

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

type workspaceInspectionManager struct {
	*assistantTaskManager
	workspace, task, session string
	query                    models.WorkspaceContentQuery
}

func (m *workspaceInspectionManager) WorkspaceTaskContent(_ context.Context, workspace, task string, query models.WorkspaceContentQuery) (any, error) {
	m.workspace, m.task, m.query = workspace, task, query
	return map[string]any{"content": "Generic result", "has_more": false}, nil
}
func (m *workspaceInspectionManager) WorkspaceTaskPermissions(_ context.Context, workspace, task, session string) (any, error) {
	m.workspace, m.task, m.session = workspace, task, session
	return map[string]any{"permissions": []any{}}, nil
}
func (m *workspaceInspectionManager) WorkspaceTaskDetails(context.Context, string, string) (any, error) {
	return map[string]any{"task": "task", "messages": []string{"sample"}, "session_results": []string{"sample"}}, nil
}

func TestWorkspaceInspectionUsesSignedScopeAndRejectsExpiredRun(t *testing.T) {
	s, _, task := newRuntime(t)
	m := &workspaceInspectionManager{assistantTaskManager: &assistantTaskManager{}}
	s.Manager = m
	router, token, run := workspaceControlCaller(t, s, task)
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/tasks/target/content?workspace_id=foreign&session_id=session&message_id=message&offset=3000&limit=2000", token, run, nil)
	require.Equal(t, 403, response.Code)
	response = runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/tasks/target/content?session_id=session&message_id=message&offset=3000&limit=2000", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, "ws", m.workspace)
	require.Equal(t, "target", m.task)
	require.Equal(t, 3000, m.query.Offset)
	response = runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/tasks/target/permissions?session_id=session", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, "session", m.session)
	response = runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/tasks/target/details?include_result=false", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "sample")
	require.NoError(t, s.Runs.FinishRun(context.Background(), run, "finished", nil))
	for _, path := range []string{"content", "permissions"} {
		response = runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/tasks/target/"+path, token, run, nil)
		require.Equal(t, 403, response.Code)
	}
}
