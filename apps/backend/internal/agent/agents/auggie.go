package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/auggie_light.svg
var auggieLogoLight []byte

//go:embed logos/auggie_dark.svg
var auggieLogoDark []byte

var (
	_ Agent            = (*Auggie)(nil)
	_ PassthroughAgent = (*Auggie)(nil)
)

// Auggie implements Agent for the Augment Coding Agent.
type Auggie struct {
	StandardPassthrough
}

func NewAuggie() *Auggie {
	return &Auggie{
		StandardPassthrough: StandardPassthrough{
			PermSettings: auggiePermSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "CLI Passthrough",
				Description:       "Show terminal directly instead of chat interface",
				PassthroughCmd:    NewCommand("npx", "-y", "@augmentcode/auggie"),
				ModelFlag:         NewParam("--model", "{model}"),
				IdleTimeout:       3 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				ResumeFlag:        NewParam("-c"),
				SessionResumeFlag: NewParam("--resume"),
			},
		},
	}
}

func (a *Auggie) ID() string          { return "auggie" }
func (a *Auggie) Name() string        { return "Augment Coding Agent" }
func (a *Auggie) DisplayName() string { return "Auggie" }
func (a *Auggie) Description() string { return "Auggie CLI-powered autonomous coding agent." }
func (a *Auggie) Enabled() bool       { return true }

func (a *Auggie) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return auggieLogoDark
	}
	return auggieLogoLight
}

func (a *Auggie) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx,
		WithFileExists("~/.augment/.auggie.json"),
		WithCommand("auggie"),
	)
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = []string{expandHomePath("~/.augment/.auggie.json")}
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: false,
		SupportsShell:         false,
		SupportsWorkspaceOnly: false,
	}
	return result, nil
}

func (a *Auggie) DefaultModel() string { return "sonnet4.5" }

func (a *Auggie) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: auggieStaticModels(), SupportsDynamic: false}, nil
}

func (a *Auggie) CreateAdapter(cfg *adapter.Config, log *logger.Logger) (adapter.AgentAdapter, error) {
	return newACPAdapterWrapper(acp.NewAdapter(cfg.ToSharedConfig(), log)), nil
}

func (a *Auggie) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", "@augmentcode/auggie@0.15.0", "--acp").
		Model(NewParam("--model", "{model}"), opts.Model).
		Resume(NewParam("--resume"), opts.SessionID, false).
		Permissions("--permission", auggiePermTools, opts).
		Settings(auggiePermSettings, opts.PermissionValues).
		Build()
}


func (a *Auggie) Runtime() *RuntimeConfig {
	canRecover := false
	return &RuntimeConfig{
		Image:      "kandev/multi-agent",
		Tag:        "latest",
		Cmd:        Cmd("npx", "-y", "@augmentcode/auggie@0.15.0", "--acp").Build(),
		WorkingDir: "/workspace",
		RequiredEnv: []string{"AUGMENT_SESSION_AUTH"},
		Env:         map[string]string{},
		Mounts: []MountTemplate{
			{Source: "{workspace}", Target: "/workspace"},
		},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Capabilities:   []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
		Protocol:       agent.ProtocolACP,
		ModelFlag:      NewParam("--model", "{model}"),
		WorkspaceFlag:  "--workspace-root",
		SessionConfig: SessionConfig{
			ResumeFlag:         NewParam("--resume"),
			CanRecover:         &canRecover,
			SessionDirTemplate: "{home}/.augment/sessions",
			SessionDirTarget:   "/root/.augment/sessions",
		},
	}
}

func (a *Auggie) PermissionSettings() map[string]PermissionSetting {
	return auggiePermSettings
}

// --- Private ---

var auggiePermTools = []string{"launch-process", "save-file", "str-replace-editor", "remove-files"}

var auggiePermSettings = map[string]PermissionSetting{
	"auto_approve": {Supported: true, Default: true, Label: "Auto-approve", Description: "Automatically approve tool calls"},
	"allow_indexing": {Supported: true, Default: true, Label: "Allow indexing", Description: "Enable workspace indexing without confirmation",
		ApplyMethod: "cli_flag", CLIFlag: "--allow-indexing"},
}

func auggieStaticModels() []Model {
	return []Model{
		{ID: "sonnet4.5", Name: "Sonnet 4.5", Description: "Great for everyday tasks", Provider: "anthropic", ContextWindow: 200000, IsDefault: true, Source: "static"},
		{ID: "opus4.5", Name: "Claude Opus 4.5", Description: "Best for complex tasks", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
		{ID: "haiku4.5", Name: "Haiku 4.5", Description: "Fast and efficient responses", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
		{ID: "sonnet4", Name: "Sonnet 4", Description: "Legacy model", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
		{ID: "gpt5.1", Name: "GPT-5.1", Description: "Strong reasoning and planning", Provider: "openai", ContextWindow: 128000, Source: "static"},
		{ID: "gpt5", Name: "GPT-5", Description: "OpenAI GPT-5 legacy", Provider: "openai", ContextWindow: 128000, Source: "static"},
	}
}
