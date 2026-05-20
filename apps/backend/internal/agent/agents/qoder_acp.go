//nolint:dupl // Native-binary ACP agents (Cursor, Kimi, Kiro, Qoder, Trae) follow the same minimal scaffold; differences are the binary name and subcommand.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/qoder_acp_light.svg
var qoderACPLogoLight []byte

//go:embed logos/qoder_acp_dark.svg
var qoderACPLogoDark []byte

const qoderACPBin = "qodercli"

var (
	_ Agent            = (*QoderACP)(nil)
	_ PassthroughAgent = (*QoderACP)(nil)
	_ InferenceAgent   = (*QoderACP)(nil)
)

// QoderACP implements Agent for the Qoder CLI using ACP.
type QoderACP struct {
	StandardPassthrough
}

func NewQoderACP() *QoderACP {
	return &QoderACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(qoderACPBin),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *QoderACP) ID() string          { return "qoder-acp" }
func (a *QoderACP) Name() string        { return "Qoder ACP Agent" }
func (a *QoderACP) DisplayName() string { return "Qoder" }
func (a *QoderACP) Description() string {
	return "Qoder coding agent using the ACP protocol via qodercli --acp."
}
func (a *QoderACP) Enabled() bool     { return true }
func (a *QoderACP) DisplayOrder() int { return 16 }

func (a *QoderACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return qoderACPLogoDark
	}
	return qoderACPLogoLight
}

func (a *QoderACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(qoderACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *QoderACP) BuildCommand(opts CommandOptions) Command {
	return Cmd(qoderACPBin, "--acp").Build()
}

func (a *QoderACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd(qoderACPBin, "--acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			// TODO: set SessionDirTemplate once the Qoder session dir is
			// confirmed. Without it, the Docker runtime skips mounting the
			// session dir, so NativeSessionResume has no persistence across
			// container restarts.
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *QoderACP) RemoteAuth() *RemoteAuth { return nil }

func (a *QoderACP) InstallScript() string {
	return "Install Qoder CLI from https://qoder.com"
}

func (a *QoderACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *QoderACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(qoderACPBin, "--acp"),
	}
}

func (a *QoderACP) BillingType() usage.BillingType { return defaultBillingType() }
