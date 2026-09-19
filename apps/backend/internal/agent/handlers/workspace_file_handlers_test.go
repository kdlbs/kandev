package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestWorkspaceFileHandlersValidateRequestsBeforeLookup(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		payload any
		invoke  func(*WorkspaceFileHandlers, context.Context, *ws.Message) (*ws.Message, error)
	}{
		{name: "tree session", action: ws.ActionWorkspaceFileTreeGet, payload: map[string]any{}, invoke: (*WorkspaceFileHandlers).wsGetFileTree},
		{name: "content path", action: ws.ActionWorkspaceFileContentGet, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsGetFileContent},
		{name: "ref path", action: ws.ActionWorkspaceFileContentGetRef, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsGetFileContentAtRef},
		{name: "ref value", action: ws.ActionWorkspaceFileContentGetRef, payload: map[string]any{"session_id": "s", "path": "a"}, invoke: (*WorkspaceFileHandlers).wsGetFileContentAtRef},
		{name: "update diff", action: ws.ActionWorkspaceFileContentUpdate, payload: map[string]any{"session_id": "s", "path": "a"}, invoke: (*WorkspaceFileHandlers).wsUpdateFileContent},
		{name: "create path", action: ws.ActionWorkspaceFileCreate, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsCreateFile},
		{name: "delete session", action: ws.ActionWorkspaceFileDelete, payload: map[string]any{}, invoke: (*WorkspaceFileHandlers).wsDeleteFile},
		{name: "search session", action: ws.ActionWorkspaceFilesSearch, payload: map[string]any{}, invoke: (*WorkspaceFileHandlers).wsSearchFiles},
		{name: "rename old path", action: ws.ActionWorkspaceFileRename, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsRenameFile},
		{name: "rename new path", action: ws.ActionWorkspaceFileRename, payload: map[string]any{"session_id": "s", "old_path": "a"}, invoke: (*WorkspaceFileHandlers).wsRenameFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewWorkspaceFileHandlers(nil, newTestLogger())
			msg, _ := ws.NewRequest("id", tt.action, tt.payload)
			response, err := tt.invoke(h, context.Background(), msg)
			if err != nil || response == nil {
				t.Fatalf("response = (%#v, %v)", response, err)
			}
			assertWorkspaceContentSearchErrorCode(t, response, ws.ErrorCodeValidation)
		})
	}
}

