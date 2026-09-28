package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type stubAgentProfileVerifier struct {
	known map[string]bool
	err   error
	calls int
}

func (s *stubAgentProfileVerifier) AgentProfileExists(_ context.Context, profileID string) (bool, error) {
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	return s.known[profileID], nil
}

// AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.3, .7
// `current_task` and `workspace_default` are values of the
// mcp_task_agent_profile_default user setting, not argument values. The tool
// description names them, so callers pass them; they must be refused with a
// message that says where they belong.
func TestValidateExplicitAgentProfileRejectsPolicyValues(t *testing.T) {
	for _, value := range []string{"current_task", "workspace_default"} {
		t.Run(value, func(t *testing.T) {
			verifier := &stubAgentProfileVerifier{}
			h := &Handlers{agentProfileVerifier: verifier, logger: testLogger(t).WithFields()}

			err := h.validateExplicitAgentProfile(context.Background(), value)

			require.Error(t, err)
			require.ErrorIs(t, err, errMCPAgentProfileInvalid)
			assert.Contains(t, err.Error(), "mcp_task_agent_profile_default")
			assert.Zero(t, verifier.calls, "a policy value must not reach the profile store")
		})
	}
}

// AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.1, .8
func TestValidateExplicitAgentProfileRejectsUnknownProfile(t *testing.T) {
	verifier := &stubAgentProfileVerifier{known: map[string]bool{"known-profile": true}}
	h := &Handlers{agentProfileVerifier: verifier, logger: testLogger(t).WithFields()}

	err := h.validateExplicitAgentProfile(context.Background(), "5b2f0f4e-0000-0000-0000-000000000000")

	require.Error(t, err)
	require.ErrorIs(t, err, errMCPAgentProfileInvalid)
	assert.Equal(t, 1, verifier.calls)
}

func TestValidateExplicitAgentProfileAcceptsKnownProfile(t *testing.T) {
	verifier := &stubAgentProfileVerifier{known: map[string]bool{"known-profile": true}}
	h := &Handlers{agentProfileVerifier: verifier, logger: testLogger(t).WithFields()}

	require.NoError(t, h.validateExplicitAgentProfile(context.Background(), "known-profile"))
}

// AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.5
// An omitted argument keeps the documented resolution precedence, so it must
// not be validated and must not read the profile store.
func TestValidateExplicitAgentProfileSkipsOmittedArgument(t *testing.T) {
	verifier := &stubAgentProfileVerifier{}
	h := &Handlers{agentProfileVerifier: verifier, logger: testLogger(t).WithFields()}

	require.NoError(t, h.validateExplicitAgentProfile(context.Background(), ""))
	assert.Zero(t, verifier.calls)
}

// A store outage must fail the call rather than create a task with an
// unverified profile: a created task with no session is the outcome this
// requirement removes.
func TestValidateExplicitAgentProfileFailsOnStoreError(t *testing.T) {
	verifier := &stubAgentProfileVerifier{err: errors.New("store unavailable")}
	h := &Handlers{agentProfileVerifier: verifier, logger: testLogger(t).WithFields()}

	err := h.validateExplicitAgentProfile(context.Background(), "some-profile")

	require.Error(t, err)
	assert.NotErrorIs(t, err, errMCPAgentProfileInvalid, "a store outage is not a caller error")
}

// Without a wired verifier the check must not silently reject every explicit
// profile; it degrades to the previous behavior.
func TestValidateExplicitAgentProfileWithoutVerifier(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}

	require.NoError(t, h.validateExplicitAgentProfile(context.Background(), "some-profile"))
}

// AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.6, .9
// The auto-start runs after the tool has already reported success, so its
// failure has to land somewhere a person can see.
func TestRecordAutoStartFailureStoresReasonOnTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)

	created, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		Title:       "probe",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	h.recordAutoStartFailure(ctx, created.Task.ID, "validate profile current_task: no rows")

	stored, err := repo.GetTask(ctx, created.Task.ID)
	require.NoError(t, err)
	recorded, ok := stored.Metadata[models.MetaKeyAutoStartError].(map[string]interface{})
	require.True(t, ok, "auto-start failure should be recorded on the task metadata")
	assert.Equal(t, "validate profile current_task: no rows", recorded["reason"])
	assert.NotEmpty(t, recorded["occurred_at"])
}
