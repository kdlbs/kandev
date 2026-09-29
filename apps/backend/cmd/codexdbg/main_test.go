package main

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/codexdbg"
	"github.com/kandev/kandev/pkg/codexappserver"
)

func TestCommandParsingAndErrors(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		wantErr string
	}{
		{name: "probe defaults", command: "probe", wantErr: ""},
		{name: "prompt", command: "prompt", args: []string{"--prompt", "hello"}},
		{name: "prompt requires prompt", command: "prompt", wantErr: "--prompt is required"},
		{name: "thread read", command: "thread-read", args: []string{"--thread-id", "thread-1"}},
		{name: "thread read requires id", command: "thread-read", wantErr: "--thread-id is required"},
		{name: "thread resume", command: "thread-resume", args: []string{"--thread-id", "thread-1", "--prompt", "hello"}},
		{name: "thread resume requires fields", command: "thread-resume", args: []string{"--thread-id", "thread-1"}, wantErr: "--thread-id and --prompt are required"},
		{name: "thread fork", command: "thread-fork", args: []string{"--thread-id", "thread-1", "--through-turn", "turn-1"}},
		{name: "thread fork requires turn", command: "thread-fork", args: []string{"--thread-id", "thread-1"}, wantErr: "--thread-id and --through-turn are required"},
		{name: "interrupt", command: "interrupt", args: []string{"--prompt", "hello"}},
		{name: "interrupt requires prompt", command: "interrupt", wantErr: "--prompt is required"},
		{name: "mcp probe", command: "mcp-probe"},
		{name: "unknown command", command: "promptx", wantErr: "unknown subcommand"},
		{name: "reject positional", command: "probe", args: []string{"codex"}, wantErr: "unexpected positional"},
		{name: "reject negative linger", command: "prompt", args: []string{"--prompt", "hello", "--linger", "-1s"}, wantErr: "cannot be negative"},
		{name: "reject zero timeout", command: "probe", args: []string{"--timeout", "0s"}, wantErr: "timeout must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, err := parseCommand(test.command, test.args, io.Discard)
			if test.wantErr != "" {
				if err == nil || !bytes.Contains([]byte(err.Error()), []byte(test.wantErr)) {
					t.Fatalf("parseCommand error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCommand: %v", err)
			}
			if test.command == "probe" && (len(opts.args) != 1 || opts.args[0] != "app-server") {
				t.Fatalf("default app-server args = %#v", opts.args)
			}
		})
	}
}

func TestInspectScopesAndFiltersUsageWithoutPrintingRawFrames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	recorder, err := codexdbg.NewRecorder(path, codexappserver.SupportedCodexVersion)
	if err != nil {
		t.Fatal(err)
	}
	frames := []struct {
		direction codexappserver.FrameDirection
		frame     string
	}{
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","method":"rawResponse/completed","params":{"threadId":"thread-1","turnId":"turn-1","responseId":"response-1","usage":{"input_tokens":1000,"cached_input_tokens":600,"output_tokens":200}}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","method":"rawResponse/completed","params":{"threadId":"thread-1","turnId":"turn-1","responseId":"response-1","usage":{"input_tokens":1000,"cached_input_tokens":600,"output_tokens":200}}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","id":"permission-9","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"item-9","command":"secret-command"}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"child-activity-1","type":"subAgentActivity","childThreadId":"child-thread-1"}}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":"permission-9"}}`},
		{codexappserver.FrameSent, `{"jsonrpc":"2.0","id":"permission-9","result":{"decision":"decline"}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","method":"thread/tokenUsage/updated","params":{"threadId":"thread-1","tokenUsage":{"total":{"inputTokens":1200,"outputTokens":200}}}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed"}}}`},
		{codexappserver.FrameSent, `{"jsonrpc":"2.0","id":19,"method":"account/usage/read","params":{"threadId":"thread-1"}}`},
		{codexappserver.FrameReceived, `{"jsonrpc":"2.0","id":19,"result":{"estimatedUsageUsdMicros":250,"credits":{"hasCredits":true}}}`},
	}
	for _, frame := range frames {
		if err := recorder.RecordFrame(frame.direction, json.RawMessage(frame.frame)); err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.Close("completed"); err != nil {
		t.Fatal(err)
	}
	entries, err := codexdbg.ReadCapture(path)
	if err != nil {
		t.Fatal(err)
	}
	items := inspectEntries(entries, "thread-1", "")
	scopes := map[string]bool{}
	for _, item := range items {
		if item.Scope != "" {
			scopes[item.Scope] = true
		}
	}
	for _, scope := range []string{"response", "thread", "turn", "provider_thread_estimate"} {
		if !scopes[scope] {
			t.Errorf("missing usage scope %q in %#v", scope, items)
		}
	}
	turnItems := inspectEntries(entries, "thread-1", "turn-1")
	for _, item := range turnItems {
		if item.TurnID != "turn-1" {
			t.Errorf("turn filter returned entry without matching turn id: %#v", item)
		}
	}
	var output bytes.Buffer
	if err := runInspect([]string{"--file", path, "--thread-id", "thread-1"}, &output, io.Discard); err != nil {
		t.Fatalf("runInspect: %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte("\"jsonrpc\"")) || bytes.Contains(output.Bytes(), []byte("\"params\"")) {
		t.Fatalf("inspect emitted a raw JSON-RPC frame: %s", output.String())
	}
	for _, field := range [][]byte{
		[]byte(`"response_id": "response-1"`),
		[]byte(`"response_to": "item/commandExecution/requestApproval"`),
		[]byte(`"resolves_request_id": "permission-9"`),
		[]byte(`"item_type": "subAgentActivity"`),
	} {
		if !bytes.Contains(output.Bytes(), field) {
			t.Errorf("inspect output is missing correlation field %s: %s", field, output.String())
		}
	}
	if got := bytes.Count(output.Bytes(), []byte(`"response_id": "response-1"`)); got != 2 {
		t.Errorf("inspect returned %d observations for replayed response ID, want 2", got)
	}
	if bytes.Contains(output.Bytes(), []byte("secret-command")) {
		t.Fatalf("inspect emitted the raw approval request payload: %s", output.String())
	}
}

func TestInspectRequiresCaptureFile(t *testing.T) {
	err := runInspect(nil, io.Discard, io.Discard)
	if err == nil || err.Error() != "--file is required" {
		t.Fatalf("runInspect error = %v", err)
	}
}
