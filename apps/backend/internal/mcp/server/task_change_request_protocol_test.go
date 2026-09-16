package mcp

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskChangeRequestProtocolCatalogAndCallAcrossProtocolEras(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id":  "task-current",
		"complete": true,
	}}
	server := New(backend, "protocol-session", "task-current", 10005, newTestLogger(t), "", false, ModeTask, []string{"github", "gitlab"})
	t.Cleanup(func() { require.NoError(t, server.Close(t.Context())) })

	modernList := postMCPRequest(t, server.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{"_meta": modernRequestMeta()},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/list",
	})
	require.Equal(t, 200, modernList.Code)
	modernNames := protocolToolNames(t, modernList)
	assertNeutralChangeRequestCatalog(t, modernNames)

	modernCall := postMCPRequest(t, server.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get_task_change_requests_kandev",
			"arguments": map[string]any{},
			"_meta":     modernRequestMeta(),
		},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/call",
		nameHeader:            "get_task_change_requests_kandev",
	})
	require.Equal(t, 200, modernCall.Code)
	assert.NotContains(t, decodeProtocolResponse(t, modernCall), "error")
	assert.Equal(t, "mcp.get_task_change_requests", backend.lastAction)

	backend.lastAction = ""
	legacyInitialize := postMCPRequest(t, server.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": legacyProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "legacy-test", "version": "1.0.0"},
		},
	}, map[string]string{protocolVersionHeader: legacyProtocolVersion})
	require.Equal(t, 200, legacyInitialize.Code)
	sessionID := legacyInitialize.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)

	legacyList := postMCPRequest(t, server.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/list",
		"params":  map[string]any{},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
		"Mcp-Session-Id":      sessionID,
	})
	require.Equal(t, 200, legacyList.Code)
	legacyNames := protocolToolNames(t, legacyList)
	assertNeutralChangeRequestCatalog(t, legacyNames)

	legacyCall := postMCPRequest(t, server.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get_task_change_requests_kandev",
			"arguments": map[string]any{},
		},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
		"Mcp-Session-Id":      sessionID,
	})
	require.Equal(t, 200, legacyCall.Code)
	assert.NotContains(t, decodeProtocolResponse(t, legacyCall), "error")
	assert.Equal(t, "mcp.get_task_change_requests", backend.lastAction)

	backend.lastAction = ""
	unknownCall := postMCPRequest(t, server.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      6,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "link_task_pr_kandev",
			"arguments": map[string]any{},
		},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
		"Mcp-Session-Id":      sessionID,
	})
	require.Equal(t, 200, unknownCall.Code)
	assert.Contains(t, decodeProtocolResponse(t, unknownCall), "error")
	assert.Empty(t, backend.lastAction, "a superseded tool call must not reach the backend")
}

func protocolToolNames(t *testing.T, response *httptest.ResponseRecorder) map[string]bool {
	t.Helper()
	message := decodeProtocolResponse(t, response)
	result, ok := message["result"].(map[string]any)
	require.True(t, ok, "tools/list response = %v", message)
	tools, ok := result["tools"].([]any)
	require.True(t, ok, "tools/list result = %v", result)
	names := make(map[string]bool, len(tools))
	for _, candidate := range tools {
		tool, ok := candidate.(map[string]any)
		if ok {
			if name, ok := tool["name"].(string); ok {
				names[name] = true
			}
		}
	}
	return names
}

func assertNeutralChangeRequestCatalog(t *testing.T, names map[string]bool) {
	t.Helper()
	for _, name := range []string{
		"get_task_change_requests_kandev",
		"manage_task_change_request_kandev",
		"update_task_change_request_automation_kandev",
		"report_change_request_auto_fix_outcome_kandev",
	} {
		assert.True(t, names[name], "catalog must expose %q", name)
	}
	for _, name := range []string{
		"get_task_pr_automation_kandev", "update_task_pr_automation_kandev",
		"get_task_mr_automation_kandev", "update_task_mr_automation_kandev",
		"link_task_pr_kandev", "unlink_task_pr_kandev", "replace_task_pr_kandev",
		"report_pr_auto_fix_outcome_kandev",
	} {
		assert.False(t, names[name], "catalog must not expose superseded %q", name)
	}
}
