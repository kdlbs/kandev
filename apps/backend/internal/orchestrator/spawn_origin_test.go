package orchestrator

import (
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
