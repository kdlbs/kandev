package agents

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/opencode_light.svg
var opencodeLogoLight []byte

//go:embed logos/opencode_dark.svg
var opencodeLogoDark []byte

const opencodePkg = "opencode-ai@1.1.59"

var (
	_ Agent            = (*OpenCodeACP)(nil)
	_ PassthroughAgent = (*OpenCodeACP)(nil)
	_ InferenceAgent   = (*OpenCodeACP)(nil)
)

// OpenCodeACP is the ACP protocol variant of OpenCode.
// Uses JSON-RPC 2.0 over stdin/stdout via "opencode acp" instead of REST/SSE.
type OpenCodeACP struct {
	StandardPassthrough
}

func NewOpenCodeACP() *OpenCodeACP {
	return &OpenCodeACP{
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

func (a *OpenCodeACP) ID() string          { return "opencode-acp" }
func (a *OpenCodeACP) Name() string        { return "OpenCode AI Agent (ACP)" }
func (a *OpenCodeACP) DisplayName() string { return "OpenCode" }
func (a *OpenCodeACP) Description() string {
	return "OpenCode coding agent using ACP protocol over stdin/stdout."
}
func (a *OpenCodeACP) Enabled() bool     { return true }
func (a *OpenCodeACP) DisplayOrder() int { return 4 }

func (a *OpenCodeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return opencodeLogoDark
	}
	return opencodeLogoLight
}

func (a *OpenCodeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	install := OSPaths{
		Linux: []string{"~/.opencode", "~/.config/opencode"},
		MacOS: []string{"~/.opencode", "~/.config/opencode", "~/Library/Application Support/ai.opencode.desktop"},
	}

	result, err := Detect(ctx, WithFileExists(install.Resolve()...))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.InstallationPaths = install.Expanded()
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OpenCodeACP) DefaultModel() string { return "opencode/gpt-5-nano" }

func (a *OpenCodeACP) ListModels(ctx context.Context) (*ModelList, error) {
	models, err := execAndParse(ctx, 30*time.Second, opencodeParseModels, "opencode", "models")
	if err != nil {
		return &ModelList{Models: opencodeStaticModels(), SupportsDynamic: false}, nil
	}
	return &ModelList{Models: models, SupportsDynamic: true}, nil
}

func (a *OpenCodeACP) BuildCommand(opts CommandOptions) Command {
	return Cmd("opencode", "acp").Build()
}

func (a *OpenCodeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("opencode", "acp").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}/.opencode",
		},
	}
}

func (a *OpenCodeACP) RemoteAuth() *RemoteAuth { return nil }

func (a *OpenCodeACP) InstallScript() string {
	return "npm install -g " + opencodePkg
}

func (a *OpenCodeACP) PermissionSettings() map[string]PermissionSetting {
	return opencodePermSettings
}

// InferenceConfig returns configuration for one-shot inference using ACP.
func (a *OpenCodeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand("opencode", "acp"),
		ModelFlag: NewParam("--model", "{model}"),
	}
}

// InferenceModels returns models available for one-shot inference.
func (a *OpenCodeACP) InferenceModels() []InferenceModel {
	return ModelsToInferenceModels(opencodeStaticModels())
}

var opencodePermSettings = map[string]PermissionSetting{
	"auto_approve": {
		Supported: true, Default: true, Label: "Auto-approve", Description: "Automatically approve tool calls",
		ApplyMethod: "env",
	},
}

func opencodeStaticModels() []Model {
	return []Model{
		{ID: "opencode/gpt-5-nano", Name: "GPT-5 Nano", Description: "OpenAI GPT-5 Nano", Provider: ProviderOpenAI, ContextWindow: 200000, IsDefault: true, Source: ModelSourceStatic},
		{ID: "anthropic/claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Description: "Anthropic Claude Sonnet 4", Provider: ProviderAnthropic, ContextWindow: 200000, Source: ModelSourceStatic},
		{ID: "anthropic/claude-opus-4-20250514", Name: "Claude Opus 4", Description: "Anthropic Claude Opus 4", Provider: ProviderAnthropic, ContextWindow: 200000, Source: ModelSourceStatic},
		{ID: "openai/gpt-4.1", Name: "GPT-4.1", Description: "OpenAI GPT-4.1", Provider: ProviderOpenAI, ContextWindow: 200000, Source: ModelSourceStatic},
		{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", Description: "Google Gemini 2.5 Pro", Provider: ProviderGoogle, ContextWindow: 2000000, Source: ModelSourceStatic},
	}
}

// opencodeParseModels parses "opencode models" output.
// Format: one model ID per line, optionally in "provider/model" format.
func opencodeParseModels(output string) ([]Model, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	models := make([]Model, 0, len(lines))
	defaultModel := "opencode/gpt-5-nano"

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
			Source:    ModelSourceDynamic,
		})
	}
	return models, nil
}
