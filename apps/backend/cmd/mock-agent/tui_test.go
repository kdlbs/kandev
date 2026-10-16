package main

import (
	"context"
	"encoding/json"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParsePromptFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flag returns empty",
			args: []string{"mock-agent"},
			want: "",
		},
		{
			name: "separate flag and value",
			args: []string{"mock-agent", "--prompt", "fix the bug"},
			want: "fix the bug",
		},
		{
			name: "equals syntax",
			args: []string{"mock-agent", "--prompt=fix the bug"},
			want: "fix the bug",
		},
		{
			name: "prompt as last arg without value",
			args: []string{"mock-agent", "--prompt"},
			want: "",
		},
		{
			name: "prompt with other flags before",
			args: []string{"mock-agent", "--model", "mock-fast", "--prompt", "hello world"},
			want: "hello world",
		},
		{
			name: "prompt with other flags after",
			args: []string{"mock-agent", "--prompt", "hello world", "--tui"},
			want: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePromptFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("parsePromptFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestTuiDelay(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"mock-fast", 100},
		{"mock-slow", 2000},
		{"mock-default", 500},
		{"unknown", 500},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := tuiDelay(tt.model)
			if got != tt.want {
				t.Errorf("tuiDelay(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

func TestTUITraceRecordsRemoteProcessAndPrompt(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	priorArgs, priorInput := os.Args, os.Stdin
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Args, os.Stdin = priorArgs, priorInput; _ = input.Close() })
	os.Args = []string{"mock-agent", "--tui", "--tui-trace"}
	os.Stdin = input
	runTUI("mock-fast", "trace-one", false)
	data, err := os.ReadFile(filepath.Join(root, ".kandev", "e2e-passthrough.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("trace must contain one start and one prompt, got %s", data)
	}
	var start struct {
		Kind string `json:"kind"`
		PID  int    `json:"pid"`
		CWD  string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &start); err != nil {
		t.Fatal(err)
	}
	if start.Kind != "start" || start.PID != os.Getpid() || start.CWD != root {
		t.Fatalf("incorrect process identity: %+v", start)
	}
	var prompt struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &prompt); err != nil {
		t.Fatal(err)
	}
	if prompt.Prompt != "trace-one" {
		t.Fatalf("prompt: %+v", prompt)
	}
}

func TestTUIToolCallUsesPassthroughMCPConfig(t *testing.T) {
	var calls atomic.Int32
	mcpServer := server.NewMCPServer("TUI MCP fixture", "1.0.0")
	mcpServer.AddTool(mcp.NewTool("list_task_sessions_kandev", mcp.WithString("task_id", mcp.Required())), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		if request.GetString("task_id", "") != "task-remote" {
			t.Errorf("unexpected task: %v", request.GetArguments())
		}
		return mcp.NewToolResultText(`{"sessions":[{"id":"session-remote","is_current":true}]}`), nil
	})
	transport := server.NewTestStreamableHTTPServer(mcpServer)
	t.Cleanup(transport.Close)
	path := filepath.Join(t.TempDir(), "mcp.json")
	data, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"kandev": map[string]any{"type": "http", "url": transport.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	oldArgs := os.Args
	os.Args = []string{"mock-agent", "--tui", "--mcp-config", path}
	t.Cleanup(func() { os.Args = oldArgs })
	t.Setenv("KANDEV_TASK_ID", "task-remote")
	processTUIPrompt("/e2e:mcp-context", "mock-fast")
	if calls.Load() != 1 {
		t.Fatalf("remote MCP calls: %d, want 1", calls.Load())
	}
}
