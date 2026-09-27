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

// R1-C: modeForProfile must also resolve the coordinator surface, or a
// coordinator instance constructed via NewWithProfile records s.mode as the
// ModeTask default (docs/specs/coordinator/system-design/copilot.md
// #attended-only), which then hides a later SetMode(coordinator) transition
// behind the "already in this mode" no-op guard.
func TestModeForProfile_MapsTheCoordinatorSurface(t *testing.T) {
	require.Equal(t, ModeCoordinator, modeForProfile(mcpprofile.NewCoordinator()))
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

// R1-C: SetMode(coordinator) must land on the fixed six-tool coordinator
// catalog, exactly like SetMode(automation) above
// (docs/specs/coordinator/system-design/copilot.md#attended-only). Before the
// fix, normalizeMode/surfaceForMode had no case for mcpmode.Coordinator, so a
// live SetMcpMode("coordinator") call on an existing (task-mode) instance
// silently fell back to the full task-mode tool catalog instead of narrowing
// to the coordinator allowlist.
func TestSetMode_CoordinatorUsesFixedSixToolCatalog(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10007, log, "", false, ModeTask)
	require.Contains(t, getRegisteredToolNames(s), "ask_user_question_kandev")

	s.SetMode(ModeCoordinator)

	require.Equal(t, mcpprofile.SurfaceCoordinator, s.Profile().Surface)
	cold := NewWithProfile(backend, "test-session", "test-task", 10008, log, "", false, mcpprofile.NewCoordinator())
	assert.ElementsMatch(t, getRegisteredToolNames(cold), getRegisteredToolNames(s))
	assert.NotContains(t, getRegisteredToolNames(s), "ask_user_question_kandev")
}
