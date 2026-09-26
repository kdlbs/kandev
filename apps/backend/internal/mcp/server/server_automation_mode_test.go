package mcp

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeForProfile_MapsTheAutomationSurface(t *testing.T) {
	require.Equal(t, ModeAutomation, modeForProfile(mcpprofile.NewAutomation()))
}

// SetMode(automation) must land on exactly the fixed coordinator catalog a
// cold automation launch gets. SetMode carries the previous profile's
// capabilities across the surface change, and the user-question group is
// capability-gated rather than surface-gated, so without the drop a task-mode
// instance switched at runtime kept CapabilityUserQuestion and advertised
// ask_user_question_kandev on a surface whose execution-time allowlist
// refuses it.
func TestSetMode_AutomationDropsTaskLocalQuestionCapabilities(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	require.Contains(t, getRegisteredToolNames(s), "ask_user_question_kandev")

	s.SetMode(ModeAutomation)

	cold := NewWithProfile(backend, "test-session", "test-task", 10006, log, "", false, mcpprofile.NewAutomation())
	assert.ElementsMatch(t, getRegisteredToolNames(cold), getRegisteredToolNames(s))
	assert.NotContains(t, getRegisteredToolNames(s), "ask_user_question_kandev")

	s.SetMode(ModeTask)
	assert.Contains(t, getRegisteredToolNames(s), "ask_user_question_kandev")
}
