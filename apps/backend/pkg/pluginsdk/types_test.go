package pluginsdk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventProtoRoundTrip(t *testing.T) {
	e := &Event{
		EventID:     "evt-1",
		EventType:   "task.created",
		OccurredAt:  "2026-07-15T12:00:00Z",
		WorkspaceID: "ws-1",
		Payload: map[string]any{
			"task_id": "task-1",
			"count":   float64(3),
		},
	}

	proto, err := e.toProto()
	require.NoError(t, err)
	require.Equal(t, "evt-1", proto.GetEventId())
	require.Equal(t, "task.created", proto.GetEventType())

	back, err := eventFromProto(proto)
	require.NoError(t, err)
	require.Equal(t, e, back)
}

func TestEventProtoRoundTrip_NilPayload(t *testing.T) {
	e := &Event{EventID: "evt-2", EventType: "task.updated"}

	proto, err := e.toProto()
	require.NoError(t, err)
	require.Nil(t, proto.GetPayload())

	back, err := eventFromProto(proto)
	require.NoError(t, err)
	require.Nil(t, back.Payload)
}

func TestToolRequestResponseProtoRoundTrip(t *testing.T) {
	req := &ToolRequest{
		ToolCallID: "call-1",
		ToolName:   "search",
		Input:      map[string]any{"query": "kandev"},
		Context: &ToolContext{
			TaskID:          "task-1",
			AgentInstanceID: "agent-1",
			SessionID:       "session-1",
		},
	}

	proto, err := req.toProto()
	require.NoError(t, err)

	back, err := toolRequestFromProto(proto)
	require.NoError(t, err)
	require.Equal(t, req, back)

	resp := &ToolResponse{Output: map[string]any{"result": "ok"}}
	protoResp, err := resp.toProto()
	require.NoError(t, err)

	backResp, err := toolResponseFromProto(protoResp)
	require.NoError(t, err)
	require.Equal(t, resp, backResp)
}

func TestWebhookRequestResponseProtoRoundTrip(t *testing.T) {
	req := &WebhookRequest{
		WebhookKey: "key-1",
		Method:     "POST",
		Path:       "/foo",
		Query:      "a=b",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"ok":true}`),
	}

	proto := req.toProto()
	back := webhookRequestFromProto(proto)
	require.Equal(t, req, back)

	resp := &WebhookResponse{
		Status:  200,
		Headers: map[string]string{"X-Test": "1"},
		Body:    []byte("done"),
	}
	protoResp := resp.toProto()
	backResp := webhookResponseFromProto(protoResp)
	require.Equal(t, resp, backResp)
}

func TestStateEntryProtoRoundTrip(t *testing.T) {
	e := &StateEntry{
		Key:       "foo",
		Value:     map[string]any{"bar": "baz"},
		UpdatedAt: "2026-07-15T12:00:00Z",
	}

	proto, err := e.toProto()
	require.NoError(t, err)

	back, err := stateEntryFromProto(proto)
	require.NoError(t, err)
	require.Equal(t, e, back)
}
