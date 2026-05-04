package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/kilocode_acp_light.svg
var kilocodeACPLogoLight []byte

//go:embed logos/kilocode_acp_dark.svg
var kilocodeACPLogoDark []byte

const kilocodeACPPkg = "@kilocode/cli"

var (
	_ Agent            = (*KilocodeACP)(nil)
	_ PassthroughAgent = (*KilocodeACP)(nil)
	_ InferenceAgent   = (*KilocodeACP)(nil)
)

// KilocodeACP implements Agent for Kilocode using ACP.
type KilocodeACP struct {
	StandardPassthrough
}

func NewKilocodeACP() *KilocodeACP {
	return &KilocodeACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", kilocodeACPPkg),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *KilocodeACP) ID() string          { return "kilocode-acp" }
func (a *KilocodeACP) Name() string        { return "Kilocode ACP Agent" }
func (a *KilocodeACP) DisplayName() string { return "Kilocode" }
func (a *KilocodeACP) Description() string {
	return "Kilocode coding agent using the ACP protocol over stdin/stdout."
}
func (a *KilocodeACP) Enabled() bool     { return true }
func (a *KilocodeACP) DisplayOrder() int { return 11 }

func (a *KilocodeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return kilocodeACPLogoDark
	}
	return kilocodeACPLogoLight
}

func (a *KilocodeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("kilo"), WithCommand("kilocode"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *KilocodeACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", kilocodeACPPkg, "acp").Build()
}

func (a *KilocodeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", kilocodeACPPkg, "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.kilocode",
		},
	}
}

func (a *KilocodeACP) RemoteAuth() *RemoteAuth { return nil }

func (a *KilocodeACP) InstallScript() string {
	return "npm install -g " + kilocodeACPPkg
}

func (a *KilocodeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *KilocodeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", kilocodeACPPkg, "acp"),
	}
}
