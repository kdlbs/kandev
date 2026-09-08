package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	modernProtocolVersion = "2026-07-28"
	legacyProtocolVersion = "2025-06-18"
	protocolVersionHeader = "Mcp-Protocol-Version"
	methodHeader          = "Mcp-Method"
	nameHeader            = "Mcp-Name"
	protocolEchoToolName  = "protocol_" + "echo"
)

func TestMCPProtocolCompatibility_AutomaticDiscoveryAdvertisesModernVersion(t *testing.T) {
	s := newProtocolCompatibilityServer(t)

	response := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "server/discover",
		"params": map[string]any{
			"_meta": modernRequestMeta(),
		},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "server/discover",
	})

	require.Equal(t, http.StatusOK, response.Code)
	message := decodeProtocolResponse(t, response)
	result, ok := message["result"].(map[string]any)
	require.True(t, ok, "discovery response = %v", message)
	versions, ok := result["supportedVersions"].([]any)
	require.True(t, ok, "supported versions = %v", result)
	require.NotEmpty(t, versions)
	assert.Equal(t, modernProtocolVersion, versions[0])
	assert.Equal(t, float64(0), result["ttlMs"])
	assert.Equal(t, string(mcp.CacheScopePrivate), result["cacheScope"])
}

func TestMCPProtocolCompatibility_DirectModernRequestListsAndCallsTools(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.mcpServer.AddTool(
		mcp.NewTool(protocolEchoToolName, mcp.WithString("text")),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(request.GetString("text", "")), nil
		},
	)

	listResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params": map[string]any{
			"_meta": modernRequestMeta(),
		},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/list",
	})
	require.Equal(t, http.StatusOK, listResponse.Code)
	listMessage := decodeProtocolResponse(t, listResponse)
	require.NotContains(t, listMessage, "error")
	listResult, ok := listMessage["result"].(map[string]any)
	require.True(t, ok, "tools/list response = %v", listMessage)
	tools, ok := listResult["tools"].([]any)
	require.True(t, ok, "listed tools = %v", listResult["tools"])
	var echoTool map[string]any
	for _, candidate := range tools {
		if tool, ok := candidate.(map[string]any); ok && tool["name"] == protocolEchoToolName {
			echoTool = tool
			break
		}
	}
	require.NotNil(t, echoTool, "listed tools = %v", tools)
	assert.Equal(t, "object", echoTool["inputSchema"].(map[string]any)["type"])
	assert.Equal(t, float64(0), listResult["ttlMs"])
	assert.Equal(t, string(mcp.CacheScopePrivate), listResult["cacheScope"])

	callResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      protocolEchoToolName,
			"arguments": map[string]any{"text": "modern"},
			"_meta":     modernRequestMeta(),
		},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/call",
		nameHeader:            protocolEchoToolName,
	})
	require.Equal(t, http.StatusOK, callResponse.Code)
	callMessage := decodeProtocolResponse(t, callResponse)
	require.NotContains(t, callMessage, "error")
	callResult, ok := callMessage["result"].(map[string]any)
	require.True(t, ok, "tools/call response = %v", callMessage)
	content, ok := callResult["content"].([]any)
	require.True(t, ok, "call result = %v", callResult)
	require.Len(t, content, 1)
	assert.Equal(t, "modern", content[0].(map[string]any)["text"])
}

