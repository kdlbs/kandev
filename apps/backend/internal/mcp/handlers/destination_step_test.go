package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// The MCP create resolves launch metadata from a pendingTask that does not exist
// yet, so the destination step has to be decided before that lookup. Leaving it
// empty pinned the agent profile, and the deferred-launch record built from it,
// to the start step's profile while CreateTask sent an agent start to the
// auto-start step.
func TestResolveMCPDestinationStep(t *testing.T) {
	h, _, repo := setupImportHandlers(t)
	ctx := context.Background()
	h.workflowCtrl = workflowctrl.NewController(h.workflowSvc)

	require.NoError(t, repo.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "backlog", WorkflowID: "wf-test", Name: "Backlog", Position: 0, IsStartStep: true,
	}))
	require.NoError(t, repo.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "in-progress", WorkflowID: "wf-test", Name: "In Progress", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}))
	require.NoError(t, repo.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "deploy", WorkflowID: "wf-test", Name: "Deploy", Position: 2,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}))

	assert.Equal(t, "in-progress", h.resolveMCPDestinationStep(ctx, "wf-test", true),
		"an agent start belongs on the first step that runs agents")
	assert.Equal(t, "backlog", h.resolveMCPDestinationStep(ctx, "wf-test", false),
		"a create with no agent belongs on the start step")
	assert.Empty(t, h.resolveMCPDestinationStep(ctx, "wf-missing", true),
		"an unreadable workflow must leave resolution to CreateTask")
	assert.Empty(t, h.resolveMCPDestinationStep(ctx, "", true),
		"no workflow means no destination to pre-resolve")

	h.workflowCtrl = nil
	assert.Empty(t, h.resolveMCPDestinationStep(ctx, "wf-test", true),
		"without a workflow controller the handler must not guess")
}
