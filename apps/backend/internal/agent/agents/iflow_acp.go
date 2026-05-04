package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/iflow_acp_light.svg
var iflowACPLogoLight []byte

//go:embed logos/iflow_acp_dark.svg
var iflowACPLogoDark []byte

const iflowACPPkg = "@iflow-ai/iflow-cli"

var (
	_ Agent            = (*IFlowACP)(nil)
	_ PassthroughAgent = (*IFlowACP)(nil)
	_ InferenceAgent   = (*IFlowACP)(nil)
)

// IFlowACP implements Agent for the iFlow CLI using its experimental ACP mode.
// The upstream flag is `--experimental-acp`; treat this agent as beta until
// iFlow stabilises the protocol.
type IFlowACP struct {
	StandardPassthrough
}

func NewIFlowACP() *IFlowACP {
	return &IFlowACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", iflowACPPkg),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *IFlowACP) ID() string          { return "iflow-acp" }
func (a *IFlowACP) Name() string        { return "iFlow ACP Agent" }
func (a *IFlowACP) DisplayName() string { return "iFlow (beta)" }
func (a *IFlowACP) Description() string {
	return "iFlow coding agent using ACP via the experimental --experimental-acp flag."
}
func (a *IFlowACP) Enabled() bool     { return true }
func (a *IFlowACP) DisplayOrder() int { return 9 }

func (a *IFlowACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return iflowACPLogoDark
	}
	return iflowACPLogoLight
}

func (a *IFlowACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("iflow"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	return result, nil
}

func (a *IFlowACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", iflowACPPkg, "--experimental-acp").Build()
}

func (a *IFlowACP) Runtime() *RuntimeConfig {
	canRecover := false
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", iflowACPPkg, "--experimental-acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			CanRecover:         &canRecover,
			SessionDirTemplate: "{home}/.iflow",
		},
	}
}

func (a *IFlowACP) RemoteAuth() *RemoteAuth { return nil }

func (a *IFlowACP) InstallScript() string {
	return "npm install -g " + iflowACPPkg
}

func (a *IFlowACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *IFlowACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", iflowACPPkg, "--experimental-acp"),
	}
}
