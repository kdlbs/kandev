package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/auggie_light.svg
var auggieLogoLight []byte

//go:embed logos/auggie_dark.svg
var auggieLogoDark []byte

const auggiePkg = "@augmentcode/auggie"

var (
	_ Agent            = (*Auggie)(nil)
	_ PassthroughAgent = (*Auggie)(nil)
	_ InferenceAgent   = (*Auggie)(nil)
)

// Auggie implements Agent for the Augment Coding Agent.
type Auggie struct {
	StandardPassthrough
}

func NewAuggie() *Auggie {
	return &Auggie{
		StandardPassthrough: StandardPassthrough{
			PermSettings: auggiePermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", "@augmentcode/auggie"),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("-c"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *Auggie) ID() string          { return "auggie" }
func (a *Auggie) Name() string        { return "Augment Coding Agent" }
func (a *Auggie) DisplayName() string { return "Auggie" }
func (a *Auggie) Description() string { return "Auggie CLI-powered autonomous coding agent." }
func (a *Auggie) Enabled() bool       { return true }
func (a *Auggie) DisplayOrder() int   { return 3 }

func (a *Auggie) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return auggieLogoDark
	}
	return auggieLogoLight
}

func (a *Auggie) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the auggie CLI on PATH. Auth state is surfaced later by the
	// ACP probe.
	result, err := Detect(ctx, WithCommand("auggie"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
		SupportsShell:         false,
		SupportsWorkspaceOnly: false,
	}
	return result, nil
}

func (a *Auggie) BuildCommand(opts CommandOptions) Command {
	// Model and mode are applied via ACP session/set_model and session/set_mode
	// after session creation — no --model CLI flag. The --allow-indexing flag
	// used to be applied here from PermissionValues; it now flows through
	// AgentProfile.CLIFlags and is appended by CommandBuilder.BuildCommand.
	return Cmd("npx", "-y", auggiePkg, "--acp").Build()
}

func (a *Auggie) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:       "kandev/multi-agent",
		Tag:         "latest",
		Cmd:         Cmd("npx", "-y", auggiePkg, "--acp").Build(),
		WorkingDir:  "/workspace",
		RequiredEnv: []string{"AUGMENT_SESSION_AUTH"},
		Env:         map[string]string{},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		WorkspaceFlag:  "--workspace-root",
		AssumeMcpSse:   true,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *Auggie) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy session files",
				SourceFiles: map[string][]string{
					"darwin": {".augment/session.json"},
					"linux":  {".augment/session.json"},
				},
				TargetRelDir: ".augment",
			},
			{
				Type:   "env",
				EnvVar: "AUGMENT_SESSION_AUTH",
			},
		},
	}
}

func (a *Auggie) InstallScript() string {
	return "npm install -g " + auggiePkg
}

func (a *Auggie) PermissionSettings() map[string]PermissionSetting {
	return auggiePermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *Auggie) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("auggie", "--acp", "--allow-indexing"),
	}
}

// --- Private ---

// auggiePermSettings exposes only the auggie-specific allow_indexing flag.
// All other permission concerns are handled via ACP session modes and the
// per-tool-call permission_request UI.
var auggiePermSettings = map[string]PermissionSetting{
	"allow_indexing": {
		Supported: true, Default: true, Label: "Allow indexing", Description: "Enable workspace indexing without confirmation",
		ApplyMethod: PermissionApplyMethodCLIFlag, CLIFlag: "--allow-indexing",
	},
}
