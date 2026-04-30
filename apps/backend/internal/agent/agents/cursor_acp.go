package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/cursor_light.svg
var cursorACPLogoLight []byte

//go:embed logos/cursor_dark.svg
var cursorACPLogoDark []byte

const cursorACPPkg = "cursor-acp"

// cursorFullAutoShell wraps the cursor-acp bridge so the underlying cursor-agent
// runs in full-auto mode. The bridge hardcodes its argv and only honors the
// CURSOR_AGENT_EXECUTABLE env to override the binary, so we point that env at a
// shim that re-exec's cursor-agent with --force --trust --sandbox disabled.
const cursorFullAutoShell = `set -e; SHIM=$(mktemp -t cursor-agent-shim.XXXXXX); printf '#!/bin/sh\nexec cursor-agent --force --trust --sandbox disabled "$@"\n' > "$SHIM"; chmod +x "$SHIM"; CURSOR_AGENT_EXECUTABLE="$SHIM" exec npx -y ` + cursorACPPkg

var (
	_ Agent            = (*CursorACP)(nil)
	_ PassthroughAgent = (*CursorACP)(nil)
	_ InferenceAgent   = (*CursorACP)(nil)
)

// CursorACP implements Agent for the cursor-acp npm package, which bridges
// the Cursor CLI agent (cursor-agent) to ACP (JSON-RPC 2.0 over stdin/stdout).
type CursorACP struct {
	StandardPassthrough
}

func NewCursorACP() *CursorACP {
	return &CursorACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("cursor-agent", "-f"),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				ResumeFlag:     NewParam("--resume"),
			},
		},
	}
}

func (a *CursorACP) ID() string          { return "cursor-acp" }
func (a *CursorACP) Name() string        { return "Cursor ACP Agent" }
func (a *CursorACP) DisplayName() string { return "Cursor" }
func (a *CursorACP) Description() string {
	return "Cursor coding agent using the ACP protocol via the cursor-acp bridge."
}
func (a *CursorACP) Enabled() bool     { return true }
func (a *CursorACP) DisplayOrder() int { return 8 }

func (a *CursorACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return cursorACPLogoDark
	}
	return cursorACPLogoLight
}

func (a *CursorACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the underlying cursor-agent CLI on PATH. The ACP bridge itself
	// is fetched on demand via npx; auth state is surfaced by the ACP probe.
	result, err := Detect(ctx, WithCommand("cursor-agent"))
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
	return Cmd("sh", "-c", cursorFullAutoShell).Build()
}

func (a *CursorACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:       "kandev/multi-agent",
		Tag:         "latest",
		Cmd:         Cmd("sh", "-c", cursorFullAutoShell).Build(),
		WorkingDir:  "{workspace}",
		RequiredEnv: []string{}, // Auth via CURSOR_API_KEY or ~/.cursor credentials (see RemoteAuth)
		Env:         map[string]string{},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
			{Source: "{home}/.cursor", Target: "/root/.cursor"},
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
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
				Type:  "files",
				Label: "Copy auth files",
				SourceFiles: map[string][]string{
					"darwin": {".cursor/cli-config.json"},
					"linux":  {".cursor/cli-config.json"},
				},
				TargetRelDir: ".cursor",
			},
			{
				Type:      "env",
				EnvVar:    "CURSOR_API_KEY",
				SetupHint: "Generate an API key from cursor.com/settings and export it as CURSOR_API_KEY",
			},
		},
	}
}

func (a *CursorACP) InstallScript() string {
	return "npm install -g " + cursorACPPkg
}

func (a *CursorACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *CursorACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("sh", "-c", cursorFullAutoShell),
	}
}
