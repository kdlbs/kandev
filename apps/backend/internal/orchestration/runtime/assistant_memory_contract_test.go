package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMemoryOwnerPagination(t *testing.T) {
	s, _, _, _ := assistantContextFixture(t)
	ctx := context.Background()
	b, err := s.Repo.AssistantBinding(ctx, "owner")
	require.NoError(t, err)
	for i := 0; i < 205; i++ {
		id := fmt.Sprintf("m%03d", i)
		require.NoError(t, s.Repo.SaveAssistantMemory(ctx, &models.AgentMemory{ID: id, AgentProfileID: b.OrchestratorID, OwnerUserID: b.OwnerUserID, Layer: "user", Key: id, Content: "Example preference", Scope: "workspace", ScopeID: b.WorkspaceID}, 0))
	}
	expired := time.Now().Add(-time.Hour)
	for _, row := range []*models.AgentMemory{
		{ID: "a-foreign", OwnerUserID: "foreign"}, {ID: "b-expired", OwnerUserID: "owner", ExpiresAt: &expired}, {ID: "c-forgotten", OwnerUserID: "owner"},
	} {
		row.AgentProfileID, row.Layer, row.Key, row.Content, row.Scope, row.ScopeID = b.OrchestratorID, "user", row.ID, "Excluded preference", "workspace", b.WorkspaceID
		require.NoError(t, s.Repo.SaveAssistantMemory(ctx, row, 0))
	}
	require.NoError(t, s.Repo.ForgetAssistantMemory(ctx, b.OrchestratorID, "c-forgotten", 1))
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/memory"
	read := func(query string) ([]models.AgentMemory, string) {
		t.Helper()
		response := runtimeRequest(t, router, "GET", path+query, "", "", nil)
		require.Equal(t, 200, response.Code, response.Body.String())
		var page struct {
			Memory []models.AgentMemory `json:"memory"`
			Next   string               `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
		return page.Memory, page.Next
	}
	rows, cursor := read("")
	require.Len(t, rows, 50)
	require.Equal(t, "m000", rows[0].ID)
	require.NotEmpty(t, cursor)
	require.Equal(t, 400, runtimeRequest(t, router, "GET", path+"?after="+url.QueryEscape(cursor)+"&scope=task", "", "", nil).Code)
	require.Equal(t, 400, runtimeRequest(t, router, "GET", path+"?after=foreign-id", "", "", nil).Code)
	rows, cursor = read("?limit=150")
	require.Len(t, rows, 100)
	// An edit between pages does not move ID-ordered rows or invalidate unrelated cursors.
	edited, err := s.Repo.AssistantMemory(ctx, b.OrchestratorID, "m102")
	require.NoError(t, err)
	edited.Content = "Corrected example preference"
	require.NoError(t, s.Repo.SaveAssistantMemory(ctx, edited, 1))
	require.ErrorIs(t, s.Repo.SaveAssistantMemory(ctx, edited, 1), models.ErrConflict)
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.ID] = true
	}
	for cursor != "" {
		rows, cursor = read("?limit=100&after=" + url.QueryEscape(cursor))
		for _, row := range rows {
			require.False(t, seen[row.ID])
			seen[row.ID] = true
		}
	}
	require.Len(t, seen, 205)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", path, "", "", nil).Code)
}

func TestAssistantMemoryScopeValidation(t *testing.T) {
	s, _, _, _ := assistantContextFixture(t)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/memory/preference"
	req := map[string]any{"key": "preference", "content": "Keep this preference", "scope": "workspace", "source_comment_id": "source", "confirmed": true, "expected_revision": 0}
	require.Equal(t, 200, runtimeRequest(t, router, "PUT", path, "", "", req).Code)
	for _, scope := range []string{"workspace", "user", "project", "task", "environment"} {
		req["scope"], req["scope_id"], req["expected_revision"], req["content"] = scope, "foreign", 1, "Rejected replacement"
		require.Equal(t, 422, runtimeRequest(t, router, "PUT", path, "", "", req).Code, scope)
		row, err := s.Repo.AssistantMemory(context.Background(), "chief", "preference")
		require.NoError(t, err)
		require.Equal(t, "Keep this preference", row.Content)
		require.EqualValues(t, 1, row.Revision)
	}
	s.Tasks.(*testTasks).tasks["own-task"] = &taskmodels.Task{ID: "own-task", WorkspaceID: "ws"}
	req["scope"], req["scope_id"], req["content"] = "task", "own-task", "Task preference"
	require.Equal(t, 200, runtimeRequest(t, router, "PUT", path, "", "", req).Code)
	delete(s.Tasks.(*testTasks).tasks, "own-task")
	require.Equal(t, 404, runtimeRequest(t, router, "GET", path, "", "", nil).Code)
}
