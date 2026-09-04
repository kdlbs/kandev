//nolint:dupl,goconst // Native-binary ACP agents (Kiro, Qoder, Hermes, Devin, ...) follow the same minimal scaffold; differences are the binary name, argv, and auth surface. Shared literals live in every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/goose_light.svg
var gooseLogoLight []byte

//go:embed logos/goose_dark.svg
var gooseLogoDark []byte

const gooseBin = "goose"

const gooseACPHelpMarker = "run goose as an acp agent"

var (
	_ Agent            = (*GooseACP)(nil)
	_ PassthroughAgent = (*GooseACP)(nil)
	_ InferenceAgent   = (*GooseACP)(nil)
	_ LoginAgent       = (*GooseACP)(nil)
)

// GooseACP implements Agent for the Goose coding agent (Agentic AI
// Foundation / AAIF, formerly Block) using native ACP over stdin/stdout
// (`goose acp`). Goose is installed as a standalone native binary
// (`download_cli.sh`, Homebrew `block-goose-cli`, or `pip install goose-ai`),
// so the launch command is the bare `goose` binary discovered on PATH.
//
// Auth is declarative: Goose reads its provider + credential config from
// ~/.config/goose (not environment variables), so `goose configure` is the
// interactive setup, and the primary remote-auth path is a config-file copy.
type GooseACP struct {
	StandardPassthrough
}

func NewGooseACP() *GooseACP {
	return &GooseACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(gooseBin),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *GooseACP) ID() string          { return "goose-acp" }
func (a *GooseACP) Name() string        { return "Goose ACP Agent" }
func (a *GooseACP) DisplayName() string { return "Goose" }
func (a *GooseACP) Description() string {
	return "AAIF Goose coding agent using the ACP protocol via goose acp. Local, extensible, open source."
}
func (a *GooseACP) Enabled() bool     { return true }
func (a *GooseACP) DisplayOrder() int { return 22 }

func (a *GooseACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return gooseLogoDark
	}
	return gooseLogoLight
}

func (a *GooseACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// `goose acp` is a blocking server, so check its help surface instead of
	// starting the server or opening provider configuration. The subcommand
	// check also rejects unrelated binaries that happen to be named `goose`.
	result, err := Detect(ctx, detectGoose)
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func detectGoose(ctx context.Context) (bool, string, error) {
	path, err := exec.LookPath(gooseBin)
	if err != nil {
		return false, "", nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, commandCheckTimeout)
	defer cancel()
	output, err := exec.CommandContext(checkCtx, path, "acp", "--help").CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return false, "", ctx.Err()
		}
		return false, "", nil
	}

	help := strings.ToLower(string(output))
	if !strings.Contains(help, gooseACPHelpMarker) {
		return false, "", nil
	}
	return true, path, nil
}

func (a *GooseACP) BuildCommand(_ CommandOptions) Command {
	return Cmd(gooseBin, "acp").Build()
}

func (a *GooseACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd(gooseBin, "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: DefaultResourceLimits,
		Protocol:       agent.ProtocolACP,
		// Goose supports Stdio and HTTP (StreamableHttp) MCP servers but
		// rejects SSE ("migrate to streamable_http"); do not force SSE.
		// Goose authenticates via its ~/.config/goose config files (not
		// provider API-key env vars), so no StripEnv is declared.
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.local/share/goose",
			SessionDirTarget:    "/root/.local/share/goose",
		},
	}
}

func (a *GooseACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy Goose config files",
				SourceFiles: map[string][]string{
					"darwin": {".config/goose/config.yaml", ".config/goose/extensions.yaml", ".config/goose/secrets.yaml"},
					"linux":  {".config/goose/config.yaml", ".config/goose/extensions.yaml", ".config/goose/secrets.yaml"},
				},
				// Provider + model config only. Sessions, checkpoints, and
				// history under ~/.local/share/goose are deliberately
				// excluded; credentials stored in the OS keyring are not
				// represented by config files and must be set up remotely.
				TargetRelDir: ".config/goose",
			},
		},
	}
}

// goose configure is the interactive provider/model setup wizard (OAuth
// flows, API keys). Credentials land in ~/.config/goose.
func (a *GooseACP) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{gooseBin, "configure"},
		Description: "Configure Goose model provider credentials.",
	}
}

func (a *GooseACP) InstallScript() string {
	// Keep the installer in a temporary file so curl failures cannot be hidden
	// by a successful bash exit status. Install into the first writable absolute
	// directory already on PATH so the parent Kandev process can discover Goose
	// immediately after the install job finishes.
	return `set -eu
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL https://github.com/aaif-goose/goose/releases/download/stable/download_cli.sh -o "$tmp"
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
  echo "Error: no writable absolute directory is available on PATH for Goose" >&2
  exit 1
fi
GOOSE_BIN_DIR="$install_dir" CONFIGURE=false bash "$tmp"
command -v goose >/dev/null 2>&1
resolved="$(command -v goose)"
if [ "$resolved" != "$install_dir/goose" ]; then
  echo "Error: Goose installed at $install_dir/goose but another goose is earlier on PATH" >&2
  exit 1
fi
"$install_dir/goose" acp --help >/dev/null 2>&1`
}

func (a *GooseACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *GooseACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(gooseBin, "acp"),
	}
}

func (a *GooseACP) BillingType() usage.BillingType { return defaultBillingType() }
