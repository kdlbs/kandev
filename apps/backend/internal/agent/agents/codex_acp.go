package agents

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/codex_light.svg
var codexACPLogoLight []byte

//go:embed logos/codex_dark.svg
var codexACPLogoDark []byte

const codexACPPackage = "@agentclientprotocol/codex-acp"

var (
	_ Agent                  = (*CodexACP)(nil)
	_ PassthroughAgent       = (*CodexACP)(nil)
	_ InferenceAgent         = (*CodexACP)(nil)
	_ ManagedNPMRuntimeAgent = (*CodexACP)(nil)
	_ FilesystemPolicyAgent  = (*CodexACP)(nil)
)

// CodexACP implements Agent for the Agent Client Protocol codex-acp package.
// It speaks the ACP protocol (JSON-RPC 2.0 over stdin/stdout) through the npm
// bridge for OpenAI Codex. Used for A/B comparison against the native Codex agent.
type CodexACP struct {
	StandardPassthrough
}

// codexPassthroughPermSettings maps passthrough-only toggles to @openai/codex CLI
// flags. Not returned from PermissionSettings(): ACP auto-approve uses agentctl
// approval_policy. The legacy --full-auto flag was removed; auto_approve uses
// --ask-for-approval never.
var codexPassthroughPermSettings = map[string]PermissionSetting{
	PermissionKeyAutoApprove: {
		Supported:    true,
		Default:      true,
		Label:        "Auto approve",
		Description:  "Skip command approval prompts (--ask-for-approval never)",
		ApplyMethod:  PermissionApplyMethodCLIFlag,
		CLIFlag:      "--ask-for-approval",
		CLIFlagValue: "never",
	},
}

func NewCodexACP() *CodexACP {
	return &CodexACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: codexPassthroughPermSettings,
			Cfg: PassthroughConfig{
				Supported:        true,
				Label:            "CLI Passthrough",
				Description:      "Show terminal directly instead of chat interface",
				PassthroughCmd:   NewCommand("npx", "-y", "@openai/codex"),
				ModelFlag:        NewParam("--model", "{model}"),
				IdleTimeout:      3 * time.Second,
				BufferMaxBytes:   DefaultBufferMaxBytes,
				MCPStrategy:      mcpconfig.CodexStrategy{},
				AutoInjectPrompt: true,
				SubmitSequence:   "\r",
			},
		},
	}
}

func (a *CodexACP) ID() string          { return "codex-acp" }
func (a *CodexACP) Name() string        { return "Codex ACP Agent" }
func (a *CodexACP) DisplayName() string { return "Codex" }
func (a *CodexACP) Description() string {
	return "OpenAI Codex coding agent using the Agent Client Protocol bridge."
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
	return a.ManagedNPMRuntime().ACPCommand(opts.ManagedRuntimeVersion)
}

func (a *CodexACP) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	return newManagedNPMRuntimeSpec(codexACPPackage)
}

func (a *CodexACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:       "kandev/multi-agent",
		Tag:         "latest",
		Cmd:         a.ManagedNPMRuntime().CachedACPCommand(),
		WorkingDir:  "{workspace}",
		RequiredEnv: []string{"OPENAI_API_KEY"},
		Env:         map[string]string{},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
		},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: ".agents/skills",
		UserSkillDir:    ".codex/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			// Use the same SessionDirTemplate pattern every other ACP agent
			// uses; the docker container manager wires this into a kandev-owned
			// per-container session dir, isolated from the host's ~/.codex
			// (which carries host-absolute rollout paths in state.db that
			// don't resolve inside the container).
			SessionDirTemplate: "{home}/.codex",
			SessionDirTarget:   "/root/.codex",
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
					"darwin": {".codex/auth.json"},
					"linux":  {".codex/auth.json"},
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

func (a *CodexACP) PortableConfig() *PortableConfig {
	return &PortableConfig{Bundles: []PortableConfigBundle{
		{
			ID:    "codex.config",
			Label: "Copy Codex configuration",
			Files: []PortableConfigFile{
				{SourcePaths: map[string]string{
					"darwin":  ".codex/config.toml",
					"linux":   ".codex/config.toml",
					"windows": ".codex/config.toml",
				}, TargetPath: ".codex/config.toml"},
			},
		},
	}}
}

