package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type pluginContextsBackend struct {
	invocations []map[string]any
	localName   string
	revision    uint64
}

func (b *pluginContextsBackend) RequestPayload(_ context.Context, action string, payload, result interface{}) error {
	switch action {
	case ws.ActionMCPListPluginTools:
		localName := b.localName
		if localName == "" {
			localName = "get_provider_usage"
		}
		revision := b.revision
		if revision == 0 {
			revision = 1
		}
		snapshot := plugintools.Snapshot{
			Generation: "provider-usage", Revision: revision,
			Tools: []plugintools.Definition{{
				PluginID: "kandev-provider-usage", LocalName: localName,
				ExposedName:     plugintools.ExposedName("kandev-provider-usage", localName),
				Description:     "Returns the cached, instance-wide provider usage snapshot.",
				Surfaces:        []string{plugintools.SurfaceKanban, plugintools.SurfaceCoordinator, plugintools.SurfaceExternal},
				InvocationScope: plugintools.InvocationScopeInstance,
				InstallationID:  "installation-1",
				PolicyVersion:   "policy-1",
				InputSchema:     []byte(`{"type":"object","additionalProperties":false}`),
				ReadOnlyHint:    true,
			}},
		}
		return decodeInto(snapshot, result)
	case ws.ActionMCPInvokePluginTool:
		values, _ := payload.(map[string]any)
		b.invocations = append(b.invocations, values)
		return decodeInto(map[string]any{
			"text": "Provider usage is instance-wide cached telemetry.",
			"structured_content": map[string]any{
				"schema_version": "1", "partial": false, "stale_after_seconds": 600,
				"scope": map[string]any{"usage_scope": "instance", "user_scoped": false, "invocation_workspace_id": ""},
				"providers": []any{map[string]any{
					"provider_id": "codex", "availability_state": "available", "stale": false,
					"windows": []any{map[string]any{
						"window_id": "window-1", "window_name": "5-hour",
						"utilization_percentage": 25, "remaining_percentage": 75,
					}},
				}},
			},
		}, result)
	default:
		return nil
	}
}

func decodeInto(value, result interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

// extractJSONRPCIDLine returns the "data:" payload carrying the requested
// JSON-RPC id. A hook that rebuilds the catalog can emit a list_changed
// notification before the actual response on the same stream.
func extractJSONRPCIDLine(t *testing.T, body string, id int) string {
	t.Helper()
	want := `"id":` + strconv.Itoa(id)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "data:")
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.Contains(line, want) {
			return line
		}
	}
	t.Fatalf("no JSON-RPC response with id %d in %s", id, body)
	return ""
}

func TestProviderUsageMCPContexts(t *testing.T) {
	log := newTestLogger(t)
	backend := &pluginContextsBackend{}
	srv := NewExternal(backend, log, "")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	srv.RegisterBackendRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	resp := postJSONRPC(t, ts.URL+"/mcp", initReq, "")
	require.Equal(t, http.StatusOK, resp.statusCode, resp.body)
	sessionID := resp.sessionID
	require.NotEmpty(t, sessionID)

	listReq := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	resp = postJSONRPC(t, ts.URL+"/mcp", listReq, sessionID)
	require.Equal(t, http.StatusOK, resp.statusCode, resp.body)
	listLine := extractJSONRPCIDLine(t, resp.body, 2)
	require.NotEmpty(t, listLine)
	require.Contains(t, listLine, "kandev_kandev_provider_usage_get_provider_usage")
	require.Contains(t, listLine, `"readOnlyHint":true`)

	callReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kandev_kandev_provider_usage_get_provider_usage","arguments":{}}}`
	resp = postJSONRPC(t, ts.URL+"/mcp", callReq, sessionID)
	require.Equal(t, http.StatusOK, resp.statusCode, resp.body)
	callLine := extractJSONRPCIDLine(t, resp.body, 3)
	require.NotEmpty(t, callLine)
	require.Contains(t, callLine, `"utilization_percentage":25`)
	require.Contains(t, callLine, `"remaining_percentage":75`)
	require.Contains(t, callLine, `"invocation_workspace_id":""`)

	require.Len(t, backend.invocations, 1)
	invocation := backend.invocations[0]
	require.Equal(t, plugintools.SurfaceExternal, invocation["surface"])
	require.Equal(t, "kandev-provider-usage", invocation["plugin_id"])
	require.Equal(t, "get_provider_usage", invocation["local_name"])
	require.Empty(t, invocation["task_id"])
	require.Empty(t, invocation["session_id"])
	require.Empty(t, invocation["workspace_id"])
	arguments, ok := invocation["arguments"].(map[string]any)
	require.True(t, ok)
	require.Empty(t, arguments)
	require.False(t, strings.Contains(strings.ToLower(callLine), "authorization"))
}

