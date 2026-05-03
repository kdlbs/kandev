package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/kimi_acp_light.svg
var kimiACPLogoLight []byte

//go:embed logos/kimi_acp_dark.svg
var kimiACPLogoDark []byte

var (
	_ Agent            = (*KimiACP)(nil)
	_ PassthroughAgent = (*KimiACP)(nil)
	_ InferenceAgent   = (*KimiACP)(nil)
)

// KimiACP implements Agent for Moonshot's Kimi CLI using ACP.
// Not on npm — users must install the kimi binary from Moonshot AI.
type KimiACP struct {
	StandardPassthrough
}

func NewKimiACP() *KimiACP {
	return &KimiACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("kimi"),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *KimiACP) ID() string          { return "kimi-acp" }
func (a *KimiACP) Name() string        { return "Kimi ACP Agent" }
func (a *KimiACP) DisplayName() string { return "Kimi" }
func (a *KimiACP) Description() string {
	return "Moonshot AI Kimi coding agent using the ACP protocol over stdin/stdout."
}
func (a *KimiACP) Enabled() bool     { return true }
func (a *KimiACP) DisplayOrder() int { return 14 }

func (a *KimiACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return kimiACPLogoDark
	}
	return kimiACPLogoLight
}

func (a *KimiACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("kimi"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	return result, nil
}

func (a *KimiACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("kimi", "acp").Build()
}

func (a *KimiACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("kimi", "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.kimi",
		},
	}
}

func (a *KimiACP) RemoteAuth() *RemoteAuth { return nil }

func (a *KimiACP) InstallScript() string {
	return "Install Kimi CLI from Moonshot AI"
}

func (a *KimiACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *KimiACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("kimi", "acp"),
	}
}
