package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.2, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationRegisteredRoutes(t *testing.T) {
	h := newIssueMutationHarness(t, nil)
	gh, store := issueMutationAuthenticatedGitHub(t, h, issueMutationRemote)
	router := issueMutationRouter(t, h, gh)
	ctx := context.Background()
	task, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	task.Metadata = issueMutationMetadata(7)
	require.NoError(t, h.repos[0].UpdateTask(ctx, task))
	for _, operation := range []struct {
		method, path, body string
		issue              int
	}{
		{http.MethodPut, "/api/v1/github/tasks/issue-task/issue", `{"issue":"https://github.com/acme/api/issues/42"}`, 42},
		{http.MethodPatch, "/api/v1/tasks/issue-task", `{"title":"Accepted title","priority":"high"}`, 42},
		{http.MethodPatch, "/api/v1/tasks/issue-task/port-forwarding", `{"enabled":true}`, 42},
		{http.MethodDelete, "/api/v1/github/tasks/issue-task/issue", ``, 0},
	} {
		response := issueMutationHTTP(ctx, router, operation.method, operation.path, operation.body)
		require.Equal(t, 200, response.Code, response.Body.String())
		stored, err := h.repos[0].GetTask(ctx, "issue-task")
		require.NoError(t, err)
		assertIssueMutationIdentity(t, stored.Metadata, operation.issue)
		current := issueMutationHTTP(ctx, router, http.MethodGet, "/api/v1/tasks/issue-task", "")
		require.Equal(t, 200, current.Code, current.Body.String())
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(current.Body.Bytes(), &body))
		encoded, err := json.Marshal(dto.FromTask(stored))
		require.NoError(t, err)
		var expected map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &expected))
		// The registered task GET enriches repository data. Compare the persisted task projection.
		require.Equal(t, expected["metadata"], body["metadata"])
		require.Equal(t, expected["title"], body["title"])
		require.Equal(t, expected["updated_at"], body["updated_at"])
		index := 0
		if operation.method == http.MethodPatch {
			index = 1
		}
		published := h.buses[index].snapshot()
		event := published[len(published)-1]
		require.Equal(t, events.TaskUpdated, event.Type)
		data := event.Data.(map[string]interface{})
		require.Equal(t, stored.Title, data["title"])
		require.Equal(t, stored.Metadata, data["metadata"])
	}
	stored, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	require.Equal(t, "Accepted title", stored.Title)
	require.Equal(t, "high", stored.Priority)
	require.Equal(t, true, stored.Metadata["port_forwarding_enabled"])
	require.Equal(t, "before", stored.Metadata["alpha"])
	for _, eb := range h.buses {
		require.Len(t, eb.snapshot(), 2)
		for _, event := range eb.snapshot() {
			require.Equal(t, events.TaskUpdated, event.Type)
		}
	}
	connection, err := store.GetWorkspaceConnection(ctx, "issue-ws")
	require.NoError(t, err)
	require.Equal(t, "fixture", connection.Login)
}