// TestProviderUsageMCPContextsSSE exercises the legacy SSE transport with the
// same eligible catalog so the instance-read tool works across both endpoints.
func TestProviderUsageMCPContextsSSE(t *testing.T) {
	log := newTestLogger(t)
	backend := &pluginContextsBackend{}
	srv := NewExternal(backend, log, "")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	srv.RegisterBackendRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, ts.URL+"/mcp/sse", nil)
	require.NoError(t, err)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseResp, err := http.DefaultClient.Do(sseReq)
	require.NoError(t, err)
	defer func() { _ = sseResp.Body.Close() }()

	lines := make(chan string, 64)
	go func() {
		scanner := bufio.NewScanner(sseResp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	endpoint := nextSSEData(t, lines, func(value string) bool { return strings.HasPrefix(value, "/mcp/message?sessionId=") })
	messageURL := ts.URL + endpoint

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	postSSEMessage(t, messageURL, initReq)
	nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":1`) })

	listReq := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	postSSEMessage(t, messageURL, listReq)
	listLine := nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":2`) })
	require.Contains(t, listLine, "kandev_kandev_provider_usage_get_provider_usage")

	callReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kandev_kandev_provider_usage_get_provider_usage","arguments":{}}}`
	postSSEMessage(t, messageURL, callReq)
	callLine := nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":3`) })
	require.Contains(t, callLine, `"utilization_percentage":25`)
	require.Contains(t, callLine, `"invocation_workspace_id":""`)
}

func postSSEMessage(t *testing.T, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func nextSSEData(t *testing.T, lines <-chan string, match func(string) bool) string {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("SSE stream closed before a matching data line")
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if match(value) {
				return value
			}
		case <-timer.C:
			t.Fatal("timed out waiting for an SSE data line")
		}
	}
}

func TestProviderUsageExternalRefreshNotifiesInitializedSSEClient(t *testing.T) {
	log := newTestLogger(t)
	backend := &pluginContextsBackend{}
	srv := NewExternal(backend, log, "")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	srv.RegisterBackendRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, ts.URL+"/mcp/sse", nil)
	require.NoError(t, err)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseResp, err := http.DefaultClient.Do(sseReq)
	require.NoError(t, err)
	defer func() { _ = sseResp.Body.Close() }()

	lines := make(chan string, 64)
	go func() {
		scanner := bufio.NewScanner(sseResp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	endpoint := nextSSEData(t, lines, func(value string) bool { return strings.HasPrefix(value, "/mcp/message?sessionId=") })
	messageURL := ts.URL + endpoint
	postSSEMessage(t, messageURL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`)
	nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":1`) })

	postSSEMessage(t, messageURL, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	firstList := nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":2`) })
	require.Contains(t, firstList, "kandev_kandev_provider_usage_get_provider_usage")

	backend.localName = "get_provider_usage_v2"
	backend.revision = 2
	require.NoError(t, srv.RefreshPluginTools(context.Background()))
	nextSSEData(t, lines, func(value string) bool {
		return strings.Contains(value, `"method":"notifications/tools/list_changed"`)
	})

	postSSEMessage(t, messageURL, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
	secondList := nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":3`) })
	require.Contains(t, secondList, "kandev_kandev_provider_usage_get_provider_usage_v2")
	require.NotContains(t, secondList, "kandev_kandev_provider_usage_get_provider_usage\"")
}
