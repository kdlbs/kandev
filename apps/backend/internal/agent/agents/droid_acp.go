package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/droid_acp_light.svg
var droidACPLogoLight []byte

//go:embed logos/droid_acp_dark.svg
var droidACPLogoDark []byte

const droidACPPkg = "droid"

var (
	_ Agent            = (*DroidACP)(nil)
	_ PassthroughAgent = (*DroidACP)(nil)
	_ InferenceAgent   = (*DroidACP)(nil)
)

// DroidACP implements Agent for Factory's Droid CLI using ACP.
//
// Factory's public CLI reference documents `droid exec` as "single-shot
// execution" mode and does not list `acp` as a valid `--output-format` value.
// We rely on the same invocation acpx uses (`droid exec --output-format acp`).
// Native session resume is intentionally disabled until we can verify the
// process stays alive across multiple session/prompt calls — if it doesn't,
// our fork-session fallback handles continuity by re-invoking with -s.
type DroidACP struct {
	StandardPassthrough
}

func NewDroidACP() *DroidACP {
	return &DroidACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", droidACPPkg),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *DroidACP) ID() string          { return "droid-acp" }
func (a *DroidACP) Name() string        { return "Factory Droid ACP Agent" }
func (a *DroidACP) DisplayName() string { return "Droid" }
func (a *DroidACP) Description() string {
	return "Factory.ai Droid coding agent via `droid exec --output-format acp`."
}
func (a *DroidACP) Enabled() bool     { return true }
func (a *DroidACP) DisplayOrder() int { return 10 }

func (a *DroidACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return droidACPLogoDark
	}
	return droidACPLogoLight
}

func (a *DroidACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("droid"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	return result, nil
}

func (a *DroidACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", droidACPPkg, "exec", "--output-format", "acp").Build()
}

func (a *DroidACP) Runtime() *RuntimeConfig {
	canRecover := false
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", droidACPPkg, "exec", "--output-format", "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: false,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.factory",
		},
	}
}

func (a *DroidACP) RemoteAuth() *RemoteAuth { return nil }

func (a *DroidACP) InstallScript() string {
	return "npm install -g " + droidACPPkg
}

func (a *DroidACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *DroidACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", droidACPPkg, "exec", "--output-format", "acp"),
	}
}

func (a *DroidACP) BillingType() usage.BillingType { return defaultBillingType() }
