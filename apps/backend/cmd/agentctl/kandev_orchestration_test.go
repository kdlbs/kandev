package main

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrchestrationCreateCarriesRouteAndOperationIdentity(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.WriteHeader(201)
	}))
	defer server.Close()
	t.Setenv("KANDEV_API_URL", server.URL)
	t.Setenv("KANDEV_API_KEY", "token")
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", "/api/v1/orchestration")
	t.Setenv("KANDEV_INTENT_REVISION", "7")
	require.Zero(t, taskCreate([]string{"--title", "Known change", "--workflow", "workflow", "--step", "implementation", "--mode", "execute", "--objective", "goal", "--context", "packet", "--operation-id", "create-1"}))
	require.Equal(t, "implementation", request["workflow_step_id"])
	require.Equal(t, "execute", request["execution_mode"])
	require.Equal(t, "goal", request["objective_id"])
	require.Equal(t, "packet", request["context_ref"])
	require.Equal(t, "create-1", request["operation_id"])
	require.EqualValues(t, 7, request["expected_intent_revision"])
}

func TestOrchestrationObjectiveUsesBoundedTypedPayload(t *testing.T) {
	var request map[string]any
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.WriteHeader(201)
	}))
	defer server.Close()
	t.Setenv("KANDEV_API_URL", server.URL)
	t.Setenv("KANDEV_API_KEY", "token")
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", "/api/v1/orchestration")
	t.Setenv("KANDEV_INTENT_REVISION", "7")
	require.Zero(t, runOrchestrationCLI([]string{"objective", "create", "--title", "Inspect", "--mode", "inspect", "--source-comment", "comment", "--acceptance", `[{"id":"summary","description":"Summarize"}]`, "--operation-id", "goal-1"}))
	require.Equal(t, "/api/v1/orchestration/runtime/objectives", path)
	require.Equal(t, "inspect", request["mode"])
	require.Len(t, request["acceptance"], 1)
}

func TestOrchestrationRuntimeUsesOwnAPI(t *testing.T) {
	var path, token, run string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		token = r.Header.Get("Authorization")
		run = r.Header.Get("X-Kandev-Run-Id")
		w.WriteHeader(200)
	}))
	defer server.Close()
	t.Setenv("KANDEV_API_URL", server.URL)
	t.Setenv("KANDEV_API_KEY", "test-token")
	t.Setenv("KANDEV_RUN_ID", "run")
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", "/api/v1/orchestration")
	client, err := newKandevClient()
	require.NoError(t, err)
	_, _, err = client.do("POST", "/api/v1/office/runtime/tasks", map[string]string{"title": "Review"})
	require.NoError(t, err)
	require.Equal(t, "/api/v1/orchestration/runtime/tasks", path)
	require.Equal(t, "Bearer test-token", token)
	require.Equal(t, "run", run)
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", "")
	client, err = newKandevClient()
	require.NoError(t, err)
	_, _, err = client.do("GET", "/api/v1/office/agents", nil)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/office/agents", path)
}
