//nolint:dupl // Native-binary ACP agents (Kiro, Qoder) follow the same minimal scaffold; differences are the binary name and subcommand.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/kiro_acp_light.svg
var kiroACPLogoLight []byte

//go:embed logos/kiro_acp_dark.svg
var kiroACPLogoDark []byte

var (
	_ Agent            = (*KiroACP)(nil)
	_ PassthroughAgent = (*KiroACP)(nil)
	_ InferenceAgent   = (*KiroACP)(nil)
)

// KiroACP implements Agent for AWS Kiro using ACP. The CLI binary
// (kiro-cli-chat) is installed via AWS-provided tooling.
type KiroACP struct {
	StandardPassthrough
}

func NewKiroACP() *KiroACP {
	return &KiroACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("kiro-cli-chat"),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *KiroACP) ID() string          { return "kiro-acp" }
func (a *KiroACP) Name() string        { return "Kiro ACP Agent" }
func (a *KiroACP) DisplayName() string { return "Kiro" }
func (a *KiroACP) Description() string {
	return "AWS Kiro coding agent using the ACP protocol via kiro-cli-chat."
}
func (a *KiroACP) Enabled() bool     { return true }
func (a *KiroACP) DisplayOrder() int { return 15 }

func (a *KiroACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return kiroACPLogoDark
	}
	return kiroACPLogoLight
}

func (a *KiroACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("kiro-cli-chat"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	return result, nil
}

func (a *KiroACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("kiro-cli-chat", "acp").Build()
}

func (a *KiroACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("kiro-cli-chat", "acp").Build(),
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

func (a *KiroACP) RemoteAuth() *RemoteAuth { return nil }

func (a *KiroACP) InstallScript() string {
	return "Install Kiro CLI from AWS"
}

func (a *KiroACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *KiroACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("kiro-cli-chat", "acp"),
	}
}
