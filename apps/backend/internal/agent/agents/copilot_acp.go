package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/copilot_light.svg
var copilotACPLogoLight []byte

//go:embed logos/copilot_dark.svg
var copilotACPLogoDark []byte

const copilotACPPkg = "@github/copilot"

var (
	_ Agent            = (*CopilotACP)(nil)
	_ PassthroughAgent = (*CopilotACP)(nil)
	_ InferenceAgent   = (*CopilotACP)(nil)
)

// CopilotACP implements Agent for GitHub Copilot using ACP protocol mode.
// It runs the same @github/copilot CLI with the --acp flag, speaking standard
// ACP over stdin/stdout. Used for A/B comparison against the Go SDK-based Copilot agent.
type CopilotACP struct {
	StandardPassthrough
}

func NewCopilotACP() *CopilotACP {
	return &CopilotACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: copilotPermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", copilotACPPkg),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("--continue"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *CopilotACP) ID() string          { return "copilot-acp" }
func (a *CopilotACP) Name() string        { return "Copilot ACP Agent" }
func (a *CopilotACP) DisplayName() string { return "Copilot" }
func (a *CopilotACP) Description() string {
	return "GitHub Copilot coding agent using the ACP protocol over stdin/stdout."
}
func (a *CopilotACP) Enabled() bool     { return true }
func (a *CopilotACP) DisplayOrder() int { return 6 }

func (a *CopilotACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return copilotACPLogoDark
	}
	return copilotACPLogoLight
}

func (a *CopilotACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
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

func (a *CopilotACP) DefaultModel() string { return "gpt-4.1" }

func (a *CopilotACP) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: copilotStaticModels(), SupportsDynamic: false}, nil
}

func (a *CopilotACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", copilotACPPkg, "--acp").
		Model(NewParam("--model", "{model}"), opts.Model).
		Settings(copilotPermSettings, opts.PermissionValues).
		Build()
}

func (a *CopilotACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", copilotACPPkg, "--acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		ModelFlag:      NewParam("--model", "{model}"),
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.copilot",
		},
	}
}

func (a *CopilotACP) RemoteAuth() *RemoteAuth { return nil }

func (a *CopilotACP) InstallScript() string {
	return "npm install -g " + copilotACPPkg
}

func (a *CopilotACP) PermissionSettings() map[string]PermissionSetting {
	return copilotPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *CopilotACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", copilotACPPkg, "--acp"),
		ModelFlag: NewParam("--model", "{model}"),
	}
}

// InferenceModels returns models available for one-shot inference.
func (a *CopilotACP) InferenceModels() []InferenceModel {
	return ModelsToInferenceModels(copilotStaticModels())
}
