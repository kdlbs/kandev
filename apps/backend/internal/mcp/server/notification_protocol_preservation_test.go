package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-MCP-PROTOCOL-001.9
func TestSupplementNotificationProtocolVersion_PreservesAmbiguousMembers(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate params with explicit metadata": `{"jsonrpc":"2.0","method":"m","params":{"_meta":null},"params":{}}`,
		"duplicate params with malformed shape":   `{"jsonrpc":"2.0","method":"m","params":[],"params":{}}`,
		"duplicate method":                        `{"jsonrpc":"2.0","method":"old","method":"m"}`,
		"duplicate version":                       `{"jsonrpc":"1.0","jsonrpc":"2.0","method":"m"}`,
		"duplicate parameter":                     `{"jsonrpc":"2.0","method":"m","params":{"n":1,"n":2}}`,
		"escaped duplicate member":                `{"jsonrpc":"2.0","method":"m","params":{},"\u0070arams":{}}`,
		"unicode folded version":                  `{"jſonrpc":"2.0","method":"m"}`,
		"unicode folded params":                   `{"jsonrpc":"2.0","method":"m","paramſ":{}}`,
		"null metadata":                           `{"jsonrpc":"2.0","method":"m","params":{"_meta":null}}`,
		"malformed metadata":                      `{"jsonrpc":"2.0","method":"m","params":{"_meta":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, body, string(supplementNotificationProtocolVersion([]byte(body), modernProtocolVersion)))
		})
	}
}

func TestStreamableHTTPHandler_PreservesDuplicateParamsValidation(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	body := `{"jsonrpc":"2.0","method":"notifications/roots/list_changed","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1999-01-01"}},"params":{}}`
	direct := postRawNotification(s.httpServer, body)
	require.Equal(t, http.StatusBadRequest, direct.Code)
	adapted := postRawNotification(s.streamableHTTPHandler(), body)
	assert.Equal(t, direct.Code, adapted.Code)
	assert.Equal(t, direct.Body.String(), adapted.Body.String())
}

func TestRegisteredMCPRoutes_NotificationThenToolUse(t *testing.T) {
	for _, backend := range []bool{false, true} {
		name := "agentctl"
		if backend {
			name = "backend"
		}
		t.Run(name, func(t *testing.T) {
			s := newProtocolCompatibilityServer(t)
			s.mcpServer.AddTool(mcp.NewTool(protocolEchoToolName),
				func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("after-notification"), nil
				})
			router := gin.New()
			if backend {
				s.RegisterBackendRoutes(router)
			} else {
				s.RegisterRoutes(router)
			}
			notification := postRawNotification(router, `{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`)
			require.Equal(t, http.StatusAccepted, notification.Code)
			require.Empty(t, notification.Body.String())
			listed := postMCPRequest(t, router, map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "tools/list",
				"params": map[string]any{"_meta": modernRequestMeta()},
			}, map[string]string{protocolVersionHeader: modernProtocolVersion, methodHeader: "tools/list"})
			require.Equal(t, http.StatusOK, listed.Code)
			require.NotContains(t, decodeProtocolResponse(t, listed), "error")
			require.Contains(t, listed.Body.String(), protocolEchoToolName)
			called := postMCPRequest(t, router, map[string]any{
				"jsonrpc": "2.0", "id": 2, "method": "tools/call",
				"params": map[string]any{"_meta": modernRequestMeta(), "name": protocolEchoToolName},
			}, map[string]string{
				protocolVersionHeader: modernProtocolVersion, methodHeader: "tools/call", nameHeader: protocolEchoToolName,
			})
			require.Equal(t, http.StatusOK, called.Code)
			require.NotContains(t, decodeProtocolResponse(t, called), "error")
			assert.Contains(t, called.Body.String(), "after-notification")
		})
	}
}

func TestNotificationBody_ForwardsReadAndCloseErrors(t *testing.T) {
	readErr := errors.New("partial read failed")
	closeErr := errors.New("close failed")
	original := &notificationErrorBody{readErr: readErr, closeErr: closeErr}
	body := &notificationBody{original: original, version: modernProtocolVersion}
	_, err := io.ReadAll(body)
	require.ErrorIs(t, err, readErr)
	require.ErrorIs(t, body.Close(), closeErr)
	assert.True(t, original.closed)
}

func TestStreamableHTTPHandler_DoesNotWrapModernToolRequests(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	s.mcpServer.AddTool(mcp.NewTool(protocolEchoToolName),
		func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ordinary-tool-request"), nil
		})
	for _, method := range []string{"tools/list", "tools/call", "server/discover"} {
		t.Run(method, func(t *testing.T) {
			params := map[string]any{"_meta": modernRequestMeta(), "extra": strings.Repeat("x", 65536)}
			if method == "tools/call" {
				params["name"] = protocolEchoToolName
			}
			payload, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": method,
				"params": params,
			})
			require.NoError(t, err)
			originalBody := &struct{ io.ReadCloser }{io.NopCloser(bytes.NewReader(payload))}
			request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", originalBody)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set(protocolVersionHeader, modernProtocolVersion)
			request.Header.Set(methodHeader, method)
			if method == "tools/call" {
				request.Header.Set(nameHeader, protocolEchoToolName)
			}
			response := httptest.NewRecorder()
			s.streamableHTTPHandler().ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, "body = %s", response.Body.String())
			require.NotContains(t, decodeProtocolResponse(t, response), "error")
			assert.Same(t, originalBody, request.Body, "ordinary requests must reach the SDK without a body wrapper")
		})
	}
}

func TestStreamableHTTPHandler_NotificationWithoutMethodHeader(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	headers := notificationHeaders()
	delete(headers, methodHeader)
	response := postMCPRequest(t, s.streamableHTTPHandler(), map[string]any{
		"jsonrpc": "2.0", "method": rootsListChanged,
	}, headers)
	assert.Equal(t, http.StatusAccepted, response.Code, "body = %s", response.Body.String())
	assert.Empty(t, response.Body.String())
}

type notificationErrorBody struct {
	readErr  error
	closeErr error
	closed   bool
}

func (b *notificationErrorBody) Read(p []byte) (int, error) {
	return copy(p, `{"jsonrpc":"2.0"}`), b.readErr
}

func (b *notificationErrorBody) Close() error {
	b.closed = true
	return b.closeErr
}

func postRawNotification(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range notificationHeaders() {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
