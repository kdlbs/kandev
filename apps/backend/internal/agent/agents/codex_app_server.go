package agents

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

const codexAppServerPackage = "@openai/codex"

var (
	_ Agent                  = (*CodexAppServer)(nil)
	_ InferenceAgent         = (*CodexAppServer)(nil)
	_ ManagedNPMRuntimeAgent = (*CodexAppServer)(nil)
	_ StoredProfilePreserver = (*CodexAppServer)(nil)
)

// CodexAppServer is the native app-server agent. Its registered descriptor
// remains available for stored-profile reads while the feature is disabled.
type CodexAppServer struct {
	enabled bool
}

func NewCodexAppServer(enabled bool) *CodexAppServer {
	return &CodexAppServer{enabled: enabled}
}

func (a *CodexAppServer) ID() string          { return "codex-app-server" }
func (a *CodexAppServer) Name() string        { return "OpenAI Codex app-server" }
func (a *CodexAppServer) DisplayName() string { return "Codex app server" }
func (a *CodexAppServer) Description() string {
	return "OpenAI Codex using its native app-server protocol."
}
func (a *CodexAppServer) Enabled() bool                                    { return a.enabled }
func (a *CodexAppServer) PreserveStoredProfilesWhenDisabled() bool         { return true }
func (a *CodexAppServer) DisplayOrder() int                                { return 3 }
func (a *CodexAppServer) Logo(_ LogoVariant) []byte                        { return nil }
func (a *CodexAppServer) PermissionSettings() map[string]PermissionSetting { return emptyPermSettings }
func (a *CodexAppServer) BillingType() usage.BillingType                   { return codexBillingType() }
func (a *CodexAppServer) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	return newManagedNPMRuntimeSpec(codexAppServerPackage, "app-server")
}
func (a *CodexAppServer) BuildCommand(opts CommandOptions) Command {
	return a.ManagedNPMRuntime().RuntimeCommand(opts.ManagedRuntimeVersion)
}

func (a *CodexAppServer) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Protocol:  agent.ProtocolCodexAppServer,
		Command:   a.ManagedNPMRuntime().RuntimeCommand(""),
	}
}

func (a *CodexAppServer) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("npx"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{SupportsSessionResume: true}
	return result, nil
}

func (a *CodexAppServer) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:           "kandev/multi-agent",
		Tag:             "latest",
		Cmd:             a.ManagedNPMRuntime().CachedCommand(),
		WorkingDir:      "{workspace}",
		RequiredEnv:     []string{"OPENAI_API_KEY"},
		Env:             map[string]string{},
		Mounts:          []MountTemplate{{Source: "{workspace}", Target: "/workspace"}},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2, Timeout: time.Hour},
		Protocol:        agent.ProtocolCodexAppServer,
		ProjectSkillDir: ".agents/skills",
		UserSkillDir:    ".codex/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.codex",
			SessionDirTarget:    "/root/.codex",
		},
	}
}

func (a *CodexAppServer) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{Methods: []RemoteAuthMethod{
		{
			Type:  remoteAuthMethodTypeFiles,
			Label: remoteAuthLabelCopyFiles,
			SourceFiles: map[string][]string{
				"darwin": {".codex/auth.json"},
				"linux":  {".codex/auth.json"},
			},
			TargetRelDir: ".codex",
		},
		{Type: "env", EnvVar: "OPENAI_API_KEY"},
	}}
}

func (a *CodexAppServer) PortableConfig() *PortableConfig {
	return &PortableConfig{Bundles: []PortableConfigBundle{{
		ID:    "codex.config",
		Label: "Copy Codex configuration",
		Files: []PortableConfigFile{{
			SourcePaths: map[string]string{
				"darwin":  ".codex/config.toml",
				"linux":   ".codex/config.toml",
				"windows": ".codex/config.toml",
			},
			TargetPath: ".codex/config.toml",
		}},
	}}}
}

func (a *CodexAppServer) LoginCommand() *LoginCommand {
	return &LoginCommand{Cmd: []string{"codex", "login", "--device-auth"}, Description: "Sign in with your OpenAI account."}
}

func (a *CodexAppServer) InstallScript() string {
	return "npm install -g " + codexAppServerPackage
}
