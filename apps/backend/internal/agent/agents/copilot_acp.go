package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/copilot_light.svg
var copilotACPLogoLight []byte

//go:embed logos/copilot_dark.svg
var copilotACPLogoDark []byte

const copilotACPPkg = "@github/copilot"

var (
	_ Agent            = (*CopilotACP)(nil)
	_ PassthroughAgent = (*CopilotACP)(nil)
	_ InferenceAgent   = (*CopilotACP)(nil)
)

// CopilotACP implements Agent for GitHub Copilot using ACP protocol mode.
// It runs the same @github/copilot CLI with the --acp flag, speaking standard
// ACP over stdin/stdout. Used for A/B comparison against the Go SDK-based Copilot agent.
type CopilotACP struct {
	StandardPassthrough
}

func NewCopilotACP() *CopilotACP {
	return &CopilotACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: copilotPermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", copilotACPPkg),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("--continue"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *CopilotACP) ID() string          { return "copilot-acp" }
func (a *CopilotACP) Name() string        { return "Copilot ACP Agent" }
func (a *CopilotACP) DisplayName() string { return "Copilot" }
func (a *CopilotACP) Description() string {
	return "GitHub Copilot coding agent using the ACP protocol over stdin/stdout."
}
func (a *CopilotACP) Enabled() bool     { return true }
func (a *CopilotACP) DisplayOrder() int { return 6 }

func (a *CopilotACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return copilotACPLogoDark
	}
	return copilotACPLogoLight
}

func (a *CopilotACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the copilot CLI on PATH. Auth state is surfaced later by
	// the ACP probe, not by scanning ~/.copilot.
	result, err := Detect(ctx, WithCommand("copilot"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *CopilotACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", copilotACPPkg, "--acp").Build()
}

func (a *CopilotACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", copilotACPPkg, "--acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.copilot",
		},
	}
}

func (a *CopilotACP) RemoteAuth() *RemoteAuth { return nil }

func (a *CopilotACP) InstallScript() string {
	return "npm install -g " + copilotACPPkg
}

func (a *CopilotACP) PermissionSettings() map[string]PermissionSetting {
	return copilotPermSettings
}

// copilotPermSettings enumerates the GitHub Copilot CLI flags that benefit
// from being surfaced as curated profile toggles. At profile-creation time
// these seed the AgentProfile.CLIFlags list; the launch path then consults
// the profile (not this map) so users can freely add/toggle/remove entries.
//
// The CLI's `--autopilot` flag is intentionally omitted: empirical testing
// (see acp-debug captures) shows it does not suppress session/request_permission
// frames. Only `--allow-all-tools` and the other `--allow-all-*` flags do.
var copilotPermSettings = map[string]PermissionSetting{
	"allow_all_tools": {
		Supported:   true,
		Default:     false,
		Label:       "Allow all tools",
		Description: "Skip confirmation for every tool call (--allow-all-tools)",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--allow-all-tools",
	},
	"allow_all_paths": {
		Supported:   true,
		Default:     false,
		Label:       "Allow all paths",
		Description: "Allow file access outside the workspace (--allow-all-paths)",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--allow-all-paths",
	},
	"allow_all_urls": {
		Supported:   true,
		Default:     false,
		Label:       "Allow all URLs",
		Description: "Allow network access to any URL (--allow-all-urls)",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--allow-all-urls",
	},
	"no_ask_user": {
		Supported:   true,
		Default:     false,
		Label:       "Disable ask_user tool",
		Description: "Agent works autonomously without asking clarifying questions (--no-ask-user)",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--no-ask-user",
	},
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *CopilotACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", copilotACPPkg, "--acp"),
	}
}
