package acp

import (
	"bufio"
	"encoding/json"
	"io"
	"reflect"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestClientSideConnection_InterceptsCursorTaskRequest(t *testing.T) {
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})

	paramsCh := make(chan json.RawMessage, 1)
	_ = acpsdk.NewClientSideConnection(
		NewClient(WithCursorTaskHandler(func(params json.RawMessage) {
			paramsCh <- append(json.RawMessage(nil), params...)
		})),
		clientToAgentWriter,
		agentToClientReader,
	)

	if err := writeJSONRPCLine(agentToClientWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  cursorTaskMethod,
		"params": map[string]any{
			"toolCallId": "tool-1",
			"prompt":     "summarize",
		},
	}); err != nil {
		t.Fatalf("write cursor/task request: %v", err)
	}

	response := readJSONRPCLine(t, clientToAgentReader)
	if !reflect.DeepEqual(response["result"], map[string]any{}) {
		t.Fatalf("response result = %#v, want empty success object", response["result"])
	}
	if response["error"] != nil {
		t.Fatalf("response error = %#v, want nil", response["error"])
	}

	select {
	case got := <-paramsCh:
		var gotParams map[string]any
		if err := json.Unmarshal(got, &gotParams); err != nil {
			t.Fatalf("unmarshal params %q: %v", string(got), err)
		}
		wantParams := map[string]any{"toolCallId": "tool-1", "prompt": "summarize"}
		if !reflect.DeepEqual(gotParams, wantParams) {
			t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for cursor/task handler")
	}
}

func TestClientSideConnection_UnknownVendorRequestReturnsMethodNotFound(t *testing.T) {
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})

	_ = acpsdk.NewClientSideConnection(NewClient(), clientToAgentWriter, agentToClientReader)

	if err := writeJSONRPCLine(agentToClientWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      8,
		"method":  "foo/bar",
		"params":  map[string]any{"x": 1},
	}); err != nil {
		t.Fatalf("write vendor request: %v", err)
	}

	response := readJSONRPCLine(t, clientToAgentReader)
	errorMap, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("response error = %#v, want JSON-RPC error object", response["error"])
	}
	if errorMap["code"] != float64(-32601) {
		t.Fatalf("error code = %#v, want -32601", errorMap["code"])
	}
}

func writeJSONRPCLine(w io.Writer, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = w.Write(encoded)
	return err
}

func readJSONRPCLine(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	line, err := bufio.NewReader(r).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read response line: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(line, &decoded); err != nil {
		t.Fatalf("unmarshal response %q: %v", string(line), err)
	}
	return decoded
}
