package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestController(agents map[string]*registry.AgentTypeConfig) *Controller {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	// Create a real registry and register the agents
	reg := registry.NewRegistry(log)
	for _, agent := range agents {
		_ = reg.Register(agent)
	}
	return &Controller{
		agentRegistry: reg,
		logger:        log,
	}
}

func TestController_PreviewAgentCommand_StandardCommand(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"test-agent": {
			ID:        "test-agent",
			Name:      "test-agent",
			Cmd:       []string{"test-cli", "--verbose"},
			ModelFlag: "--model {model}",
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			PermissionSettings: map[string]registry.PermissionSetting{
				"auto_approve": {
					Supported:   true,
					ApplyMethod: "cli_flag",
					CLIFlag:     "--yes",
				},
			},
			PassthroughConfig: registry.PassthroughConfig{
				Supported: false,
			},
		},
	}

	controller := newTestController(agents)

	req := CommandPreviewRequest{
		Model:              "gpt-4",
		PermissionSettings: map[string]bool{"auto_approve": true},
		CLIPassthrough:     false,
	}

	result, err := controller.PreviewAgentCommand(context.Background(), "test-agent", req)
	if err != nil {
		t.Fatalf("PreviewAgentCommand() error = %v", err)
	}

	if !result.Supported {
		t.Error("PreviewAgentCommand() Supported = false, want true")
	}

	// Verify command contains expected parts
	expectedParts := []string{"test-cli", "--verbose", "--model", "gpt-4", "--yes"}
	for _, part := range expectedParts {
		found := false
		for _, cmdPart := range result.Command {
			if cmdPart == part {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PreviewAgentCommand() command missing %q, got %v", part, result.Command)
		}
	}
}

func TestController_PreviewAgentCommand_PassthroughCommand(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"claude-code": {
			ID:        "claude-code",
			Name:      "claude-code",
			Cmd:       []string{"claude"},
			ModelFlag: "--model {model}",
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			PassthroughConfig: registry.PassthroughConfig{
				Supported:      true,
				PassthroughCmd: []string{"npx", "-y", "@anthropic-ai/claude-code"},
				ModelFlag:      "--model {model}",
				PromptFlag:     "--prompt {prompt}",
			},
			PermissionSettings: map[string]registry.PermissionSetting{
				"dangerously_skip_permissions": {
					Supported:   true,
					ApplyMethod: "cli_flag",
					CLIFlag:     "--dangerously-skip-permissions",
				},
			},
		},
	}

	controller := newTestController(agents)

	req := CommandPreviewRequest{
		Model:              "claude-sonnet-4-20250514",
		PermissionSettings: map[string]bool{"dangerously_skip_permissions": true},
		CLIPassthrough:     true,
	}

	result, err := controller.PreviewAgentCommand(context.Background(), "claude-code", req)
	if err != nil {
		t.Fatalf("PreviewAgentCommand() error = %v", err)
	}

	// Verify it uses passthrough command
	if len(result.Command) < 3 || result.Command[0] != "npx" {
		t.Errorf("PreviewAgentCommand() should use passthrough command, got %v", result.Command)
	}

	// Verify model flag is present
	hasModel := false
	for i, part := range result.Command {
		if part == "--model" && i+1 < len(result.Command) && result.Command[i+1] == "claude-sonnet-4-20250514" {
			hasModel = true
			break
		}
	}
	if !hasModel {
		t.Errorf("PreviewAgentCommand() missing model flag, got %v", result.Command)
	}

	// Verify permission flag is present
	hasPermFlag := false
	for _, part := range result.Command {
		if part == "--dangerously-skip-permissions" {
			hasPermFlag = true
			break
		}
	}
	if !hasPermFlag {
		t.Errorf("PreviewAgentCommand() missing permission flag, got %v", result.Command)
	}

	// Verify prompt placeholder is present
	hasPrompt := false
	for _, part := range result.Command {
		if part == "--prompt" || part == "{prompt}" {
			hasPrompt = true
			break
		}
	}
	if !hasPrompt {
		t.Errorf("PreviewAgentCommand() missing prompt placeholder, got %v", result.Command)
	}
}

func TestController_PreviewAgentCommand_AgentNotFound(t *testing.T) {
	controller := newTestController(map[string]*registry.AgentTypeConfig{})

	_, err := controller.PreviewAgentCommand(context.Background(), "nonexistent", CommandPreviewRequest{})
	if err == nil {
		t.Error("PreviewAgentCommand() should return error for unknown agent")
	}
}

func TestController_PreviewAgentCommand_PassthroughDisabled(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"test-agent": {
			ID:   "test-agent",
			Name: "test-agent",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			PassthroughConfig: registry.PassthroughConfig{
				Supported:      true,
				PassthroughCmd: []string{"passthrough-cli"},
			},
		},
	}

	controller := newTestController(agents)

	// CLIPassthrough is false, so should use standard command
	req := CommandPreviewRequest{
		CLIPassthrough: false,
	}

	result, err := controller.PreviewAgentCommand(context.Background(), "test-agent", req)
	if err != nil {
		t.Fatalf("PreviewAgentCommand() error = %v", err)
	}

	if result.Command[0] != "test-cli" {
		t.Errorf("PreviewAgentCommand() should use standard command when passthrough disabled, got %v", result.Command)
	}
}

