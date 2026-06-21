package agents

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/antigravity_light.svg
var antigravityLogoLight []byte

//go:embed logos/antigravity_dark.svg
var antigravityLogoDark []byte

const antigravityBinary = "agy"

// antigravityModelsSubcommand lists available models, one human-readable name
// per line (e.g. "Gemini 3.5 Flash (Medium)"). The CLI exposes no separate ID
// or JSON output, so the printed name is also the value accepted by --model.
const antigravityModelsSubcommand = "models"

// antigravityModelsTimeout bounds the `agy models` subprocess so a hung CLI
// can't stall agent discovery (which runs under its own 15s budget).
const antigravityModelsTimeout = 5 * time.Second

var (
	_ Agent                 = (*Antigravity)(nil)
	_ PassthroughAgent      = (*Antigravity)(nil)
	_ LoginAgent            = (*Antigravity)(nil)
	_ PassthroughTrustAgent = (*Antigravity)(nil)
)

// antigravityTrustValue is the value `agy` writes in trustedFolders.json to
// mark a directory trusted (skipping the interactive trust prompt).
const antigravityTrustValue = "TRUST_FOLDER"

// antigravityShimBinary is the ACP shim (cmd/antigravity-acp) that speaks ACP
// toward agentctl and drives `agy --print` underneath, giving Antigravity a
// structured chat dialog. Kandev launches this — not `agy` directly — for ACP
// sessions; the raw `agy` TUI is still reachable via the CLI-passthrough toggle.
const antigravityShimBinary = "antigravity-acp"

// Antigravity implements Google's Antigravity CLI. `agy` itself speaks no ACP,
// so Kandev runs it two ways: by default through the antigravity-acp ACP shim
// (chat dialog), or — when the user enables CLI passthrough — as the raw `agy`
// TUI in a terminal.
type Antigravity struct {
	StandardPassthrough
	// binaryPath is the absolute host path to the antigravity-acp shim, set by
	// the registry at startup (configureAntigravity). Empty falls back to the
	// bare name resolved on PATH (containers/SSH/e2e).
	binaryPath string
}

// SetBinaryPath sets the absolute path to the antigravity-acp shim binary.
func (a *Antigravity) SetBinaryPath(path string) { a.binaryPath = path }

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
	// Antigravity is passthrough-only, so the host utility manager never probes
	// it for models via ACP. Source the model list from `agy models` instead.
	// A listing failure is non-fatal: the agent is still installed and usable,
	// the model picker just falls back to a free-text field.
	if result.Available {
		if models := listAntigravityModels(ctx); len(models) > 0 {
			result.Models = models
		}
	}
	return result, nil
}

// listAntigravityModels runs `agy models` and parses its output. Returns nil on
// any error so discovery degrades gracefully rather than failing.
func listAntigravityModels(ctx context.Context) []DiscoveredModel {
	ctx, cancel := context.WithTimeout(ctx, antigravityModelsTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, antigravityBinary, antigravityModelsSubcommand).Output()
	if err != nil {
		return nil
	}
	return parseAntigravityModels(string(out))
}

// parseAntigravityModels turns the line-per-model output of `agy models` into
// DiscoveredModel entries. Blank lines are skipped; each remaining line is both
// the ID (the value passed to --model) and the display name.
func parseAntigravityModels(out string) []DiscoveredModel {
	var models []DiscoveredModel
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		models = append(models, DiscoveredModel{ID: name, Name: name})
	}
	return models
}

// BuildCommand returns the ACP-shim invocation used to launch Antigravity. On
// host (standalone) runtimes it prefers the absolute shim path resolved at
// startup; containerized/SSH runtimes use the bare name resolved on PATH inside
// the execution environment. The selected model is passed so the shim can use
// it as the session default.
func (a *Antigravity) BuildCommand(opts CommandOptions) Command {
	binary := antigravityShimBinary
	if !opts.Runtime.IsContainerized() && opts.Runtime != agentruntime.RuntimeSSH && a.binaryPath != "" {
		binary = a.binaryPath
	}
	b := Cmd(binary)
	if opts.Model != "" {
		b = b.Flag("--model", opts.Model)
	}
	return b.Build()
}

// Runtime returns the container/process runtime config for Antigravity. The
// agent process is the antigravity-acp ACP shim (which drives `agy` underneath).
func (a *Antigravity) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:           "kandev/multi-agent",
		Tag:             "latest",
		Cmd:             Cmd(antigravityShimBinary).Build(),
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

// TrustedFoldersFile returns the path to `agy`'s trusted-folders registry and
// the value that marks a folder trusted. Kandev seeds the task workspace here
// so the CLI does not block on its first-run "trust this folder?" prompt.
func (a *Antigravity) TrustedFoldersFile() (string, string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", "", false
	}
	return filepath.Join(home, ".gemini", "trustedFolders.json"), antigravityTrustValue, true
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
