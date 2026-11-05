package controller

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/stretchr/testify/require"
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

func TestController_PreviewAgentCommand_WrapperInBothTransports(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{"claude-acp": agents.NewClaudeACP()})
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			plain, err := controller.PreviewAgentCommand(t.Context(), "claude-acp", CommandPreviewRequest{CLIPassthrough: native})
			require.NoError(t, err)
			wrapped, err := controller.PreviewAgentCommand(t.Context(), "claude-acp", CommandPreviewRequest{CLIPassthrough: native, CommandPrefix: `env 'LABEL=quoted value' --`})
			require.NoError(t, err)
			require.Equal(t, append([]string{"env", "LABEL=quoted value", "--"}, plain.Command...), wrapped.Command)
		})
	}
}
