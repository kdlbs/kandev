package agents

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/opencode_light.svg
var opencodeACPLogoLight []byte

//go:embed logos/opencode_dark.svg
var opencodeACPLogoDark []byte

const opencodeACPPackage = "opencode-ai"

var (
	_ Agent                            = (*OpenCodeACP)(nil)
	_ PassthroughAgent                 = (*OpenCodeACP)(nil)
	_ InferenceAgent                   = (*OpenCodeACP)(nil)
	_ ManagedNPMRuntimeAgent           = (*OpenCodeACP)(nil)
	_ FilesystemPolicyAgent            = (*OpenCodeACP)(nil)
	_ FilesystemPolicyEnvironmentAgent = (*OpenCodeACP)(nil)
)

// OpenCodeACP is the ACP protocol variant of OpenCode.
// Uses JSON-RPC 2.0 over stdin/stdout via "opencode acp" instead of REST/SSE.
type OpenCodeACP struct {
	StandardPassthrough
}

func NewOpenCodeACP() *OpenCodeACP {
	return &OpenCodeACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("opencode"),
				ModelFlag:      NewParam("--model", "{model}"),
				PromptFlag:     NewParam("--prompt", "{prompt}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				ResumeFlag:     NewParam("-c"),
				// opencode has no MCP flag; write a temp opencode.json and point
				// it there via the OPENCODE_CONFIG env var (merges, never writes
				// ~/.config/opencode).
				MCPStrategy: mcpconfig.OpenCodeStrategy{},
			},
		},
	}
}

func (a *OpenCodeACP) ID() string          { return "opencode-acp" }
func (a *OpenCodeACP) Name() string        { return "OpenCode AI Agent (ACP)" }
func (a *OpenCodeACP) DisplayName() string { return "OpenCode" }
func (a *OpenCodeACP) Description() string {
	return "OpenCode coding agent using ACP protocol over stdin/stdout."
}
func (a *OpenCodeACP) Enabled() bool     { return true }
func (a *OpenCodeACP) DisplayOrder() int { return 4 }

func (a *OpenCodeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return opencodeACPLogoDark
	}
	return opencodeACPLogoLight
}

func (a *OpenCodeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the opencode CLI on PATH. Any installed version is eligible
	// for the ACP probe; auth and protocol compatibility are surfaced there.
	result, err := Detect(ctx, WithCommand("opencode"))
	if err != nil || !result.Available {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OpenCodeACP) BuildCommand(opts CommandOptions) Command {
	return a.ManagedNPMRuntime().ACPCommand(opts.ManagedRuntimeVersion)
}

func (a *OpenCodeACP) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	return newManagedNPMRuntimeSpec(
		opencodeACPPackage,
		"acp", "--print-logs", "--log-level", "ERROR",
	)
}

func (a *OpenCodeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             a.ManagedNPMRuntime().CachedACPCommand(),
		WorkingDir:      "{workspace}",
		Env:             map[string]string{},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: ".agents/skills",
		UserSkillDir:    ".config/opencode/skills",
		// opencode acp runs its HTTP server + MCP child tree alongside the
		// ACP stdin/stdout. Closing stdin doesn't terminate the process, so
		// skip the graceful wait and reap its process group immediately.
		// See GH issue #1247.
		RequiresProcessKill: true,
		SessionConfig: SessionConfig{
			NativeSessionResume:         true,
			NewSessionOnWorkspaceRebind: true,
			CanRecover:                  &canRecover,
			// Auth lives under .local/share/opencode and configuration under
			// .config/opencode. Mount the isolated executor home so both trees
			// remain visible without mounting the host home.
			SessionDirTemplate: "{home}",
			SessionDirTarget:   "/root",
		},
	}
}

func (a *OpenCodeACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy auth files",
				SourceFiles: map[string][]string{
					"darwin": {".local/share/opencode/auth.json"},
					"linux":  {".local/share/opencode/auth.json"},
				},
				TargetRelDir: ".local/share/opencode",
			},
		},
	}
}

func (a *OpenCodeACP) PortableConfig() *PortableConfig {
	return &PortableConfig{Bundles: []PortableConfigBundle{
		{
			ID:    "opencode.config",
			Label: "Copy OpenCode configuration",
			Files: []PortableConfigFile{
				{SourcePaths: map[string]string{
					"darwin":  ".config/opencode/opencode.json",
					"linux":   ".config/opencode/opencode.json",
					"windows": ".config/opencode/opencode.json",
				}, TargetPath: ".config/opencode/opencode.json"},
				{SourcePaths: map[string]string{
					"darwin":  ".config/opencode/opencode.jsonc",
					"linux":   ".config/opencode/opencode.jsonc",
					"windows": ".config/opencode/opencode.jsonc",
				}, TargetPath: ".config/opencode/opencode.jsonc"},
			},
		},
	}}
}

func (a *OpenCodeACP) InstallScript() string {
	return "npm install -g " + opencodeACPPackage
}

func (a *OpenCodeACP) BillingType() usage.BillingType { return defaultBillingType() }

func (a *OpenCodeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// ApplyFilesystemPolicy renders the server-authored Git metadata policy using
// OpenCode's native permission configuration. Inline config has the highest
// runtime precedence, so project or user config cannot widen these rules.
func (a *OpenCodeACP) ApplyFilesystemPolicy(env map[string]string, policy FilesystemPolicy) error {
	if env == nil {
		return errors.New("filesystem policy environment is unavailable")
	}
	if policy.Name != "kandev_task_git_metadata" {
		return fmt.Errorf("unsupported filesystem policy %q", policy.Name)
	}
	rules := map[string]map[string]string{
		// Git add/commit and normal agent workflows execute through OpenCode's
		// bash tool. Path isolation remains enforced by the executor/container
		// boundary and the read/edit/external_directory rules below.
		"bash":               {"*": "allow"},
		"external_directory": {"*": "deny"},
		"read":               {"*": "deny"},
		"edit":               {"*": "deny"},
	}
	for _, rule := range policy.Rules {
		path := strings.TrimSpace(rule.Path)
		if path == "" || path == ":minimal" {
			continue
		}
		pattern := opencodeFilesystemPattern(path)
		switch rule.Access {
		case FilesystemAccessRead:
			rules["read"][pattern] = "allow"
			rules["external_directory"][pattern] = "allow"
		case FilesystemAccessWrite:
			rules["read"][pattern] = "allow"
			rules["edit"][pattern] = "allow"
			rules["external_directory"][pattern] = "allow"
		case FilesystemAccessDeny:
			for _, action := range []string{"external_directory", "read", "edit"} {
				rules[action][pattern] = "deny"
			}
		default:
			return fmt.Errorf("unsupported filesystem access %q", rule.Access)
		}
	}
	encoded, err := json.Marshal(map[string]any{"permission": rules})
	if err != nil {
		return fmt.Errorf("encode OpenCode filesystem policy: %w", err)
	}
	env["OPENCODE_CONFIG_CONTENT"] = string(encoded)
	return nil
}

func (a *OpenCodeACP) FilesystemPolicyEnvironmentKeys() []string {
	return []string{"OPENCODE_CONFIG_CONTENT"}
}

func opencodeFilesystemPattern(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/*"
	}
	return path + "/**"
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *OpenCodeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   a.ManagedNPMRuntime().CachedACPCommand(),
	}
}
