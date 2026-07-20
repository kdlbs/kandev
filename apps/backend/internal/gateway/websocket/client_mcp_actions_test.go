package websocket

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestClient_HandleMessageRejectsMCPNamespaceBeforeDispatch(t *testing.T) {
	for _, action := range []string{"mcp.stop_task", "mcp.future_action"} {
		t.Run(action, func(t *testing.T) {
			dispatcher := ws.NewDispatcher()
			handlerCalls := 0
			dispatcher.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
				handlerCalls++
				return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"called": true})
			})

			hub := newTestHub(t)
			hub.dispatcher = dispatcher
			client := newTestClient("raw-client")
			client.hub = hub

			request, err := ws.NewRequest("request-1", action, map[string]string{
				"task_id":        "child-task",
				"sender_task_id": "forged-parent-task",
			})
			if err != nil {
				t.Fatalf("build request: %v", err)
			}

			client.handleMessage(request)

			response := receiveClientMessage(t, client)
			if response.Type != ws.MessageTypeError {
				t.Fatalf("response type = %q, want %q", response.Type, ws.MessageTypeError)
			}
			if response.ID != request.ID || response.Action != request.Action {
				t.Fatalf("response correlation = (%q, %q), want (%q, %q)",
					response.ID, response.Action, request.ID, request.Action)
			}

			var payload ws.ErrorPayload
			if err := response.ParsePayload(&payload); err != nil {
				t.Fatalf("parse error payload: %v", err)
			}
			if payload.Code != ws.ErrorCodeForbidden {
				t.Fatalf("error code = %q, want %q", payload.Code, ws.ErrorCodeForbidden)
			}
			if handlerCalls != 0 {
				t.Fatalf("dispatcher handler calls = %d, want 0", handlerCalls)
			}
		})
	}
}

func TestClient_HandleMessageStillDispatchesPublicActions(t *testing.T) {
	const action = "test.public"

	dispatcher := ws.NewDispatcher()
	handlerCalls := 0
	dispatcher.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		handlerCalls++
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"ok": true})
	})

	hub := newTestHub(t)
	hub.dispatcher = dispatcher
	client := newTestClient("raw-client")
	client.hub = hub

	request, err := ws.NewRequest("request-1", action, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	client.handleMessage(request)

	response := receiveClientMessage(t, client)
	if response.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want %q", response.Type, ws.MessageTypeResponse)
	}
	if handlerCalls != 1 {
		t.Fatalf("dispatcher handler calls = %d, want 1", handlerCalls)
	}
}

func TestDispatcher_DirectMCPDispatchRemainsAvailable(t *testing.T) {
	const action = "mcp.stop_task"

	dispatcher := ws.NewDispatcher()
	handlerCalls := 0
	dispatcher.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		handlerCalls++
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"stopped": true})
	})

	request, err := ws.NewRequest("request-1", action, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := dispatcher.Dispatch(context.Background(), request)
	if err != nil {
		t.Fatalf("direct dispatch: %v", err)
	}
	if response == nil || response.Type != ws.MessageTypeResponse {
		t.Fatalf("direct dispatch response = %#v, want response", response)
	}
	if handlerCalls != 1 {
		t.Fatalf("dispatcher handler calls = %d, want 1", handlerCalls)
	}
}

func receiveClientMessage(t *testing.T, client *Client) *ws.Message {
	t.Helper()

	select {
	case data := <-client.send:
		var message ws.Message
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatalf("decode client message: %v", err)
		}
		return &message
	default:
		t.Fatal("client produced no response")
		return nil
	}
}
