package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/copilot_light.svg
var copilotLogoLight []byte

//go:embed logos/copilot_dark.svg
var copilotLogoDark []byte

const copilotPkg = "@github/copilot@0.0.406"

var (
	_ Agent            = (*Copilot)(nil)
	_ PassthroughAgent = (*Copilot)(nil)
	_ InferenceAgent   = (*Copilot)(nil)
)

type Copilot struct {
	StandardPassthrough
}

func NewCopilot() *Copilot {
	return &Copilot{
		StandardPassthrough: StandardPassthrough{
			PermSettings: copilotPermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", "@github/copilot"),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("--continue"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *Copilot) ID() string          { return "copilot" }
func (a *Copilot) Name() string        { return "GitHub Copilot Agent" }
func (a *Copilot) DisplayName() string { return "Copilot" }
func (a *Copilot) Description() string {
	return "GitHub Copilot CLI-powered autonomous coding agent using the Copilot SDK protocol."
}
func (a *Copilot) Enabled() bool     { return false }
func (a *Copilot) DisplayOrder() int { return 6 }

func (a *Copilot) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return copilotLogoDark
	}
	return copilotLogoLight
}

func (a *Copilot) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	install := OSPaths{
		MacOS: []string{"~/.copilot/pkg"},
	}
	mcp := OSPaths{
		Linux: []string{"~/.copilot/mcp-config.json"},
		MacOS: []string{"~/.copilot/mcp-config.json"},
	}

	result, err := Detect(ctx, WithFileExists(install.Resolve()...))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = install.Expanded()
	result.MCPConfigPaths = mcp.Expanded()
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *Copilot) DefaultModel() string { return "gpt-5-mini" }

func (a *Copilot) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: copilotStaticModels(), SupportsDynamic: false}, nil
}

func (a *Copilot) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", copilotPkg, "--server", "--log-level", "error").
		Model(NewParam("--model", "{model}"), opts.Model).
		Settings(copilotPermSettings, opts.PermissionValues).
		Build()
}

func (a *Copilot) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", copilotPkg, "--server", "--log-level", "error").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolCopilot,
		ModelFlag:      NewParam("--model", "{model}"),
		AssumeMcpSse:   true,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			ResumeFlag:          NewParam("--resume"),
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.copilot",
		},
	}
}

func (a *Copilot) RemoteAuth() *RemoteAuth { return nil }

func (a *Copilot) InstallScript() string {
	return "npm install -g " + copilotPkg
}

func (a *Copilot) PermissionSettings() map[string]PermissionSetting {
	return copilotPermSettings
}

// InferenceConfig returns configuration for one-shot inference.
func (a *Copilot) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported:    true,
		Command:      NewCommand("github-copilot", "-p"),
		ModelFlag:    NewParam("--model", "{model}"),
		OutputFormat: "text",
		StdinInput:   true,
	}
}

// InferenceModels returns models available for one-shot inference.
func (a *Copilot) InferenceModels() []InferenceModel {
	return ModelsToInferenceModels(copilotStaticModels())
}

var copilotPermSettings = map[string]PermissionSetting{
	"auto_approve": {
		Supported: true, Default: true, Label: "Auto-approve (YOLO mode)", Description: "Enable all tools, folders and urls without confirmation.",
		ApplyMethod: "cli_flag", CLIFlag: "--yolo",
	},
}

func copilotStaticModels() []Model {
	return []Model{
		{ID: "claude-sonnet-4.6", Name: "Claude Sonnet 4.6", Provider: "anthropic", Source: "static"},
		{ID: "claude-sonnet-4.5", Name: "Claude Sonnet 4.5", Provider: "anthropic", Source: "static"},
		{ID: "claude-opus-4.6", Name: "Claude Opus 4.6", Provider: "anthropic", Source: "static"},
		{ID: "claude-opus-4.5", Name: "Claude Opus 4.5", Provider: "anthropic", Source: "static"},
		{ID: "claude-haiku-4.5", Name: "Claude Haiku 4.5", Provider: "anthropic", Source: "static"},
		{ID: "claude-sonnet-4", Name: "Claude Sonnet 4", Provider: "anthropic", Source: "static"},
		{ID: "gemini-3-pro-preview", Name: "Gemini 3 Pro (Preview)", Provider: "google", Source: "static"},
		{ID: "gpt-5.4", Name: "GPT-5.4", Provider: "openai", Source: "static"},
		{ID: "gpt-5.3-codex", Name: "GPT-5.3-Codex", Provider: "openai", Source: "static"},
		{ID: "gpt-5.2-codex", Name: "GPT-5.2-Codex", Provider: "openai", Source: "static"},
		{ID: "gpt-5.2", Name: "GPT-5.2", Provider: "openai", Source: "static"},
		{ID: "gpt-5.1-codex-max", Name: "GPT-5.1-Codex-Max", Provider: "openai", Source: "static"},
		{ID: "gpt-5.1-codex", Name: "GPT-5.1-Codex", Provider: "openai", Source: "static"},
		{ID: "gpt-5.1", Name: "GPT-5.1", Provider: "openai", Source: "static"},
		{ID: "gpt-5.1-codex-mini", Name: "GPT-5.1-Codex-Mini", Provider: "openai", Source: "static"},
		{ID: "gpt-5-mini", Name: "GPT-5 Mini", Provider: "openai", Source: "static", IsDefault: true},
		{ID: "gpt-4.1", Name: "GPT-4.1", Provider: "openai", Source: "static"},
	}
}
