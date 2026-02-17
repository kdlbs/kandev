package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/common/logger"
)

// testAgent is a minimal implementation of agents.Agent for testing purposes.
// Embeds StandardPassthrough to optionally satisfy agents.PassthroughAgent.
type testAgent struct {
	agents.StandardPassthrough
	id                 string
	name               string
	displayName        string
	description        string
	enabled            bool
	runtime            *agents.RuntimeConfig
	permissionSettings map[string]agents.PermissionSetting
	logoData           []byte
}

func (a *testAgent) ID() string          { return a.id }
func (a *testAgent) Name() string        { return a.name }
func (a *testAgent) DisplayName() string { return a.displayName }
func (a *testAgent) Description() string { return a.description }
func (a *testAgent) Enabled() bool       { return a.enabled }
func (a *testAgent) DisplayOrder() int   { return 0 }

func (a *testAgent) Logo(v agents.LogoVariant) []byte { return a.logoData }


func (a *testAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	return &agents.DiscoveryResult{Available: false}, nil
}

func (a *testAgent) DefaultModel() string { return "" }

func (a *testAgent) ListModels(ctx context.Context) (*agents.ModelList, error) {
	return &agents.ModelList{}, nil
}

// BuildCommand builds a command using runtime config, model flag, and permission flags.
func (a *testAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	rt := a.Runtime()
	if rt == nil {
		return agents.Command{}
	}
	cmd := make([]string, len(rt.Cmd.Args()))
	copy(cmd, rt.Cmd.Args())

	if opts.Model != "" && !rt.ModelFlag.IsEmpty() {
		for _, arg := range rt.ModelFlag.Args() {
			cmd = append(cmd, strings.ReplaceAll(arg, "{model}", opts.Model))
		}
	}

	cmd = applyTestPermissionFlags(cmd, a.permissionSettings, opts.PermissionValues)
	return agents.NewCommand(cmd...)
}

func applyTestPermissionFlags(cmd []string, permSettings map[string]agents.PermissionSetting, values map[string]bool) []string {
	if permSettings == nil || values == nil {
		return cmd
	}
	for name, setting := range permSettings {
		if !setting.Supported || setting.ApplyMethod != "cli_flag" || setting.CLIFlag == "" {
			continue
		}
		v, ok := values[name]
		if !ok || !v {
			continue
		}
		if setting.CLIFlagValue != "" {
			cmd = append(cmd, setting.CLIFlag, setting.CLIFlagValue)
		} else {
			parts := strings.Fields(setting.CLIFlag)
			cmd = append(cmd, parts...)
		}
	}
	return cmd
}

func (a *testAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return a.permissionSettings
}

func (a *testAgent) Runtime() *agents.RuntimeConfig {
	return a.runtime
}

func newTestController(agentList map[string]agents.Agent) *Controller {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	reg := registry.NewRegistry(log)
	for _, ag := range agentList {
		_ = reg.Register(ag)
	}
	return &Controller{
		agentRegistry: reg,
		modelCache:    modelfetcher.NewCache(),
		logger:        log,
	}
}

func TestController_PreviewAgentCommand_StandardCommand(t *testing.T) {
	agentList := map[string]agents.Agent{
		"test-agent": &testAgent{
			id:      "test-agent",
			name:    "test-agent",
			enabled: true,
			runtime: &agents.RuntimeConfig{
				Cmd:       agents.NewCommand("test-cli", "--verbose"),
				ModelFlag: agents.NewParam("--model", "{model}"),
			},
			permissionSettings: map[string]agents.PermissionSetting{
				"auto_approve": {
					Supported:   true,
					ApplyMethod: "cli_flag",
					CLIFlag:     "--yes",
				},
			},
		},
	}

	controller := newTestController(agentList)

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
	agentList := map[string]agents.Agent{
		"claude-code": &testAgent{
			id:      "claude-code",
			name:    "claude-code",
			enabled: true,
			runtime: &agents.RuntimeConfig{
				Cmd:       agents.NewCommand("claude"),
				ModelFlag: agents.NewParam("--model", "{model}"),
			},
			StandardPassthrough: agents.StandardPassthrough{
				Cfg: agents.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: agents.NewCommand("npx", "-y", "@anthropic-ai/claude-code"),
					ModelFlag:      agents.NewParam("--model", "{model}"),
					PromptFlag:     agents.NewParam("--prompt", "{prompt}"),
				},
				PermSettings: map[string]agents.PermissionSetting{
					"dangerously_skip_permissions": {
						Supported:   true,
						ApplyMethod: "cli_flag",
						CLIFlag:     "--dangerously-skip-permissions",
					},
				},
			},
			permissionSettings: map[string]agents.PermissionSetting{
				"dangerously_skip_permissions": {
					Supported:   true,
					ApplyMethod: "cli_flag",
					CLIFlag:     "--dangerously-skip-permissions",
				},
			},
		},
	}

	controller := newTestController(agentList)

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
	controller := newTestController(map[string]agents.Agent{})

	_, err := controller.PreviewAgentCommand(context.Background(), "nonexistent", CommandPreviewRequest{})
	if err == nil {
		t.Error("PreviewAgentCommand() should return error for unknown agent")
	}
}

func TestController_PreviewAgentCommand_PassthroughDisabled(t *testing.T) {
	agentList := map[string]agents.Agent{
		"test-agent": &testAgent{
			id:      "test-agent",
			name:    "test-agent",
			enabled: true,
			runtime: &agents.RuntimeConfig{
				Cmd: agents.NewCommand("test-cli"),
			},
			StandardPassthrough: agents.StandardPassthrough{
				Cfg: agents.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: agents.NewCommand("passthrough-cli"),
				},
			},
		},
	}

	controller := newTestController(agentList)

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

func TestBuildCommandString(t *testing.T) {
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
			result := buildCommandString(tt.cmd)
			if result != tt.expected {
				t.Errorf("buildCommandString(%v) = %q, want %q", tt.cmd, result, tt.expected)
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

func TestController_GetAgentLogo_Success(t *testing.T) {
	logoBytes := []byte("<svg>test</svg>")
	agentList := map[string]agents.Agent{
		"test-agent": &testAgent{
			id:       "test-agent",
			name:     "test-agent",
			enabled:  true,
			logoData: logoBytes,
		},
	}
	ctrl := newTestController(agentList)

	data, err := ctrl.GetAgentLogo(context.Background(), "test-agent", agents.LogoLight)
	if err != nil {
		t.Fatalf("GetAgentLogo() error = %v", err)
	}
	if string(data) != string(logoBytes) {
		t.Errorf("GetAgentLogo() = %q, want %q", data, logoBytes)
	}
}

func TestController_GetAgentLogo_AgentNotFound(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{})

	_, err := ctrl.GetAgentLogo(context.Background(), "nonexistent", agents.LogoLight)
	if err != ErrAgentNotFound {
		t.Errorf("GetAgentLogo() error = %v, want ErrAgentNotFound", err)
	}
}

func TestController_GetAgentLogo_NoLogoData(t *testing.T) {
	agentList := map[string]agents.Agent{
		"test-agent": &testAgent{
			id:      "test-agent",
			name:    "test-agent",
			enabled: true,
			// logoData is nil
		},
	}
	ctrl := newTestController(agentList)

	_, err := ctrl.GetAgentLogo(context.Background(), "test-agent", agents.LogoLight)
	if err != ErrLogoNotAvailable {
		t.Errorf("GetAgentLogo() error = %v, want ErrLogoNotAvailable", err)
	}
}
