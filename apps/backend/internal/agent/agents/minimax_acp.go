//nolint:dupl,goconst // Native ACP agents share the same identity and runtime scaffold.
package agents

import (
	"context"
	_ "embed"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/minimax_acp_light.svg
var minimaxACPLogoLight []byte

//go:embed logos/minimax_acp_dark.svg
var minimaxACPLogoDark []byte

const minimaxBin = "mcode"

var (
	_ Agent            = (*MiniMaxACP)(nil)
	_ PassthroughAgent = (*MiniMaxACP)(nil)
	_ InferenceAgent   = (*MiniMaxACP)(nil)
	_ LoginAgent       = (*MiniMaxACP)(nil)
)

// MiniMaxACP runs the official MiniMax Code CLI's native ACP server.
type MiniMaxACP struct{ StandardPassthrough }

func NewMiniMaxACP() *MiniMaxACP {
	return &MiniMaxACP{StandardPassthrough: StandardPassthrough{
		PermSettings: emptyPermSettings,
		Cfg: PassthroughConfig{
			Supported: true, Label: "CLI Passthrough",
			Description:    "Show terminal directly instead of chat interface",
			PassthroughCmd: NewCommand(minimaxBin),
			ModelFlag:      NewParam("--model", "{model}"),
			ResumeFlag:     NewParam("--continue"), SessionResumeFlag: NewParam("--session"),
			IdleTimeout: 3 * time.Second, BufferMaxBytes: DefaultBufferMaxBytes,
		},
	}}
}

func (a *MiniMaxACP) ID() string          { return "minimax-acp" }
func (a *MiniMaxACP) Name() string        { return "MiniMax Code ACP" }
func (a *MiniMaxACP) DisplayName() string { return "MiniMax" }
func (a *MiniMaxACP) Description() string {
	return "MiniMax Code using its native ACP server and subscription login."
}
func (a *MiniMaxACP) Enabled() bool     { return true }
func (a *MiniMaxACP) DisplayOrder() int { return 24 }
func (a *MiniMaxACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return minimaxACPLogoDark
	}
	return minimaxACPLogoLight
}

func (a *MiniMaxACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommandCheck(minimaxBin, "--version"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities.SupportsSessionResume = true
	return result, nil
}

func (a *MiniMaxACP) BuildCommand(_ CommandOptions) Command {
	return NewCommand(minimaxBin, "acp")
}

func (a *MiniMaxACP) BuildPassthroughCommand(opts PassthroughOptions) Command {
	opts.Model = minimaxCLIModel(opts.Model)
	return a.StandardPassthrough.BuildPassthroughCommand(opts)
}

// minimaxCLIModel converts only complete ACP identities. Unrecognized values
// remain explicit CLI inputs so a malformed selection cannot become a default.
func minimaxCLIModel(model string) string {
	parts := strings.Split(model, ":")
	if len(parts) < 4 || parts[0] != "m" {
		return model
	}
	expected := 4
	if parts[3] == "v" {
		expected = 5
	} else if parts[3] != "u" {
		return model
	}
	if len(parts) != expected {
		return model
	}
	provider, err := url.PathUnescape(parts[1])
	if err != nil || provider == "" {
		return model
	}
	name, err := url.PathUnescape(parts[2])
	if err != nil || name == "" {
		return model
	}
	value := provider + "/" + name
	if parts[3] == "v" {
		variant, decodeErr := url.PathUnescape(parts[4])
		if decodeErr != nil {
			return model
		}
		if variant == "" {
			variant = "none-thinking"
		}
		value += "#" + variant
	}
	return value
}

func (a *MiniMaxACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd: NewCommand(minimaxBin, "acp"), WorkingDir: "{workspace}",
		Env: map[string]string{}, ResourceLimits: DefaultResourceLimits,
		Protocol: agent.ProtocolACP,
		// The persisted executor mount and login both own the default data root.
		StripEnv: minimaxDataOverrides(),
		SessionConfig: SessionConfig{
			NativeSessionResume: true, CanRecover: &canRecover,
			SessionDirTemplate: "{home}/.minimax", SessionDirTarget: "/root/.minimax",
		},
	}
}

func minimaxDataOverrides() []string {
	return []string{"MINIMAX_DATA_DIR", "MAVIS_DATA_DIR", "__MAVIS_RUNTIME_DATA_DIR", "__MAVIS_RUNTIME_PROFILE"}
}

// OAuth credential records include the absolute auth-home identity. A copied
// host token is not a credential for a different executor home.
func (a *MiniMaxACP) RemoteAuth() *RemoteAuth { return nil }

func (a *MiniMaxACP) LoginCommand() *LoginCommand {
	cmd := []string{"env"}
	for _, key := range minimaxDataOverrides() {
		cmd = append(cmd, "-u", key)
	}
	cmd = append(cmd, minimaxBin, "login", "--no-browser")
	variants := make(map[string][]string, 2)
	for _, region := range []string{"cn", "global"} {
		variants[region] = append(append([]string{}, cmd...), "--region", region)
	}
	return &LoginCommand{Cmd: cmd, Variants: variants}
}

func (a *MiniMaxACP) InstallScript() string {
	return "npm install -g @minimax-ai/code@0.5.10 --registry=https://registry.npmjs.org/ --ignore-scripts=false --include=optional --allow-scripts=@minimax-ai/code,better-sqlite3"
}
func (a *MiniMaxACP) PermissionSettings() map[string]PermissionSetting { return emptyPermSettings }
func (a *MiniMaxACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{Supported: true, Command: NewCommand(minimaxBin, "acp")}
}
func (a *MiniMaxACP) BillingType() usage.BillingType { return defaultBillingType() }
