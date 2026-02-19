package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/gemini_light.svg
var geminiLogoLight []byte

//go:embed logos/gemini_dark.svg
var geminiLogoDark []byte

var (
	_ Agent            = (*Gemini)(nil)
	_ PassthroughAgent = (*Gemini)(nil)
)

type Gemini struct {
	StandardPassthrough
}

func NewGemini() *Gemini {
	return &Gemini{
		StandardPassthrough: StandardPassthrough{
			PermSettings: geminiPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "@google/gemini-cli"),
				ModelFlag:      NewParam("--model", "{model}"),
				PromptFlag:     NewParam("--prompt-interactive", "{prompt}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				ResumeFlag:     NewParam("--resume", "latest"),
			},
		},
	}
}

func (a *Gemini) ID() string          { return "gemini" }
func (a *Gemini) Name() string        { return "Google Gemini CLI Agent" }
func (a *Gemini) DisplayName() string { return "Gemini" }
func (a *Gemini) Description() string {
	return "Google Gemini CLI-powered autonomous coding agent using ACP protocol."
}
func (a *Gemini) Enabled() bool     { return true }
func (a *Gemini) DisplayOrder() int { return 5 }

func (a *Gemini) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return geminiLogoDark
	}
	return geminiLogoLight
}

func (a *Gemini) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	install := OSPaths{
		Linux: []string{"~/.gemini/oauth_creds.json", "~/.gemini/installation_id"},
		MacOS: []string{"~/.gemini/oauth_creds.json", "~/.gemini/installation_id"},
	}
	mcp := OSPaths{
		Linux: []string{"~/.gemini/settings.json"},
		MacOS: []string{"~/.gemini/settings.json"},
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

func (a *Gemini) DefaultModel() string { return "gemini-3-flash-preview" }

func (a *Gemini) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: geminiStaticModels(), SupportsDynamic: false}, nil
}

func (a *Gemini) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", "@google/gemini-cli@0.25.2", "--experimental-acp").
		Model(NewParam("--model", "{model}"), opts.Model).
		Settings(geminiPermSettings, opts.PermissionValues).
		Build()
}

func (a *Gemini) Runtime() *RuntimeConfig {
	canRecover := false
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", "@google/gemini-cli@0.25.2", "--experimental-acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		ModelFlag:      NewParam("--model", "{model}"),
		SessionConfig: SessionConfig{
			CanRecover:         &canRecover,
			SessionDirTemplate: "{home}/.gemini",
		},
	}
}

func (a *Gemini) PermissionSettings() map[string]PermissionSetting {
	return geminiPermSettings
}

var geminiPermSettings = map[string]PermissionSetting{
	"auto_approve": {
		Supported: true, Default: true, Label: "Auto-approve (YOLO mode)", Description: "Automatically approve all tool calls",
		ApplyMethod: "cli_flag", CLIFlag: "--yolo --allowed-tools run_shell_command",
	},
}

func geminiStaticModels() []Model {
	return []Model{
		{ID: "gemini-3-flash-preview", Name: "3 Flash", Description: "Fast and efficient model", Provider: "google", ContextWindow: 1000000, IsDefault: true, Source: "static"},
		{ID: "gemini-3-pro-preview", Name: "3 Pro", Description: "Most capable model with 2M context", Provider: "google", ContextWindow: 2000000, Source: "static"},
	}
}
