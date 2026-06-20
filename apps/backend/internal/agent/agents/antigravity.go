package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/antigravity_light.svg
var antigravityLogoLight []byte

//go:embed logos/antigravity_dark.svg
var antigravityLogoDark []byte

const antigravityBinary = "agy"

var (
	_ Agent                = (*Antigravity)(nil)
	_ PassthroughAgent     = (*Antigravity)(nil)
	_ PassthroughOnlyAgent = (*Antigravity)(nil)
	_ LoginAgent           = (*Antigravity)(nil)
)

// Antigravity implements Google's Antigravity CLI. The public CLI surface is a
// TUI/headless command, so Kandev integrates it through passthrough mode rather
// than claiming an ACP endpoint that the CLI does not document.
type Antigravity struct {
	StandardPassthrough
}

// NewAntigravity builds the Antigravity agent with its passthrough profile:
// the `agy` command, model/resume flags, prompt auto-injection, and the
// Gemini-config MCP strategy.
func NewAntigravity() *Antigravity {
	return &Antigravity{
		StandardPassthrough: StandardPassthrough{
			PermSettings: antigravityPermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand(antigravityBinary),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("--continue"),
				SessionResumeFlag: NewParam("--conversation"),
				MCPStrategy:       mcpconfig.AntigravityStrategy{},
				AutoInjectPrompt:  true,
				SubmitSequence:    "\r",
			},
		},
	}
}

// ID returns the stable registry identifier for the agent.
func (a *Antigravity) ID() string { return "antigravity" }

// Name returns the agent's full human-readable name.
func (a *Antigravity) Name() string { return "Antigravity CLI Agent" }

// DisplayName returns the short label shown in the UI agent picker.
func (a *Antigravity) DisplayName() string { return "Antigravity" }

// Description returns a one-line summary of the agent.
func (a *Antigravity) Description() string {
	return "Google Antigravity CLI coding agent using terminal passthrough."
}

// Enabled reports whether the agent is selectable by default.
func (a *Antigravity) Enabled() bool { return true }

// DisplayOrder returns the agent's sort position in the picker.
func (a *Antigravity) DisplayOrder() int { return 8 }

// PassthroughOnly marks Antigravity as passthrough-only so profiles default to
// CLI passthrough and interactive MCP tools are not registered.
func (a *Antigravity) PassthroughOnly() bool { return true }

// Logo returns the embedded SVG logo for the requested theme variant.
func (a *Antigravity) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return antigravityLogoDark
	}
	return antigravityLogoLight
}

// IsInstalled detects the `agy` binary on PATH and reports MCP support, the
// shared Gemini MCP config path, and session-resume capability.
func (a *Antigravity) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(antigravityBinary))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.MCPConfigPaths = []string{".gemini/config/mcp_config.json"}
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

// BuildCommand returns the bare `agy` invocation used to launch the CLI.
func (a *Antigravity) BuildCommand(_ CommandOptions) Command {
	return Cmd(antigravityBinary).Build()
}

// Runtime returns the container/process runtime config for Antigravity,
// including the multi-agent image, resource limits, skill dirs, and the
// Gemini session directory used for native resume.
func (a *Antigravity) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:           "kandev/multi-agent",
		Tag:             "latest",
		Cmd:             Cmd(antigravityBinary).Build(),
		WorkingDir:      "{workspace}",
		Env:             map[string]string{"ANTIGRAVITY_AGENT": "1"},
		Mounts:          []MountTemplate{{Source: "{workspace}", Target: "/workspace"}},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: ".agents/skills",
		UserSkillDir:    ".gemini/config/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.gemini/antigravity-cli",
		},
	}
}

// RemoteAuth lists the Google account and Antigravity config files copied to a
// remote executor so the CLI is authenticated there.
func (a *Antigravity) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy Google auth files",
				SourceFiles: map[string][]string{
					"darwin": {
						".gemini/oauth_creds.json",
						".gemini/google_accounts.json",
						".gemini/settings.json",
					},
					"linux": {
						".gemini/oauth_creds.json",
						".gemini/google_accounts.json",
						".gemini/settings.json",
					},
				},
				TargetRelDir: ".gemini",
			},
			{
				Type:  "files",
				Label: "Copy Antigravity config files",
				SourceFiles: map[string][]string{
					"darwin": {".gemini/config/config.json", ".gemini/config/mcp_config.json"},
					"linux":  {".gemini/config/config.json", ".gemini/config/mcp_config.json"},
				},
				TargetRelDir: ".gemini/config",
			},
		},
	}
}

// LoginCommand runs the bare CLI so the user completes the Google sign-in flow,
// then quits.
func (a *Antigravity) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{antigravityBinary},
		Description: "Sign in with your Google account in the Antigravity CLI, then quit.",
	}
}

// InstallScript returns the shell command that installs the Antigravity CLI.
func (a *Antigravity) InstallScript() string {
	return "curl -fsSL https://antigravity.google/cli/install.sh | bash"
}

// BillingType reports how usage for this agent is billed.
func (a *Antigravity) BillingType() usage.BillingType { return defaultBillingType() }

// PermissionSettings returns the toggleable CLI permission flags Antigravity
// supports (skip-permissions, sandbox).
func (a *Antigravity) PermissionSettings() map[string]PermissionSetting {
	return antigravityPermSettings
}

// antigravityPermSettings are the permission toggles exposed for the agent,
// each mapped to the CLI flag it applies.
var antigravityPermSettings = map[string]PermissionSetting{
	PermissionKeyDangerouslySkipPermissions: {
		Supported:   true,
		Default:     false,
		Label:       "Skip permission prompts",
		Description: "Pass --dangerously-skip-permissions so Antigravity does not prompt for tool approvals.",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--dangerously-skip-permissions",
	},
	"enable_sandbox": {
		Supported:   true,
		Default:     false,
		Label:       "Sandbox terminal commands",
		Description: "Run terminal commands with Antigravity sandbox restrictions enabled (--sandbox).",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--sandbox",
	},
}
