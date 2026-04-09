package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/gemini_light.svg
var geminiLogoLight []byte

//go:embed logos/gemini_dark.svg
var geminiLogoDark []byte

const geminiPkg = "@google/gemini-cli"

var (
	_ Agent            = (*Gemini)(nil)
	_ PassthroughAgent = (*Gemini)(nil)
	_ InferenceAgent   = (*Gemini)(nil)
)

type Gemini struct {
	StandardPassthrough
}

func NewGemini() *Gemini {
	return &Gemini{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "@google/gemini-cli"),
				ModelFlag:      NewParam("--model", "{model}"),
				PromptFlag:     NewParam("--prompt-interactive", "{prompt}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				ResumeFlag:     NewParam("--resume", "latest"),
			},
		},
	}
}

func (a *Gemini) ID() string          { return "gemini" }
func (a *Gemini) Name() string        { return "Google Gemini CLI Agent" }
func (a *Gemini) DisplayName() string { return "Gemini" }
func (a *Gemini) Description() string {
	return "Google Gemini CLI-powered autonomous coding agent using ACP protocol."
}
func (a *Gemini) Enabled() bool     { return true }
func (a *Gemini) DisplayOrder() int { return 5 }

func (a *Gemini) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return geminiLogoDark
	}
	return geminiLogoLight
}

func (a *Gemini) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Check for the gemini CLI on PATH. Auth state is surfaced later by the
	// ACP probe, not by scanning ~/.gemini.
	result, err := Detect(ctx, WithCommand("gemini"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *Gemini) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", geminiPkg, "--acp").Build()
}

func (a *Gemini) Runtime() *RuntimeConfig {
	canRecover := false
	return &RuntimeConfig{
		Cmd:            Cmd("npx", "-y", geminiPkg, "--acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			CanRecover:         &canRecover,
			SessionDirTemplate: "{home}/.gemini",
		},
	}
}

func (a *Gemini) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:  "files",
				Label: "Copy auth files",
				SourceFiles: map[string][]string{
					"darwin": {".gemini/oauth_creds.json", ".gemini/settings.json", ".gemini/google_accounts.json"},
					"linux":  {".gemini/oauth_creds.json", ".gemini/settings.json", ".gemini/google_accounts.json"},
				},
				TargetRelDir: ".gemini",
			},
			{
				Type:   "env",
				EnvVar: "GEMINI_API_KEY",
			},
		},
	}
}

func (a *Gemini) InstallScript() string {
	return "npm install -g " + geminiPkg
}

func (a *Gemini) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
// Gemini CLI speaks ACP natively via the --acp flag, so there's no separate
// _acp variant.
func (a *Gemini) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("npx", "-y", geminiPkg, "--acp"),
	}
}