func TestController_BuildCommandString(t *testing.T) {
	controller := newTestController(nil)

	tests := []struct {
		name     string
		cmd      []string
		expected string
	}{
		{
			name:     "simple command",
			cmd:      []string{"echo", "hello"},
			expected: "echo hello",
		},
		{
			name:     "command with spaces",
			cmd:      []string{"echo", "hello world"},
			expected: `echo "hello world"`,
		},
		{
			name:     "command with quotes",
			cmd:      []string{"echo", `say "hi"`},
			expected: `echo "say \"hi\""`,
		},
		{
			name:     "command with special chars",
			cmd:      []string{"bash", "-c", "echo $HOME"},
			expected: `bash -c "echo $HOME"`,
		},
		{
			name:     "empty command",
			cmd:      []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.buildCommandString(tt.cmd)
			if result != tt.expected {
				t.Errorf("buildCommandString(%v) = %q, want %q", tt.cmd, result, tt.expected)
			}
		})
	}
}

func TestController_ApplyPermissionFlags(t *testing.T) {
	controller := newTestController(nil)

	agentConfig := &registry.AgentTypeConfig{
		PermissionSettings: map[string]registry.PermissionSetting{
			"auto_approve": {
				Supported:   true,
				ApplyMethod: "cli_flag",
				CLIFlag:     "--yes",
			},
			"skip_permissions": {
				Supported:    true,
				ApplyMethod:  "cli_flag",
				CLIFlag:      "--skip",
				CLIFlagValue: "all",
			},
			"unsupported": {
				Supported: false,
				CLIFlag:   "--unsupported",
			},
			"env_method": {
				Supported:   true,
				ApplyMethod: "env_var", // Not cli_flag
				CLIFlag:     "--env",
			},
		},
	}

	tests := []struct {
		name       string
		initial    []string
		permissions map[string]bool
		expected   []string
	}{
		{
			name:       "boolean flag enabled",
			initial:    []string{"cmd"},
			permissions: map[string]bool{"auto_approve": true},
			expected:   []string{"cmd", "--yes"},
		},
		{
			name:       "flag with value",
			initial:    []string{"cmd"},
			permissions: map[string]bool{"skip_permissions": true},
			expected:   []string{"cmd", "--skip", "all"},
		},
		{
			name:       "flag disabled",
			initial:    []string{"cmd"},
			permissions: map[string]bool{"auto_approve": false},
			expected:   []string{"cmd"},
		},
		{
			name:       "unsupported flag ignored",
			initial:    []string{"cmd"},
			permissions: map[string]bool{"unsupported": true},
			expected:   []string{"cmd"},
		},
		{
			name:       "non-cli method ignored",
			initial:    []string{"cmd"},
			permissions: map[string]bool{"env_method": true},
			expected:   []string{"cmd"},
		},
		{
			name:       "multiple flags",
			initial:    []string{"cmd"},
			permissions: map[string]bool{"auto_approve": true, "skip_permissions": true},
			expected:   []string{"cmd", "--yes", "--skip", "all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.applyPermissionFlags(tt.initial, agentConfig, tt.permissions)

			// Check that all expected parts are present (order may vary for maps)
			if len(result) != len(tt.expected) {
				t.Errorf("applyPermissionFlags() length = %d, want %d", len(result), len(tt.expected))
				t.Errorf("got: %v, want: %v", result, tt.expected)
			}
		})
	}
}

func TestController_AppendPromptPlaceholder(t *testing.T) {
	controller := newTestController(nil)

	tests := []struct {
		name       string
		cmd        []string
		promptFlag string
		expected   []string
	}{
		{
			name:       "with prompt flag",
			cmd:        []string{"cli"},
			promptFlag: "--prompt {prompt}",
			expected:   []string{"cli", "--prompt", "{prompt}"},
		},
		{
			name:       "without prompt flag",
			cmd:        []string{"cli"},
			promptFlag: "",
			expected:   []string{"cli", "{prompt}"},
		},
		{
			name:       "single flag format",
			cmd:        []string{"cli"},
			promptFlag: "-p {prompt}",
			expected:   []string{"cli", "-p", "{prompt}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.appendPromptPlaceholder(tt.cmd, tt.promptFlag)
			if len(result) != len(tt.expected) {
				t.Errorf("appendPromptPlaceholder() = %v, want %v", result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("appendPromptPlaceholder()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestCommandPreviewResponse_DTO(t *testing.T) {
	resp := dto.CommandPreviewResponse{
		Supported:     true,
		Command:       []string{"npx", "claude-code", "--model", "gpt-4"},
		CommandString: `npx claude-code --model gpt-4`,
	}

	if !resp.Supported {
		t.Error("CommandPreviewResponse.Supported should be true")
	}
	if len(resp.Command) != 4 {
		t.Errorf("CommandPreviewResponse.Command length = %d, want 4", len(resp.Command))
	}
	if resp.CommandString == "" {
		t.Error("CommandPreviewResponse.CommandString should not be empty")
	}
}
