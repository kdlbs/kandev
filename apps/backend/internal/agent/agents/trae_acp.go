package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/trae_acp_light.svg
var traeACPLogoLight []byte

//go:embed logos/trae_acp_dark.svg
var traeACPLogoDark []byte

const traeACPBin = "traecli"

var (
	_ Agent            = (*TraeACP)(nil)
	_ PassthroughAgent = (*TraeACP)(nil)
	_ InferenceAgent   = (*TraeACP)(nil)
)

// TraeACP implements Agent for ByteDance's Trae IDE CLI using ACP.
type TraeACP struct {
	StandardPassthrough
}

func NewTraeACP() *TraeACP {
	return &TraeACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(traeACPBin),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *TraeACP) ID() string          { return "trae-acp" }
func (a *TraeACP) Name() string        { return "Trae ACP Agent" }
func (a *TraeACP) DisplayName() string { return "Trae" }
func (a *TraeACP) Description() string {
	return "ByteDance Trae IDE coding agent using the ACP protocol via traecli acp serve."
}
func (a *TraeACP) Enabled() bool     { return true }
func (a *TraeACP) DisplayOrder() int { return 17 }

func (a *TraeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return traeACPLogoDark
	}
	return traeACPLogoLight
}

func (a *TraeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(traeACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *TraeACP) BuildCommand(opts CommandOptions) Command {
	return Cmd(traeACPBin, "acp", "serve").Build()
}

func (a *TraeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd(traeACPBin, "acp", "serve").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			// TODO: set SessionDirTemplate once the Trae session dir is
			// confirmed. Without it, the Docker runtime skips mounting the
			// session dir, so NativeSessionResume has no persistence across
			// container restarts.
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *TraeACP) RemoteAuth() *RemoteAuth { return nil }

func (a *TraeACP) InstallScript() string {
	return "Install Trae IDE CLI from https://trae.ai"
}

func (a *TraeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *TraeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(traeACPBin, "acp", "serve"),
	}
}

func (a *TraeACP) BillingType() usage.BillingType { return defaultBillingType() }
