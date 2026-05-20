//nolint:dupl,goconst // Native-binary ACP agents (Cursor, Kimi, Kiro, Omp, Qoder, Trae) follow the same minimal scaffold; differences are the binary name and subcommand. Shared literals (`CLI Passthrough`, `Show terminal directly instead of chat interface`, `{workspace}`) live in every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/omp_acp_light.svg
var ompACPLogoLight []byte

//go:embed logos/omp_acp_dark.svg
var ompACPLogoDark []byte

const ompACPBin = "omp"

var (
	_ Agent            = (*OmpACP)(nil)
	_ PassthroughAgent = (*OmpACP)(nil)
	_ InferenceAgent   = (*OmpACP)(nil)
)

// OmpACP implements Agent for the Oh My Pi (omp) coding agent via its native
// `omp acp` subcommand. Distributed as a single binary (bun-installed); BYO
// API key — omp reads any of the dozen provider env vars (ANTHROPIC_API_KEY,
// OPENAI_API_KEY, GEMINI_API_KEY, ...) directly.
type OmpACP struct {
	StandardPassthrough
}

func NewOmpACP() *OmpACP {
	return &OmpACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand(ompACPBin),
				ModelFlag:         NewParam("--model", "{model}"),
				ResumeFlag:        NewParam("-c"),
				SessionResumeFlag: NewParam("--resume"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *OmpACP) ID() string          { return "omp-acp" }
func (a *OmpACP) Name() string        { return "Oh My Pi ACP Agent" }
func (a *OmpACP) DisplayName() string { return ompACPBin }
func (a *OmpACP) Description() string {
	return "Oh My Pi (omp) coding agent using the ACP protocol via the `omp acp` subcommand."
}
func (a *OmpACP) Enabled() bool     { return true }
func (a *OmpACP) DisplayOrder() int { return 18 }

func (a *OmpACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return ompACPLogoDark
	}
	return ompACPLogoLight
}

func (a *OmpACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(ompACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OmpACP) BuildCommand(opts CommandOptions) Command {
	return Cmd(ompACPBin, "acp").Build()
}

func (a *OmpACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             Cmd(ompACPBin, "acp").Build(),
		WorkingDir:      "{workspace}",
		Env:             map[string]string{},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: ".omp/skills",
		UserSkillDir:    ".omp/agent/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.omp",
		},
	}
}

func (a *OmpACP) RemoteAuth() *RemoteAuth { return nil }

func (a *OmpACP) InstallScript() string {
	return "bun install -g @oh-my-pi/pi-coding-agent"
}

func (a *OmpACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *OmpACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(ompACPBin, "acp"),
	}
}

func (a *OmpACP) BillingType() usage.BillingType { return defaultBillingType() }
