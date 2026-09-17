package main

import (
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func TestOrchestrationContextFetchesScopedPacket(t *testing.T) {
	t.Setenv("KANDEV_PERSONAL_ASSISTANT_ENABLED", "true")
	captured := setupMockTransport(t, 200, `{"id":"packet"}`)
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", orchestrationAPIPrefix)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "synthetic-token")
	require.Zero(t, runKandevCLI([]string{"context", "--objective", "goal", "--profile", "personal", "--task", "worker"}))
	require.Equal(t, "GET", captured.Method)
	require.Equal(t, "/api/v1/orchestration/runtime/context/goal", captured.Path)
	query, err := url.ParseQuery(captured.Query)
	require.NoError(t, err)
	require.Equal(t, "personal", query.Get("profile_id"))
	require.Equal(t, "worker", query.Get("task_id"))
}

func TestOrchestrationContextMemoryUsesScopedContinuation(t *testing.T) {
	t.Setenv("KANDEV_PERSONAL_ASSISTANT_ENABLED", "true")
	captured := setupMockTransport(t, 200, `{"memory":[],"next_cursor":""}`)
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", orchestrationAPIPrefix)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "synthetic-token")
	require.Zero(t, runKandevCLI([]string{"context", "--objective", "goal", "--profile", "personal", "--task", "worker", "--memory", "--after", "scoped-cursor", "--limit", "100"}))
	require.Equal(t, "/api/v1/orchestration/runtime/context/goal/memory", captured.Path)
	query, err := url.ParseQuery(captured.Query)
	require.NoError(t, err)
	require.Equal(t, "scoped-cursor", query.Get("after"))
	require.Equal(t, "100", query.Get("limit"))
	require.Equal(t, "worker", query.Get("task_id"))
}
