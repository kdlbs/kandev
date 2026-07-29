package handlers

import (
	"context"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWorkspaceFileHandlersRegisterContentSearch(t *testing.T) {
	handler := NewWorkspaceFileHandlers(nil, newTestLogger())
	dispatcher := ws.NewDispatcher()
	handler.RegisterHandlers(dispatcher)

	if !dispatcher.HasHandler(ws.ActionWorkspaceContentSearch) {
		t.Fatal("workspace.content.search handler was not registered")
	}
}

func TestWorkspaceContentSearchRequiresSessionID(t *testing.T) {
	handler := NewWorkspaceFileHandlers(nil, newTestLogger())
	msg, err := ws.NewRequest("request-1", ws.ActionWorkspaceContentSearch, map[string]any{
		"query": "needle",
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := handler.wsSearchContent(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsSearchContent returned error: %v", err)
	}
	assertWorkspaceContentSearchErrorCode(t, response, ws.ErrorCodeValidation)
}

func TestWorkspaceContentSearchRejectsLongQueryBeforeLifecycleLookup(t *testing.T) {
	handler := NewWorkspaceFileHandlers(nil, newTestLogger())
	msg, err := ws.NewRequest("request-1", ws.ActionWorkspaceContentSearch, map[string]any{
		"session_id": "session-1",
		"query":      strings.Repeat("界", 201),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := handler.wsSearchContent(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsSearchContent returned error: %v", err)
	}
	assertWorkspaceContentSearchErrorCode(t, response, ws.ErrorCodeValidation)
}

func TestDecodeWorkspaceContentSearchRequestUsesPerRepositoryLimit(t *testing.T) {
	msg, err := ws.NewRequest("request-1", ws.ActionWorkspaceContentSearch, map[string]any{
		"session_id":     "session-1",
		"query":          "needle",
		"limit_per_repo": 37,
		"limit":          9,
	})
	if err != nil {
		t.Fatal(err)
	}

	request, err := decodeWorkspaceContentSearchRequest(msg)
	if err != nil {
		t.Fatalf("decodeWorkspaceContentSearchRequest returned error: %v", err)
	}
	if request.LimitPerRepo != 37 {
		t.Fatalf("limit_per_repo = %d, want 37", request.LimitPerRepo)
	}
}

func assertWorkspaceContentSearchErrorCode(t *testing.T, response *ws.Message, want string) {
	t.Helper()
	if response == nil || response.Type != ws.MessageTypeError {
		t.Fatalf("response = %#v, want error message", response)
	}
	var payload ws.ErrorPayload
	if err := response.ParsePayload(&payload); err != nil {
		t.Fatalf("parse error payload: %v", err)
	}
	if payload.Code != want {
		t.Fatalf("error code = %q, want %q", payload.Code, want)
	}
}
