package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACPDBGProtocolVersion(t *testing.T) {
	for _, tc := range []struct {
		name, operation, want string
		args                  []string
	}{
		{name: "v1 default", operation: "probe", want: "protocol_version: 1"},
		{name: "v2 selected", operation: "probe", want: "protocol_version: 2"},
		{name: "legacy envelope", operation: "probe", want: "legacy v1 envelope"},
		{name: "v1 selected", operation: "probe", want: "selected ACP v1"},
		{name: "ack before running", operation: "prompt", args: []string{"--prompt", "example", "--model", "selected"}, want: "fixture answer"},
		{name: "ack after completion", operation: "prompt", args: []string{"--prompt", "example"}, want: "fixture answer"},
		{name: "replay", operation: "session-load", args: []string{"--session-id", "saved", "--prompt", "example"}, want: "fixture answer"},
		{name: "MCP", operation: "mcp-probe", want: "initialize_observed: true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ACPDBG_WIRE_CASE", tc.name)
			path := filepath.Join(t.TempDir(), "capture.jsonl")
			args := []string{tc.operation, "--file", path, "--exec", cliPeerCommand(t), "--timeout", "5s"}
			if tc.name != "v1 default" {
				args = append(args, "--protocol-version", "2")
			}
			stdout, stderr, err := runCLIHelper(t, append(args, tc.args...))
			wantError := tc.name == "legacy envelope" || tc.name == "v1 selected"
			if (err != nil) != wantError || !bytes.Contains(append(stdout, stderr...), []byte(tc.want)) {
				t.Fatalf("exit = %v, stdout = %s, stderr = %s", err, stdout, stderr)
			}
			capture, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if wantError && bytes.Contains(capture, []byte(`"method":"session/new"`)) {
				t.Fatal("incompatible peer received a session request")
			}
			if tc.name == "replay" && (!bytes.Contains(capture, []byte(`"event":"replay_complete"`)) ||
				bytes.Contains(stdout, []byte("replayed answer"))) {
				t.Fatalf("missing replay boundary or replay leaked into prompt output: %s", stdout)
			}
		})
	}
}

func serveWirePeer(t *testing.T, scenario string) {
	t.Helper()
	peer := wirePeer{t: t, enc: json.NewEncoder(os.Stdout), scenario: scenario}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			t.Fatal(err)
		}
		params, _ := req["params"].(map[string]any)
		switch req["method"] {
		case "initialize":
			peer.initialize(req["id"], params)
		case "session/new":
			peer.newSession(req["id"], params)
		case "session/resume":
			if replay, _ := params["replayFrom"].(map[string]any); params["sessionId"] != "saved" || replay["type"] != "start" {
				t.Fatalf("wrong resume params: %v", params)
			}
			peer.update("saved", map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": "old", "content": map[string]any{"type": "text", "text": "replayed answer"}})
			peer.reply(req["id"], map[string]any{"sessionId": "saved"})
			peer.update("saved", map[string]any{"sessionUpdate": "replay_complete"})
		case "session/set_config_option":
			if params["configId"] != "model-id" || params["type"] != "id" || params["value"] != "selected" {
				t.Fatalf("wrong config params: %v", params)
			}
			peer.reply(req["id"], map[string]any{})
		case "session/prompt":
			peer.prompt(req["id"], params)
		default:
			t.Fatalf("unexpected method in protocol fixture: %v", req["method"])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

type wirePeer struct {
	t        *testing.T
	enc      *json.Encoder
	scenario string
}

func (p wirePeer) initialize(id any, params map[string]any) {
	if p.scenario == "v1 default" {
		if params["protocolVersion"] != float64(1) || params["clientInfo"] == nil {
			p.t.Fatalf("wrong v1 initialize params: %v", params)
		}
		p.reply(id, map[string]any{"protocolVersion": 1})
		return
	}
	if params["protocolVersion"] != float64(2) || params["info"] == nil || params["clientInfo"] != nil {
		p.t.Fatalf("wrong v2 initialize params: %v", params)
	}
	info := map[string]any{"name": "wire-fixture", "version": "1.0"}
	switch p.scenario {
	case "legacy envelope":
		p.reply(id, map[string]any{"protocolVersion": 2, "agentInfo": info})
	case "v1 selected":
		p.reply(id, map[string]any{"protocolVersion": 1, "agentInfo": info})
	default:
		p.reply(id, map[string]any{"protocolVersion": 2, "info": info, "capabilities": map[string]any{
			"session": map[string]any{"mcp": map[string]any{"http": map[string]any{}}},
		}})
	}
}

func (p wirePeer) newSession(id any, params map[string]any) {
	if p.scenario == "MCP" {
		servers, _ := params["mcpServers"].([]any)
		if len(servers) != 1 {
			p.t.Fatalf("expected one MCP server: %v", params)
		}
		server, _ := servers[0].(map[string]any)
		url, _ := server["url"].(string)
		client := http.Client{}
		resp, err := client.Post(url, "application/json", strings.NewReader(
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`))
		if err != nil {
			p.t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			p.t.Error(err)
		}
		if err := resp.Body.Close(); err != nil {
			p.t.Error(err)
		}
	}
	p.reply(id, map[string]any{"sessionId": "fresh", "configOptions": []any{map[string]any{
		"configId": "model-id", "type": "select", "category": "model", "currentValue": "default",
		"options": []any{map[string]any{"value": "selected", "name": "Selected"}},
	}}})
}

func (p wirePeer) prompt(id any, params map[string]any) {
	session, _ := params["sessionId"].(string)
	if session == "" || params["idempotencyKey"] == nil {
		p.t.Fatalf("wrong prompt params: %v", params)
	}
	if p.scenario != "ack after completion" {
		p.reply(id, map[string]any{"messageId": "user-1"})
	}
	p.update(session, map[string]any{"sessionUpdate": "state_update", "state": "running"})
	p.update("unrelated", map[string]any{"sessionUpdate": "state_update", "state": "idle", "stopReason": "error"})
	p.update(session, map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": "answer", "content": map[string]any{"type": "text", "text": "fixture answer"}})
	p.update(session, map[string]any{"sessionUpdate": "state_update", "state": "idle", "stopReason": "end_turn"})
	if p.scenario == "ack after completion" {
		p.reply(id, map[string]any{"messageId": "user-1"})
	}
}

func (p wirePeer) reply(id any, result map[string]any) {
	p.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (p wirePeer) update(session string, update map[string]any) {
	p.write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": session, "update": update}})
}

func (p wirePeer) write(frame map[string]any) {
	if err := p.enc.Encode(frame); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
