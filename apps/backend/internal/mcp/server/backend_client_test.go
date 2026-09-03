package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestChannelBackendClientCloseBeforeRequestDoesNotPublish(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	err := client.RequestPayload(ctx, "test.action", map[string]any{"value": "test"}, nil)
	require.EqualError(t, err, "MCP backend client is closed")

	select {
	case msg := <-client.GetRequestChannel():
		t.Fatalf("closed client published request %q", msg.Action)
	default:
	}
}

func TestChannelBackendClientCloseReleasesPublishedRequest(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()

	select {
	case <-client.GetRequestChannel():
	case <-time.After(time.Second):
		t.Fatal("request was not published")
	}
	client.Close()

	select {
	case err := <-errCh:
		require.EqualError(t, err, "MCP backend client is closed")
	case <-time.After(time.Second):
		t.Fatal("published request remained blocked after client close")
	}
}

// TestChannelBackendClientPreservesStructuredErrorDetails is a regression test
// for a bug where a backend error response's structured Details (e.g. a
// related-task-read denial's "reason") were discarded, leaving MCP tool
// callers with only a flattened "backend error [CODE]: message" string and no
// way to branch on the denial reason programmatically.
func TestChannelBackendClientPreservesStructuredErrorDetails(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()

	var reqMsg *ws.Message
	select {
	case reqMsg = <-client.GetRequestChannel():
	case <-time.After(time.Second):
		t.Fatal("request was not published")
	}

	errPayload := ws.ErrorPayload{
		Code:    "FORBIDDEN",
		Message: "related task access denied",
		Details: map[string]interface{}{"reason": "related_task_scope_required"},
	}
	payloadBytes, err := json.Marshal(errPayload)
	require.NoError(t, err)
	client.HandleResponse(&ws.Message{
		ID:      reqMsg.ID,
		Type:    ws.MessageTypeError,
		Action:  reqMsg.Action,
		Payload: payloadBytes,
	})

	select {
	case err := <-errCh:
		require.Error(t, err)
		var backendErr *BackendError
		require.ErrorAs(t, err, &backendErr)
		assert.Equal(t, "FORBIDDEN", backendErr.Code)
		assert.Equal(t, "related task access denied", backendErr.Message)
		assert.Equal(t, "related_task_scope_required", backendErr.Details["reason"])
	case <-time.After(time.Second):
		t.Fatal("RequestPayload did not return")
	}
}

func TestChannelBackendClientRedactsPluginInvocationPayload(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	client := NewChannelBackendClient(log)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = client.RequestPayload(ctx, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "echo", "arguments": map[string]any{"token": "secret-value"},
	}, nil)

	entries := observed.FilterMessage("sending MCP request through agent stream").All()
	require.Len(t, entries, 1)
	payload, ok := entries[0].ContextMap()["payload"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "echo", payload["plugin_id"])
	_, loggedArguments := payload["arguments"]
	require.False(t, loggedArguments)
}
