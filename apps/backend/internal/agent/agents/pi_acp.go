package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/pi_acp_light.svg
var piACPLogoLight []byte

//go:embed logos/pi_acp_dark.svg
var piACPLogoDark []byte

const piACPPkg = "pi-acp"

var (
	_ Agent            = (*PiACP)(nil)
	_ PassthroughAgent = (*PiACP)(nil)
	_ InferenceAgent   = (*PiACP)(nil)
)

// PiACP implements Agent for the Pi Coding Agent via the pi-acp adapter.
type PiACP struct {
	StandardPassthrough
}

func NewPiACP() *PiACP {
	return &PiACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", piACPPkg),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *PiACP) ID() string          { return "pi-acp" }
func (a *PiACP) Name() string        { return "Pi Coding Agent ACP" }
func (a *PiACP) DisplayName() string { return "Pi" }
func (a *PiACP) Description() string {
	return "Pi Coding Agent using the ACP protocol via the pi-acp adapter."
}
func (a *PiACP) Enabled() bool     { return true }
func (a *PiACP) DisplayOrder() int { return 12 }

func (a *PiACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return piACPLogoDark
	}
	return piACPLogoLight
}

func (a *PiACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("pi-acp"), WithCommand("pi"), WithNpxRunnable(piACPPkg))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	return result, nil
}

func (a *PiACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", piACPPkg).Build()
}

func (a *PiACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", piACPPkg).Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *PiACP) RemoteAuth() *RemoteAuth { return nil }

func (a *PiACP) InstallScript() string {
	return "npm install -g " + piACPPkg
}

func (a *PiACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *PiACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", piACPPkg),
	}
}
