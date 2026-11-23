package controller

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-CLI-PASSTHROUGH-LAUNCH-001.5
func TestController_PreviewAgentCommand_PassthroughIncludesCLIFlags(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{
		"claude-acp": agents.NewClaudeACP(),
	})

	result, err := controller.PreviewAgentCommand(context.Background(), "claude-acp", CommandPreviewRequest{
		CLIPassthrough: true,
		CLIFlags: []dto.CLIFlagDTO{
			{Flag: "--verbose", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("PreviewAgentCommand() error = %v", err)
	}

	want := []string{"npx", "-y", "@anthropic-ai/claude-code", "--verbose"}
	if !slices.Equal(result.Command, want) {
		t.Fatalf("preview command = %#v, want %#v", result.Command, want)
	}
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-002.1, AC-AGENTS-COPILOT-CONTEXT-TIER-002.2, AC-AGENTS-COPILOT-CONTEXT-TIER-002.3
func TestController_PreviewAgentCommand_CopilotContextTier(t *testing.T) {
	copilot := agents.NewCopilotACP()
	controller := newTestController(map[string]agents.Agent{
		agents.CopilotACPAgentID: copilot,
	})
	baseACP := copilot.ManagedNPMRuntime().CachedACPCommand().Args()

	t.Run("ACP preview includes --context long_context before user CLI flags", func(t *testing.T) {
		result, err := controller.PreviewAgentCommand(context.Background(), agents.CopilotACPAgentID, CommandPreviewRequest{
			Model:         "gpt-5.4",
			ConfigOptions: map[string]string{"context_tier": "long_context"},
			CLIFlags:      []dto.CLIFlagDTO{{Flag: "--allow-all-tools", Enabled: true}},
		})
		if err != nil {
			t.Fatalf("PreviewAgentCommand() error = %v", err)
		}
		want := append(append([]string(nil), baseACP...), "--context", "long_context", "--allow-all-tools")
		if !slices.Equal(result.Command, want) {
			t.Fatalf("preview command = %#v, want %#v", result.Command, want)
		}
	})

	t.Run("explicit --context in cli_flags overrides config_options", func(t *testing.T) {
		result, err := controller.PreviewAgentCommand(context.Background(), agents.CopilotACPAgentID, CommandPreviewRequest{
			ConfigOptions: map[string]string{"context_tier": "long_context"},
			CLIFlags:      []dto.CLIFlagDTO{{Flag: "--context default", Enabled: true}},
		})
		if err != nil {
			t.Fatalf("PreviewAgentCommand() error = %v", err)
		}
		want := append(append([]string(nil), baseACP...), "--context", "default")
		if !slices.Equal(result.Command, want) {
			t.Fatalf("preview command = %#v, want %#v", result.Command, want)
		}
	})

	t.Run("passthrough preview includes --context long_context", func(t *testing.T) {
		result, err := controller.PreviewAgentCommand(context.Background(), agents.CopilotACPAgentID, CommandPreviewRequest{
			Model:          "gpt-5.4",
			CLIPassthrough: true,
			ConfigOptions:  map[string]string{"context_tier": "long_context"},
		})
		if err != nil {
			t.Fatalf("PreviewAgentCommand() error = %v", err)
		}
		want := []string{"npx", "--yes", "--prefer-offline", "@github/copilot", "--model", "gpt-5.4", "--context", "long_context"}
		if !slices.Equal(result.Command, want) {
			t.Fatalf("passthrough preview = %#v, want %#v", result.Command, want)
		}
	})
}
