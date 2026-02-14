package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/amp_light.svg
var ampLogoLight []byte

//go:embed logos/amp_dark.svg
var ampLogoDark []byte

var (
	_ Agent            = (*Amp)(nil)
	_ PassthroughAgent = (*Amp)(nil)
)

type Amp struct {
	StandardPassthrough
}

func NewAmp() *Amp {
	return &Amp{
		StandardPassthrough: StandardPassthrough{
			PermSettings: ampPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", "@sourcegraph/amp@latest"),
				ModelFlag:      NewParam("-m", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *Amp) ID() string          { return "amp" }
func (a *Amp) Name() string        { return "Sourcegraph Amp Agent" }
func (a *Amp) DisplayName() string { return "Amp" }
func (a *Amp) Description() string {
	return "Sourcegraph Amp CLI-powered autonomous coding agent using stream-json protocol."
}
func (a *Amp) Enabled() bool { return true }

func (a *Amp) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return ampLogoDark
	}
	return ampLogoLight
}

func (a *Amp) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	install := OSPaths{
		Linux: []string{"~/.amp/bin"},
		MacOS: []string{"~/.amp/bin"},
	}
	mcp := OSPaths{
		Linux: []string{"~/.config/amp/settings.json"},
		MacOS: []string{"~/.amp/bin"},
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

func (a *Amp) DefaultModel() string { return "smart" }

func (a *Amp) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: ampStaticModels(), SupportsDynamic: false}, nil
}

func (a *Amp) CreateAdapter(cfg *adapter.Config, log *logger.Logger) (adapter.AgentAdapter, error) {
	return adapter.NewAmpAdapter(cfg, log), nil
}

func (a *Amp) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", "@sourcegraph/amp@latest", "--execute", "--stream-json", "--stream-json-input").
		Model(NewParam("-m", "{model}"), opts.Model).
		Settings(ampPermSettings, opts.PermissionValues).
		Build()
}

func (a *Amp) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", "@sourcegraph/amp@latest", "--execute", "--stream-json", "--stream-json-input").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Capabilities:   []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
		Protocol:       agent.ProtocolAmp,
		ModelFlag:      NewParam("-m", "{model}"),
		SessionConfig: SessionConfig{
			CanRecover:         &canRecover,
			SessionDirTemplate: "{home}/.config/amp",
			ForkSessionCmd:     Cmd("npx", "-y", "@sourcegraph/amp@latest", "threads", "fork").Build(),
			ContinueSessionCmd: Cmd("npx", "-y", "@sourcegraph/amp@latest", "threads", "continue", "--execute", "--stream-json", "--stream-json-input").Build(),
		},
	}
}

func (a *Amp) PermissionSettings() map[string]PermissionSetting {
	return ampPermSettings
}

var ampPermSettings = map[string]PermissionSetting{
	"auto_approve": {
		Supported: true, Default: true, Label: "Auto-approve (Dangerously Allow All)", Description: "Automatically approve all tool calls including shell commands",
		ApplyMethod: "cli_flag", CLIFlag: "--dangerously-allow-all",
	},
}

func ampStaticModels() []Model {
	return []Model{
		{ID: "smart", Name: "Smart Mode", Description: "State-of-the-art models for maximum capability and autonomy", Provider: "amp", IsDefault: true, Source: "static"},
		{ID: "deep", Name: "Deep Mode", Description: "Deep reasoning with extended thinking on complex problems", Provider: "amp", Source: "static"},
	}
}
