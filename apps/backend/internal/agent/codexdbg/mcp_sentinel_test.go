package codexdbg

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestMCPSentinelRecordsOnlyObservedProtocolTraffic(t *testing.T) {
	sentinel := NewMCPSentinel()
	t.Cleanup(sentinel.Close)
	call := func(id int, method string, params any) {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Post(sentinel.server.URL+"/mcp", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("POST %s: %v", method, err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("POST %s status = %d", method, response.StatusCode)
		}
	}
	call(1, "initialize", map[string]any{})
	call(2, "tools/list", map[string]any{})
	call(3, "tools/call", map[string]any{"name": mcpSentinelTool})
	got := sentinel.Summary()
	if !got.InitializeObserved || !got.ToolsListObserved || !got.ToolCallObserved || got.ToolCount != 1 {
		t.Fatalf("sentinel observations = %#v", got)
	}
}

func TestMCPSentinelRejectsMalformedAndUnknownTools(t *testing.T) {
	sentinel := NewMCPSentinel()
	t.Cleanup(sentinel.Close)
	response, err := http.Post(sentinel.server.URL+"/mcp", "application/json", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unknown"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var body struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Error) == 0 {
		t.Fatal("unknown sentinel tool did not return a JSON-RPC error")
	}
	if got := sentinel.Summary(); got.ToolCallObserved {
		t.Fatal("unknown tool call was recorded as observed")
	}
}
