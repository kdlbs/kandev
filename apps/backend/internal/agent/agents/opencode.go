package agents

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/opencode"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/opencode_light.svg
var opencodeLogoLight []byte

//go:embed logos/opencode_dark.svg
var opencodeLogoDark []byte

var (
	_ Agent            = (*OpenCode)(nil)
	_ PassthroughAgent = (*OpenCode)(nil)
)

type OpenCode struct {
	StandardPassthrough
}

func NewOpenCode() *OpenCode {
	return &OpenCode{
		StandardPassthrough: StandardPassthrough{
			PermSettings: opencodePermSettings,
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
			},
		},
	}
}

func (a *OpenCode) ID() string          { return "opencode" }
func (a *OpenCode) Name() string        { return "OpenCode AI Agent" }
func (a *OpenCode) DisplayName() string { return "OpenCode" }
func (a *OpenCode) Description() string {
	return "OpenCode CLI-powered autonomous coding agent using REST/SSE protocol."
}
func (a *OpenCode) Enabled() bool { return true }

func (a *OpenCode) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return opencodeLogoDark
	}
	return opencodeLogoLight
}

func (a *OpenCode) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	install := OSPaths{
		Linux: []string{"~/.opencode", "~/.config/opencode"},
		MacOS: []string{"~/.opencode", "~/.config/opencode", "~/Library/Application Support/ai.opencode.desktop"},
	}
	mcp := OSPaths{
		Linux: []string{"~/.config/opencode/opencode.json", "~/.config/opencode/opencode.jsonc"},
		MacOS: []string{"~/.config/opencode/opencode.json", "~/.config/opencode/opencode.jsonc", "~/Library/Application Support/opencode/opencode.json"},
	}

	result, err := Detect(ctx, WithFileExists(install.Resolve()...))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = install.Expanded()
	result.MCPConfigPaths = mcp.Expanded()
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OpenCode) DefaultModel() string { return "anthropic/claude-sonnet-4-20250514" }

func (a *OpenCode) ListModels(ctx context.Context) (*ModelList, error) {
	models, err := execAndParse(ctx, 30*time.Second, opencodeParseModels, "opencode", "models")
	if err != nil {
		return &ModelList{Models: opencodeStaticModels(), SupportsDynamic: true}, nil
	}
	return &ModelList{Models: models, SupportsDynamic: true}, nil
}

func (a *OpenCode) CreateAdapter(cfg *adapter.Config, log *logger.Logger) (adapter.AgentAdapter, error) {
	return newOpenCodeAdapterWrapper(opencode.NewAdapter(cfg.ToSharedConfig(), log)), nil
}

func (a *OpenCode) BuildCommand(opts CommandOptions) Command {
	return Cmd("npx", "-y", "opencode-ai@1.1.25", "serve", "--hostname", "127.0.0.1", "--port", "0").Build()
}


func (a *OpenCode) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:         Cmd("npx", "-y", "opencode-ai@1.1.25", "serve", "--hostname", "127.0.0.1", "--port", "0").Build(),
		WorkingDir:  "{workspace}",
		Env:         map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Capabilities:   []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
		Protocol:       agent.ProtocolOpenCode,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.opencode",
		},
	}
}

func (a *OpenCode) PermissionSettings() map[string]PermissionSetting {
	return opencodePermSettings
}

var opencodePermSettings = map[string]PermissionSetting{
	"auto_approve": {Supported: true, Default: true, Label: "Auto-approve", Description: "Automatically approve tool calls",
		ApplyMethod: "env"},
}

func opencodeStaticModels() []Model {
	return []Model{
		{ID: "anthropic/claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Description: "Anthropic Claude Sonnet 4", Provider: "anthropic", ContextWindow: 200000, IsDefault: true, Source: "static"},
		{ID: "anthropic/claude-opus-4-20250514", Name: "Claude Opus 4", Description: "Anthropic Claude Opus 4", Provider: "anthropic", ContextWindow: 200000, Source: "static"},
		{ID: "openai/gpt-4.1", Name: "GPT-4.1", Description: "OpenAI GPT-4.1", Provider: "openai", ContextWindow: 200000, Source: "static"},
		{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", Description: "Google Gemini 2.5 Pro", Provider: "google", ContextWindow: 2000000, Source: "static"},
	}
}

// opencodeParseModels parses "opencode models" output.
// Format: one model ID per line, optionally in "provider/model" format.
func opencodeParseModels(output string) ([]Model, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	models := make([]Model, 0, len(lines))
	defaultModel := "anthropic/claude-sonnet-4-20250514"

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var provider, modelID, name string
		if idx := strings.LastIndex(line, "/"); idx > 0 {
			provider = line[:idx]
			modelID = line
			name = line[idx+1:]
		} else {
			modelID = line
			name = line
		}

		models = append(models, Model{
			ID:        modelID,
			Name:      name,
			Provider:  provider,
			IsDefault: modelID == defaultModel,
			Source:    "dynamic",
		})
	}
	return models, nil
}
