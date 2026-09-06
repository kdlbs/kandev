package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
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

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.2
func TestChannelBackendClientDoesNotQueueRequestWithoutStreamConsumer(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancel)

	err := client.RequestPayload(ctx, "test.action", nil, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	select {
	case msg := <-client.GetRequestChannel():
		t.Fatalf("request %q was queued without a stream consumer", msg.ID)
	default:
	}
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.5
func TestChannelBackendClientDisconnectOnlyFailsRequestsOwnedByThatStream(t *testing.T) {
	client := NewChannelBackendClient(nil)
	t.Cleanup(client.Close)

	oldErrCh := make(chan error, 1)
	go func() {
		oldErrCh <- client.RequestPayload(context.Background(), "old.action", nil, nil)
	}()
	oldRequest := <-client.GetRequestChannel()
	client.BindRequestToStream(oldRequest.ID, "old-stream")

	newErrCh := make(chan error, 1)
	go func() {
		newErrCh <- client.RequestPayload(context.Background(), "new.action", nil, nil)
	}()
	newRequest := <-client.GetRequestChannel()
	client.BindRequestToStream(newRequest.ID, "new-stream")

	client.FailStreamRequests("old-stream", errors.New("agent stream disconnected"))
	require.EqualError(t, <-oldErrCh, "agent stream disconnected")
	select {
	case err := <-newErrCh:
		t.Fatalf("replacement stream request completed early: %v", err)
	default:
	}

	response, err := ws.NewResponse(newRequest.ID, newRequest.Action, nil)
	require.NoError(t, err)
	client.HandleResponse(response)
	require.NoError(t, <-newErrCh)
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.7
func TestChannelBackendClientTerminalFailureLogIncludesConfiguredSession(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	client := NewChannelBackendClient(log)
	t.Cleanup(client.Close)
	_ = New(client, "session-log", "task-log", 0, log, "", false, ModeTask)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RequestPayload(context.Background(), "test.action", nil, nil)
	}()
	request := <-client.GetRequestChannel()
	client.BindRequestToStream(request.ID, "failed-stream")
	client.FailStreamRequests("failed-stream", errors.New("agent stream disconnected"))
	require.EqualError(t, <-errCh, "agent stream disconnected")

	entries := observed.FilterMessage("MCP request failed after publication").All()
	require.Len(t, entries, 1)
	require.Equal(t, "session-log", entries[0].ContextMap()["session_id"])
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

func TestChannelBackendClientRedactsPendingMoveCancellationPayload(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	client := NewChannelBackendClient(log)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = client.RequestPayload(ctx, ws.ActionMCPCancelPendingMove, map[string]any{
		"pending_move_id": "secret-target-row", "task_id": "secret-target-task",
	}, nil)

	entries := observed.FilterMessage("sending MCP request through agent stream").All()
	require.Len(t, entries, 1)
	require.Equal(t, "<redacted>", entries[0].ContextMap()["payload"])
}

func TestChannelBackendClientRedactsPendingMoveReadPayload(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	client := NewChannelBackendClient(log)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = client.RequestPayload(ctx, ws.ActionMCPReadPendingMove, map[string]any{
		"task_id": "secret-target-task",
	}, nil)

	entries := observed.FilterMessage("sending MCP request through agent stream").All()
	require.Len(t, entries, 1)
	require.Equal(t, "<redacted>", entries[0].ContextMap()["payload"])
}
