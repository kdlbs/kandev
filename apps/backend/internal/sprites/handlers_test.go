package sprites

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestNormalizeSpriteStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "running", input: "Running", want: "running"},
		{name: "cold", input: "Cold", want: "cold"},
		{name: "stopped", input: "STOPPED", want: "stopped"},
		{name: "with whitespace", input: "  Running  ", want: "running"},
		{name: "empty string", input: "", want: "unknown"},
		{name: "whitespace only", input: "   ", want: "unknown"},
		{name: "already lowercase", input: "starting", want: "starting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSpriteStatus(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

// wsHandler is the signature shared by every sprites WS handler.
type wsHandler func(context.Context, *ws.Message) (*ws.Message, error)

// newTestHandler builds a Handler with no secret store, so every code path
// short-circuits on "API token not configured" before touching the network.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	return NewHandler(nil, log)
}

func newTestMessage(action string, payload string) *ws.Message {
	msg := &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: action}
	if payload != "" {
		msg.Payload = json.RawMessage(payload)
	}
	return msg
}

func requireErrorPayload(t *testing.T, resp *ws.Message, action, code string) ws.ErrorPayload {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, resp.Type)
	require.Equal(t, "req-1", resp.ID)
	require.Equal(t, action, resp.Action)

	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Equal(t, code, payload.Code)
	return payload
}

func TestWSHandlersRejectMalformedPayload(t *testing.T) {
	h := newTestHandler(t)

	handlers := []struct {
		name   string
		action string
		fn     wsHandler
	}{
		{name: "status", action: ws.ActionSpritesStatus, fn: h.wsStatus},
		{name: "instances list", action: ws.ActionSpritesInstancesList, fn: h.wsListInstances},
		{name: "instances destroy", action: ws.ActionSpritesInstancesDestroy, fn: h.wsDestroyInstance},
		{name: "test", action: ws.ActionSpritesTest, fn: h.wsTest},
		{name: "network policy get", action: ws.ActionSpritesNetworkPolicyGet, fn: h.wsGetNetworkPolicy},
		{name: "network policy update", action: ws.ActionSpritesNetworkPolicyUpdate, fn: h.wsUpdateNetworkPolicy},
	}
	payloads := []struct {
		name string
		body string
	}{
		{name: "truncated json", body: `{"secret_id":`},
		{name: "wrong field type", body: `{"secret_id": 42}`},
		{name: "not an object", body: `["secret-1"]`},
	}

	for _, handler := range handlers {
		for _, payload := range payloads {
			t.Run(handler.name+"/"+payload.name, func(t *testing.T) {
				resp, err := handler.fn(context.Background(), newTestMessage(handler.action, payload.body))
				require.NoError(t, err)

				errPayload := requireErrorPayload(t, resp, handler.action, ws.ErrorCodeBadRequest)
				require.Equal(t, "invalid payload", errPayload.Message)
			})
		}
	}
}

func TestWSStatusAcceptsValidPayload(t *testing.T) {
	h := newTestHandler(t)

	// An absent payload is legal: ParsePayload treats a nil payload as a no-op,
	// so the handler answers for the zero-valued secret ID instead of erroring.
	for _, body := range []string{"", `{"secret_id":"sec-1"}`} {
		resp, err := h.wsStatus(context.Background(), newTestMessage(ws.ActionSpritesStatus, body))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		require.Equal(t, ws.ActionSpritesStatus, resp.Action)

		var status SpritesStatus
		require.NoError(t, json.Unmarshal(resp.Payload, &status))
		require.False(t, status.TokenConfigured)
		require.False(t, status.Connected)
	}
}

func TestWSListInstancesAcceptsValidPayload(t *testing.T) {
	h := newTestHandler(t)

	for _, body := range []string{"", `{"secret_id":"sec-1"}`} {
		resp, err := h.wsListInstances(context.Background(), newTestMessage(ws.ActionSpritesInstancesList, body))
		require.NoError(t, err)

		// No secret store is wired, so the request gets past payload parsing and
		// fails downstream — an internal error, never a bad request.
		errPayload := requireErrorPayload(t, resp, ws.ActionSpritesInstancesList, ws.ErrorCodeInternalError)
		require.Contains(t, errPayload.Message, "API token not configured")
	}
}

func TestWSTestAcceptsValidPayload(t *testing.T) {
	h := newTestHandler(t)

	for _, body := range []string{"", `{"secret_id":"sec-1"}`} {
		resp, err := h.wsTest(context.Background(), newTestMessage(ws.ActionSpritesTest, body))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		require.Equal(t, ws.ActionSpritesTest, resp.Action)

		var result SpritesTestResult
		require.NoError(t, json.Unmarshal(resp.Payload, &result))
		require.False(t, result.Success)
		require.Len(t, result.Steps, 1)
		require.Equal(t, "Get API token", result.Steps[0].Name)
		require.False(t, result.Steps[0].Success)
	}
}
