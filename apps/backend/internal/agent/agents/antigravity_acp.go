//nolint:dupl,goconst // Native-binary ACP agents follow the same minimal scaffold; GOOS names and remote-auth Type/OS literals are shared vocabulary across every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/antigravity_acp_light.svg
var antigravityACPLogoLight []byte

//go:embed logos/antigravity_acp_dark.svg
var antigravityACPLogoDark []byte

// antigravityHarnessPathEnv, when set to a non-empty value, tells the
// Antigravity ACP server where to find its harness without searching its own
// executable's directory. A set value satisfies discovery's harness check
// without inspecting the directory.
const antigravityHarnessPathEnv = "ANTIGRAVITY_HARNESS_PATH"

// antigravityConfigRelDir is the server's config root, home-relative.
const antigravityConfigRelDir = ".gemini/antigravity-acp"

var (
	_ Agent          = (*AntigravityACP)(nil)
	_ InferenceAgent = (*AntigravityACP)(nil)
)

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

func (a *AntigravityACP) ID() string          { return "antigravity-acp" }
func (a *AntigravityACP) Name() string        { return "Antigravity ACP Agent" }
func (a *AntigravityACP) DisplayName() string { return "Antigravity" }
func (a *AntigravityACP) Description() string {
	return "Google's standalone Antigravity ACP server, distributed separately from the agy CLI. " +
		"Not auto-installed: download the antigravity-acp entry from the ACP registry, extract both " +
		"archive entries into one directory, and add that directory to PATH. Kandev looks for " +
		"agy_acp_server.par (agy_acp_server.exe on Windows)."
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

// antigravityHarnessNames returns the harness executable names to search
// for, in first-match-wins order, suffixed for the host platform.
func antigravityHarnessNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"localharness_external.exe", "localharness.exe"}
	}
	return []string{"localharness_external", "localharness"}
}

// antigravityHarnessPresent reports whether one of the harness names exists
// as a usable regular file in dir. A directory, a dangling symlink, or an
// unstat-able name does not match.
func antigravityHarnessPresent(dir string) bool {
	for _, name := range antigravityHarnessNames() {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return true
	}
	return false
}

// IsInstalled reports the agent available only when the server executable is
// on PATH and its harness sibling can be found beside it (or
// ANTIGRAVITY_HARNESS_PATH names one), so a partial install never reads as
// healthy.
func (a *AntigravityACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(antigravityACPBinaryName())
	if err != nil {
		return &DiscoveryResult{Available: false}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	harnessSatisfied := os.Getenv(antigravityHarnessPathEnv) != "" || antigravityHarnessPresent(filepath.Dir(path))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !harnessSatisfied {
		return &DiscoveryResult{Available: false}, nil
	}
	return &DiscoveryResult{
		Available:   true,
		MatchedPath: path,
		SupportsMCP: true,
		Capabilities: DiscoveryCapabilities{
			SupportsSessionResume: true,
		},
	}, nil
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
				Type: "files",
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

func (a *AntigravityACP) BillingType() usage.BillingType { return defaultBillingType() }
