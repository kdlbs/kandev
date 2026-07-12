package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// mockPassthroughProfileResolver is a mock for testing passthrough verification
type mockPassthroughProfileResolver struct {
	cliPassthrough bool
	envVars        []settingsmodels.ProfileEnvVar
	err            error
	agentName      string
}

func (m *mockPassthroughProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &AgentProfileInfo{
		ProfileID:      profileID,
		AgentName:      m.agentName,
		CLIPassthrough: m.cliPassthrough,
		EnvVars:        m.envVars,
	}, nil
}

func assertClaudePassthroughMCPConfig(t *testing.T, cmd agents.Command, wantURL string) {
	t.Helper()

	args := cmd.Args()
	for i, arg := range args {
		if arg != "--mcp-config" {
			continue
		}
		if i+1 >= len(args) {
			t.Fatalf("--mcp-config missing path in command %v", args)
		}
		if i != len(args)-2 {
			t.Fatalf("--mcp-config must be final flag pair for claude variadic parsing: %v", args)
		}
		var payload struct {
			MCPServers map[string]struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"mcpServers"`
		}
		data, err := os.ReadFile(args[i+1])
		if err != nil {
			t.Fatalf("read generated MCP config: %v", err)
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("generated MCP config is not JSON: %v\n%s", err, data)
		}
		kandev := payload.MCPServers["kandev"]
		if kandev.Type != "http" {
			t.Fatalf("kandev MCP type = %q, want http", kandev.Type)
		}
		if kandev.URL != wantURL {
			t.Fatalf("kandev MCP url = %q, want %q", kandev.URL, wantURL)
		}
		return
	}
	t.Fatalf("passthrough command missing --mcp-config: %v", args)
}

func newPassthroughMCPTestManager(t *testing.T, agentName string) (*Manager, *AgentExecution, *AgentProfileInfo) {
	t.Helper()

	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	mgr.profileResolver = &mockPassthroughProfileResolver{
		agentName:      agentName,
		cliPassthrough: true,
	}
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		Metadata: map[string]interface{}{
			"standalone_port": 45678,
		},
		standalonePort: 45678,
	}
	profile := &AgentProfileInfo{
		ProfileID:      "profile-1",
		AgentName:      agentName,
		Model:          "default",
		CLIPassthrough: true,
	}
	return mgr, execution, profile
}

func newClaudePassthroughMCPTestManager(t *testing.T) (*Manager, *AgentExecution, *AgentProfileInfo) {
	t.Helper()
	return newPassthroughMCPTestManager(t, "claude-acp")
}

func TestBuildPassthroughCommand(t *testing.T) {
	tests := []struct {
		name    string
		agent   agents.PassthroughAgent
		opts    agents.PassthroughOptions
		wantCmd []string
	}{
		{
			name: "basic command without options",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli", "--verbose")},
				},
			},
			opts:    agents.PassthroughOptions{Resume: true},
			wantCmd: []string{"test-cli", "--verbose"},
		},
		{
			name: "command with model",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli"), ModelFlag: agents.NewParam("--model", "{model}")},
				},
			},
			opts:    agents.PassthroughOptions{Model: "gpt-4"},
			wantCmd: []string{"test-cli", "--model", "gpt-4"},
		},
		{
			name: "resume with single-word flag",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli"), ResumeFlag: agents.NewParam("-c")},
				},
			},
			opts:    agents.PassthroughOptions{Resume: true},
			wantCmd: []string{"test-cli", "-c"},
		},
		{
			name: "resume with multi-word flag",
			agent: &testAgent{
				id: "gemini-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("gemini"), ResumeFlag: agents.NewParam("--resume", "latest")},
				},
			},
			opts:    agents.PassthroughOptions{Resume: true},
			wantCmd: []string{"gemini", "--resume", "latest"},
		},
		{
			name: "permission settings as CLI flags",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli")},
					PermSettings: map[string]agents.PermissionSetting{
						"auto_approve": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--yes"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				PermissionValues: map[string]bool{"auto_approve": true},
			},
			wantCmd: []string{"test-cli", "--yes"},
		},
		{
			name: "full resume with model + settings + resume flag",
			agent: &testAgent{
				id: "claude-code",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("npx", "-y", "@anthropic-ai/claude-code"),
						ModelFlag:      agents.NewParam("--model", "{model}"),
						ResumeFlag:     agents.NewParam("-c"),
					},
					PermSettings: map[string]agents.PermissionSetting{
						"dangerously_skip_permissions": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--dangerously-skip-permissions"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				Model:            "claude-sonnet-4",
				Resume:           true,
				PermissionValues: map[string]bool{"dangerously_skip_permissions": true},
			},
			wantCmd: []string{"npx", "-y", "@anthropic-ai/claude-code", "--model", "claude-sonnet-4", "--dangerously-skip-permissions", "-c"},
		},
		{
			name: "permission setting with cli_flag_value",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli")},
					PermSettings: map[string]agents.PermissionSetting{
						"auto_approve": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--approve-level", CLIFlagValue: "all"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				PermissionValues: map[string]bool{"auto_approve": true},
			},
			wantCmd: []string{"test-cli", "--approve-level", "all"},
		},
		{
			name: "new session with prompt (positional)",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli")},
				},
			},
			opts:    agents.PassthroughOptions{Prompt: "fix the bug"},
			wantCmd: []string{"test-cli", "fix the bug"},
		},
		{
			name: "new session with prompt flag",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{Supported: true, PassthroughCmd: agents.NewCommand("test-cli"), PromptFlag: agents.NewParam("--prompt", "{prompt}")},
				},
			},
			opts:    agents.PassthroughOptions{Prompt: "fix the bug"},
			wantCmd: []string{"test-cli", "--prompt", "fix the bug"},
		},
		{
			name: "resume with session ID",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:         true,
						PassthroughCmd:    agents.NewCommand("test-cli"),
						SessionResumeFlag: agents.NewParam("--resume"),
					},
				},
			},
			opts:    agents.PassthroughOptions{SessionID: "sess-123"},
			wantCmd: []string{"test-cli", "--resume", "sess-123"},
		},
		{
			name: "mock agent resume with -c flag",
			agent: &testAgent{
				id: "mock-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("mock-agent", "--tui"),
						ModelFlag:      agents.NewParam("--model", "{model}"),
						ResumeFlag:     agents.NewParam("-c"),
					},
				},
			},
			opts:    agents.PassthroughOptions{Model: "mock-fast", Resume: true},
			wantCmd: []string{"mock-agent", "--tui", "--model", "mock-fast", "-c"},
		},
		{
			name: "mock agent session resume with --resume flag",
			agent: &testAgent{
				id: "mock-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:         true,
						PassthroughCmd:    agents.NewCommand("mock-agent", "--tui"),
						ModelFlag:         agents.NewParam("--model", "{model}"),
						SessionResumeFlag: agents.NewParam("--resume"),
					},
				},
			},
			opts:    agents.PassthroughOptions{Model: "mock-fast", SessionID: "sess-123"},
			wantCmd: []string{"mock-agent", "--tui", "--model", "mock-fast", "--resume", "sess-123"},
		},
		{
			name: "user cli flag tokens appended after model + settings",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("test-cli"),
						ModelFlag:      agents.NewParam("--model", "{model}"),
						ResumeFlag:     agents.NewParam("-c"),
					},
					PermSettings: map[string]agents.PermissionSetting{
						"auto_approve": {Supported: true, ApplyMethod: "cli_flag", CLIFlag: "--yes"},
					},
				},
			},
			opts: agents.PassthroughOptions{
				Model:            "gpt-4",
				Resume:           true,
				PermissionValues: map[string]bool{"auto_approve": true},
				CLIFlagTokens:    []string{"--debug", "--log-level", "trace"},
			},
			wantCmd: []string{"test-cli", "--model", "gpt-4", "--yes", "--debug", "--log-level", "trace", "-c"},
		},
		{
			name: "mcp args go after positional prompt because claude treats --mcp-config as variadic",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("test-cli"),
					},
				},
			},
			opts: agents.PassthroughOptions{
				Prompt:  "fix the bug",
				MCPArgs: []string{"--mcp-config", "/tmp/kandev-mcp.json"},
			},
			wantCmd: []string{"test-cli", "fix the bug", "--mcp-config", "/tmp/kandev-mcp.json"},
		},
		{
			name: "mcp args are appended after the resume flag",
			agent: &testAgent{
				id: "test-agent",
				StandardPassthrough: agents.StandardPassthrough{
					Cfg: agents.PassthroughConfig{
						Supported:      true,
						PassthroughCmd: agents.NewCommand("test-cli"),
						ResumeFlag:     agents.NewParam("--resume"),
					},
				},
			},
			opts: agents.PassthroughOptions{
				Resume:  true,
				MCPArgs: []string{"-c", `mcp_servers.kandev.url="http://x/mcp"`},
			},
			wantCmd: []string{"test-cli", "--resume", "-c", `mcp_servers.kandev.url="http://x/mcp"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.BuildPassthroughCommand(tt.opts).Args()

			if len(got) != len(tt.wantCmd) {
				t.Errorf("BuildPassthroughCommand() = %v, want %v", got, tt.wantCmd)
				return
			}

			for i, arg := range got {
				if arg != tt.wantCmd[i] {
					t.Errorf("BuildPassthroughCommand()[%d] = %q, want %q", i, arg, tt.wantCmd[i])
				}
			}
		})
	}
}

