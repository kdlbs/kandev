package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func assistantRuntimeRouter(s *Service) *gin.Engine {
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	return router
}

func assistantContextFixture(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	s, _, task := newRuntime(t)
	router, token, run := assistantRuntimeCaller(t, s, task)
	require.NoError(t, s.Repo.PutComment(context.Background(), &models.TaskComment{
		ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Body: "Inspect my staging setup", Source: "user",
	}))
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives", token, run, map[string]any{
		"title": "Inspect staging", "mode": "inspect", "source_comment_id": "source",
		"acceptance":   []map[string]string{{"id": "summary", "description": "Report evidence"}},
		"operation_id": "context-goal", "expected_intent_revision": 0,
	})
	require.Equal(t, 201, response.Code, response.Body.String())
	var goal models.Objective
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &goal))
	return s, token, run, goal.ID
}

func TestAssistantCredentialScopedReferenceOnly(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	router := assistantRouter(s)
	agentRouter := assistantRuntimeRouter(s)
	path := "/api/v1/orchestration/assistant/credentials/staging"
	request := map[string]any{
		"resolver": "bitwarden", "reference": "synthetic-item-id", "purpose": "Staging dashboard",
		"profile_id": "personal", "account": "staging-only", "environment": "staging",
		"scope": "workspace", "fields": []string{"username", "password"},
		"unlock_policy": "Ask owner to unlock Bitwarden", "expected_revision": 0,
	}
	response := runtimeRequest(t, router, "PUT", path, "", "", request)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "\"value\"")
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", path, "", "", nil).Code)
	require.Equal(t, 403, runtimeRequest(t, agentRouter, "PUT", path, token, run, request).Code)
	for _, taskID := range []string{"first-task", "second-task"} {
		s.Tasks.(*testTasks).tasks[taskID] = &taskmodels.Task{ID: taskID, WorkspaceID: "ws"}
		packet := runtimeRequest(t, agentRouter, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal&task_id="+taskID, token, run, nil)
		require.Equal(t, 200, packet.Code, packet.Body.String())
		require.Contains(t, packet.Body.String(), "synthetic-item-id")
		require.Contains(t, packet.Body.String(), "unavailable")
	}
	s.Tasks.(*testTasks).tasks["foreign-task"] = &taskmodels.Task{ID: "foreign-task", WorkspaceID: "foreign"}
	require.Equal(t, 422, runtimeRequest(t, agentRouter, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal&task_id=foreign-task", token, run, nil).Code)
	require.NoError(t, s.Personas.Profiles.CreateAgentProfile(context.Background(), &settings.AgentProfile{ID: "work", AgentID: "claude", Name: "Work"}))
	other := runtimeRequest(t, agentRouter, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=work", token, run, nil)
	require.Equal(t, 200, other.Code, other.Body.String())
	require.NotContains(t, other.Body.String(), "synthetic-item-id")
	for _, state := range []string{"locked", "ready", "SYNTHETIC_SECRET_CANARY"} {
		s.Credentials = syntheticCredentialHealth(state)
		response := runtimeRequest(t, agentRouter, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal", token, run, nil)
		require.Equal(t, 200, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), "SYNTHETIC_SECRET_CANARY")
		if state == "locked" {
			require.Contains(t, response.Body.String(), "\"unblock_action\":\"Ask owner to unlock Bitwarden\"")
		}
		if state == "ready" {
			require.NotContains(t, response.Body.String(), "\"unblock_action\"")
		}
	}
	request["value"] = "SYNTHETIC_SECRET_CANARY"
	require.Equal(t, 422, runtimeRequest(t, router, "PUT", path, "", "", request).Code)
	require.Equal(t, 200, runtimeRequest(t, router, "DELETE", path, "", "", map[string]any{"expected_revision": 1}).Code)
	packet := runtimeRequest(t, agentRouter, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal", token, run, nil)
	require.NotContains(t, packet.Body.String(), "synthetic-item-id")
}

type syntheticCredentialHealth string

func (h syntheticCredentialHealth) CredentialHealth(context.Context, string, models.CredentialDescriptor) models.CredentialValidation {
	now := time.Now().UTC()
	return models.CredentialValidation{Status: string(h), ValidatedAt: &now, ConfigurationGeneration: "synthetic-generation"}
}
