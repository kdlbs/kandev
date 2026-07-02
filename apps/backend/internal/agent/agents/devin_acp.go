//nolint:dupl,goconst // Native-binary ACP agents (Cursor, Kimi, Kiro, Qoder, Trae, Omp, Devin) follow the same minimal scaffold; shared literals (CLI Passthrough, etc.) live in every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/devin_acp_light.svg
var devinACPLogoLight []byte

//go:embed logos/devin_acp_dark.svg
var devinACPLogoDark []byte

const devinACPBin = "devin"

const (
	devinCredentialsDir     = ".local/share/devin"
	devinCredentialsRelPath = devinCredentialsDir + "/credentials.toml"
	devinDefaultAPIServer   = "https://server.self-serve.windsurf.com"
)

var (
	_ Agent            = (*DevinACP)(nil)
	_ PassthroughAgent = (*DevinACP)(nil)
	_ InferenceAgent   = (*DevinACP)(nil)
)

// DevinACP implements Agent for Cognition's Devin CLI using ACP.
// The CLI binary (devin) is installed via the Devin Desktop app or
// standalone installer. It speaks ACP natively via the `devin acp` subcommand.
//
// Credential handling: `devin acp` checks the ACP_BACKEND environment variable.
// When set (e.g. by Windsurf Next), it requires the ACP host to call
// `authenticate` and refuses local credentials. When unset, it falls back to
// reading ~/.local/share/devin/credentials.toml directly. The process manager
// strips ACP_BACKEND from the child environment so Devin uses the fall-back
// path — no protocol-level authenticate needed.
type DevinACP struct {
	StandardPassthrough
}

func NewDevinACP() *DevinACP {
	return &DevinACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(devinACPBin),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *DevinACP) ID() string          { return "devin-acp" }
func (a *DevinACP) Name() string        { return "Devin ACP Agent" }
func (a *DevinACP) DisplayName() string { return "Devin" }
func (a *DevinACP) Description() string {
	return "Cognition Devin coding agent using the ACP protocol via `devin acp`."
}
func (a *DevinACP) Enabled() bool     { return true }
func (a *DevinACP) DisplayOrder() int { return 19 }

func (a *DevinACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return devinACPLogoDark
	}
	return devinACPLogoLight
}

func (a *DevinACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(devinACPBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *DevinACP) BuildCommand(opts CommandOptions) Command {
	return Cmd(devinACPBin, "acp").Build()
}

func (a *DevinACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd(devinACPBin, "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		// Devin advertises mcpCapabilities {http: false, sse: false} but
		// actually supports streamable HTTP MCP (used by Windsurf Next).
		// Without these overrides, filterMcpServersByCapabilities drops the
		// Kandev MCP server (exposed via HTTP /mcp and SSE /sse), and the
		// agent loses access to task tools (get_task_plan_kandev, etc.).
		AssumeMcpSse:  true,
		AssumeMcpHttp: true,
		// See DevinACP doc comment for the ACP_BACKEND rationale.
		StripEnv: []string{"ACP_BACKEND"},
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.local/share/devin",
			SessionDirTarget:    "/root/.local/share/devin",
		},
	}
}

func (a *DevinACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy Devin CLI credentials",
				SourceFiles: map[string][]string{
					"darwin": {devinCredentialsRelPath},
					"linux":  {devinCredentialsRelPath},
				},
				TargetRelDir: devinCredentialsDir,
			},
			{
				Type:      "env",
				EnvVar:    "WINDSURF_API_KEY",
				SetupHint: "Set WINDSURF_API_KEY to authenticate Devin CLI in remote or headless environments.",
				SetupScript: `mkdir -p "${HOME}/.local/share/devin"
escape_toml() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}
api_key="$(escape_toml "${WINDSURF_API_KEY}")"
api_server="$(escape_toml "${WINDSURF_API_SERVER_URL:-` + devinDefaultAPIServer + `}")"
umask 077
cat > "${HOME}/.local/share/devin/credentials.toml" <<CREDS
windsurf_api_key = "${api_key}"
api_server_url = "${api_server}"
CREDS
chmod 600 "${HOME}/.local/share/devin/credentials.toml"`,
			},
		},
	}
}

func (a *DevinACP) InstallScript() string {
	return `set -e
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL https://cli.devin.ai/install.sh -o "$tmp"
if ! bash "$tmp" && [ ! -x "$HOME/.local/bin/devin" ]; then
  exit 1
fi
export PATH="$HOME/.local/bin:$PATH"
persist_devin_path() {
  grep -qxF 'export PATH="$HOME/.local/bin:$PATH"' "$1" 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$1"
}
persist_devin_path "$HOME/.profile"
persist_devin_path "$HOME/.bash_profile"
persist_devin_path "$HOME/.bashrc"
persist_devin_path "$HOME/.zprofile"
persist_devin_path "$HOME/.zshrc"
devin --version >/dev/null`
}

func (a *DevinACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *DevinACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(devinACPBin, "acp"),
	}
}

func (a *DevinACP) BillingType() usage.BillingType { return defaultBillingType() }
