package mcp

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectCoordinatorCatalogIsRestrictedAndCannotSwitchModes(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceProjectCoordinator, nil, nil))

	want := []string{
		"get_agent_project_kandev",
		"list_agent_project_workers_kandev",
		"create_agent_project_worker_kandev",
		"message_agent_project_worker_kandev",
		"stop_agent_project_worker_kandev",
		"ask_user_question_kandev",
	}
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))

	s.SetMode(ModeTask)
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))
}

func TestProjectWorkerCatalogIsRestrictedAndCannotAcquireCoordinatorTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceProjectWorker, []mcpprofile.Capability{mcpprofile.CapabilityParentQuestion}, nil))

	want := []string{"get_agent_project_task_kandev", "ask_parent_question_kandev"}
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))

	s.SetMode(ModeProjectCoordinator)
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))

	s.SetProfile(mcpprofile.New(mcpprofile.SurfaceProjectCoordinator, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, nil))
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))
	require.Equal(t, mcpprofile.SurfaceProjectWorker, s.Profile().Surface)
}