func TestMCPProtocolCompatibility_LegacyInitializeListAndCallRemainSupported(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.mcpServer.AddTool(
		mcp.NewTool(protocolEchoToolName, mcp.WithString("text")),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(request.GetString("text", "")), nil
		},
	)

	initializeResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      10,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": legacyProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "legacy-test-client",
				"version": "1.0.0",
			},
		},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
	})
	require.Equal(t, http.StatusOK, initializeResponse.Code)
	require.NotEmpty(t, initializeResponse.Header().Get("Mcp-Session-Id"))
	require.NotContains(t, decodeProtocolResponse(t, initializeResponse), "error")

	sessionID := initializeResponse.Header().Get("Mcp-Session-Id")
	listResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      11,
		"method":  "tools/list",
		"params":  map[string]any{},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
		"Mcp-Session-Id":      sessionID,
	})
	require.Equal(t, http.StatusOK, listResponse.Code)
	require.NotContains(t, decodeProtocolResponse(t, listResponse), "error")

	callResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      12,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      protocolEchoToolName,
			"arguments": map[string]any{"text": "legacy"},
		},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
		"Mcp-Session-Id":      sessionID,
	})
	require.Equal(t, http.StatusOK, callResponse.Code)
	callMessage := decodeProtocolResponse(t, callResponse)
	require.NotContains(t, callMessage, "error")
	callResult := callMessage["result"].(map[string]any)
	assert.Equal(t, "legacy", callResult["content"].([]any)[0].(map[string]any)["text"])
}

func TestMCPProtocolCompatibility_LegacyDeleteKeepsConnectionOwnedEvidence(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-legacy"})
	events := make(chan streams.MCPAttachmentEvidence, 4)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })

	initializeResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      13,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": legacyProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "legacy", "version": "1"},
		},
	}, map[string]string{protocolVersionHeader: legacyProtocolVersion})
	require.Equal(t, http.StatusOK, initializeResponse.Code)
	sessionID := initializeResponse.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)
	assert.Equal(t, streams.MCPAttachmentEvidenceInitializeObserved, nextMCPObservation(t, events).Kind)
	assert.Equal(t, streams.MCPAttachmentEvidenceSessionAccepted, nextMCPObservation(t, events).Kind)

	request := httptest.NewRequest(http.MethodDelete, "http://localhost/mcp", nil)
	request.Header.Set(protocolVersionHeader, legacyProtocolVersion)
	request.Header.Set("Mcp-Session-Id", sessionID)
	response := httptest.NewRecorder()
	s.httpServer.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	closed := nextMCPObservation(t, events)
	assert.Equal(t, streams.MCPAttachmentEvidenceConnectionClosed, closed.Kind)
	assert.Equal(t, "attempt-legacy", closed.AttemptID)
	assert.NotEmpty(t, closed.ConnectionID)
}

func TestMCPProtocolCompatibility_ModernAndLegacyRequestsCanRunConcurrently(t *testing.T) {
	s := newProtocolCompatibilityServer(t)

	modernStarted := make(chan struct{})
	releaseModern := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseModern) }) }
	defer release()
	s.mcpServer.GetHooks().AddBeforeListTools(func(ctx context.Context, _ any, _ *mcp.ListToolsRequest) {
		if !mcpserver.IsModernRequest(ctx) {
			return
		}
		close(modernStarted)
		<-releaseModern
	})

	modernResponse := make(chan *httptest.ResponseRecorder, 1)
	legacyResponse := make(chan *httptest.ResponseRecorder, 1)

	go func() {
		modernResponse <- postMCPRequest(t, s.httpServer, map[string]any{
			"jsonrpc": "2.0",
			"id":      20,
			"method":  "tools/list",
			"params":  map[string]any{"_meta": modernRequestMeta()},
		}, map[string]string{
			protocolVersionHeader: modernProtocolVersion,
			methodHeader:          "tools/list",
		})
	}()
	select {
	case <-modernStarted:
	case <-time.After(time.Second):
		t.Fatal("modern request did not reach the deterministic overlap barrier")
	}

	go func() {
		legacyResponse <- postMCPRequest(t, s.httpServer, map[string]any{
			"jsonrpc": "2.0",
			"id":      21,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": legacyProtocolVersion,
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "legacy", "version": "1"},
			},
		}, map[string]string{
			protocolVersionHeader: legacyProtocolVersion,
		})
	}()

	var legacy *httptest.ResponseRecorder
	select {
	case legacy = <-legacyResponse:
	case <-time.After(time.Second):
		t.Fatal("legacy request did not complete while modern request was blocked")
	}
	release()

	var modern *httptest.ResponseRecorder
	select {
	case modern = <-modernResponse:
	case <-time.After(time.Second):
		t.Fatal("modern request did not complete after the overlap barrier was released")
	}
	require.Equal(t, http.StatusOK, modern.Code)
	require.Equal(t, http.StatusOK, legacy.Code)
	modernMessage := decodeProtocolResponse(t, modern)
	require.NotContains(t, modernMessage, "error")
	modernResult, ok := modernMessage["result"].(map[string]any)
	require.True(t, ok, "modern tools/list response = %v", modernMessage)
	assert.Equal(t, float64(0), modernResult["ttlMs"])
	assert.Equal(t, string(mcp.CacheScopePrivate), modernResult["cacheScope"])

	legacyMessage := decodeProtocolResponse(t, legacy)
	require.NotContains(t, legacyMessage, "error")
	legacyResult, ok := legacyMessage["result"].(map[string]any)
	require.True(t, ok, "legacy initialize response = %v", legacyMessage)
	assert.Equal(t, legacyProtocolVersion, legacyResult["protocolVersion"])
}