func TestWorkspaceFileHandlersSuccessContracts(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		path      string
		method    string
		payload   any
		invoke    func(*WorkspaceFileHandlers, context.Context, *ws.Message) (*ws.Message, error)
		assertReq func(*testing.T, *http.Request)
	}{
		{name: "tree", action: ws.ActionWorkspaceFileTreeGet, path: "/api/v1/workspace/tree", method: http.MethodGet, payload: map[string]any{"session_id": "s", "path": "src", "depth": 2}, invoke: (*WorkspaceFileHandlers).wsGetFileTree, assertReq: expectWorkspaceQuery("path", "src")},
		{name: "content", action: ws.ActionWorkspaceFileContentGet, path: "/api/v1/workspace/file/content", method: http.MethodGet, payload: map[string]any{"session_id": "s", "path": "a.go", "repo": "repo"}, invoke: (*WorkspaceFileHandlers).wsGetFileContent, assertReq: expectWorkspaceQuery("repo", "repo")},
		{name: "content at ref", action: ws.ActionWorkspaceFileContentGetRef, path: "/api/v1/workspace/file/content-at-ref", method: http.MethodGet, payload: map[string]any{"session_id": "s", "path": "a.go", "ref": "HEAD", "repo": "repo"}, invoke: (*WorkspaceFileHandlers).wsGetFileContentAtRef, assertReq: expectWorkspaceQuery("ref", "HEAD")},
		{name: "update", action: ws.ActionWorkspaceFileContentUpdate, path: "/api/v1/workspace/file/content", method: http.MethodPost, payload: map[string]any{"session_id": "s", "path": "a.go", "diff": "patch", "original_hash": "old", "repo": "repo"}, invoke: (*WorkspaceFileHandlers).wsUpdateFileContent, assertReq: expectWorkspaceJSON("diff", "patch")},
		{name: "create", action: ws.ActionWorkspaceFileCreate, path: "/api/v1/workspace/file/create", method: http.MethodPost, payload: map[string]any{"session_id": "s", "path": "new.go", "repo": "repo"}, invoke: (*WorkspaceFileHandlers).wsCreateFile, assertReq: expectWorkspaceJSON("path", "new.go")},
		{name: "delete", action: ws.ActionWorkspaceFileDelete, path: "/api/v1/workspace/file", method: http.MethodDelete, payload: map[string]any{"session_id": "s", "path": "old.go", "repo": "repo"}, invoke: (*WorkspaceFileHandlers).wsDeleteFile, assertReq: expectWorkspaceQuery("path", "old.go")},
		{name: "file search defaults limit", action: ws.ActionWorkspaceFilesSearch, path: "/api/v1/workspace/search", method: http.MethodGet, payload: map[string]any{"session_id": "s", "query": "go"}, invoke: (*WorkspaceFileHandlers).wsSearchFiles, assertReq: expectWorkspaceQuery("limit", "20")},
		{name: "content search", action: ws.ActionWorkspaceContentSearch, path: "/api/v1/workspace/content-search", method: http.MethodGet, payload: map[string]any{"session_id": "s", "query": "needle", "limit_per_repo": 7}, invoke: (*WorkspaceFileHandlers).wsSearchContent, assertReq: expectWorkspaceQuery("limit_per_repo", "7")},
		{name: "rename", action: ws.ActionWorkspaceFileRename, path: "/api/v1/workspace/file/rename", method: http.MethodPost, payload: map[string]any{"session_id": "s", "old_path": "a", "new_path": "b", "repo": "repo"}, invoke: (*WorkspaceFileHandlers).wsRenameFile, assertReq: expectWorkspaceJSON("new_path", "b")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := workspaceHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path || r.Method != tt.method {
					t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, tt.method, tt.path)
				}
				tt.assertReq(t, r)
				_, _ = w.Write([]byte(`{"success":true,"path":"a.go","content":"body","results":[],"matches":[]}`))
			})
			msg, _ := ws.NewRequest("id", tt.action, tt.payload)
			response, err := tt.invoke(h, context.Background(), msg)
			if err != nil || response == nil {
				t.Fatalf("response = (%s, %v)", response.Payload, err)
			}
		})
	}
}

