package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/opencode_light.svg
var opencodeACPLogoLight []byte

//go:embed logos/opencode_dark.svg
var opencodeACPLogoDark []byte

const opencodeACPPkg = "opencode-ai"

var (
	_ Agent            = (*OpenCodeACP)(nil)
	_ PassthroughAgent = (*OpenCodeACP)(nil)
	_ InferenceAgent   = (*OpenCodeACP)(nil)
)

// OpenCodeACP is the ACP protocol variant of OpenCode.
// Uses JSON-RPC 2.0 over stdin/stdout via "opencode acp" instead of REST/SSE.
type OpenCodeACP struct {
	StandardPassthrough
}

func NewOpenCodeACP() *OpenCodeACP {
	return &OpenCodeACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("opencode"),
				ModelFlag:      NewParam("--model", "{model}"),
				PromptFlag:     NewParam("--prompt", "{prompt}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				ResumeFlag:     NewParam("-c"),
			},
		},
	}
}

func (a *OpenCodeACP) ID() string          { return "opencode-acp" }
func (a *OpenCodeACP) Name() string        { return "OpenCode AI Agent (ACP)" }
func (a *OpenCodeACP) DisplayName() string { return "OpenCode" }
func (a *OpenCodeACP) Description() string {
	return "OpenCode coding agent using ACP protocol over stdin/stdout."
}
func (a *OpenCodeACP) Enabled() bool     { return true }
func (a *OpenCodeACP) DisplayOrder() int { return 4 }

func (a *OpenCodeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return opencodeACPLogoDark
	}
	return opencodeACPLogoLight
}

func (a *OpenCodeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the opencode CLI on PATH. Auth state is surfaced later by
	// the ACP probe, not by scanning ~/.opencode.
	result, err := Detect(ctx, WithCommand("opencode"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OpenCodeACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("opencode", "acp").Build()
}

func (a *OpenCodeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("opencode", "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.opencode",
		},
	}
}

func (a *OpenCodeACP) RemoteAuth() *RemoteAuth { return nil }

func (a *OpenCodeACP) InstallScript() string {
	return "npm install -g " + opencodeACPPkg
}

func (a *OpenCodeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *OpenCodeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("opencode", "acp"),
	}
}
