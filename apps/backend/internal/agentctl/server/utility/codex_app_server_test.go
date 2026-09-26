package utility

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestResolveCodexAppServerCommandAllowList(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		want    []string
		ok      bool
	}{
		{
			name:    "managed npm runtime",
			command: []string{"npx", "--yes", "--prefer-offline", "@openai/codex@0.154.0", "app-server"},
			want:    []string{"--yes", "--prefer-offline", "@openai/codex@0.154.0", "app-server"},
			ok:      true,
		},
		{name: "native command", command: []string{"codex", "app-server"}, want: []string{"app-server"}, ok: true},
		{name: "other package", command: []string{"npx", "--yes", "--prefer-offline", "example/other@1.2.3", "app-server"}},
		{name: "unexpected arguments", command: []string{"npx", "--yes", "@openai/codex@0.154.0", "app-server", "--danger"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, args, err := resolveCodexAppServerCommand(&InferenceConfigDTO{Command: tt.command})
			if tt.ok {
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(args, tt.want) {
					t.Fatalf("args = %#v, want %#v", args, tt.want)
				}
				if (tt.name == "native command") != (command == "codex") {
					t.Fatalf("command = %q", command)
				}
				return
			}
			if err == nil {
				t.Fatal("expected command to be rejected")
			}
		})
	}
}

// @covers AC-AGENTS-CODEX-NATIVE-001.1, AC-AGENTS-CODEX-NATIVE-002.1
func TestResolveCodexAppServerCommandAcceptsGeneratedManagedCommands(t *testing.T) {
	appServer := agents.NewCodexAppServer(true)
	tests := []struct {
		name    string
		command []string
	}{
		{name: "default version", command: appServer.InferenceConfig().Command.Args()},
		{name: "selected exact version", command: appServer.BuildCommand(agents.CommandOptions{ManagedRuntimeVersion: "0.155.1"}).Args()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCommand, gotArgs, err := resolveCodexAppServerCommand(&InferenceConfigDTO{Command: tt.command})
			if err != nil {
				t.Fatalf("generated command rejected: %v; command=%#v", err, tt.command)
			}
			if gotCommand != "npx" {
				t.Fatalf("executable = %q, want npx", gotCommand)
			}
			if !slices.Equal(gotArgs, tt.command[1:]) {
				t.Fatalf("arguments = %#v, want %#v", gotArgs, tt.command[1:])
			}
		})
	}
}

func TestResolveCodexAppServerCommandRejectsUntrustedManagedPrefixes(t *testing.T) {
	base := []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, "@openai/codex@0.154.0", "app-server"}
	tests := []struct {
		name   string
		mutate func([]string) []string
	}{
		{name: "arbitrary prefix", mutate: func(command []string) []string {
			command[4] = "/tmp/untrusted"
			return command
		}},
		{name: "extra argument", mutate: func(command []string) []string {
			return append(command, "--danger")
		}},
		{name: "wrapper executable", mutate: func(command []string) []string {
			command[0] = "sh"
			return command
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.mutate(slices.Clone(base))
			if _, _, err := resolveCodexAppServerCommand(&InferenceConfigDTO{Command: command}); err == nil {
				t.Fatalf("command unexpectedly accepted: %#v", command)
			}
		})
	}
}

func TestCodexUtilityMCPConfigKeepsHTTPHeaders(t *testing.T) {
	config := codexUtilityMCPConfig([]MCPServerDTO{{
		Name: "kandev", Type: "http", URL: "http://localhost:4231/mcp",
		HeaderKVs: []HTTPHeaderDTO{{Name: "Authorization", Value: "Bearer test-token"}},
	}})
	servers, ok := config["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatal("MCP server config is missing")
	}
	server, ok := servers["kandev"].(map[string]any)
	if !ok || server["url"] != "http://localhost:4231/mcp" {
		t.Fatalf("MCP server config = %#v", servers["kandev"])
	}
	headers, ok := server["http_headers"].(map[string]string)
	if !ok || headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("MCP headers = %#v", server["http_headers"])
	}
}
