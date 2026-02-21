package agents

import (
	"context"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

var _ Agent = (*OpenCodeACP)(nil)

// OpenCodeACP is the ACP protocol variant of OpenCode.
// Uses JSON-RPC 2.0 over stdin/stdout via "opencode acp" instead of REST/SSE.
type OpenCodeACP struct{}

func NewOpenCodeACP() *OpenCodeACP { return &OpenCodeACP{} }

func (a *OpenCodeACP) ID() string          { return "opencode-acp" }
func (a *OpenCodeACP) Name() string        { return "OpenCode AI Agent (ACP)" }
func (a *OpenCodeACP) DisplayName() string { return "OpenCode (ACP)" }
func (a *OpenCodeACP) Description() string {
	return "OpenCode coding agent using ACP protocol over stdin/stdout."
}
func (a *OpenCodeACP) Enabled() bool     { return true }
func (a *OpenCodeACP) DisplayOrder() int { return 5 }

func (a *OpenCodeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return opencodeLogoDark
	}
	return opencodeLogoLight
}

func (a *OpenCodeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Same detection as SSE variant — both share the same binary
	install := OSPaths{
		Linux: []string{"~/.opencode", "~/.config/opencode"},
		MacOS: []string{"~/.opencode", "~/.config/opencode", "~/Library/Application Support/ai.opencode.desktop"},
	}

	result, err := Detect(ctx, WithFileExists(install.Resolve()...))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = install.Expanded()
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OpenCodeACP) DefaultModel() string { return "opencode/gpt-5-nano" }

func (a *OpenCodeACP) ListModels(ctx context.Context) (*ModelList, error) {
	return (&OpenCode{}).ListModels(ctx)
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

func (a *OpenCodeACP) PermissionSettings() map[string]PermissionSetting {
	return opencodePermSettings
}
