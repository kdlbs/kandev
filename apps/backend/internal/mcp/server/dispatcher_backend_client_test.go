package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDispatcher is a minimal Dispatcher used to drive DispatcherBackendClient tests.
type fakeDispatcher struct {
	resp *ws.Message
	err  error

	calls                    []*ws.Message
	trustedExternalTransport bool
}

func (f *fakeDispatcher) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	f.calls = append(f.calls, msg)
	f.trustedExternalTransport = mcporigin.IsTrustedExternalTransport(ctx)
	return f.resp, f.err
}

func TestExternalDispatcherBackendClientAttestsTransport(t *testing.T) {
	respMsg, err := ws.NewResponse("ignored", "test.action", map[string]bool{"ok": true})
	require.NoError(t, err)
	d := &fakeDispatcher{resp: respMsg}
	client := NewExternalDispatcherBackendClient(d, newTestLogger(t))

	require.NoError(t, client.RequestPayload(context.Background(), "test.action", nil, nil))
	assert.True(t, d.trustedExternalTransport)
}

func TestDispatcherBackendClient_RoundTrip(t *testing.T) {
	log := newTestLogger(t)

	type result struct {
		Hello string `json:"hello"`
	}
	respMsg, err := ws.NewResponse("ignored", "test.action", result{Hello: "world"})
	require.NoError(t, err)

	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	var got result
	err = client.RequestPayload(context.Background(), "test.action", map[string]string{"k": "v"}, &got)
	require.NoError(t, err)
	assert.Equal(t, "world", got.Hello)

	require.Len(t, d.calls, 1)
	assert.Equal(t, "test.action", d.calls[0].Action)
	assert.NotEmpty(t, d.calls[0].ID)
	assert.False(t, d.trustedExternalTransport)
}

func TestDispatcherBackendClient_ErrorResponse(t *testing.T) {
	log := newTestLogger(t)

	errPayload, _ := json.Marshal(map[string]string{"code": "BAD", "message": "boom"})
	respMsg := &ws.Message{
		ID:      "x",
		Action:  "test.action",
		Type:    ws.MessageTypeError,
		Payload: errPayload,
	}

	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BAD")
	assert.Contains(t, err.Error(), "boom")
}

// TestDispatcherBackendClient_ErrorResponsePreservesDetails is a regression
// test for a bug where a backend error response's structured Details (e.g. a
// related-task-read denial's "reason") were discarded, leaving MCP tool
// callers with only a flattened "backend error [CODE]: message" string and no
// way to branch on the denial reason programmatically.
func TestDispatcherBackendClient_ErrorResponsePreservesDetails(t *testing.T) {
	log := newTestLogger(t)

	errPayload, _ := json.Marshal(map[string]any{
		"code":    "FORBIDDEN",
		"message": "related task access denied",
		"details": map[string]any{"reason": "related_task_scope_required"},
	})
	respMsg := &ws.Message{
		ID:      "x",
		Action:  "test.action",
		Type:    ws.MessageTypeError,
		Payload: errPayload,
	}

	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.Error(t, err)
	var backendErr *BackendError
	require.ErrorAs(t, err, &backendErr)
	assert.Equal(t, "FORBIDDEN", backendErr.Code)
	assert.Equal(t, "related task access denied", backendErr.Message)
	assert.Equal(t, "related_task_scope_required", backendErr.Details["reason"])
}

func TestDispatcherBackendClient_DispatchError(t *testing.T) {
	log := newTestLogger(t)

	d := &fakeDispatcher{err: errors.New("dispatcher boom")}
	client := NewDispatcherBackendClient(d, log)

	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatcher boom")
}

func TestDispatcherBackendClient_NilResponse(t *testing.T) {
	log := newTestLogger(t)

	d := &fakeDispatcher{resp: nil}
	client := NewDispatcherBackendClient(d, log)

	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil response")
}

func TestDispatcherBackendClient_NilResultIsAllowed(t *testing.T) {
	log := newTestLogger(t)

	respMsg, err := ws.NewResponse("ignored", "test.action", map[string]string{"ok": "yes"})
	require.NoError(t, err)

	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	err = client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.NoError(t, err)
}
