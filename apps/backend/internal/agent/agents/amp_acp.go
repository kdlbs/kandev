package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/amp_light.svg
var ampACPLogoLight []byte

//go:embed logos/amp_dark.svg
var ampACPLogoDark []byte

const ampACPPkg = "amp-acp"

var (
	_ Agent            = (*AmpACP)(nil)
	_ PassthroughAgent = (*AmpACP)(nil)
	_ InferenceAgent   = (*AmpACP)(nil)
)

// AmpACP implements Agent for Sourcegraph Amp using the ACP protocol.
// It runs the community amp-acp npm package which bridges Amp to ACP over
// stdin/stdout. Used for A/B comparison against the stream-json Amp agent.
type AmpACP struct {
	StandardPassthrough
}

func NewAmpACP() *AmpACP {
	return &AmpACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", ampACPPkg),
				ModelFlag:      NewParam("-m", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *AmpACP) ID() string          { return "amp-acp" }
func (a *AmpACP) Name() string        { return "Amp ACP Agent" }
func (a *AmpACP) DisplayName() string { return "Amp" }
func (a *AmpACP) Description() string {
	return "Sourcegraph Amp coding agent using the ACP protocol via the community bridge."
}
func (a *AmpACP) Enabled() bool     { return true }
func (a *AmpACP) DisplayOrder() int { return 7 }

func (a *AmpACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return ampACPLogoDark
	}
	return ampACPLogoLight
}

func (a *AmpACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
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

func (a *AmpACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", ampACPPkg).Build()
}

func (a *AmpACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", ampACPPkg).Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.config/amp",
		},
	}
}

func (a *AmpACP) RemoteAuth() *RemoteAuth { return nil }

func (a *AmpACP) InstallScript() string {
	return "npm install -g " + ampACPPkg
}

func (a *AmpACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *AmpACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", ampACPPkg),
	}
}
