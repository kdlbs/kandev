package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type assistantDelayedTransport struct{ calls int }

func (r *assistantDelayedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.calls++
	timer := time.NewTimer(31 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"accepted"}`)), Request: request}, nil
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
}

func TestAssistantBrokerWaitsForColdSessionResumeWithoutRetry(t *testing.T) {
	for _, action := range []string{"start", "message"} {
		t.Run(action, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				transport := &assistantDelayedTransport{}
				client := &kandevClient{apiURL: "http://example.invalid", http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}
				result, err := callAssistantBroker(client, assistantBrokerTool{name: "manage_task", method: http.MethodPost, path: "/runtime/tasks/:id/manage"}, map[string]any{"id": "sample", "request": map[string]any{"action": action}})
				require.NoError(t, err)
				require.False(t, result.IsError, "%+v", result.Content)
				require.Equal(t, 1, transport.calls)
				require.Equal(t, 30*time.Second, client.http.Timeout, "shared read client must retain its deadline")
			})
		})
	}
}

func TestAssistantBrokerPreservesReadDeadlineAndDoesNotRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := &assistantDelayedTransport{}
		client := &kandevClient{apiURL: "http://example.invalid", http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}
		result, err := callAssistantBroker(client, assistantBrokerTool{name: "task_details", method: http.MethodGet, path: "/runtime/tasks/:id/details"}, map[string]any{"id": "sample"})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Equal(t, 1, transport.calls)
	})
}
