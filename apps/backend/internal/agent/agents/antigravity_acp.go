//nolint:dupl,goconst // Native-binary ACP agents follow the same minimal scaffold; GOOS names and remote-auth Type/OS literals are shared vocabulary across every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/antigravity_acp_light.svg
var antigravityACPLogoLight []byte

//go:embed logos/antigravity_acp_dark.svg
var antigravityACPLogoDark []byte

// AntigravityACPAgentID is the registry identity of the Antigravity agent.
const AntigravityACPAgentID = acpcompat.AntigravityAgentID

// antigravityConfigRelDir is the server's config root, home-relative.
const antigravityConfigRelDir = ".gemini/antigravity-acp"

var (
	_ Agent                  = (*AntigravityACP)(nil)
	_ InferenceAgent         = (*AntigravityACP)(nil)
	_ LocalRuntimeAgent      = (*AntigravityACP)(nil)
	_ SavedLocalRuntimeAgent = (*AntigravityACP)(nil)
	_ LoginAgent             = (*AntigravityACP)(nil)
	_ ContextLoginAgent      = (*AntigravityACP)(nil)
)

const antigravityCLILoginVariant = "antigravity-cli"

// AntigravityACP implements Agent for Google's standalone Antigravity ACP
// server (registry entry "antigravity-acp"), a signed native binary distinct
// from the `agy` CLI. Discovery, launch argv, config root, and remote-auth
// methods are declared here; the server owns its own credentials, session
// storage, and harness subprocess (see
// docs/specs/agents/system-design/antigravity-acp-agent.md).
type AntigravityACP struct{}

func NewAntigravityACP() *AntigravityACP {
	return &AntigravityACP{}
}

func (a *AntigravityACP) ID() string          { return AntigravityACPAgentID }
func (a *AntigravityACP) Name() string        { return "Antigravity ACP Agent" }
func (a *AntigravityACP) DisplayName() string { return "Antigravity" }
func (a *AntigravityACP) Description() string {
	return "Uses the compatible Antigravity CLI (`agy acp`) from PATH when available, otherwise the standalone antigravity-acp server " +
		"(`agy_acp_server.par` or `agy_acp_server.exe`). Install or check the standalone ACP server in Settings. " +
		"Each runtime uses its own Google sign-in and conversation store."
}
func (a *AntigravityACP) Enabled() bool     { return true }
func (a *AntigravityACP) DisplayOrder() int { return 22 }

func (a *AntigravityACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return antigravityACPLogoDark
	}
	return antigravityACPLogoLight
}

// antigravityACPBinaryName returns the platform-specific ACP server
// executable name. It is defined on every platform and consults nothing but
// runtime.GOOS.
func antigravityACPBinaryName() string {
	if runtime.GOOS == "windows" {
		return "agy_acp_server.exe"
	}
	return "agy_acp_server.par"
}

// antigravityACPArgv returns the deterministic launch argument vector for the
// host platform. It is defined and non-empty on every platform, and it
// consults only runtime.GOOS, never PATH, the filesystem, or the environment.
// argv[0] is built from antigravityACPBinaryName so the launched name can
// never drift from the name discovery looked up.
func antigravityACPArgv() []string {
	if runtime.GOOS == "linux" {
		return []string{antigravityACPBinaryName(), "--uid="}
	}
	return []string{antigravityACPBinaryName()}
}

// IsInstalled reports whether one complete runtime exists. A compatible host
// CLI is preferred; otherwise a complete standalone bundle must be available.
func (a *AntigravityACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	selected, err := antigravityruntime.ResolveLocal(ctx, antigravityruntime.ResolveOptions{})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, antigravityruntime.ErrUnavailable) {
			return &DiscoveryResult{Available: false}, nil
		}
		return nil, err
	}
	return &DiscoveryResult{
		Available:   true,
		MatchedPath: selected.Args[0],
		SupportsMCP: true,
		Capabilities: DiscoveryCapabilities{
			SupportsSessionResume: true,
		},
	}, nil
}

// SupportsLocalInstall reports the explicit settings installation capability.
func (a *AntigravityACP) SupportsLocalInstall() bool { return true }

// LocalInstall installs the reviewed bundle for the host platform.
func (a *AntigravityACP) LocalInstall(ctx context.Context, progress func(string)) error {
	_, err := antigravityruntime.Install(ctx, antigravityruntime.InstallOptions{Progress: progress})
	return err
}

