package agents

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"regexp"
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

const (
	opencodeACPPackage     = "opencode-ai"
	opencodeACPVersion     = "1.18.4"
	opencodeACPPackageSpec = opencodeACPPackage + "@" + opencodeACPVersion
	opencodeVersionTimeout = 5 * time.Second
)

var opencodeVersionPattern = regexp.MustCompile(
	`(?:^|[^0-9A-Za-z])[vV]?([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)(?:$|[^0-9A-Za-z.+-])`,
)

var (
	_ Agent            = (*OpenCodeACP)(nil)
	_ PassthroughAgent = (*OpenCodeACP)(nil)
	_ InferenceAgent   = (*OpenCodeACP)(nil)
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
	// Check for the opencode CLI on PATH. Auth state is surfaced later by
	// the ACP probe, not by scanning ~/.opencode.
	result, err := Detect(ctx, WithCommand("opencode"))
	if err != nil || !result.Available {
		return result, err
	}
	version, err := probeOpenCodeVersion(ctx, result.MatchedPath)
	if err != nil {
		return unsupportedOpenCodeResult(result.MatchedPath), fmt.Errorf(
			"cannot verify OpenCode %s at %q: %w; reinstall with `%s`",
			opencodeACPVersion, result.MatchedPath, err, a.InstallScript(),
		)
	}
	if version != opencodeACPVersion {
		return unsupportedOpenCodeResult(result.MatchedPath), fmt.Errorf(
			"unsupported OpenCode version %s at %q; Kandev requires %s; reinstall with `%s`",
			version, result.MatchedPath, opencodeACPVersion, a.InstallScript(),
		)
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func probeOpenCodeVersion(ctx context.Context, path string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, opencodeVersionTimeout)
	defer cancel()

	output, err := exec.CommandContext(probeCtx, path, "--version").CombinedOutput()
	if err != nil {
		if probeCtx.Err() != nil {
			return "", probeCtx.Err()
		}
		return "", fmt.Errorf("run --version: %w", err)
	}
	match := opencodeVersionPattern.FindStringSubmatch(strings.TrimSpace(string(output)))
	if len(match) != 2 {
		return "", fmt.Errorf("parse --version output %q", strings.TrimSpace(string(output)))
	}
	return match[1], nil
}

func unsupportedOpenCodeResult(path string) *DiscoveryResult {
	return &DiscoveryResult{MatchedPath: path}
}

func (a *OpenCodeACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("opencode", "acp").Build()
}

func (a *OpenCodeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             Cmd("opencode", "acp").Build(),
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
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.opencode",
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

func (a *OpenCodeACP) InstallScript() string {
	return "npm install -g " + opencodeACPPackageSpec
}

func (a *OpenCodeACP) BillingType() usage.BillingType { return defaultBillingType() }

func (a *OpenCodeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *OpenCodeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("opencode", "acp"),
	}
}
