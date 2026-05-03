package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/codex_light.svg
var codexACPLogoLight []byte

//go:embed logos/codex_dark.svg
var codexACPLogoDark []byte

const codexACPPkg = "@zed-industries/codex-acp"

var (
	_ Agent            = (*CodexACP)(nil)
	_ PassthroughAgent = (*CodexACP)(nil)
	_ InferenceAgent   = (*CodexACP)(nil)
)

// CodexACP implements Agent for the Zed Industries codex-acp package.
// It speaks the ACP protocol (JSON-RPC 2.0 over stdin/stdout) via a Rust binary
// wrapping OpenAI Codex. Used for A/B comparison against the native Codex agent.
type CodexACP struct {
	StandardPassthrough
}

func NewCodexACP() *CodexACP {
	return &CodexACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", "@openai/codex", "--full-auto"),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *CodexACP) ID() string          { return "codex-acp" }
func (a *CodexACP) Name() string        { return "Codex ACP Agent" }
func (a *CodexACP) DisplayName() string { return "Codex" }
func (a *CodexACP) Description() string {
	return "OpenAI Codex coding agent using the ACP protocol via the Zed Industries bridge."
}
func (a *CodexACP) Enabled() bool     { return true }
func (a *CodexACP) DisplayOrder() int { return 2 }

func (a *CodexACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return codexACPLogoDark
	}
	return codexACPLogoLight
}

func (a *CodexACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the CLI binary on PATH. Auth state is surfaced later by the
	// ACP probe (session/new returns auth_required if the user hasn't logged in).
	result, err := Detect(ctx, WithCommand("codex-acp"), WithCommand("codex"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	return result, nil
}

func (a *CodexACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", codexACPPkg).Build()
}

func (a *CodexACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:       "kandev/multi-agent",
		Tag:         "latest",
		Cmd:         Cmd("npx", "-y", codexACPPkg).Build(),
		WorkingDir:  "{workspace}",
		RequiredEnv: []string{"OPENAI_API_KEY"},
		Env:         map[string]string{},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
			{Source: "{home}/.codex", Target: "/root/.codex"},
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *CodexACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy auth files",
				SourceFiles: map[string][]string{
					"darwin": {".codex/auth.json", ".codex/config.toml"},
					"linux":  {".codex/auth.json", ".codex/config.toml"},
				},
				TargetRelDir: ".codex",
			},
			{
				Type:   "env",
				EnvVar: "OPENAI_API_KEY",
			},
		},
	}
}

func (a *CodexACP) InstallScript() string {
	return "npm install -g " + codexACPPkg
}

func (a *CodexACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *CodexACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", codexACPPkg),
	}
}