// ResolveLocalRuntime returns the complete host-local command and the
// environment entry that pins the validated harness. It is only called for
// local executions; remote executors keep the base command.
func (a *AntigravityACP) ResolveLocalRuntime(ctx context.Context) (LocalRuntimeSelection, error) {
	selected, err := antigravityruntime.ResolveLocal(ctx, antigravityruntime.ResolveOptions{})
	if err != nil {
		return LocalRuntimeSelection{}, err
	}
	return antigravitySelection(selected), nil
}

// ResolveSavedLocalRuntime preserves the executable family that owns the
// native conversation. Legacy records without a runtime tag use standalone.
func (a *AntigravityACP) ResolveSavedLocalRuntime(ctx context.Context, adapterVersion string) (LocalRuntimeSelection, error) {
	selected, err := antigravityruntime.ResolveSaved(ctx, antigravityruntime.ResolveOptions{}, adapterVersion)
	if err != nil {
		return LocalRuntimeSelection{}, err
	}
	return antigravitySelection(selected), nil
}

// LoginCommand exposes CLI sign-in only when the compatible CLI is the
// selected host runtime. Standalone sign-in keeps its dedicated ACP flow.
func (a *AntigravityACP) LoginCommand() *LoginCommand {
	command, err := a.ResolveLoginCommand(context.Background(), antigravityCLILoginVariant)
	if err != nil {
		return nil
	}
	return command
}

// ResolveLoginCommand rechecks the runtime after the settings page submits its
// selected variant. This prevents a stale CLI action from starting a PTY.
func (a *AntigravityACP) ResolveLoginCommand(
	ctx context.Context,
	variant string,
) (*LoginCommand, error) {
	if variant != antigravityCLILoginVariant {
		return nil, ErrLoginRuntimeChanged
	}
	selection, err := a.ResolveLocalRuntime(ctx)
	if err != nil || selection.Variant != acpcompat.AntigravityRuntimeCLI {
		return nil, ErrLoginRuntimeChanged
	}
	command := []string{selection.Command.Args()[0], "acp", "--login"}
	return &LoginCommand{
		Cmd:      command,
		Variants: map[string][]string{antigravityCLILoginVariant: command},
	}, nil
}

func antigravitySelection(selected *antigravityruntime.LocalRuntime) LocalRuntimeSelection {
	policy := acpcompat.AntigravityAuthPolicyForRuntime(selected.Variant, selected.Version, false)
	selection := LocalRuntimeSelection{
		Command:        NewCommand(selected.Args...),
		Variant:        selected.Variant,
		Version:        selected.Version,
		AdapterVersion: policy.AdapterVersion,
	}
	if selected.Bundle != nil {
		selection.Env = map[string]string{acpcompat.AntigravityHarnessPathEnv: selected.Bundle.HarnessPath}
	}
	if selection.AdapterVersion == "" {
		selection.AdapterVersion = fmt.Sprintf("antigravity-%s", selected.Variant)
	}
	return selection
}

func (a *AntigravityACP) BuildCommand(opts CommandOptions) Command {
	return NewCommand(antigravityACPArgv()...)
}

func (a *AntigravityACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             NewCommand(antigravityACPArgv()...),
		WorkingDir:      "{workspace}",
		Env:             map[string]string{},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: DefaultProjectSkillDir,
		UserSkillDir:    ".gemini/config/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/" + antigravityConfigRelDir,
			// SessionDirTarget stays empty because the config root has no
			// container bind mount.
		},
	}
}

func (a *AntigravityACP) RemoteAuth() *RemoteAuth {
	files := []string{
		antigravityConfigRelDir + "/settings.json",
		antigravityConfigRelDir + "/acp_token.json",
		antigravityConfigRelDir + "/acp_business_token.json",
	}
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type: remoteAuthMethodTypeFiles,
				SourceFiles: map[string][]string{
					"darwin": files,
					"linux":  files,
				},
				TargetRelDir: antigravityConfigRelDir,
			},
			{
				Type:   "env",
				EnvVar: "GEMINI_API_KEY",
			},
			{
				Type:   "env",
				EnvVar: "GOOGLE_API_KEY",
			},
		},
	}
}

func (a *AntigravityACP) InstallScript() string {
	return ""
}

func (a *AntigravityACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *AntigravityACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(antigravityACPArgv()...),
	}
}

func (a *AntigravityACP) BillingType() usage.BillingType { return usage.BillingTypeSubscription }
