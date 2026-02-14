package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/codex"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/codex_light.svg
var codexLogoLight []byte

//go:embed logos/codex_dark.svg
var codexLogoDark []byte

var (
	_ Agent            = (*Codex)(nil)
	_ PassthroughAgent = (*Codex)(nil)
)

type Codex struct {
	StandardPassthrough
}

func NewCodex() *Codex {
	return &Codex{
		StandardPassthrough: StandardPassthrough{
			PermSettings: codexPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("npx", "-y", "@openai/codex", "--full-auto"),
				ModelFlag:      NewParam("--model", "{model}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *Codex) ID() string          { return "codex" }
func (a *Codex) Name() string        { return "OpenAI Codex Agent" }
func (a *Codex) DisplayName() string { return "Codex" }
func (a *Codex) Description() string {
	return "OpenAI Codex CLI-powered autonomous coding agent using the Codex app-server protocol."
}
func (a *Codex) Enabled() bool { return true }

func (a *Codex) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return codexLogoDark
	}
	return codexLogoLight
}

func (a *Codex) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithFileExists("~/.codex/auth.json"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = []string{expandHomePath("~/.codex/auth.json")}
	return result, nil
}

func (a *Codex) DefaultModel() string { return "gpt-5.2-codex" }

func (a *Codex) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: codexStaticModels(), SupportsDynamic: false}, nil
}

func (a *Codex) CreateAdapter(cfg *adapter.Config, log *logger.Logger) (adapter.AgentAdapter, error) {
	return newCodexAdapterWrapper(codex.NewAdapter(cfg.ToSharedConfig(), log)), nil
}

func (a *Codex) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", "@openai/codex@0.98.0", "app-server").
		Model(NewParam("-c", "model=\"{model}\""), opts.Model).
		Settings(codexPermSettings, opts.PermissionValues).
		Build()
}


func (a *Codex) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Image:      "kandev/multi-agent",
		Tag:        "latest",
		Cmd:        Cmd("npx", "-y", "@openai/codex@0.98.0", "app-server").Build(),
		WorkingDir: "/workspace",
		Env:        map[string]string{},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
			{Source: "{home}/.codex", Target: "/root/.codex"},
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Capabilities:   []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
		Protocol:       agent.ProtocolCodex,
		ModelFlag:      NewParam("-c", "model=\"{model}\""),
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
		},
	}
}

func (a *Codex) PermissionSettings() map[string]PermissionSetting {
	return codexPermSettings
}

var codexPermSettings = map[string]PermissionSetting{
	"auto_approve": {Supported: true, Default: true, Label: "Auto-approve", Description: "Skip approval requests from Codex",
		ApplyMethod: "acp", CLIFlag: "--full-auto"},
}

func codexStaticModels() []Model {
	return []Model{
		{ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex", Description: "Latest frontier agentic coding model", Provider: "openai", ContextWindow: 200000, IsDefault: true, Source: "static"},
		{ID: "gpt-5.1-codex-max", Name: "GPT-5.1 Codex Max", Description: "Codex-optimized flagship for deep and fast reasoning", Provider: "openai", ContextWindow: 200000, Source: "static"},
		{ID: "gpt-5.1-codex-mini", Name: "GPT-5.1 Codex Mini", Description: "Optimized for codex. Cheaper, faster, but less capable", Provider: "openai", ContextWindow: 200000, Source: "static"},
		{ID: "gpt-5.2", Name: "GPT-5.2", Description: "Latest frontier model with improvements across knowledge, reasoning and coding", Provider: "openai", ContextWindow: 200000, Source: "static"},
	}
}
