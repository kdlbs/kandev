package mcp

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowBackend blocks every request until released.
type slowBackend struct{ release <-chan struct{} }

func (b *slowBackend) RequestPayload(ctx context.Context, _ string, _, _ interface{}) error {
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestEveryKandevToolStreamsKeepAliveWithNeutralMessage proves the keepalive is
// not specific to ask_user_question: a plain tool streams progress with a
// per-call token and the neutral message while the backend is slow.
func TestEveryKandevToolStreamsKeepAliveWithNeutralMessage(t *testing.T) {
	prev := toolKeepAliveInterval
	toolKeepAliveInterval = 5 * time.Millisecond
	t.Cleanup(func() { toolKeepAliveInterval = prev })

	release := make(chan struct{})
	s := New(&slowBackend{release: release}, "sess-ka", "task-ka", 0, newTestLogger(t), "", false, ModeTask)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	s.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	initResp := postJSONRPC(t, ts.URL+"/mcp", initReq, "")
	require.Equal(t, http.StatusOK, initResp.statusCode, "init: %s", initResp.body)

	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_workspaces_kandev","arguments":{}}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(call))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", initResp.sessionID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	released, progress, final := false, 0, false
	for scanner.Scan() && !final {
		payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "data:"))
		switch {
		case strings.Contains(payload, "notifications/progress"):
			progress++
			assert.Contains(t, payload, "kandev:sess-ka:")
			assert.Contains(t, payload, toolKeepAliveText)
			assert.NotContains(t, payload, askQuestionKeepAliveText)
			if !released {
				close(release)
				released = true
			}
		case strings.Contains(payload, `"id":2`):
			final = true
		}
	}
	if !released {
		close(release)
	}
	assert.GreaterOrEqual(t, progress, 1)
	assert.True(t, final)
}

func TestEmitKeepAlivePingsStartsNoSendOnceStopIsClosed(t *testing.T) {
	stop := make(chan struct{})
	close(stop)
	sent := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		emitKeepAlivePings(context.Background(), stop, time.Millisecond, func() { sent++ })
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not return")
	}
	assert.Zero(t, sent)
}