func TestMCPProtocolCompatibility_ModernEvidenceUsesRequestAttemptSnapshotAcrossRollover(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-before-rollover"})
	events := make(chan streams.MCPAttachmentEvidence, 4)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })

	modernStarted := make(chan struct{})
	releaseModern := make(chan struct{})
	s.mcpServer.GetHooks().AddBeforeListTools(func(ctx context.Context, _ any, _ *mcp.ListToolsRequest) {
		if !mcpserver.IsModernRequest(ctx) {
			return
		}
		close(modernStarted)
		<-releaseModern
	})

	responseCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseCh <- postMCPRequest(t, s.httpServer, map[string]any{
			"jsonrpc": "2.0",
			"id":      22,
			"method":  "tools/list",
			"params":  map[string]any{"_meta": modernRequestMeta()},
		}, map[string]string{
			protocolVersionHeader: modernProtocolVersion,
			methodHeader:          "tools/list",
		})
	}()

	select {
	case <-modernStarted:
	case <-time.After(time.Second):
		close(releaseModern)
		t.Fatal("modern request did not reach the rollover barrier")
	}
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-after-rollover"})
	close(releaseModern)

	response := <-responseCh
	require.Equal(t, http.StatusOK, response.Code)
	accepted := nextMCPObservation(t, events)
	list := nextMCPObservation(t, events)
	assert.Equal(t, streams.MCPAttachmentEvidenceProtocolAccepted, accepted.Kind)
	assert.Equal(t, "attempt-before-rollover", accepted.AttemptID)
	assert.Equal(t, streams.MCPAttachmentEvidenceToolsListObserved, list.Kind)
	assert.Equal(t, "attempt-before-rollover", list.AttemptID)
}

func TestMCPProtocolCompatibility_UnregisteredLegacyRequestDoesNotUseAttemptFallback(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-current"})
	initializeResponse := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      23,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": legacyProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "legacy", "version": "1"},
		},
	}, map[string]string{protocolVersionHeader: legacyProtocolVersion})
	require.Equal(t, http.StatusOK, initializeResponse.Code)
	sessionID := initializeResponse.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)

	events := make(chan streams.MCPAttachmentEvidence, 4)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })
	s.attachmentMu.Lock()
	delete(s.attachmentAttempts, sessionID)
	s.attachmentMu.Unlock()

	response := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      24,
		"method":  "tools/list",
		"params":  map[string]any{},
	}, map[string]string{
		protocolVersionHeader: legacyProtocolVersion,
		"Mcp-Session-Id":      sessionID,
	})
	require.Equal(t, http.StatusOK, response.Code)

	select {
	case evidence := <-events:
		t.Fatalf("unregistered legacy request emitted evidence: %+v", evidence)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMCPProtocolCompatibility_InvalidModernMetadataIsNotDowngradedToLegacy(t *testing.T) {
	s := newProtocolCompatibilityServer(t)

	response := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0",
		"id":      30,
		"method":  "tools/list",
		"params":  map[string]any{},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/list",
	})

	assert.Equal(t, http.StatusBadRequest, response.Code)
	message := decodeProtocolResponse(t, response)
	errorDetails, ok := message["error"].(map[string]any)
	require.True(t, ok, "response = %v", message)
	assert.NotEqual(t, float64(-32601), errorDetails["code"], "must not enter legacy method handling")
	assert.Empty(t, response.Header().Get("Mcp-Session-Id"))
}

