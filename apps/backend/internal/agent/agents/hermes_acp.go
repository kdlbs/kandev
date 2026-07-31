//nolint:dupl // Native-binary ACP agents (Kiro, Qoder, Hermes, ...) follow the same minimal scaffold; differences are the binary name, argv, and auth surface.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/hermes_light.svg
var hermesLogoLight []byte

//go:embed logos/hermes_dark.svg
var hermesLogoDark []byte

const hermesBin = "hermes"

var (
	_ Agent            = (*HermesACP)(nil)
	_ PassthroughAgent = (*HermesACP)(nil)
	_ InferenceAgent   = (*HermesACP)(nil)
	_ LoginAgent       = (*HermesACP)(nil)
)

// HermesACP implements Agent for Nous Research's Hermes Agent using native
// ACP over stdin/stdout (`hermes acp`). Hermes is installed as a Python
// package by the official installer, which bundles the `[acp]` extra, so the
// launch command is the bare `hermes` binary.
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
				PassthroughCmd: NewCommand(hermesBin),
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
	return "Nous Research Hermes coding agent using the ACP protocol via hermes acp."
}
func (a *HermesACP) Enabled() bool     { return true }
func (a *HermesACP) DisplayOrder() int { return 21 }

func (a *HermesACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return hermesLogoDark
	}
	return hermesLogoLight
}

func (a *HermesACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(hermesBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *HermesACP) BuildCommand(_ CommandOptions) Command {
	return Cmd(hermesBin, "acp").Build()
}

func (a *HermesACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:        Cmd(hermesBin, "acp").Build(),
		WorkingDir: "{workspace}",
		Env: map[string]string{
			// Hermes starts every globally configured MCP server from
			// ~/.hermes/config.yaml before its ACP loop begins. Kandev owns
			// MCP itself and passes session servers through ACP session/new,
			// so the global startup is redundant — and an unrelated slow
			// server would otherwise delay initialize. Only the global
			// discovery is skipped; session/new MCP servers still register.
			"HERMES_ACP_SKIP_CONFIGURED_MCP": "1",
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		UserSkillDir:   ".hermes/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.hermes",
			SessionDirTarget:    "/root/.hermes",
		},
	}
}

func (a *HermesACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy auth files",
				SourceFiles: map[string][]string{
					"darwin": {".hermes/.env", ".hermes/config.yaml"},
					"linux":  {".hermes/.env", ".hermes/config.yaml"},
				},
				// Credentials + provider/model config only — sessions, skills,
				// checkpoints, and state.db are deliberately excluded.
				TargetRelDir: ".hermes",
			},
		},
	}
}

// hermes model is the interactive provider/model setup wizard (add providers,
// OAuth flows, API keys). Credentials land in ~/.hermes/.env + config.yaml.
func (a *HermesACP) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{hermesBin, "model"},
		Description: "Configure provider credentials and select a model.",
	}
}

func (a *HermesACP) InstallScript() string {
	return "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash"
}

func (a *HermesACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *HermesACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(hermesBin, "acp"),
	}
}

func (a *HermesACP) BillingType() usage.BillingType { return defaultBillingType() }
