package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/claude_code_light.svg
var claudeACPLogoLight []byte

//go:embed logos/claude_code_dark.svg
var claudeACPLogoDark []byte

const claudeACPPkg = "@agentclientprotocol/claude-agent-acp"

var (
	_ Agent            = (*ClaudeACP)(nil)
	_ PassthroughAgent = (*ClaudeACP)(nil)
	_ InferenceAgent   = (*ClaudeACP)(nil)
)

// ClaudeACP implements Agent for the agentclientprotocol claude-agent-acp package.
// It speaks the ACP protocol (JSON-RPC 2.0 over stdin/stdout) and wraps the
// Claude Agent SDK. Used for A/B comparison against the stream-json Claude Code agent.
type ClaudeACP struct {
	StandardPassthrough
}

func NewClaudeACP() *ClaudeACP {
	return &ClaudeACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", "@anthropic-ai/claude-code", "--verbose"),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("-c"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *ClaudeACP) ID() string          { return "claude-acp" }
func (a *ClaudeACP) Name() string        { return "Claude ACP Agent" }
func (a *ClaudeACP) DisplayName() string { return "Claude" }
func (a *ClaudeACP) Description() string {
	return "Anthropic Claude coding agent using the ACP protocol via the agentclientprotocol bridge."
}
func (a *ClaudeACP) Enabled() bool     { return true }
func (a *ClaudeACP) DisplayOrder() int { return 1 }

func (a *ClaudeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return claudeACPLogoDark
	}
	return claudeACPLogoLight
}

func (a *ClaudeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the Claude Code CLI on PATH. Auth state is surfaced later by
	// the ACP probe, not by the presence of ~/.claude.json.
	result, err := Detect(ctx, WithCommand("claude"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *ClaudeACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", claudeACPPkg).Build()
}

func (a *ClaudeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:       "kandev/multi-agent",
		Tag:         "latest",
		Cmd:         Cmd("npx", "-y", claudeACPPkg).Build(),
		WorkingDir:  "{workspace}",
		RequiredEnv: []string{}, // Auth via ANTHROPIC_API_KEY or OAuth credentials file (see RemoteAuth)
		Env: map[string]string{
			"MCP_TIMEOUT": "7200000",
		},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.claude",
		},
	}
}

func (a *ClaudeACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:      "env",
				EnvVar:    "CLAUDE_CODE_OAUTH_TOKEN",
				SetupHint: "Run `claude setup-token` to generate a long-lived OAuth token",
				SetupScript: `mkdir -p "${HOME}/.claude"
cat > "${HOME}/.claude/.credentials.json" <<CREDS
{"claudeAiOauth":{"accessToken":"${CLAUDE_CODE_OAUTH_TOKEN}","expiresAt":4102444800000}}
CREDS
cat > "${HOME}/.claude.json" <<'JSON'
{"hasCompletedOnboarding":true}
JSON
chmod 600 "${HOME}/.claude/.credentials.json"
chmod 600 "${HOME}/.claude.json"`,
			},
		},
	}
}

func (a *ClaudeACP) InstallScript() string {
	return "npm install -g " + claudeACPPkg
}

func (a *ClaudeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *ClaudeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", claudeACPPkg),
	}
}
