package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDispatcher is a minimal Dispatcher used to drive DispatcherBackendClient tests.
type fakeDispatcher struct {
	resp *ws.Message
	err  error

	calls []*ws.Message
}

func (f *fakeDispatcher) Dispatch(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	f.calls = append(f.calls, msg)
	return f.resp, f.err
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