func TestMCPProtocolCompatibility_ModernEvidenceUsesAttemptWithoutConnectionLifecycle(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-modern"})
	events := make(chan streams.MCPAttachmentEvidence, 8)
	s.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { events <- evidence })
	s.mcpServer.AddTool(
		mcp.NewTool(protocolEchoToolName, mcp.WithString("text")),
		func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(request.GetString("text", "")), nil
		},
	)

	discovery := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0", "id": 40, "method": "server/discover",
		"params": map[string]any{"_meta": modernRequestMeta()},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "server/discover",
	})
	require.Equal(t, http.StatusOK, discovery.Code)
	accepted := nextMCPObservation(t, events)
	assert.Equal(t, streams.MCPAttachmentEvidenceProtocolAccepted, accepted.Kind)
	assert.Equal(t, "attempt-modern", accepted.AttemptID)
	assert.Empty(t, accepted.ConnectionID)

	list := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0", "id": 41, "method": "tools/list",
		"params": map[string]any{"_meta": modernRequestMeta()},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/list",
	})
	require.Equal(t, http.StatusOK, list.Code)
	assert.Equal(t, streams.MCPAttachmentEvidenceProtocolAccepted, nextMCPObservation(t, events).Kind)
	listEvidence := nextMCPObservation(t, events)
	assert.Equal(t, streams.MCPAttachmentEvidenceToolsListObserved, listEvidence.Kind)
	assert.Equal(t, listEvidence.ToolCount, len(listEvidence.Tools))
	require.NotEmpty(t, listEvidence.Tools)
	var foundEcho bool
	for _, tool := range listEvidence.Tools {
		if tool.Name == protocolEchoToolName {
			foundEcho = true
			break
		}
	}
	assert.True(t, foundEcho, "tool evidence = %+v", listEvidence.Tools)
	assert.Empty(t, listEvidence.ConnectionID)

	call := postMCPRequest(t, s.httpServer, map[string]any{
		"jsonrpc": "2.0", "id": 42, "method": "tools/call",
		"params": map[string]any{
			"name": protocolEchoToolName, "arguments": map[string]any{"text": "modern"},
			"_meta": modernRequestMeta(),
		},
	}, map[string]string{
		protocolVersionHeader: modernProtocolVersion,
		methodHeader:          "tools/call",
		nameHeader:            protocolEchoToolName,
	})
	require.Equal(t, http.StatusOK, call.Code)
	assert.Equal(t, streams.MCPAttachmentEvidenceProtocolAccepted, nextMCPObservation(t, events).Kind)
	callEvidence := nextMCPObservation(t, events)
	assert.Equal(t, streams.MCPAttachmentEvidenceToolCallObserved, callEvidence.Kind)
	assert.Empty(t, callEvidence.ConnectionID)

	select {
	case evidence := <-events:
		t.Fatalf("unexpected extra modern evidence: %+v", evidence)
	case <-time.After(50 * time.Millisecond):
	}
}

func newProtocolCompatibilityServer(t *testing.T) *Server {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	s := New(nil, "protocol-session", "protocol-task", 10005, log, "", false, ModeTask)
	t.Cleanup(func() {
		_ = s.Close(context.Background())
	})
	return s
}

func modernRequestMeta() map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/protocolVersion": modernProtocolVersion,
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"name":    "modern-test-client",
			"version": "1.0.0",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
}

func postMCPRequest(t *testing.T, handler http.Handler, payload map[string]any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeProtocolResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var message map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &message), "body = %q", response.Body.String())
	return message
}

func nextMCPObservation(t *testing.T, events <-chan streams.MCPAttachmentEvidence) streams.MCPAttachmentEvidence {
	t.Helper()
	select {
	case evidence := <-events:
		return evidence
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for MCP evidence")
		return streams.MCPAttachmentEvidence{}
	}
}
