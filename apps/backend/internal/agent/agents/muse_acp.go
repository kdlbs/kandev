//nolint:dupl,goconst // Native-binary ACP agents (Goose, Hermes, Devin, ...) follow the same minimal scaffold; differences are the binary name, argv, and auth surface. Shared literals live in every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/muse_acp_light.svg
var museACPLogoLight []byte

//go:embed logos/muse_acp_dark.svg
var museACPLogoDark []byte

const (
	museACPPkg = "@bex-co/muse-code-acp"
	museBin    = "muse"
)

var (
	_ Agent                  = (*MuseACP)(nil)
	_ PassthroughAgent       = (*MuseACP)(nil)
	_ InferenceAgent         = (*MuseACP)(nil)
	_ LoginAgent             = (*MuseACP)(nil)
	_ ManagedNPMRuntimeAgent = (*MuseACP)(nil)
)

// MuseACP implements Agent for Meta's Muse Code. Muse has no native ACP
// server: it speaks the Muse Session Protocol through `muse serve`. The
// community @bex-co/muse-code-acp adapter bridges MSP to ACP over stdio and
// spawns the native `muse` binary it finds on PATH (or MUSE_CODE_EXECUTABLE),
// so structured sessions run the managed npm adapter while installation,
// login, and CLI passthrough target the native `muse` binary.
type MuseACP struct {
	StandardPassthrough
}

func NewMuseACP() *MuseACP {
	return &MuseACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(museBin),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *MuseACP) ID() string          { return "muse-acp" }
func (a *MuseACP) Name() string        { return "Muse Code ACP" }
func (a *MuseACP) DisplayName() string { return "Muse" }
func (a *MuseACP) Description() string {
	return "Meta Muse Code using the ACP protocol via the @bex-co/muse-code-acp adapter."
}
func (a *MuseACP) Enabled() bool     { return true }
func (a *MuseACP) DisplayOrder() int { return 23 }

func (a *MuseACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return museACPLogoDark
	}
	return museACPLogoLight
}

func (a *MuseACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// The adapter carries no Muse executable; it needs the native `muse`
	// binary that owns model access, sandboxing, and persistence. A
	// non-interactive version check filters unrelated tools with the same
	// name. The ACP capability probe validates the adapter separately.
	result, err := Detect(ctx, WithCommandCheck(museBin, "--version"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *MuseACP) BuildCommand(opts CommandOptions) Command {
	return a.ManagedNPMRuntime().ACPCommand(opts.ManagedRuntimeVersion)
}

func (a *MuseACP) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	return newManagedNPMRuntimeSpec(museACPPkg)
}

func (a *MuseACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            a.ManagedNPMRuntime().CachedACPCommand(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: DefaultResourceLimits,
		Protocol:       agent.ProtocolACP,
		// The adapter forwards ACP session/new MCP servers (stdio and HTTP)
		// into a temporary Muse config overlay, so no ProjectMCPStrategy.
		SessionConfig: SessionConfig{
			// Muse persists native sessions under $XDG_DATA_HOME/muse
			// (default ~/.local/share/muse/sessions); the adapter resumes
			// them through `muse serve` and replays history via `muse export`.
			SessionDirTemplate:  "{home}/.local/share/muse",
			SessionDirTarget:    "/root/.local/share/muse",
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *MuseACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  remoteAuthMethodTypeFiles,
				Label: "Copy Muse auth file",
				// `muse login` stores credentials in
				// $XDG_CONFIG_HOME/muse/auth.json (default ~/.config).
				SourceFiles: map[string][]string{
					"darwin": {".config/muse/auth.json"},
					"linux":  {".config/muse/auth.json"},
				},
				TargetRelDir: ".config/muse",
			},
			{
				// META_API_KEY takes priority over stored auth in both Muse
				// and the adapter; it is the headless credential.
				Type:   "env",
				EnvVar: "META_API_KEY",
			},
		},
	}
}

// muse login runs the browser sign-in and stores credentials in
// ~/.config/muse/auth.json.
func (a *MuseACP) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{museBin, "login"},
		Description: "Sign in to Muse Code with your Meta account.",
	}
}

func (a *MuseACP) InstallScript() string {
	// Keep the installer in a temporary file so curl failures cannot be hidden
	// by a successful bash exit status. MUSE_INSTALL_DIR points the official
	// installer at the first writable absolute directory already on PATH, so
	// the parent Kandev process can discover `muse` immediately afterwards.
	// The ACP adapter itself is fetched on demand by npx.
	return `set -eu
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL https://dev.meta.ai/install.sh -o "$tmp"
install_dir=""
old_ifs="$IFS"
IFS=:
for dir in ${PATH:-}; do
  [ -n "$dir" ] || continue
  case "$dir" in
    /*) ;;
    *) continue ;;
  esac
  dir=${dir%/}
  [ -n "$dir" ] || dir=/
  if [ -d "$dir" ] && [ -w "$dir" ]; then
    install_dir="$dir"
    break
  fi
done
IFS="$old_ifs"
if [ -z "$install_dir" ]; then
  echo "Error: no writable absolute directory is available on PATH for Muse" >&2
  exit 1
fi
MUSE_INSTALL_DIR="$install_dir" bash "$tmp"
command -v muse >/dev/null 2>&1
resolved="$(command -v muse)"
if [ "$resolved" != "$install_dir/muse" ]; then
  echo "Error: Muse installed at $install_dir/muse but another muse is earlier on PATH" >&2
  exit 1
fi
"$install_dir/muse" --version >/dev/null 2>&1`
}

func (a *MuseACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *MuseACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   a.ManagedNPMRuntime().CachedACPCommand(),
	}
}

func (a *MuseACP) BillingType() usage.BillingType { return defaultBillingType() }