func TestPassthroughAgentCommandInjectsKandevMCPConfig(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)

	_, _, _, cmd, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	assertClaudePassthroughMCPConfig(t, cmd, "http://localhost:45678/mcp")
}

func TestFreshPassthroughCommandInjectsKandevMCPConfig(t *testing.T) {
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)

	_, _, cmd, err := mgr.freshPassthroughCommand(context.Background(), execution)
	if err != nil {
		t.Fatalf("freshPassthroughCommand returned error: %v", err)
	}

	assertClaudePassthroughMCPConfig(t, cmd, "http://localhost:45678/mcp")
}

func TestResumePassthroughCommandInjectsKandevMCPConfig(t *testing.T) {
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)

	resolved, err := mgr.resolvePassthroughAgent(context.Background(), execution)
	if err != nil {
		t.Fatalf("resolvePassthroughAgent returned error: %v", err)
	}
	cmd, err := mgr.resumePassthroughCommand(context.Background(), execution, resolved, true)
	if err != nil {
		t.Fatalf("resumePassthroughCommand returned error: %v", err)
	}

	assertClaudePassthroughMCPConfig(t, cmd, "http://localhost:45678/mcp")
}

func TestPassthroughCodexInjectsKandevMCPArgs(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "codex-acp")

	_, _, _, cmd, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	joined := strings.Join(cmd.Args(), " ")
	if want := `-c mcp_servers.kandev.url="http://localhost:45678/mcp"`; !strings.Contains(joined, want) {
		t.Fatalf("codex command missing kandev override %q in: %v", want, cmd.Args())
	}
	// Codex injects via CLI flags only — it must not write any config file.
	if files := getPassthroughMCPFiles(execution); len(files) != 0 {
		t.Fatalf("codex must not write MCP config files, got %v", files)
	}
}

