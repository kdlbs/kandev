package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A spawned session's first turn must keep the spawner-attribution block: the
// spawned agent has no other way to learn who spawned it or how to reply. The
// block is prepended and simultaneously whitelisted, so the first-turn
// canonicalizer keeps it instead of stripping it as untrusted content.
func TestApplySpawnOriginContext_SurvivesFirstTurnCanonicalization(t *testing.T) {
	origin := &SpawnOrigin{TaskID: "task-spawner", SessionID: "sess-spawner", SessionName: "planner"}

	prompt, trusted := applySpawnOriginContext("review the diff please", origin)
	require.NotEmpty(t, trusted)

	injected := sysprompt.InjectKandevContextWithOptions(
		"task-abc", "sess-new", prompt, sysprompt.KandevContextOptions{}, trusted,
	)
	assert.Contains(t, injected, "sess-spawner")
	assert.Contains(t, injected, `session "planner" (sess-spawner)`)
	assert.Contains(t, injected, "message_task_kandev")
	assert.Contains(t, injected, "review the diff please")
	assert.Contains(t, injected, "Kandev Task ID: task-abc", "the canonical task context still leads")
}

// Ordinary (non-spawned) launches carry no attribution block and nothing to whitelist.
func TestApplySpawnOriginContext_NoOrigin(t *testing.T) {
	prompt, trusted := applySpawnOriginContext("do the work", nil)
	assert.Equal(t, "do the work", prompt)
	assert.Empty(t, trusted)

	// An origin without a session (external MCP caller) has nothing to attribute.
	prompt, trusted = applySpawnOriginContext("do the work", &SpawnOrigin{TaskID: "task-spawner"})
	assert.Equal(t, "do the work", prompt)
	assert.Empty(t, trusted)
}

// Passthrough profiles skip the Kandev MCP wrap entirely, so attribution has to
// reach them as plain text — a <kandev-system> block would surface as raw markup
// in the terminal, and dropping it would leave the spawned agent unable to reply.
func TestApplySpawnOriginText_PlainForPassthrough(t *testing.T) {
	origin := &SpawnOrigin{TaskID: "task-spawner", SessionID: "sess-spawner", SessionName: "planner"}

	prompt := applySpawnOriginText("review the diff please", origin)
	assert.NotContains(t, prompt, sysprompt.TagStart)
	assert.NotContains(t, prompt, sysprompt.TagEnd)
	assert.Contains(t, prompt, `session "planner" (sess-spawner)`)
	assert.Contains(t, prompt, "message_task_kandev")
	assert.True(t, strings.HasSuffix(prompt, "review the diff please"))

	assert.Equal(t, "plain", applySpawnOriginText("plain", nil))
}

// The passthrough branch of the launch path must not reach the MCP injectors at
// all, and must still hand the spawned agent its attribution.
func TestApplyLaunchPromptContext_PassthroughKeepsAttributionWithoutMCPBlock(t *testing.T) {
	out := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:        "review the diff please",
		taskID:        "task-abc",
		sessionID:     "sess-new",
		isPassthrough: true,
		spawnOrigin:   &SpawnOrigin{TaskID: "task-spawner", SessionID: "sess-spawner"},
	})

	assert.NotContains(t, out, sysprompt.TagStart)
	assert.NotContains(t, out, "KANDEV MCP TOOLS")
	assert.Contains(t, out, "session sess-spawner of task task-spawner")
	assert.Contains(t, out, "review the diff please")
}

func TestApplyLaunchPromptContext_PassthroughAddsPendingTitleInstructionOnlyWhenNeeded(t *testing.T) {
	withTitle := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:               "review the diff please",
		isPassthrough:        true,
		includeTaskTitleTool: true,
	})
	assert.Contains(t, withTitle, "set_task_title_kandev")
	assert.Contains(t, withTitle, "targeting about 3 words")
	assert.Contains(t, withTitle, "review the diff please")
	assert.NotContains(t, withTitle, sysprompt.TagStart)

	withoutTitle := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:        "review the diff please",
		isPassthrough: true,
	})
	assert.NotContains(t, withoutTitle, "set_task_title_kandev")
	assert.Equal(t, "review the diff please", withoutTitle)
}
