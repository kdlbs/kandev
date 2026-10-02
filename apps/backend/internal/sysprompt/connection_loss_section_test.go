package sysprompt

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/config/prompts"
	"github.com/stretchr/testify/assert"
)

// TestKandevContextHasConnectionLossSection pins
// AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.4: the rendered Kandev context
// must tell the agent that a Kandev tool call can wait through a connection
// loss, and that it must never wrap the call in its own sleep or polling
// loop. Unlike step_complete_section, this guidance applies to every
// session, so the template placeholder is always resolved rather than being
// gated behind a KandevContextOptions field.
func TestKandevContextHasConnectionLossSection(t *testing.T) {
	template := prompts.Get("kandev-context")
	assert.Equal(t, 1, strings.Count(template, "{connection_loss_section}"),
		"kandev-context template should have exactly 1 {connection_loss_section} placeholder")

	cases := map[string]string{
		"KandevContext":              KandevContext(),
		"FormatKandevContextDefault": FormatKandevContextWithOptions(testTaskID, testSessionID, KandevContextOptions{}),
		"FormatKandevContextAutopilot": FormatKandevContextWithOptions(testTaskID, testSessionID, KandevContextOptions{
			Autopilot: true,
		}),
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, ctx, "CONNECTION LOSS")
			assert.Contains(t, ctx, "wait")
			assert.Contains(t, ctx, "sleep")
			assert.Contains(t, ctx, "poll")
			assert.NotContains(t, ctx, "{connection_loss_section}")
		})
	}
}
