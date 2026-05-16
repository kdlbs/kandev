package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/qwen_acp_light.svg
var qwenACPLogoLight []byte

//go:embed logos/qwen_acp_dark.svg
var qwenACPLogoDark []byte

const qwenACPPkg = "@qwen-code/qwen-code"

var (
	_ Agent            = (*QwenACP)(nil)
	_ PassthroughAgent = (*QwenACP)(nil)
	_ InferenceAgent   = (*QwenACP)(nil)
)

// QwenACP implements Agent for Alibaba's Qwen Code CLI using the ACP protocol.
type QwenACP struct {
	StandardPassthrough
}

func NewQwenACP() *QwenACP {
	return &QwenACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", qwenACPPkg),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *QwenACP) ID() string          { return "qwen-acp" }
func (a *QwenACP) Name() string        { return "Qwen Code ACP Agent" }
func (a *QwenACP) DisplayName() string { return "Qwen" }
func (a *QwenACP) Description() string {
	return "Alibaba Qwen Code coding agent using the ACP protocol over stdin/stdout."
}
func (a *QwenACP) Enabled() bool     { return true }
func (a *QwenACP) DisplayOrder() int { return 8 }

func (a *QwenACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return qwenACPLogoDark
	}
	return qwenACPLogoLight
}

func (a *QwenACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("qwen"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *QwenACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", qwenACPPkg, "--acp").Build()
}

func (a *QwenACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", qwenACPPkg, "--acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.qwen",
		},
	}
}

func (a *QwenACP) RemoteAuth() *RemoteAuth { return nil }

func (a *QwenACP) InstallScript() string {
	return "npm install -g " + qwenACPPkg
}

func (a *QwenACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *QwenACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", qwenACPPkg, "--acp"),
	}
}

func (a *QwenACP) BillingType() usage.BillingType { return defaultBillingType() }
