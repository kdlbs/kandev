package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/pi_acp_light.svg
var piACPLogoLight []byte

//go:embed logos/pi_acp_dark.svg
var piACPLogoDark []byte

const (
	piACPPkg = "pi-acp"
	piCLIBin = "pi"
	piCLIPkg = "@earendil-works/pi-coding-agent"
)

var (
	_ Agent                  = (*PiACP)(nil)
	_ PassthroughAgent       = (*PiACP)(nil)
	_ InferenceAgent         = (*PiACP)(nil)
	_ ManagedNPMRuntimeAgent = (*PiACP)(nil)
	_ PortableConfigAgent    = (*PiACP)(nil)
)

// PiACP implements Agent for the Pi Coding Agent via the pi-acp adapter.
type PiACP struct {
	StandardPassthrough
}

func NewPiACP() *PiACP {
	return &PiACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(piCLIBin),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				MCPStrategy:    mcpconfig.PiStrategy{},
			},
		},
	}
}

func (a *PiACP) ID() string          { return "pi-acp" }
func (a *PiACP) Name() string        { return "Pi Coding Agent ACP" }
func (a *PiACP) DisplayName() string { return "Pi" }
func (a *PiACP) Description() string {
	return "Pi Coding Agent using the ACP protocol via the pi-acp adapter."
}
func (a *PiACP) Enabled() bool     { return true }
func (a *PiACP) DisplayOrder() int { return 12 }

func (a *PiACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return piACPLogoDark
	}
	return piACPLogoLight
}

func (a *PiACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// The Pi package publishes the short `pi` binary used by passthrough. A
	// non-interactive version check is a best-effort filter for unrelated tools
	// with the same name; it does not prove package identity. The separate ACP
	// probe validates the structured exact-version pi-acp surface, not this binary.
	result, err := Detect(ctx, WithCommandCheck(piCLIBin, "--version"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *PiACP) BuildCommand(opts CommandOptions) Command {
	return a.ManagedNPMRuntime().ACPCommand(opts.ManagedRuntimeVersion)
}

func (a *PiACP) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	return newManagedNPMRuntimeSpec(piACPPkg)
}

func (a *PiACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:                a.ManagedNPMRuntime().CachedACPCommand(),
		WorkingDir:         "{workspace}",
		Env:                map[string]string{},
		ResourceLimits:     ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:           agent.ProtocolACP,
		UserSkillDir:       ".pi/agent/skills",
		ProjectMCPStrategy: mcpconfig.PiStrategy{},
		SessionConfig: SessionConfig{
			// Verified against pi-acp@0.0.32: the adapter keeps the ACP↔pi
			// session map in ~/.pi/pi-acp/session-map.json and reads pi's own
			// transcripts from ~/.pi/agent/sessions (relocatable via
			// PI_CODING_AGENT_DIR or a sessionDir in ~/.pi/agent/settings.json).
			// Both halves of session/load live under ~/.pi.
			SessionDirTemplate:  "{home}/.pi",
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

// piAgentDir is Pi's agent directory relative to the home directory. Pi lets
// PI_CODING_AGENT_DIR move it; the remote side uses the default.
const piAgentDir = ".pi/agent"

// piRemoteAuthEnvVars are the API-key variables of Pi's most used built-in
// providers. Pi reads a stored auth.json credential before the environment.
var piRemoteAuthEnvVars = []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "OPENROUTER_API_KEY"}

func (a *PiACP) RemoteAuth() *RemoteAuth {
	methods := []RemoteAuthMethod{{
		Type:  remoteAuthMethodTypeFiles,
		Label: "Copy Pi auth file",
		// `/login` stores subscription and API-key credentials per provider as
		// top-level entries of auth.json, so a copy merges into the remote file.
		FileConflictPolicy: RemoteAuthFileConflictPolicyMergeJSONObject,
		SourceFiles: map[string][]string{
			"darwin":  {piAgentDir + "/auth.json"},
			"linux":   {piAgentDir + "/auth.json"},
			"windows": {piAgentDir + "/auth.json"},
		},
		TargetRelDir: piAgentDir,
	}}
	for _, envVar := range piRemoteAuthEnvVars {
		methods = append(methods, RemoteAuthMethod{Type: "env", EnvVar: envVar})
	}
	return &RemoteAuth{Methods: methods}
}

// PortableConfig copies Pi's global settings and custom model definitions.
// models.json can hold inline API keys; the bundle copies it as it is.
func (a *PiACP) PortableConfig() *PortableConfig {
	file := func(name string) PortableConfigFile {
		path := piAgentDir + "/" + name
		return PortableConfigFile{
			SourcePaths: map[string]string{"darwin": path, "linux": path, "windows": path},
			TargetPath:  path,
		}
	}
	return &PortableConfig{Bundles: []PortableConfigBundle{{
		ID:    "pi.config",
		Label: "Copy Pi settings and models",
		Files: []PortableConfigFile{file("settings.json"), file("models.json")},
	}}}
}

func (a *PiACP) InstallScript() string {
	return "npm install -g --ignore-scripts " + piCLIPkg
}

func (a *PiACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *PiACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   a.ManagedNPMRuntime().CachedACPCommand(),
	}
}

func (a *PiACP) BillingType() usage.BillingType { return defaultBillingType() }