func TestPassthroughOpenCodeInjectsConfigEnv(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "opencode-acp")

	_, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	files := getPassthroughMCPFiles(execution)
	if len(files) != 1 {
		t.Fatalf("expected one opencode config file, got %v", files)
	}
	if _, err := os.Stat(files[0]); err != nil {
		t.Fatalf("opencode config not written: %v", err)
	}
	// OPENCODE_CONFIG must be merged into the passthrough environment.
	env := mgr.buildPassthroughEnv(context.Background(), execution, nil)
	if env["OPENCODE_CONFIG"] != files[0] {
		t.Fatalf("OPENCODE_CONFIG = %q, want %q", env["OPENCODE_CONFIG"], files[0])
	}
}

func TestPassthroughCursorWritesProjectFile(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "cursor-acp")
	cursorPath := filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json")

	_, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("cursor mcp.json not written: %v", err)
	}
	if !strings.Contains(string(data), kandevMCPServerName) {
		t.Fatalf("cursor mcp.json missing kandev server: %s", data)
	}
}

func TestPassthroughCursorMergesIntoExistingProjectFile(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "cursor-acp")
	cursorPath := filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// A user file with their own top-level key AND an existing MCP server.
	userContent := `{"user":"config","mcpServers":{"user-srv":{"url":"https://user"}}}`
	if err := os.WriteFile(cursorPath, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile); err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read cursor mcp.json: %v", err)
	}
	var merged struct {
		User       string `json:"user"`
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatalf("merged file not JSON: %v\n%s", err, data)
	}
	if merged.User != "config" {
		t.Errorf("user top-level key lost: %s", data)
	}
	if merged.MCPServers["user-srv"].URL != "https://user" {
		t.Errorf("user's existing server dropped: %s", data)
	}
	// Cursor emits remote servers as {url, headers} with no type field.
	if merged.MCPServers[kandevMCPServerName].URL != "http://localhost:45678/mcp" {
		t.Errorf("kandev server not merged in: %s", data)
	}
	// A file merged into the user's must NOT be tracked for cleanup (it's theirs).
	if files := getPassthroughMCPFiles(execution); len(files) != 0 {
		t.Fatalf("merged user file must not be tracked for cleanup, got %v", files)
	}
}

