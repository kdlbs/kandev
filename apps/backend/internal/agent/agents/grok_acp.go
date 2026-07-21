//nolint:dupl // Native-binary ACP agents follow the same minimal scaffold; differences are the binary name, argv, and auth surface.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/grok_acp_light.svg
var grokACPLogoLight []byte

//go:embed logos/grok_acp_dark.svg
var grokACPLogoDark []byte

const grokACPBin = "grok"

// grokACPArgv is the ACP / inference launch argv. --no-auto-update must
// precede the agent subcommand so background self-update checks do not run
// for Kandev-managed sessions.
var grokACPArgv = []string{grokACPBin, "--no-auto-update", "agent", "stdio"}

var (
	_ Agent          = (*GrokACP)(nil)
	_ InferenceAgent = (*GrokACP)(nil)
	_ LoginAgent     = (*GrokACP)(nil)
)

// GrokACP implements Agent for xAI's Grok Build CLI using native ACP over
// stdin/stdout. Raw TUI passthrough is intentionally deferred (requires a
// project-scoped PassthroughMCPStrategy — see ADR 0014).
type GrokACP struct{}

func NewGrokACP() *GrokACP {
	return &GrokACP{}
}

func (a *GrokACP) ID() string          { return "grok-acp" }
func (a *GrokACP) Name() string        { return "Grok ACP Agent" }
func (a *GrokACP) DisplayName() string { return "Grok" }
func (a *GrokACP) Description() string {
	return "xAI Grok coding agent using the ACP protocol over stdin/stdout."
}
func (a *GrokACP) Enabled() bool     { return true }
func (a *GrokACP) DisplayOrder() int { return 20 }

func (a *GrokACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return grokACPLogoDark
	}
	return grokACPLogoLight
}

func (a *GrokACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(grokACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *GrokACP) BuildCommand(opts CommandOptions) Command {
	return Cmd(grokACPArgv...).Build()
}

func (a *GrokACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             Cmd(grokACPArgv...).Build(),
		WorkingDir:      "{workspace}",
		RequiredEnv:     []string{}, // Cached OAuth or optional XAI_API_KEY — not a launch prerequisite.
		Env:             map[string]string{},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: ".grok/skills",
		UserSkillDir:    ".grok/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.grok",
			SessionDirTarget:    "/root/.grok",
		},
	}
}

func (a *GrokACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy auth files",
				SourceFiles: map[string][]string{
					"darwin": {".grok/auth.json"},
					"linux":  {".grok/auth.json"},
				},
				// Only auth.json — do not copy config.toml, sessions, logs, or caches.
				TargetRelDir: ".grok",
			},
			{
				Type:   "env",
				EnvVar: "XAI_API_KEY",
			},
		},
	}
}

// LoginCommand uses device-code auth so headless/container/SSH environments
// can complete sign-in without a local browser.
func (a *GrokACP) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{grokACPBin, "login", "--device-auth"},
		Description: "Sign in with your xAI / Grok account.",
	}
}

func (a *GrokACP) InstallScript() string {
	return "npm install -g @xai-official/grok"
}

func (a *GrokACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *GrokACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(grokACPArgv...),
	}
}

func (a *GrokACP) BillingType() usage.BillingType { return defaultBillingType() }
