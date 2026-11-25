package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestUndeliveredPromptDispatchLostReplyDoesNotRetry(t *testing.T) {
	var prompts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var request ws.Message
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			if request.Action != "agent.prompt" {
				continue
			}
			if prompts.Add(1) == 1 {
				return
			}
			response, _ := ws.NewResponse(request.ID, request.Action, map[string]bool{"success": true})
			if conn.WriteJSON(response) != nil {
				return
			}
		}
	}))
	defer server.Close()
	client := createTestClient(t, server.URL)
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.StreamUpdates(ctx, nil, nil, nil))
	log := newSessionTestLogger()
	stop := newTestStopCh(t)
	sm := NewSessionManager(log, stop)
	streams := NewStreamManager(log, StreamCallbacks{}, nil, stop)
	cleanupStreamManager(t, stop, streams)
	sm.SetDependencies(nil, streams, nil, nil)
	execution := &AgentExecution{ID: "execution", SessionID: "session", agentctl: client}
	err := sm.dispatchPrompt(ctx, execution, "original", nil, 1, false, "submission")
	require.Error(t, err)
	var notSent *agentctl.RequestNotSentError
	require.False(t, errors.As(err, &notSent))
	require.EqualValues(t, 1, prompts.Load())
}

func TestUndeliveredPromptDispatchReconnectFailureRetainsProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	client := createTestClient(t, server.URL)
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	log := newSessionTestLogger()
	stop := newTestStopCh(t)
	sm := NewSessionManager(log, stop)
	streams := NewStreamManager(log, StreamCallbacks{}, nil, stop)
	cleanupStreamManager(t, stop, streams)
	sm.SetDependencies(nil, streams, nil, nil)
	execution := &AgentExecution{ID: "execution", SessionID: "session", agentctl: client}
	err := sm.dispatchPrompt(ctx, execution, "unsent", nil, 1, false, "submission")
	var notSent *agentctl.RequestNotSentError
	require.ErrorAs(t, err, &notSent)
}
