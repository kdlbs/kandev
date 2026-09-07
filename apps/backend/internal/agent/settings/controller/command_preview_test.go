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
