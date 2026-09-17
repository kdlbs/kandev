package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssistantFeatureGateCLIWithholdsCommands(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("KANDEV_API_URL", server.URL)
	t.Setenv("KANDEV_API_KEY", "synthetic-fixture-token")
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", orchestrationAPIPrefix)
	t.Setenv("KANDEV_PERSONAL_ASSISTANT_ENABLED", "false")
	for _, args := range [][]string{{"objective", "list"}, {"context", "--objective", "example", "--profile", "example"}} {
		require.Equal(t, 1, runOrchestrationCLI(args))
	}
	require.Zero(t, calls.Load(), "disabled commands must not dispatch requests")
}
