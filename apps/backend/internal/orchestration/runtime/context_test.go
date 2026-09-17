package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMemoryOwnerConfirmationAndForget(t *testing.T) {
	s, _, task := newRuntime(t)
	_, _, _ = assistantRuntimeCaller(t, s, task)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/memory/preference"
	request := map[string]any{"key": "concise", "content": "Use short updates", "scope": "workspace", "source_comment_id": "source", "expected_revision": 0, "confirmed": true}
	require.NoError(t, s.Repo.PutComment(context.Background(), &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Body: "Use short updates", Source: "user"}))
	response := runtimeRequest(t, router, "PUT", path, "", "", request)
	require.Equal(t, 200, response.Code, response.Body.String())
	var row map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &row))
	require.Equal(t, "owner", row["owner_user_id"])
	require.Equal(t, true, row["confirmed"])
	require.EqualValues(t, 1, row["revision"])
	require.Equal(t, 409, runtimeRequest(t, router, "PUT", path, "", "", request).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", path, "", "", nil).Code)
	require.Equal(t, 200, runtimeRequest(t, router, "DELETE", path, "", "", map[string]any{"expected_revision": 1}).Code)
	list := runtimeRequest(t, router, "GET", "/api/v1/orchestration/assistant/memory", "", "", nil)
	require.Equal(t, 200, list.Code, list.Body.String())
	require.NotContains(t, list.Body.String(), "Use short updates")
}

func TestAssistantContextConfirmedPreferenceSurvivesActivity(t *testing.T) {
	s, _, task := newRuntime(t)
	router, token, run := assistantRuntimeCaller(t, s, task)
	ctx := context.Background()
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Body: "Inspect only; concise updates", Source: "user"}))
	request := map[string]any{"title": "Inspect", "mode": "inspect", "source_comment_id": "source", "acceptance": []map[string]string{{"id": "summary", "description": "Summarize"}}, "operation_id": "context-goal", "expected_intent_revision": 0}
	created := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives", token, run, request)
	require.Equal(t, 201, created.Code, created.Body.String())
	var goal models.Objective
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &goal))
	put := runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant/memory/concise", "", "", map[string]any{"key": "concise", "content": "IMPORTANT_CONFIRMED_PREFERENCE", "scope": "workspace", "source_comment_id": "source", "expected_revision": 0, "confirmed": true})
	require.Equal(t, 200, put.Code, put.Body.String())
	for i := 0; i < 30; i++ {
		require.NoError(t, s.Repo.UpsertAgentMemory(ctx, &models.AgentMemory{AgentProfileID: "chief", Layer: "activity", Key: fmt.Sprint(i), Content: "Newer activity", Metadata: "{}"}))
	}
	path := "/api/v1/orchestration/runtime/context/" + goal.ID + "?profile_id=personal"
	first := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 200, first.Code, first.Body.String())
	require.Contains(t, first.Body.String(), "IMPORTANT_CONFIRMED_PREFERENCE")
	require.LessOrEqual(t, first.Body.Len(), 12*1024)
	second := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, first.Body.String(), second.Body.String())
	persona, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	prompt, err := s.prompt(ctx, persona, task, nil)
	require.NoError(t, err)
	require.Contains(t, prompt, "IMPORTANT_CONFIRMED_PREFERENCE")
}
