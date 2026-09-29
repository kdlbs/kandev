package lifecycle

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const customACPStoredSessionID = "stored-provider-session"

type customACPLaunch struct {
	result    *InitializeResult
	err       error
	actions   []string
	loadedIDs []string
}

// launchCustomACPWithStoredSession initializes an operator-registered ACP agent
// that has a stored provider session, answering agentctl's session/load with
// loadReply (nil means success), and records the ID each session/load carried.
func launchCustomACPWithStoredSession(t *testing.T, loadReply func(ws.Message) *ws.Message) customACPLaunch {
	t.Helper()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)

	var mu sync.Mutex
	var loadedIDs []string
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.session.load" {
			return mock.defaultHandler(msg)
		}
		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := msg.ParsePayload(&req); err != nil {
			t.Errorf("parse session/load payload: %v", err)
			return wsErrorReply(t, msg, ws.ErrorCodeBadRequest, "unparseable session/load payload")
		}
		mu.Lock()
		loadedIDs = append(loadedIDs, req.SessionID)
		mu.Unlock()
		if loadReply != nil {
			return loadReply(msg)
		}
		return mock.defaultHandler(msg)
	}

	sessionManager := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect agent stream: %v", err)
	}
	waitForWSConnected(t, mock)

	agentConfig := agents.NewCustomACPAgent(agents.CustomACPAgentConfig{
		AgentID:     "operator-agent",
		AgentName:   "operator-agent",
		Command:     "operator-agent",
		CommandArgs: []string{"--acp"},
	})
	result, err := sessionManager.InitializeSession(
		context.Background(),
		client,
		agentConfig,
		customACPStoredSessionID,
		"/workspace",
		nil,
	)

	mu.Lock()
	defer mu.Unlock()
	return customACPLaunch{
		result:    result,
		err:       err,
		actions:   mock.getActionLog(),
		loadedIDs: slices.Clone(loadedIDs),
	}
}

func loadErrorReply(t *testing.T, message string) func(ws.Message) *ws.Message {
	return func(msg ws.Message) *ws.Message {
		return wsErrorReply(t, msg, ws.ErrorCodeInternalError, message)
	}
}

// wsErrorReply builds the mock agentctl error reply. The handler runs on the
// mock server's goroutine, where t.Fatalf is not allowed, so a construction
// failure is reported with t.Errorf.
func wsErrorReply(t *testing.T, msg ws.Message, code, message string) *ws.Message {
	resp, err := ws.NewError(msg.ID, msg.Action, code, message, nil)
	if err != nil {
		t.Errorf("build %s error reply: %v", msg.Action, err)
	}
	return resp
}

// @covers AC-AGENTS-CUSTOM-ACP-002.1
// @covers AC-AGENTS-CUSTOM-ACP-002.4
func TestInitializeSession_CustomACPAgentRestoresStoredSession(t *testing.T) {
	launch := launchCustomACPWithStoredSession(t, nil)

	if launch.err != nil {
		t.Fatalf("InitializeSession: %v", launch.err)
	}
	if launch.result.SessionID != customACPStoredSessionID {
		t.Errorf("session ID = %q, want the stored %q", launch.result.SessionID, customACPStoredSessionID)
	}
	if !slices.Equal(launch.loadedIDs, []string{customACPStoredSessionID}) {
		t.Errorf("session/load IDs = %v, want [%s]", launch.loadedIDs, customACPStoredSessionID)
	}
	if slices.Contains(launch.actions, "agent.session.new") {
		t.Errorf("session/new sent although the stored session was restored; actions: %v", launch.actions)
	}
}

// The capability-mismatch case is also the positive control for the
// unrecognized-failure case: with the same setup, session/new is reachable.
//
// @covers AC-AGENTS-CUSTOM-ACP-002.2
// @covers AC-AGENTS-CUSTOM-ACP-002.3
func TestInitializeSession_CustomACPAgentRestoreFailures(t *testing.T) {
	tests := []struct {
		name            string
		loadError       string
		wantReplacement bool
	}{
		{
			name:            "agent advertises no restore capability",
			loadError:       "agent does not support session loading (LoadSession capability is false)",
			wantReplacement: true,
		},
		{
			name:            "unrecognized restore failure",
			loadError:       "internal error: provider failed while loading the session",
			wantReplacement: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launch := launchCustomACPWithStoredSession(t, loadErrorReply(t, tt.loadError))

			if !slices.Equal(launch.loadedIDs, []string{customACPStoredSessionID}) {
				t.Fatalf("session/load IDs = %v, want [%s]", launch.loadedIDs, customACPStoredSessionID)
			}
			sentNew := slices.Contains(launch.actions, "agent.session.new")
			if sentNew != tt.wantReplacement {
				t.Fatalf("session/new sent = %v, want %v; actions: %v", sentNew, tt.wantReplacement, launch.actions)
			}
			if tt.wantReplacement {
				if launch.err != nil {
					t.Fatalf("InitializeSession: %v", launch.err)
				}
				if launch.result.SessionID != "test-session-123" {
					t.Errorf("session ID = %q, want the replacement session", launch.result.SessionID)
				}
				return
			}
			if launch.err == nil {
				t.Fatal("expected the unrecognized restore failure to be returned")
			}
		})
	}
}