// Verified against `codex --help`: `codex login --device-auth` is the
// dedicated sign-in subcommand. Device-auth prints a code + URL that works
// even when the kandev process can't open a browser (containers, SSH,
// headless dev boxes), and falls back to a local browser flow otherwise.
func (a *CodexACP) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{"codex", "login", "--device-auth"},
		Description: "Sign in with your OpenAI account.",
	}
}

// Install both the user-facing OpenAI codex CLI (which `codex login` runs
// against) and the ACP bridge package used for chat sessions.
func (a *CodexACP) InstallScript() string {
	return "npm install -g @openai/codex " + codexACPPackage
}

func (a *CodexACP) BillingType() usage.BillingType { return codexBillingType() }

func (a *CodexACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// ApplyFilesystemPolicy renders and validates Codex's native session
// configuration. Lifecycle remains unaware of CODEX_CONFIG and Codex sandbox
// keys so other agents can render the same neutral policy differently.
func (a *CodexACP) ApplyFilesystemPolicy(env map[string]string, policy FilesystemPolicy) error {
	return applyCodexFilesystemPolicy(env, policy)
}

func (a *CodexACP) FilesystemPolicyEnvironmentKeys() []string { return []string{"CODEX_CONFIG"} }

func applyCodexFilesystemPolicy(env map[string]string, policy FilesystemPolicy) error {
	if env == nil {
		return errors.New("filesystem policy environment is unavailable")
	}
	if err := rejectLegacyCodexSandbox(env); err != nil {
		return err
	}
	config, err := codexConfigFromEnvironment(env)
	if err != nil {
		return err
	}
	if hasLegacyCodexSandbox(config) {
		return errors.New("legacy Codex sandbox configuration conflicts with task filesystem policy")
	}
	overlay, err := renderCodexFilesystemPolicy(policy)
	if err != nil {
		return err
	}
	mergeCodexConfig(config, overlay)
	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode Codex filesystem policy: %w", err)
	}
	env["CODEX_CONFIG"] = string(encoded)
	return nil
}

func renderCodexFilesystemPolicy(policy FilesystemPolicy) (map[string]any, error) {
	if policy.Name == "" {
		return nil, fmt.Errorf("filesystem policy name is required")
	}
	rules := make(map[string]string, len(policy.Rules))
	for _, rule := range policy.Rules {
		if rule.Path == "" {
			return nil, fmt.Errorf("filesystem policy path is required")
		}
		switch rule.Access {
		case FilesystemAccessRead, FilesystemAccessWrite, FilesystemAccessDeny:
			rules[rule.Path] = string(rule.Access)
		default:
			return nil, fmt.Errorf("invalid filesystem access %q", rule.Access)
		}
	}
	return map[string]any{
		"default_permissions": policy.Name,
		"permissions": map[string]any{
			policy.Name: map[string]any{
				"extends":    ":workspace",
				"filesystem": rules,
			},
		},
	}, nil
}

func rejectLegacyCodexSandbox(env map[string]string) error {
	home := env["CODEX_HOME"]
	if home == "" {
		home = filepath.Join(env["HOME"], ".codex")
	}
	if home == "" {
		return nil
	}
	contents, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("unable to validate Codex sandbox configuration")
	}
	config := make(map[string]any)
	if err := toml.Unmarshal(contents, &config); err != nil {
		return errors.New("unable to validate Codex sandbox configuration")
	}
	if hasLegacyCodexSandbox(config) {
		return errors.New("legacy Codex sandbox configuration conflicts with task filesystem policy")
	}
	return nil
}

func codexConfigFromEnvironment(env map[string]string) (map[string]any, error) {
	config := make(map[string]any)
	if env["CODEX_CONFIG"] == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(env["CODEX_CONFIG"]), &config); err != nil {
		return nil, errors.New("unable to validate Codex session configuration")
	}
	return config, nil
}

func hasLegacyCodexSandbox(config map[string]any) bool {
	_, sandboxMode := config["sandbox_mode"]
	_, workspaceWrite := config["sandbox_workspace_write"]
	return sandboxMode || workspaceWrite
}

func mergeCodexConfig(base, overlay map[string]any) {
	for key, value := range overlay {
		if key != "permissions" {
			base[key] = value
			continue
		}
		existing, _ := base[key].(map[string]any)
		if existing == nil {
			existing = make(map[string]any)
			base[key] = existing
		}
		permissions, ok := value.(map[string]any)
		if !ok {
			continue
		}
		for name, profile := range permissions {
			existing[name] = profile
		}
	}
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *CodexACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   a.ManagedNPMRuntime().CachedACPCommand(),
	}
}
