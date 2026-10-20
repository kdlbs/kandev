package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rootsListChanged = "notifications/roots/list_changed"

func notificationHeaders() map[string]string {
	return map[string]string{protocolVersionHeader: modernProtocolVersion, methodHeader: rootsListChanged}
}

func TestStreamableHTTPHandler_AcceptsHeaderOnlyModernNotification(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	notification := map[string]any{"jsonrpc": "2.0", "method": rootsListChanged}

	direct := postMCPRequest(t, s.httpServer, notification, notificationHeaders())
	assert.Equal(t, http.StatusBadRequest, direct.Code, "SDK alone rejects header-only notifications")

	response := postMCPRequest(t, s.streamableHTTPHandler(), notification, notificationHeaders())
	assert.Equal(t, http.StatusAccepted, response.Code, "body = %q", response.Body.String())
	assert.Empty(t, response.Body.String())

	withParams := postMCPRequest(t, s.streamableHTTPHandler(), map[string]any{
		"jsonrpc": "2.0", "method": rootsListChanged, "params": map[string]any{"extra": 1},
	}, notificationHeaders())
	assert.Equal(t, http.StatusAccepted, withParams.Code, "body = %q", withParams.Body.String())
}

func TestStreamableHTTPHandler_KeepsSDKValidation(t *testing.T) {
	s := newProtocolCompatibilityServer(t)

	explicitMeta := postMCPRequest(t, s.streamableHTTPHandler(), map[string]any{
		"jsonrpc": "2.0", "method": rootsListChanged,
		"params": map[string]any{"_meta": map[string]any{mcp.MetaKeyProtocolVersion: "1999-01-01"}},
	}, notificationHeaders())
	assert.Equal(t, http.StatusBadRequest, explicitMeta.Code, "explicit metadata must not be repaired")

	request := postMCPRequest(t, s.streamableHTTPHandler(), map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{},
	}, map[string]string{protocolVersionHeader: modernProtocolVersion, methodHeader: "tools/list"})
	assert.Equal(t, http.StatusBadRequest, request.Code, "requests keep SDK metadata validation")
}

func TestStreamableHTTPHandler_ForwardsBodyReadErrors(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", io.NopCloser(failingReader{}))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set(protocolVersionHeader, modernProtocolVersion)
	request.Header.Set(methodHeader, rootsListChanged)

	response := httptest.NewRecorder()
	s.streamableHTTPHandler().ServeHTTP(response, request)
	assert.NotEqual(t, http.StatusAccepted, response.Code)
	assert.Contains(t, response.Body.String(), "read failed", "the SDK reports the original read error")
}

func TestStreamableHTTPHandler_DoesNotReadBodyBeforeTransportChecks(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	body := &countingReader{Reader: strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`)}
	request := httptest.NewRequest(http.MethodPost, "http://untrusted.example/mcp", body)
	loopback := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10005}
	request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, loopback))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set(protocolVersionHeader, modernProtocolVersion)
	request.Header.Set(methodHeader, rootsListChanged)

	response := httptest.NewRecorder()
	s.streamableHTTPHandler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code, "SDK DNS-rebinding protection rejects the request")
	assert.Zero(t, body.reads, "a rejected request body must not be read")
}

func TestSupplementNotificationProtocolVersion_AddsMetadata(t *testing.T) {
	for name, body := range map[string]string{
		"absent params":   `{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`,
		"existing params": `{"jsonrpc":"2.0","method":"notifications/roots/list_changed","params":{"n":12345678901234567890}}`,
	} {
		t.Run(name, func(t *testing.T) {
			updated := supplementNotificationProtocolVersion([]byte(body), modernProtocolVersion)
			var message struct {
				Params map[string]json.RawMessage `json:"params"`
			}
			require.NoError(t, json.Unmarshal(updated, &message))
			var meta map[string]string
			require.NoError(t, json.Unmarshal(message.Params["_meta"], &meta))
			assert.Equal(t, modernProtocolVersion, meta[mcp.MetaKeyProtocolVersion])
			if raw, ok := message.Params["n"]; ok {
				assert.Equal(t, "12345678901234567890", string(raw), "numeric precision is preserved")
			}
		})
	}
}

func TestSupplementNotificationProtocolVersion_LeavesOtherMessagesUntouched(t *testing.T) {
	for name, body := range map[string]string{
		"request id":         `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		"null id":            `{"jsonrpc":"2.0","id":null,"method":"notifications/roots/list_changed"}`,
		"cased id":           `{"jsonrpc":"2.0","ID":1,"method":"notifications/roots/list_changed"}`,
		"explicit meta":      `{"jsonrpc":"2.0","method":"m","params":{"_meta":{}}}`,
		"cased meta":         `{"jsonrpc":"2.0","method":"m","params":{"_META":{}}}`,
		"cased method":       `{"jsonrpc":"2.0","Method":"m"}`,
		"cased params":       `{"jsonrpc":"2.0","method":"m","Params":{}}`,
		"wrong jsonrpc":      `{"jsonrpc":"1.0","method":"m"}`,
		"missing method":     `{"jsonrpc":"2.0"}`,
		"non-object params":  `{"jsonrpc":"2.0","method":"m","params":[1]}`,
		"null params":        `{"jsonrpc":"2.0","method":"m","params":null}`,
		"batch":              `[{"jsonrpc":"2.0","method":"m"}]`,
		"invalid JSON":       `{"jsonrpc":`,
		"non-object message": `"text"`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, body, string(supplementNotificationProtocolVersion([]byte(body), modernProtocolVersion)))
		})
	}
}

func TestStreamableHTTPHandler_BypassesNonModernRequests(t *testing.T) {
	s := newProtocolCompatibilityServer(t)
	notification := map[string]any{"jsonrpc": "2.0", "method": rootsListChanged}
	for name, version := range map[string]string{
		"legacy version":             "2025-06-18",
		"unsupported modern version": "2099-01-01",
	} {
		t.Run(name, func(t *testing.T) {
			headers := map[string]string{protocolVersionHeader: version, methodHeader: rootsListChanged}
			direct := postMCPRequest(t, s.httpServer, notification, headers)
			viaAdapter := postMCPRequest(t, s.streamableHTTPHandler(), notification, headers)
			assert.Equal(t, direct.Code, viaAdapter.Code)
			assert.Equal(t, direct.Body.String(), viaAdapter.Body.String())
		})
	}
}

type countingReader struct {
	io.Reader
	reads int
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
