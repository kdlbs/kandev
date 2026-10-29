//go:build copilot_native

package copilotplugins_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/acpdbg"
)

type nativeSSEConnection struct {
	events chan nativeSSEEvent
}

type nativeSSEEvent struct {
	data           []byte
	toolCallResult bool
}

type nativeMCPSSESentinel struct {
	server      *httptest.Server
	mu          sync.Mutex
	nextID      uint64
	connections map[string]*nativeSSEConnection
	summary     nativeSentinelTrafficSummary
}

type nativeSSERequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  map[string]any  `json:"params"`
}

func newNativeMCPSSESentinel(recorder *acpdbg.Recorder) *nativeMCPSSESentinel {
	sentinel := &nativeMCPSSESentinel{connections: make(map[string]*nativeSSEConnection)}
	sentinel.server = httptest.NewServer(http.HandlerFunc(sentinel.serveHTTP))
	if recorder != nil {
		_ = recorder.Meta("mcp_sse_sentinel_started", map[string]any{"target": sentinel.URL()})
	}
	return sentinel
}

func (s *nativeMCPSSESentinel) URL() string {
	return s.server.URL + "/sse"
}

func (s *nativeMCPSSESentinel) HTTPURL() string {
	return s.server.URL + "/mcp"
}

func (s *nativeMCPSSESentinel) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *nativeMCPSSESentinel) Summary() acpdbg.MCPSentinelSummary {
	summary := s.TrafficSummary()
	return acpdbg.MCPSentinelSummary{
		InitializeObserved: summary.InitializeObserved,
		ToolsListObserved:  summary.ToolsListObserved,
		ToolCallObserved:   summary.ToolCallObserved,
		ToolCount:          summary.ToolCount,
	}
}

func (s *nativeMCPSSESentinel) TrafficSummary() nativeSentinelTrafficSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summary
}

func (s *nativeMCPSSESentinel) allowsReadOnlyCall(toolName string, input any) bool {
	summary := s.Summary()
	tool := nativeSentinelToolDescriptor()
	annotations, _ := tool["annotations"].(map[string]any)
	return summary.ToolsListObserved &&
		tool["name"] == toolName &&
		annotations["readOnlyHint"] == true &&
		annotations["destructiveHint"] == false &&
		nativeInputIsEmpty(input)
}

func (s *nativeMCPSSESentinel) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/sse":
		s.serveEvents(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/messages":
		s.serveMessage(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/mcp":
		s.serveHTTPMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *nativeMCPSSESentinel) serveHTTPMessage(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var request nativeSSERequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	if request.JSONRPC != "2.0" || request.Method == "" {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	response := s.response(request)
	if response == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		s.mu.Lock()
		s.nextID++
		sessionID = fmt.Sprintf("http-session-%d", s.nextID)
		s.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mcp-Session-Id", sessionID)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "invalid JSON-RPC response", http.StatusInternalServerError)
		return
	}
	if request.Method == "tools/call" && response["error"] == nil {
		s.recordToolCallResultSent()
	}
}

func (s *nativeMCPSSESentinel) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is unavailable", http.StatusInternalServerError)
		return
	}
	id, connection := s.addConnection()
	defer s.removeConnection(id)

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Content-Type", "text/event-stream")
	endpoint := s.server.URL + "/messages?sessionId=" + url.QueryEscape(id)
	if _, err := fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case event := <-connection.events:
			if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", event.data); err != nil {
				return
			}
			if event.toolCallResult {
				s.recordToolCallResultSent()
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *nativeMCPSSESentinel) serveMessage(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	connection := s.connection(r.URL.Query().Get("sessionId"))
	if connection == nil {
		http.NotFound(w, r)
		return
	}
	var request nativeSSERequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	if request.JSONRPC != "2.0" || request.Method == "" {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	response := s.response(request)
	if response != nil {
		data, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "invalid JSON-RPC response", http.StatusInternalServerError)
			return
		}
		select {
		case connection.events <- nativeSSEEvent{
			data:           data,
			toolCallResult: request.Method == "tools/call" && response["error"] == nil,
		}:
		case <-r.Context().Done():
			return
		default:
			http.Error(w, "event queue is full", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *nativeMCPSSESentinel) response(request nativeSSERequest) map[string]any {
	if !nativeSSEHasID(request.ID) {
		return nil
	}
	response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
	switch request.Method {
	case "initialize":
		s.recordMethod(request.Method)
		version, _ := request.Params["protocolVersion"].(string)
		if version == "" {
			version = "2024-11-05"
		}
		response["result"] = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "native-sentinel", "version": "1"},
		}
	case "tools/list":
		s.recordMethod(request.Method)
		response["result"] = map[string]any{"tools": []map[string]any{nativeSentinelToolDescriptor()}}
	case "tools/call":
		s.recordToolCallRequest()
		name, _ := request.Params["name"].(string)
		if !s.allowsReadOnlyCall(name, request.Params["arguments"]) {
			response["error"] = map[string]any{"code": -32602, "message": "invalid sentinel tool call"}
			return response
		}
		s.recordMethod(request.Method)
		response["result"] = map[string]any{
			"content": []map[string]string{{"type": "text", "text": "sentinel"}},
			"isError": false,
		}
	case "ping":
		response["result"] = map[string]any{}
	default:
		response["error"] = map[string]any{"code": -32601, "message": "method not found"}
	}
	return response
}

