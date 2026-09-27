package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingActionBackend blocks RequestPayload for one target action until
// release is closed, simulating a Kandev call that waits on a detached
// agentctl-backend link.
type blockingActionBackend struct {
	targetAction string
	release      <-chan struct{}
	response     map[string]interface{}
}

func (b *blockingActionBackend) RequestPayload(ctx context.Context, action string, _, result interface{}) error {
	if action == b.targetAction {
		select {
		case <-b.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if b.response != nil && result != nil {
		data, _ := json.Marshal(b.response)
		return json.Unmarshal(data, result)
	}
	return nil
}

// TestKandevCallKeepAlive_StreamsProgressWhileWaiting covers requestPayload,
// the choke point every non-bespoke MCP tool handler uses instead of calling
// BackendClient.RequestPayload directly
// (AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.2): while a Kandev call waits it
// must send MCP progress notifications at intervals of 20s or less so the
// agent's own MCP client does not idle-time-out the tool call, matching
// TestAskUserQuestion_StreamsKeepAliveDuringWait's real-transport regression
// shape for a handler that is not ask_user_question.
func TestKandevCallKeepAlive_StreamsProgressWhileWaiting(t *testing.T) {
	prev := askQuestionKeepAliveInterval
	askQuestionKeepAliveInterval = 5 * time.Millisecond
	t.Cleanup(func() { askQuestionKeepAliveInterval = prev })

	release := make(chan struct{})
	backend := &blockingActionBackend{
		targetAction: ws.ActionMCPListRelatedTasks,
		release:      release,
		response:     map[string]interface{}{},
	}

	log := newTestLogger(t)
	s := New(backend, "sess-keepalive", "task-keepalive", 0, log, "", false, ModeTask)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	s.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	initResp := postJSONRPC(t, ts.URL+"/mcp", initReq, "")
	require.Equal(t, http.StatusOK, initResp.statusCode, "init: %s", initResp.body)

	// Bounded so a missing keepalive fails fast with a clear message instead
	// of hanging the request (and this test) until the package-wide go test
	// timeout: with no progress notification ever observed, `release` is
	// never closed and the blocked backend call would otherwise wait forever.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_related_tasks_kandev","arguments":{}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/mcp", strings.NewReader(callBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", initResp.sessionID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	released := false
	progressSeen := 0
	finalSeen := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch {
		case strings.Contains(payload, "notifications/progress"):
			progressSeen++
			if !released {
				close(release)
				released = true
			}
		case strings.Contains(payload, `"id":2`):
			finalSeen = true
		}
		if finalSeen {
			break
		}
	}
	if !released {
		close(release)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded,
			"unexpected SSE read error: %v", scanErr)
	}
	assert.GreaterOrEqual(t, progressSeen, 1, "expected at least one keepalive progress notification")
	assert.True(t, finalSeen, "expected the final tool result to be delivered")
}
