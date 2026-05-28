//nolint:dupl // Native-binary ACP agents (Cursor, Kimi, Kiro, Qoder, Trae, Hermes) follow the same minimal scaffold; differences are the binary name and subcommand.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/hermes_acp_light.svg
var hermesACPLogoLight []byte

//go:embed logos/hermes_acp_dark.svg
var hermesACPLogoDark []byte

const hermesACPBin = "hermes"

var (
	_ Agent            = (*HermesACP)(nil)
	_ PassthroughAgent = (*HermesACP)(nil)
	_ InferenceAgent   = (*HermesACP)(nil)
)

// HermesACP implements Agent for Nous Research's Hermes CLI using ACP.
// Not on npm — users install via the upstream curl|bash script which puts
// the `hermes` Python entry point on PATH.
type HermesACP struct {
	StandardPassthrough
}

func NewHermesACP() *HermesACP {
	return &HermesACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(hermesACPBin),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *HermesACP) ID() string          { return "hermes-acp" }
func (a *HermesACP) Name() string        { return "Hermes ACP Agent" }
func (a *HermesACP) DisplayName() string { return "Hermes" }
func (a *HermesACP) Description() string {
	return "Nous Research Hermes self-improving coding agent using the ACP protocol over stdin/stdout."
}
func (a *HermesACP) Enabled() bool     { return true }
func (a *HermesACP) DisplayOrder() int { return 19 }

func (a *HermesACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return hermesACPLogoDark
	}
	return hermesACPLogoLight
}

func (a *HermesACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(hermesACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *HermesACP) BuildCommand(opts CommandOptions) Command {
	return Cmd(hermesACPBin, "acp").Build()
}

func (a *HermesACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd(hermesACPBin, "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.hermes",
		},
	}
}

func (a *HermesACP) RemoteAuth() *RemoteAuth { return nil }

func (a *HermesACP) InstallScript() string {
	// Hermes ships a multi-step Python/uv installer that may need system
	// packages and a one-time `hermes setup` afterwards — not a clean
	// one-shot like `npm install -g`. Point users at the upstream docs
	// instead of pretending the install button can handle it.
	return "Install Hermes from https://hermes-agent.nousresearch.com/docs/getting-started"
}

func (a *HermesACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *HermesACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(hermesACPBin, "acp"),
	}
}

func (a *HermesACP) BillingType() usage.BillingType { return defaultBillingType() }