func nativeSentinelToolDescriptor() map[string]any {
	return map[string]any{
		"name":        nativeSentinelToolName,
		"description": "Harmless read-only qualification sentinel",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false},
	}
}

func (s *nativeMCPSSESentinel) addConnection() (string, *nativeSSEConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := fmt.Sprintf("connection-%d", s.nextID)
	connection := &nativeSSEConnection{events: make(chan nativeSSEEvent, 16)}
	s.connections[id] = connection
	return id, connection
}

func (s *nativeMCPSSESentinel) removeConnection(id string) {
	s.mu.Lock()
	delete(s.connections, id)
	s.mu.Unlock()
}

func (s *nativeMCPSSESentinel) connection(id string) *nativeSSEConnection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections[id]
}

func (s *nativeMCPSSESentinel) recordMethod(method string) {
	s.mu.Lock()
	switch method {
	case "initialize":
		s.summary.InitializeObserved = true
	case "tools/list":
		s.summary.ToolsListObserved = true
		s.summary.ToolCount = 1
	case "tools/call":
		s.summary.ToolCallObserved = true
	}
	s.mu.Unlock()
}

func (s *nativeMCPSSESentinel) recordToolCallRequest() {
	s.mu.Lock()
	s.summary.ToolCallRequestCount++
	s.mu.Unlock()
}

func (s *nativeMCPSSESentinel) recordToolCallResultSent() {
	s.mu.Lock()
	s.summary.ToolCallResultSentCount++
	s.mu.Unlock()
}

func nativeSSEHasID(id json.RawMessage) bool {
	return len(id) > 0 && !bytes.Equal(bytes.TrimSpace(id), []byte("null"))
}

func TestNativeMCPSSESentinelProtocol(t *testing.T) {
	sentinel := newNativeMCPSSESentinel(nil)
	defer sentinel.Close()

	streamCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, sentinel.URL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	streamResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = streamResponse.Body.Close() }()
	if streamResponse.StatusCode != http.StatusOK {
		t.Fatalf("SSE endpoint status = %d, want %d", streamResponse.StatusCode, http.StatusOK)
	}
	reader := bufio.NewReader(streamResponse.Body)
	event, endpointValue, err := readNativeSSEEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if event != "endpoint" {
		t.Fatalf("first SSE event = %q, want endpoint", event)
	}
	endpoint, err := url.Parse(endpointValue)
	if err != nil || endpoint.Path != "/messages" || endpoint.Query().Get("sessionId") == "" {
		t.Fatalf("SSE endpoint event did not provide a message URL")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	initialize := postNativeSSERequest(t, client, endpointValue, reader, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": "2024-11-05"},
	})
	initResult, _ := initialize["result"].(map[string]any)
	if initResult["protocolVersion"] != "2024-11-05" {
		t.Fatalf("initialize did not negotiate the requested protocol version")
	}
	postNativeSSERequest(t, client, endpointValue, reader, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	list := postNativeSSERequest(t, client, endpointValue, reader, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	listResult, _ := list["result"].(map[string]any)
	tools, _ := listResult["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools/list returned %d tools, want one", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != nativeSentinelToolName {
		t.Fatalf("tools/list returned unexpected sentinel identity")
	}
	annotations, _ := tool["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false {
		t.Fatalf("tools/list returned unexpected sentinel safety annotations: %v", annotations)
	}
	invalidCall := postNativeSSERequest(t, client, endpointValue, reader, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params":  map[string]any{"name": nativeSentinelToolName, "arguments": map[string]any{"path": "outside"}},
	})
	if invalidCall["error"] == nil {
		t.Fatal("tools/call accepted a non-empty argument object")
	}
	call := postNativeSSERequest(t, client, endpointValue, reader, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params":  map[string]any{"name": nativeSentinelToolName, "arguments": map[string]any{}},
	})
	callResult, _ := call["result"].(map[string]any)
	content, _ := callResult["content"].([]any)
	if len(content) != 1 || !nativeValueContains(content, "sentinel") {
		t.Fatalf("tools/call did not return the sentinel result")
	}

	summary := sentinel.Summary()
	if !summary.InitializeObserved || !summary.ToolsListObserved || !summary.ToolCallObserved {
		t.Fatalf("SSE protocol summary omitted an observed MCP operation: %+v", summary)
	}
	traffic := sentinel.TrafficSummary()
	if traffic.ToolCallRequestCount != 2 || traffic.ToolCallResultSentCount != 1 {
		t.Fatalf("SSE tool call request/result counts = %d/%d, want rejected+accepted requests and one result",
			traffic.ToolCallRequestCount, traffic.ToolCallResultSentCount)
	}
}

