package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/claude_code_light.svg
var claudeCodeLogoLight []byte

//go:embed logos/claude_code_dark.svg
var claudeCodeLogoDark []byte

var (
	_ Agent            = (*ClaudeCode)(nil)
	_ PassthroughAgent = (*ClaudeCode)(nil)
)

type ClaudeCode struct {
	StandardPassthrough
}

func NewClaudeCode() *ClaudeCode {
	return &ClaudeCode{
		StandardPassthrough: StandardPassthrough{
			PermSettings: claudeCodePermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", "@anthropic-ai/claude-code", "--verbose"),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("-c"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *ClaudeCode) ID() string          { return "claude-code" }
func (a *ClaudeCode) Name() string        { return "Claude Code CLI Agent" }
func (a *ClaudeCode) DisplayName() string { return "Claude" }
func (a *ClaudeCode) Description() string {
	return "Anthropic Claude Code CLI-powered autonomous coding agent using the stream-json protocol."
}
func (a *ClaudeCode) Enabled() bool     { return true }
func (a *ClaudeCode) DisplayOrder() int { return 1 }

func (a *ClaudeCode) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return claudeCodeLogoDark
	}
	return claudeCodeLogoLight
}

func (a *ClaudeCode) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithFileExists("~/.claude.json"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = []string{expandHomePath("~/.claude.json")}
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *ClaudeCode) DefaultModel() string { return "claude-sonnet-4-5" }

func (a *ClaudeCode) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: claudeCodeStaticModels(), SupportsDynamic: false}, nil
}

func (a *ClaudeCode) BuildCommand(opts CommandOptions) Command {
	b := Cmd("npx", "-y", "@anthropic-ai/claude-code@2.1.50",
		"-p", "--output-format=stream-json", "--input-format=stream-json",
		"--permission-prompt-tool=stdio", "--disallowedTools=AskUserQuestion",
		"--setting-sources=user,project", "--verbose",
		"--include-partial-messages", "--replay-user-messages").
		Model(NewParam("--model", "{model}"), opts.Model).
		Resume(NewParam("--resume"), opts.SessionID, false).
		ResumeAt(NewParam("--resume-session-at"), opts.ResumeAtMessageUUID)

	// When using supervised/plan mode with hooks, bypass the built-in permission
	// system so hooks handle access control instead
	switch opts.PermissionPolicy {
	case "supervised", "plan":
		b = b.Flag("--permission-mode", "bypassPermissions")
	default:
		b = b.Settings(claudeCodePermSettings, opts.PermissionValues)
	}

	if opts.AgentType != "" {
		b = b.Flag("--agent", opts.AgentType)
	}

	return b.Build()
}

func (a *ClaudeCode) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd: Cmd("npx", "-y", "@anthropic-ai/claude-code@2.1.50",
			"-p", "--output-format=stream-json", "--input-format=stream-json",
			"--permission-prompt-tool=stdio", "--disallowedTools=AskUserQuestion",
			"--setting-sources=user,project", "--verbose",
			"--include-partial-messages", "--replay-user-messages").Build(),
		WorkingDir:     "{workspace}",
		RequiredEnv:    []string{"ANTHROPIC_API_KEY"},
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolClaudeCode,
		ModelFlag:      NewParam("--model", "{model}"),
		SessionConfig: SessionConfig{
			ResumeFlag:         NewParam("--resume"),
			CanRecover:         &canRecover,
			SessionDirTemplate: "{home}/.claude",
		},
	}
}

func (a *ClaudeCode) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:      "env",
				EnvVar:    "CLAUDE_CODE_OAUTH_TOKEN",
				SetupHint: "Run `claude setup-token` to generate a long-lived OAuth token",
			},
		},
	}
}

func (a *ClaudeCode) PermissionSettings() map[string]PermissionSetting {
	return claudeCodePermSettings
}

var claudeCodePermSettings = map[string]PermissionSetting{
	"auto_approve": {
		Supported: true, Default: true, Label: "Auto-approve", Description: "Automatically approve tool calls via stdio protocol",
		ApplyMethod: "stdio",
	},
	"dangerously_skip_permissions": {
		Supported: true, Default: true, Label: "Skip Permissions", Description: "Bypass all permission checks (dangerous but fast for trusted tasks)",
		ApplyMethod: "cli_flag", CLIFlag: "--dangerously-skip-permissions",
	},
	"permission_policy": {
		Supported: true, Default: false, Label: "Permission Policy", Description: "Control permission mode: autonomous (default), supervised (approve writes), plan (approve plan exit)",
		ApplyMethod: "custom",
	},
}

func claudeCodeStaticModels() []Model {
	return []Model{
		{ID: "claude-sonnet-4-5", Name: "Sonnet 4.5", Description: "Latest Sonnet with improved reasoning", Provider: "anthropic", ContextWindow: 200000, IsDefault: true, Source: "static"},
		{ID: "claude-opus-4-6", Name: "Opus 4.6", Description: "Latest and most capable model for complex tasks", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
		{ID: "claude-opus-4-5", Name: "Opus 4.5", Description: "Most capable model for complex tasks", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
		{ID: "claude-haiku-4-5", Name: "Haiku 4.5", Description: "Fast and affordable model for simple tasks", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
	}
}
