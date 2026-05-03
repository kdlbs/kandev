package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/cursor_acp_light.svg
var cursorACPLogoLight []byte

//go:embed logos/cursor_acp_dark.svg
var cursorACPLogoDark []byte

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
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("cursor-agent"),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
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
	return Cmd("cursor-agent", "acp").Build()
}

func (a *CursorACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("cursor-agent", "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.cursor",
		},
	}
}

func (a *CursorACP) RemoteAuth() *RemoteAuth { return nil }

func (a *CursorACP) InstallScript() string {
	return "Install Cursor CLI from https://cursor.com/cli"
}

func (a *CursorACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *CursorACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("cursor-agent", "acp"),
	}
}