func TestNativeMCPHTTPSentinelReadOnlyContract(t *testing.T) {
	sentinel := newNativeMCPSSESentinel(nil)
	defer sentinel.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	initialize := postNativeHTTPMCPRequest(t, client, sentinel.HTTPURL(), map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": "2024-11-05"},
	})
	if initialize["result"] == nil {
		t.Fatalf("initialize response omitted result: %v", initialize)
	}
	list := postNativeHTTPMCPRequest(t, client, sentinel.HTTPURL(), map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	listResult, _ := list["result"].(map[string]any)
	tools, _ := listResult["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools/list returned %d tools, want one", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	annotations, _ := tool["annotations"].(map[string]any)
	if tool["name"] != nativeSentinelToolName ||
		annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false {
		t.Fatalf("HTTP tools/list did not return the exact annotated sentinel: %v", tool)
	}

	invalidCall := postNativeHTTPMCPRequest(t, client, sentinel.HTTPURL(), map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params":  map[string]any{"name": nativeSentinelToolName, "arguments": map[string]any{"path": "outside"}},
	})
	if invalidCall["error"] == nil {
		t.Fatal("HTTP tools/call accepted a non-empty argument object")
	}
	if sentinel.Summary().ToolCallObserved {
		t.Fatal("HTTP sentinel recorded a tool call for rejected arguments")
	}

	validCall := postNativeHTTPMCPRequest(t, client, sentinel.HTTPURL(), map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params":  map[string]any{"name": nativeSentinelToolName, "arguments": map[string]any{}},
	})
	if !nativeValueContains(validCall["result"], "sentinel") {
		t.Fatalf("HTTP tools/call did not return the sentinel result: %v", validCall)
	}
	if !sentinel.allowsReadOnlyCall(nativeSentinelToolName, map[string]any{}) ||
		sentinel.allowsReadOnlyCall(nativeSentinelToolName, map[string]any{"path": "outside"}) ||
		sentinel.allowsReadOnlyCall("another_tool", map[string]any{}) {
		t.Fatal("HTTP fixture permission guard did not enforce the exact read-only empty-input operation")
	}
	traffic := sentinel.TrafficSummary()
	if traffic.ToolCallRequestCount != 2 || traffic.ToolCallResultSentCount != 1 {
		t.Fatalf("HTTP tool call request/result counts = %d/%d, want rejected+accepted requests and one result",
			traffic.ToolCallRequestCount, traffic.ToolCallResultSentCount)
	}
}

func postNativeHTTPMCPRequest(
	t *testing.T,
	client *http.Client,
	endpoint string,
	frame map[string]any,
) map[string]any {
	t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP MCP response status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func postNativeSSERequest(
	t *testing.T,
	client *http.Client,
	endpoint string,
	events *bufio.Reader,
	message map[string]any,
) map[string]any {
	t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("MCP POST status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
	if _, hasID := message["id"]; !hasID {
		return nil
	}
	event, payload, err := readNativeSSEEvent(events)
	if err != nil {
		t.Fatal(err)
	}
	if event != "message" {
		t.Fatalf("MCP response event = %q, want message", event)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func readNativeSSEEvent(reader *bufio.Reader) (string, string, error) {
	var event string
	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if event != "" || data.Len() > 0 {
				return event, data.String(), nil
			}
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
}