func TestPassthroughPiWritesProjectFile(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "pi-acp")
	piPath := filepath.Join(execution.WorkspacePath, ".pi", "mcp.json")

	if _, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile); err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	data, err := os.ReadFile(piPath)
	if err != nil {
		t.Fatalf("pi mcp.json not written: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			Transport string `json:"transport"`
			URL       string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("pi mcp.json not valid JSON: %v\n%s", err, data)
	}
	kandev := payload.MCPServers[kandevMCPServerName]
	if kandev.Transport != "streamable-http" {
		t.Fatalf("kandev transport = %q, want streamable-http", kandev.Transport)
	}
	if kandev.URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev URL = %q", kandev.URL)
	}
}

func TestPassthroughPiMergesIntoExistingProjectFile(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "pi-acp")
	piPath := filepath.Join(execution.WorkspacePath, ".pi", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(piPath), 0o755); err != nil {
		t.Fatal(err)
	}
	userContent := `{"settings":{"toolPrefix":"mcp"},"mcpServers":{"user-srv":{"command":"user-tool"}}}`
	if err := os.WriteFile(piPath, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile); err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	data, err := os.ReadFile(piPath)
	if err != nil {
		t.Fatalf("read pi mcp.json: %v", err)
	}
	var merged struct {
		Settings   map[string]string `json:"settings"`
		MCPServers map[string]struct {
			Command   string `json:"command"`
			Transport string `json:"transport"`
			URL       string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatalf("merged file not JSON: %v\n%s", err, data)
	}
	if merged.Settings["toolPrefix"] != "mcp" {
		t.Fatalf("existing settings not preserved: %+v", merged.Settings)
	}
	if merged.MCPServers["user-srv"].Command != "user-tool" {
		t.Fatalf("existing server not preserved: %+v", merged.MCPServers)
	}
	if merged.MCPServers[kandevMCPServerName].URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev server not merged: %+v", merged.MCPServers)
	}
}

