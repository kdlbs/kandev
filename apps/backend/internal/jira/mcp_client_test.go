package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

type mcpRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mcpRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mcpHTTPResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func setMCPTestTransport(c *MCPClient, transport http.RoundTripper) {
	c.http.Transport = &mcpStatusRoundTripper{base: transport}
}

func mcpMethod(t *testing.T, req *http.Request) string {
	t.Helper()
	if req.Body == nil {
		return req.Method
	}
	var body struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode MCP request: %v", err)
	}
	return body.Method
}

const (
	initializeResult = `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"fake","version":"1"}}}`
	toolResult       = `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok"}]}}`
)

func TestMCPClientSendsInitializedNotification(t *testing.T) {
	c := NewMCPClient("token-a", "cloud", "ws", "https://x.atlassian.net")
	initialized := false
	setMCPTestTransport(c, mcpRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch mcpMethod(t, req) {
		case "initialize":
			resp := mcpHTTPResponse(http.StatusOK, "application/json", initializeResult)
			resp.Header.Set("Mcp-Session-Id", "session-1")
			return resp, nil
		case "notifications/initialized":
			initialized = true
			return mcpHTTPResponse(http.StatusAccepted, "application/json", ""), nil
		case "tools/call":
			return mcpHTTPResponse(http.StatusOK, "text/event-stream", "data: "+toolResult+"\n\n"), nil
		default:
			return mcpHTTPResponse(http.StatusBadRequest, "text/plain", "unexpected method"), nil
		}
	}))

	if _, err := c.call(context.Background(), "tools/call", map[string]interface{}{"name": "test", "arguments": map[string]interface{}{}}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if !initialized {
		t.Fatal("client did not send notifications/initialized")
	}
}

func TestMCPClientAcceptsJSONToolResponse(t *testing.T) {
	c := NewMCPClient("token-a", "cloud", "ws", "https://x.atlassian.net")
	setMCPTestTransport(c, mcpRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch mcpMethod(t, req) {
		case "initialize":
			resp := mcpHTTPResponse(http.StatusOK, "application/json", initializeResult)
			resp.Header.Set("Mcp-Session-Id", "session-1")
			return resp, nil
		case "notifications/initialized":
			return mcpHTTPResponse(http.StatusAccepted, "application/json", ""), nil
		default:
			return mcpHTTPResponse(http.StatusOK, "application/json", toolResult), nil
		}
	}))

	got, err := c.call(context.Background(), "tools/call", map[string]interface{}{"name": "test", "arguments": map[string]interface{}{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %q, want ok", got)
	}
}

type recordingRefresher struct {
	mu      sync.Mutex
	token   string
	stale   []string
	rotated chan struct{}
	once    sync.Once
}

func (r *recordingRefresher) RefreshOAuthToken(_ context.Context, _ string, stale string) error {
	r.mu.Lock()
	r.stale = append(r.stale, stale)
	if stale == "token-a" {
		r.token = "token-b"
	} else {
		r.token = "token-c"
	}
	r.mu.Unlock()
	r.once.Do(func() { close(r.rotated) })
	return nil
}

func (r *recordingRefresher) RevealAccessToken(context.Context, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.token, nil
}

func TestMCPClientRefreshUsesTokenSentByFailedRequest(t *testing.T) {
	refresher := &recordingRefresher{token: "token-a", rotated: make(chan struct{})}
	c := NewMCPClient("token-a", "cloud", "ws", "https://x.atlassian.net")
	c.SetRefresher("ws", refresher)
	bothInitial := make(chan struct{})
	var initialMu sync.Mutex
	initialCount := 0
	setMCPTestTransport(c, mcpRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch mcpMethod(t, req) {
		case "initialize":
			resp := mcpHTTPResponse(http.StatusOK, "application/json", initializeResult)
			resp.Header.Set("Mcp-Session-Id", "session-1")
			return resp, nil
		case "notifications/initialized":
			return mcpHTTPResponse(http.StatusAccepted, "application/json", ""), nil
		}
		if req.Header.Get("Authorization") != "Bearer token-a" {
			return mcpHTTPResponse(http.StatusOK, "application/json", toolResult), nil
		}
		initialMu.Lock()
		initialCount++
		current := initialCount
		if current == 2 {
			close(bothInitial)
		}
		initialMu.Unlock()
		<-bothInitial
		if current == 2 {
			<-refresher.rotated
		}
		return mcpHTTPResponse(http.StatusUnauthorized, "text/plain", "expired"), nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.call(ctx, "tools/call", map[string]interface{}{"name": "test", "arguments": map[string]interface{}{}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
	}
	refresher.mu.Lock()
	defer refresher.mu.Unlock()
	if len(refresher.stale) != 2 || refresher.stale[0] != "token-a" || refresher.stale[1] != "token-a" {
		t.Fatalf("stale tokens = %v, want [token-a token-a]", refresher.stale)
	}
}

type resetSessionTransport struct {
	closed chan struct{}
	once   sync.Once
}

func (t *resetSessionTransport) Start(context.Context) error { return nil }

func (t *resetSessionTransport) SendRequest(context.Context, mcptransport.JSONRPCRequest) (*mcptransport.JSONRPCResponse, error) {
	return nil, nil
}

func (t *resetSessionTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error {
	return nil
}

func (t *resetSessionTransport) SetNotificationHandler(func(mcp.JSONRPCNotification)) {}

func (t *resetSessionTransport) Close() error {
	t.once.Do(func() { close(t.closed) })
	return nil
}

func (t *resetSessionTransport) GetSessionId() string { return "test-session" }

func TestMCPClientResetSessionClosesPreviousClient(t *testing.T) {
	transport := &resetSessionTransport{closed: make(chan struct{})}
	c := NewMCPClient("token", "cloud", "ws", "https://x.atlassian.net")
	c.session = mcpclient.NewClient(transport)

	c.resetSession()

	if c.session != nil {
		t.Fatal("session was not cleared")
	}
	select {
	case <-transport.closed:
	default:
		t.Fatal("previous session was not closed")
	}
}
