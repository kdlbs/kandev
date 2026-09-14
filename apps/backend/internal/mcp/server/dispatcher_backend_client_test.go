package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.1
// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.2
// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.9
// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.10
func TestDispatcherBackendClient_EmptyPayloadWithResultSinkErrors(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	d := &fakeDispatcher{resp: &ws.Message{Type: ws.MessageTypeResponse, Payload: nil}}
	client := NewDispatcherBackendClient(d, log)

	var result map[string]interface{}
	respErr := client.RequestPayload(context.Background(), "test.action", map[string]any{"secret": "canary-leak-9f3a"}, &result)
	require.Error(t, respErr)
	require.ErrorIs(t, respErr, ErrEmptyBackendPayload)
	require.Contains(t, respErr.Error(), "test.action")
	require.NotContains(t, respErr.Error(), "secret")

	require.Len(t, d.calls, 1)
	outboundID := d.calls[0].ID
	require.NotEmpty(t, outboundID)

	entries := observed.FilterMessage(ErrEmptyBackendPayload.Error()).All()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	require.Equal(t, outboundID, fields["request_id"])
	require.Equal(t, "test.action", fields["action"])
	_, hasSessionID := fields["session_id"]
	require.False(t, hasSessionID)
	_, hasDuration := fields["duration"]
	require.False(t, hasDuration)

	logJSON, err := json.Marshal(fields)
	require.NoError(t, err)
	require.NotContains(t, string(logJSON), "secret")
	require.NotContains(t, string(logJSON), "canary-leak-9f3a")
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.3
func TestDispatcherBackendClient_EmptyPayloadWithNilResultSucceeds(t *testing.T) {
	log := newTestLogger(t)

	d := &fakeDispatcher{resp: &ws.Message{Type: ws.MessageTypeResponse, Payload: nil}}
	client := NewDispatcherBackendClient(d, log)

	err := client.RequestPayload(context.Background(), "test.action", nil, nil)
	require.NoError(t, err)
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.4
func TestDispatcherBackendClient_ErrorTypeWithEmptyPayloadKeepsBackendErrorBehavior(t *testing.T) {
	log := newTestLogger(t)

	d := &fakeDispatcher{resp: &ws.Message{Type: ws.MessageTypeError, Payload: nil}}
	client := NewDispatcherBackendClient(d, log)

	var result map[string]interface{}
	err := client.RequestPayload(context.Background(), "test.action", nil, &result)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrEmptyBackendPayload)
	require.Contains(t, err.Error(), "backend error")
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.5
// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.8
func TestDispatcherBackendClient_EmptyObjectPayloadDecodesToNonNilEmptyMap(t *testing.T) {
	log := newTestLogger(t)

	respMsg, err := ws.NewResponse("ignored", "test.action", map[string]interface{}{})
	require.NoError(t, err)
	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	var result map[string]interface{}
	err = client.RequestPayload(context.Background(), "test.action", nil, &result)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result)
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.6
func TestDispatcherBackendClient_NullPayloadStillDecodesWithoutError(t *testing.T) {
	log := newTestLogger(t)

	respMsg, err := ws.NewResponse("ignored", "test.action", nil)
	require.NoError(t, err)
	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	result := map[string]interface{}{"stale": "value"}
	err = client.RequestPayload(context.Background(), "test.action", nil, &result)
	require.NoError(t, err)
	require.Nil(t, result)
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.7
func TestDispatcherBackendClient_InvalidJSONPayloadKeepsUnmarshalError(t *testing.T) {
	log := newTestLogger(t)

	respMsg := &ws.Message{Type: ws.MessageTypeResponse, Payload: json.RawMessage("not json")}
	d := &fakeDispatcher{resp: respMsg}
	client := NewDispatcherBackendClient(d, log)

	var result map[string]interface{}
	err := client.RequestPayload(context.Background(), "test.action", nil, &result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal")
	require.NotErrorIs(t, err, ErrEmptyBackendPayload)
}