func TestGetPassthroughMCPFilesDecodesRestartShapes(t *testing.T) {
	// After a backend restart, Metadata is rehydrated from JSON, so a []string
	// becomes []interface{} of strings. The reader must tolerate both shapes.
	t.Run("in-memory []string", func(t *testing.T) {
		exec := &AgentExecution{Metadata: map[string]interface{}{
			metadataKeyPassthroughMCPFiles: []string{"/a.json", "/b.json"},
		}}
		got := getPassthroughMCPFiles(exec)
		if len(got) != 2 || got[0] != "/a.json" || got[1] != "/b.json" {
			t.Fatalf("got %v, want [/a.json /b.json]", got)
		}
	})
	t.Run("JSON-decoded []interface{}", func(t *testing.T) {
		exec := &AgentExecution{Metadata: map[string]interface{}{
			metadataKeyPassthroughMCPFiles: []interface{}{"/a.json", 42, "/b.json"},
		}}
		got := getPassthroughMCPFiles(exec)
		// Non-string entries are dropped, not panicked on.
		if len(got) != 2 || got[0] != "/a.json" || got[1] != "/b.json" {
			t.Fatalf("got %v, want [/a.json /b.json]", got)
		}
	})
	t.Run("nil metadata", func(t *testing.T) {
		if got := getPassthroughMCPFiles(&AgentExecution{}); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}

func TestGetPassthroughMCPEnvDecodesRestartShapes(t *testing.T) {
	t.Run("in-memory map[string]string", func(t *testing.T) {
		exec := &AgentExecution{Metadata: map[string]interface{}{
			metadataKeyPassthroughMCPEnv: map[string]string{"OPENCODE_CONFIG": "/oc.json"},
		}}
		if got := getPassthroughMCPEnv(exec); got["OPENCODE_CONFIG"] != "/oc.json" {
			t.Fatalf("got %v, want OPENCODE_CONFIG=/oc.json", got)
		}
	})
	t.Run("JSON-decoded map[string]interface{}", func(t *testing.T) {
		exec := &AgentExecution{Metadata: map[string]interface{}{
			metadataKeyPassthroughMCPEnv: map[string]interface{}{"OPENCODE_CONFIG": "/oc.json", "BAD": 1},
		}}
		got := getPassthroughMCPEnv(exec)
		if got["OPENCODE_CONFIG"] != "/oc.json" {
			t.Fatalf("got %v, want OPENCODE_CONFIG=/oc.json", got)
		}
		if _, ok := got["BAD"]; ok {
			t.Fatalf("non-string env value must be dropped, got %v", got)
		}
	})
}

func TestWritePassthroughMCPFilesUnionTrackingOnRelaunch(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{Metadata: map[string]interface{}{}}
	path := filepath.Join(t.TempDir(), "cfg.json")
	file := mcpconfig.PassthroughConfigFile{Path: path, Content: []byte("{}\n")}

	// Two launches writing the same path must track it exactly once.
	if err := mgr.writePassthroughMCPFiles(exec, []mcpconfig.PassthroughConfigFile{file}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := mgr.writePassthroughMCPFiles(exec, []mcpconfig.PassthroughConfigFile{file}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if files := getPassthroughMCPFiles(exec); len(files) != 1 || files[0] != path {
		t.Fatalf("union tracking failed: got %v, want [%s]", files, path)
	}
}

type fakeMcpConfigProvider struct {
	config *mcpconfig.ProfileConfig
}

func (f *fakeMcpConfigProvider) GetConfigByProfileID(_ context.Context, _ string) (*mcpconfig.ProfileConfig, error) {
	return f.config, nil
}

// TestPassthroughMCPServersMergesProfileAndDropsKandevCollision verifies that
// profile-configured MCP servers are merged with kandev's own server, and that a
// profile server named "kandev" cannot shadow the real kandev tools server.
func TestPassthroughMCPServersMergesProfileAndDropsKandevCollision(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	mgr.mcpProvider = &fakeMcpConfigProvider{config: &mcpconfig.ProfileConfig{
		ProfileID: "profile-1",
		Enabled:   true,
		Servers: map[string]mcpconfig.ServerDef{
			"github": {Type: mcpconfig.ServerTypeStdio, Command: "npx", Args: []string{"-y", "gh"}},
			"kandev": {Type: mcpconfig.ServerTypeStdio, Command: "evil-shadow"},
		},
	}}
	// The default policy for an unknown runtime denies all transports; allow
	// stdio so the profile servers survive resolution.
	execution.Metadata["executor_mcp_policy"] = `{"allow_stdio":true}`

	if _, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile); err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}

	files := getPassthroughMCPFiles(execution)
	if len(files) != 1 {
		t.Fatalf("expected one MCP config file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read MCP config: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			URL     string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("MCP config not JSON: %v\n%s", err, data)
	}
	// kandev's own HTTP server must win over the profile's "kandev" stdio entry.
	kandev := payload.MCPServers["kandev"]
	if kandev.Type != "http" || kandev.URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev entry = %+v, want our http tools server", kandev)
	}
	if kandev.Command == "evil-shadow" {
		t.Fatal("profile 'kandev' server shadowed the real kandev server")
	}
	// The profile's own server must be merged in.
	gh := payload.MCPServers["github"]
	if gh.Type != "stdio" || gh.Command != "npx" {
		t.Fatalf("github entry = %+v, want merged stdio server", gh)
	}
}

func TestWorkspacePathEscapesViaSymlink(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()

	// A clean path inside the workspace does not escape.
	if escaped, err := workspacePathEscapes(ws, filepath.Join(ws, ".cursor", "mcp.json")); err != nil || escaped {
		t.Fatalf("clean workspace path: escaped=%v err=%v, want false/nil", escaped, err)
	}
	// A file outside the workspace (kandev's temp dir) is exempt from the check.
	if escaped, err := workspacePathEscapes(ws, filepath.Join(outside, "cfg.json")); err != nil || escaped {
		t.Fatalf("temp path: escaped=%v err=%v, want false/nil", escaped, err)
	}
	// A symlinked `.cursor` pointing outside the workspace escapes.
	if err := os.Symlink(outside, filepath.Join(ws, ".cursor")); err != nil {
		t.Fatal(err)
	}
	if escaped, err := workspacePathEscapes(ws, filepath.Join(ws, ".cursor", "mcp.json")); err != nil || !escaped {
		t.Fatalf("symlinked path: escaped=%v err=%v, want true/nil", escaped, err)
	}
}

func TestRedactPassthroughArgsRedactsCodexSecrets(t *testing.T) {
	args := []string{
		"npx", "-y", "@openai/codex",
		"-c", `mcp_servers.gh.command="npx"`,
		"-c", `mcp_servers.gh.env={"GITHUB_TOKEN":"s3cret"}`,
		"-c", `mcp_servers.remote.http_headers={"Authorization":"Bearer tok"}`,
		"-c", `mcp_servers.remote.url="http://x/mcp"`,
		"--mcp-config", "/tmp/c.json",
	}
	joined := strings.Join(redactPassthroughArgs(args), " ")

	if strings.Contains(joined, "s3cret") || strings.Contains(joined, "Bearer tok") {
		t.Fatalf("secrets not redacted: %s", joined)
	}
	if !strings.Contains(joined, "mcp_servers.gh.env=<redacted>") ||
		!strings.Contains(joined, "mcp_servers.remote.http_headers=<redacted>") {
		t.Errorf("expected redaction markers: %s", joined)
	}
	// Non-secret tokens must be preserved verbatim.
	for _, want := range []string{`mcp_servers.gh.command="npx"`, `mcp_servers.remote.url="http://x/mcp"`, "--mcp-config", "/tmp/c.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("redaction dropped non-secret %q: %s", want, joined)
		}
	}
}

func TestWritePassthroughMCPFilesSkipsDanglingLeafSymlink(t *testing.T) {
	mgr := newTestManager(t)
	ws := t.TempDir()
	outside := t.TempDir()
	outsideTarget := filepath.Join(outside, "target.json") // intentionally never created
	cursorDir := filepath.Join(ws, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(cursorDir, "mcp.json")
	if err := os.Symlink(outsideTarget, leaf); err != nil {
		t.Fatal(err)
	}

	execution := &AgentExecution{WorkspacePath: ws, Metadata: map[string]interface{}{}}
	if err := mgr.writePassthroughMCPFiles(execution, []mcpconfig.PassthroughConfigFile{
		{Path: leaf, Content: []byte(`{"mcpServers":{}}`), MergeKey: "mcpServers"},
	}); err != nil {
		t.Fatalf("writePassthroughMCPFiles: %v", err)
	}

	// The write must NOT have followed the symlink to create the outside file
	// (neither the merge-read nor the write may traverse it).
	if _, err := os.Stat(outsideTarget); !os.IsNotExist(err) {
		t.Fatalf("write followed dangling symlink to outside target (err=%v)", err)
	}
	// A skipped symlink must not be tracked for cleanup.
	if files := getPassthroughMCPFiles(execution); len(files) != 0 {
		t.Fatalf("skipped symlink leaf must not be tracked, got %v", files)
	}
}

func TestResumePassthroughSessionWithoutRunnerDoesNotWriteMCPConfig(t *testing.T) {
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)
	mgr.executorRegistry = nil
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	err := mgr.ResumePassthroughSession(context.Background(), execution.SessionID)
	if err == nil {
		t.Fatal("ResumePassthroughSession returned nil, want missing runner error")
	}
	if files := getPassthroughMCPFiles(execution); len(files) != 0 {
		t.Fatalf("passthrough MCP config files = %v, want none", files)
	}
}

func TestPassthroughAgentCommandErrorsWhenMCPPortMissing(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	execution.standalonePort = 0
	delete(execution.Metadata, "standalone_port")

	_, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err == nil {
		t.Fatal("passthroughAgentCommand returned nil, want missing MCP port error")
	}
	if err.Error() != "standalone port unavailable for passthrough MCP config" {
		t.Fatalf("error = %q, want missing MCP port error", err.Error())
	}
}

func TestFreshPassthroughCommandErrorsWhenMCPPortMissing(t *testing.T) {
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)
	execution.standalonePort = 0
	delete(execution.Metadata, "standalone_port")

	if _, _, _, err := mgr.freshPassthroughCommand(context.Background(), execution); err == nil {
		t.Fatal("freshPassthroughCommand returned nil, want missing MCP port error")
	} else if err.Error() != "standalone port unavailable for passthrough MCP config" {
		t.Fatalf("error = %q, want missing MCP port error", err.Error())
	}
}