func TestWorkspaceFileHandlersMapDependencyFailures(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		payload any
		invoke  func(*WorkspaceFileHandlers, context.Context, *ws.Message) (*ws.Message, error)
	}{
		{name: "tree", action: ws.ActionWorkspaceFileTreeGet, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsGetFileTree},
		{name: "content", action: ws.ActionWorkspaceFileContentGet, payload: map[string]any{"session_id": "s", "path": "a"}, invoke: (*WorkspaceFileHandlers).wsGetFileContent},
		{name: "content at ref", action: ws.ActionWorkspaceFileContentGetRef, payload: map[string]any{"session_id": "s", "path": "a", "ref": "HEAD"}, invoke: (*WorkspaceFileHandlers).wsGetFileContentAtRef},
		{name: "update", action: ws.ActionWorkspaceFileContentUpdate, payload: map[string]any{"session_id": "s", "path": "a", "diff": "patch"}, invoke: (*WorkspaceFileHandlers).wsUpdateFileContent},
		{name: "create", action: ws.ActionWorkspaceFileCreate, payload: map[string]any{"session_id": "s", "path": "a"}, invoke: (*WorkspaceFileHandlers).wsCreateFile},
		{name: "delete", action: ws.ActionWorkspaceFileDelete, payload: map[string]any{"session_id": "s", "path": "a"}, invoke: (*WorkspaceFileHandlers).wsDeleteFile},
		{name: "search", action: ws.ActionWorkspaceFilesSearch, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsSearchFiles},
		{name: "content search", action: ws.ActionWorkspaceContentSearch, payload: map[string]any{"session_id": "s"}, invoke: (*WorkspaceFileHandlers).wsSearchContent},
		{name: "rename", action: ws.ActionWorkspaceFileRename, payload: map[string]any{"session_id": "s", "old_path": "a", "new_path": "b"}, invoke: (*WorkspaceFileHandlers).wsRenameFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := workspaceHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"error":"dependency failed"}`))
			})
			msg, _ := ws.NewRequest("id", tt.action, tt.payload)
			response, err := tt.invoke(h, context.Background(), msg)
			if err != nil || response == nil {
				t.Fatalf("response = (%#v, %v)", response, err)
			}
			assertWorkspaceContentSearchErrorCode(t, response, ws.ErrorCodeInternalError)
		})
	}
}

func TestWorkspaceFileHandlersMissingContentIsNotFoundAndDebug(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	h := workspaceHandlerServerWithLogger(t, log, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"file not found: stat /workspace/gone.go: no such file or directory"}`))
	})
	msg, err := ws.NewRequest("id", ws.ActionWorkspaceFileContentGet, map[string]any{
		"session_id": "s",
		"path":       "gone.go",
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := h.wsGetFileContent(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsGetFileContent returned error: %v", err)
	}
	assertWorkspaceContentSearchErrorCode(t, response, ws.ErrorCodeNotFound)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("observed %d log entries, want 1: %v", len(entries), entries)
	}
	if entries[0].Level != zap.DebugLevel {
		t.Fatalf("log level = %s, want debug", entries[0].Level)
	}
	if entries[0].Message != "file not found (expected for deleted or stale files)" {
		t.Fatalf("log message = %q, want expected missing-file message", entries[0].Message)
	}
}

func TestWorkspaceFileHandlersNonMissingContentFailureRemainsError(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	h := workspaceHandlerServerWithLogger(t, log, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"file not found: permission denied"}`))
	})
	msg, err := ws.NewRequest("id", ws.ActionWorkspaceFileContentGet, map[string]any{
		"session_id": "s",
		"path":       "private.go",
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := h.wsGetFileContent(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsGetFileContent returned error: %v", err)
	}
	assertWorkspaceContentSearchErrorCode(t, response, ws.ErrorCodeInternalError)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("observed %d log entries, want 1: %v", len(entries), entries)
	}
	if entries[0].Level != zap.ErrorLevel {
		t.Fatalf("log level = %s, want error", entries[0].Level)
	}
}

func workspaceHandlerServer(t *testing.T, handler http.HandlerFunc) *WorkspaceFileHandlers {
	return workspaceHandlerServerWithLogger(t, newTestLogger(), handler)
}

func workspaceHandlerServerWithLogger(t *testing.T, log *commonlogger.Logger, handler http.HandlerFunc) *WorkspaceFileHandlers {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := agentctl.NewClient(u.Hostname(), port, newTestLogger())
	execution := &lifecycle.AgentExecution{SessionID: "s"}
	execution.SetAgentCtlClientForTesting(client)
	lookup := &mockExecutionLookup{executions: map[string]*lifecycle.AgentExecution{"s": execution}}
	return NewWorkspaceFileHandlers(lookup, log)
}

func expectWorkspaceQuery(key, want string) func(*testing.T, *http.Request) {
	return func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get(key); got != want {
			t.Fatalf("query %s = %q, want %q", key, got, want)
		}
	}
}

func expectWorkspaceJSON(key string, want any) func(*testing.T, *http.Request) {
	return func(t *testing.T, r *http.Request) {
		t.Helper()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body[key] != want {
			t.Fatalf("body[%s] = %#v, want %#v", key, body[key], want)
		}
	}
}

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
