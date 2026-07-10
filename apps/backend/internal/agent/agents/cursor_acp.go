//nolint:dupl // Native-binary ACP agents (Cursor, Kimi, Kiro, Qoder, Trae) follow the same minimal scaffold; differences are the binary name and subcommand.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/cursor_acp_light.svg
var cursorACPLogoLight []byte

//go:embed logos/cursor_acp_dark.svg
var cursorACPLogoDark []byte

const cursorACPBin = "cursor-agent"

// cursorPermSettings maps a curated CLI flag to cursor-agent's --force switch.
// In ACP mode Cursor emits session/request_permission for commands off its
// allowlist; --force ("Run Everything") suppresses those prompts. Default off
// because --force runs everything unsandboxed. Universal agentctl auto-approve
// (PermissionKeyAutoApprove) is added by CatalogPermissionSettings, not here.
var cursorPermSettings = map[string]PermissionSetting{
	PermissionKeyCursorForce: {
		Supported:   true,
		Default:     false,
		Label:       "Cursor run everything (--force)",
		Description: "Append cursor-agent --force so the CLI stops prompting for non-allowlisted commands (unsandboxed).",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--force",
	},
}

var (
	_ Agent            = (*CursorACP)(nil)
	_ PassthroughAgent = (*CursorACP)(nil)
	_ InferenceAgent   = (*CursorACP)(nil)
)

// CursorACP implements Agent for Cursor's CLI via its native ACP mode.
// Cursor isn't published to npm — users must install the cursor-agent binary
// from Cursor (Pro subscription required).
type CursorACP struct {
	StandardPassthrough
}

func NewCursorACP() *CursorACP {
	return &CursorACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: cursorPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(cursorACPBin),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				// cursor-agent has no MCP flag/env; write or merge a
				// project-local .cursor/mcp.json into the worktree.
				MCPStrategy: mcpconfig.CursorStrategy{},
			},
		},
	}
}

func (a *CursorACP) ID() string          { return "cursor-acp" }
func (a *CursorACP) Name() string        { return "Cursor ACP Agent" }
func (a *CursorACP) DisplayName() string { return "Cursor" }
func (a *CursorACP) Description() string {
	return "Cursor CLI coding agent (cursor-agent) using the ACP protocol. Requires a Cursor Pro subscription."
}
func (a *CursorACP) Enabled() bool     { return true }
func (a *CursorACP) DisplayOrder() int { return 13 }

func (a *CursorACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return cursorACPLogoDark
	}
	return cursorACPLogoLight
}

func (a *CursorACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(cursorACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *CursorACP) BuildCommand(opts CommandOptions) Command {
	// --force is applied via profile cli_flags (seeded from cursor_force), not
	// PermissionValues, so auto_approve stays agentctl-only.
	return Cmd(cursorACPBin, "acp").Build()
}

func (a *CursorACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:                Cmd(cursorACPBin, "acp").Build(),
		WorkingDir:         "{workspace}",
		Env:                map[string]string{},
		ResourceLimits:     ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:           agent.ProtocolACP,
		UserSkillDir:       ".cursor/skills",
		ProjectMCPStrategy: mcpconfig.CursorStrategy{},
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.cursor",
		},
	}
}

func (a *CursorACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:      "env",
				EnvVar:    "CURSOR_API_KEY",
				SetupHint: "Create an API key at https://cursor.com/dashboard/integrations (Cursor Pro).",
			},
		},
	}
}

// cursor-agent isn't on npm. The official installer drops the binary into
// ~/.local/bin; make sure that dir is on PATH for the rest of the prepare
// script and for future shells on the sprite.
func (a *CursorACP) InstallScript() string {
	return `set -e
tmp="$(mktemp)"
curl -fsS https://cursor.com/install -o "$tmp"
bash "$tmp"
rm -f "$tmp"
export PATH="$HOME/.local/bin:$PATH"
grep -qxF 'export PATH="$HOME/.local/bin:$PATH"' "$HOME/.bashrc" 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"`
}

func (a *CursorACP) PermissionSettings() map[string]PermissionSetting {
	return cursorPermSettings
}

func (a *CursorACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(cursorACPBin, "acp"),
	}
}

func (a *CursorACP) BillingType() usage.BillingType { return defaultBillingType() }