func TestResumePassthroughCommandErrorsWhenMCPPortMissing(t *testing.T) {
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)
	execution.standalonePort = 0
	delete(execution.Metadata, "standalone_port")

	resolved, err := mgr.resolvePassthroughAgent(context.Background(), execution)
	if err != nil {
		t.Fatalf("resolvePassthroughAgent returned error: %v", err)
	}
	if _, err := mgr.resumePassthroughCommand(context.Background(), execution, resolved, true); err == nil {
		t.Fatal("resumePassthroughCommand returned nil, want missing MCP port error")
	} else if err.Error() != "standalone port unavailable for passthrough MCP config" {
		t.Fatalf("error = %q, want missing MCP port error", err.Error())
	}
}

func TestRemoveExecutionCleansPassthroughMCPConfig(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	execution.Metadata = nil
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	_, _, _, cmd, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}
	assertClaudePassthroughMCPConfig(t, cmd, "http://localhost:45678/mcp")
	files := getPassthroughMCPFiles(execution)
	if len(files) == 0 {
		t.Fatal("passthrough MCP config file was not stored")
	}
	path := files[0]

	mgr.RemoveExecution(execution.ID)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("generated MCP config still exists or stat failed: %v", err)
	}
	if files := getPassthroughMCPFiles(execution); len(files) != 0 {
		t.Fatalf("passthrough MCP config files metadata = %v, want empty", files)
	}
}

func TestWritePassthroughMCPConfigKeepsLiteralSessionFilename(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	execution.SessionID = "session"

	_, _, _, cmd, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)
	if err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}
	assertClaudePassthroughMCPConfig(t, cmd, "http://localhost:45678/mcp")
	files := getPassthroughMCPFiles(execution)
	want := filepath.Join(mgr.dataDir, "passthrough-mcp", "session.json")
	if len(files) != 1 || files[0] != want {
		t.Fatalf("passthrough MCP config files = %v, want [%s]", files, want)
	}
}

// TestManager_HandlePassthroughExit_SkipsDuringShutdown verifies that the
// detached goroutine spawned when a passthrough child exits bails out
// immediately once graceful shutdown has begun, instead of racing the
// teardown and logging a spurious "failed to auto-restart passthrough
// session" error. Regression test for the Ctrl+C-in-terminal shutdown
// noise.
//
// Uses testing/synctest so the assertion is "the function returned without
// any time advancing" — i.e. it short-circuited before the cleanupDelay
// sleep. Under fake time, a non-short-circuit path would advance by
// cleanupDelay (and then take the nil-runner branch in the test rig).
func TestManager_HandlePassthroughExit_SkipsDuringShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager(t)

		if mgr.IsShuttingDown() {
			t.Fatal("fresh manager reports IsShuttingDown() == true")
		}

		if err := mgr.StopAllAgents(context.Background()); err != nil {
			t.Fatalf("StopAllAgents returned error: %v", err)
		}
		if !mgr.IsShuttingDown() {
			t.Fatal("StopAllAgents did not set IsShuttingDown() = true")
		}

		execution := &AgentExecution{ID: "exec-1", SessionID: "sess-1"}
		status := &agentctltypes.ProcessStatusUpdate{SessionID: "sess-1"}

		start := time.Now()
		mgr.handlePassthroughExit(execution, status, start, false)
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("handlePassthroughExit advanced fake time by %v — did not short-circuit during shutdown", elapsed)
		}
	})
}

// TestManager_HandlePassthroughExit_ResumeFallback_SkipsDuringShutdown is the
// companion to TestManager_HandlePassthroughExit_SkipsDuringShutdown for the
// new fast-fail-with-resume branch — verifies that the resume-fallback path
// also short-circuits cleanly during graceful shutdown rather than racing the
// teardown.
func TestManager_HandlePassthroughExit_ResumeFallback_SkipsDuringShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager(t)
		if err := mgr.StopAllAgents(context.Background()); err != nil {
			t.Fatalf("StopAllAgents returned error: %v", err)
		}

		execution := &AgentExecution{ID: "exec-1", SessionID: "sess-1"}
		exitCode := 1
		now := time.Now()
		status := &agentctltypes.ProcessStatusUpdate{
			SessionID: "sess-1",
			ExitCode:  &exitCode,
			Timestamp: now.Add(100 * time.Millisecond),
		}

		start := time.Now()
		mgr.handlePassthroughExit(execution, status, now, true /* usedResume */)
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("handlePassthroughExit(usedResume=true) advanced fake time by %v — did not short-circuit during shutdown", elapsed)
		}
	})
}

// TestIsFastFailExit covers the predicate that decides whether a passthrough
// exit looks like a launch failure (bad CLI flag, missing binary, auth
// rejection) and should bypass the auto-restart loop.
func TestIsFastFailExit(t *testing.T) {
	const window = 2 * time.Second
	now := time.Now()

	tests := []struct {
		name      string
		startedAt time.Time
		exitedAt  time.Time
		exitCode  int
		want      bool
	}{
		{
			name:      "fast exit with non-zero code → fast-fail",
			startedAt: now,
			exitedAt:  now.Add(100 * time.Millisecond),
			exitCode:  1,
			want:      true,
		},
		{
			name:      "slow exit with non-zero code → restart",
			startedAt: now,
			exitedAt:  now.Add(5 * time.Second),
			exitCode:  1,
			want:      false,
		},
		{
			name:      "fast exit with zero code → not fast-fail (clean exit)",
			startedAt: now,
			exitedAt:  now.Add(100 * time.Millisecond),
			exitCode:  0,
			want:      false,
		},
		{
			name:      "zero start time → check disabled (recovered execution)",
			startedAt: time.Time{},
			exitedAt:  now,
			exitCode:  1,
			want:      false,
		},
		{
			name: "exit-time-based measurement is independent of caller delays",
			// Process actually ran 50ms; caller's wall-clock-since-start would
			// be much larger (e.g. after the cleanupDelay sleep), but the
			// status.Timestamp pins true uptime to 50ms → fast-fail.
			startedAt: now,
			exitedAt:  now.Add(50 * time.Millisecond),
			exitCode:  127,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFastFailExit(tt.startedAt, tt.exitedAt, tt.exitCode, window); got != tt.want {
				t.Errorf("isFastFailExit() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestManager_ProfileCLIFlagTokens confirms profile-configured cli_flags
// reach the passthrough launch path (regression for issue #718, where the
// passthrough builder silently dropped them).
func TestManager_ProfileCLIFlagTokens(t *testing.T) {
	mgr := newTestManager(t)

	t.Run("nil profile returns nil", func(t *testing.T) {
		if got := mgr.profileCLIFlagTokens(nil); got != nil {
			t.Errorf("profileCLIFlagTokens(nil) = %v, want nil", got)
		}
	})

	t.Run("enabled flags tokenised, disabled skipped", func(t *testing.T) {
		profile := &AgentProfileInfo{
			ProfileID: "p1",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: "--allow-all-tools", Enabled: true},
				{Flag: "--skip-me", Enabled: false},
				{Flag: "--add-dir /shared", Enabled: true},
			},
		}
		got := mgr.profileCLIFlagTokens(profile)
		want := []string{"--allow-all-tools", "--add-dir", "/shared"}
		if len(got) != len(want) {
			t.Fatalf("profileCLIFlagTokens() = %v, want %v", got, want)
		}
		for i, tok := range want {
			if got[i] != tok {
				t.Errorf("profileCLIFlagTokens()[%d] = %q, want %q", i, got[i], tok)
			}
		}
	})

	t.Run("malformed flag does not abort — returns nil and warns", func(t *testing.T) {
		profile := &AgentProfileInfo{
			ProfileID: "p2",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: `--broken "unterminated`, Enabled: true},
			},
		}
		if got := mgr.profileCLIFlagTokens(profile); got != nil {
			t.Errorf("profileCLIFlagTokens(malformed) = %v, want nil", got)
		}
	})
}

func TestBuildPassthroughEnv_MergesProfileEnvVars(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{
		envVars: []settingsmodels.ProfileEnvVar{
			{Key: "PLAIN", Value: "plain-value"},
			{Key: "KANDEV_SESSION_ID", Value: "profile-session"},
		},
	}

	env := mgr.buildPassthroughEnv(context.Background(), &AgentExecution{
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
	}, nil)

	if env["PLAIN"] != "plain-value" {
		t.Fatalf("profile env var missing: %+v", env)
	}
	if env["KANDEV_SESSION_ID"] != "session-1" {
		t.Fatalf("profile env var must not override KANDEV_SESSION_ID: %+v", env)
	}
}

func TestBuildInteractiveStartRequestCarriesStripEnv(t *testing.T) {
	stripEnv := []string{"ACP_BACKEND"}
	req := buildInteractiveStartRequest(
		"session-1",
		&AgentExecution{WorkspacePath: "/workspace"},
		agents.PassthroughConfig{},
		map[string]string{"KANDEV_SESSION_ID": "session-1"},
		agents.NewCommand("devin"),
		stripEnv,
		true,
	)

	if !slices.Equal(req.StripEnv, stripEnv) {
		t.Fatalf("StripEnv = %v, want %v", req.StripEnv, stripEnv)
	}
}

func TestManager_VerifyPassthroughEnabled(t *testing.T) {
	tests := []struct {
		name      string
		profileID string
		wantErr   bool
	}{
		{
			name:      "valid profile with passthrough enabled",
			profileID: "test-profile",
			wantErr:   false,
		},
		{
			name:      "empty profile ID",
			profileID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager(t)

			// Override profile resolver for this test
			if tt.profileID != "" {
				mgr.profileResolver = &mockPassthroughProfileResolver{
					cliPassthrough: true,
				}
			}

			err := mgr.verifyPassthroughEnabled(context.Background(), "test-session", tt.profileID)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyPassthroughEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
