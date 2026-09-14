package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestPrepareWorkflowStepSession_SameProfileNew(t *testing.T) {
	for _, endPolicy := range []models.WorkflowProfileSessionEndPolicy{
		models.WorkflowProfileSessionEndPolicyComplete,
		models.WorkflowProfileSessionEndPolicyPark,
	} {
		t.Run(string(endPolicy), func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, endPolicy)
			extra := &models.TaskSession{
				ID: "session-a-existing", TaskID: "t1", AgentProfileID: "profile-a",
				ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
				State:     models.TaskSessionStateWaitingForInput,
				StartedAt: time.Now().UTC().Add(-time.Minute), UpdatedAt: time.Now().UTC().Add(-time.Minute),
			}
			require.NoError(t, fixture.repo.CreateTaskSession(ctx, extra))

			target := &wfmodels.WorkflowStep{
				ID: "step-b", WorkflowID: "wf1", Position: 1, AgentProfileID: "profile-a",
				ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
			}
			source := &wfmodels.WorkflowStep{
				ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a",
				ProfileSessionEndPolicy: endPolicy,
			}

			selected, switched, err := fixture.svc.prepareWorkflowStepSession(
				ctx, "t1", fixture.current, target, source,
			)
			require.NoError(t, err)
			require.True(t, switched)
			require.NotNil(t, selected)
			require.NotEqual(t, fixture.current.ID, selected.ID)
			require.NotEqual(t, extra.ID, selected.ID)
			require.Equal(t, "profile-a", selected.AgentProfileID)

			current, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			require.NoError(t, err)
			switch endPolicy {
			case models.WorkflowProfileSessionEndPolicyComplete:
				require.Equal(t, models.TaskSessionStateCompleted, current.State)
				require.NotNil(t, current.CompletedAt)
			case models.WorkflowProfileSessionEndPolicyPark:
				require.Equal(t, models.TaskSessionStateWaitingForInput, current.State)
				require.Nil(t, current.CompletedAt)
			}
		})
	}
}

func TestPrepareWorkflowStepSession_SameProfileReusePreservesOverrides(t *testing.T) {
	for _, startPolicy := range []models.WorkflowProfileSessionStartPolicy{
		models.WorkflowProfileSessionStartPolicyReuse,
		"",
	} {
		t.Run(string(startPolicy), func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
			overrides := models.SessionRuntimeConfig{
				Model:         "astra",
				ConfigOptions: map[string]string{"reasoning_effort": "medium"},
			}
			require.NoError(t, fixture.repo.SetSessionMetadataKey(
				ctx, fixture.current.ID, models.SessionMetaKeyRuntimeConfigOverrides, overrides,
			))

			target := &wfmodels.WorkflowStep{
				ID: "step-b", WorkflowID: "wf1", Position: 1, AgentProfileID: "profile-a",
				ProfileSessionStartPolicy: startPolicy,
			}
			source := &wfmodels.WorkflowStep{
				ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a",
				ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
			}

			selected, switched, err := fixture.svc.prepareWorkflowStepSession(
				ctx, "t1", fixture.current, target, source,
			)
			require.NoError(t, err)
			require.False(t, switched)
			require.Equal(t, fixture.current.ID, selected.ID)

			updated, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			require.NoError(t, err)
			got, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata)
			require.True(t, ok)
			require.Equal(t, overrides, got)
		})
	}
}

func TestPreflightWorkflowStepCredentials_SameProfileNew(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	require.NoError(t, fixture.repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-invalid", WorkspaceID: "ws1", Name: "widgets", SourceType: "local",
		Provider: "acme-forge", RemoteURL: "https://forge.example/acme/widgets.git",
	}))
	require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "taskrepo-invalid", TaskID: "t1", RepositoryID: "repo-invalid",
	}))
	fixture.svc.executor.SetGitHubCredentialBroker(
		fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve",
	)

	target := &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, AgentProfileID: "profile-a",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
	err := fixture.svc.preflightWorkflowStepCredentials(ctx, "t1", fixture.current, target)
	require.Error(t, err)

	current, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, current.State)
	require.True(t, current.IsPrimary)
}

func TestProcessOnEnter_SameProfileNewPromptRecipient(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	const marker = "same-profile destination marker"
	promptDelivered := make(chan struct{})
	processStarted := make(chan struct{})
	var promptOnce sync.Once
	var processOnce sync.Once
	var launchedSessionID, launchedPrompt string
	fixture.svc.executor.SetOnAgentProcessStarted(func(context.Context, string, string, string) {
		processOnce.Do(func() { close(processStarted) })
	})
	fixture.agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		if strings.Contains(req.TaskDescription, marker) {
			launchedSessionID = req.SessionID
			launchedPrompt = req.TaskDescription
			promptOnce.Do(func() { close(promptDelivered) })
		}
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-" + req.AgentProfileID}, nil
	}
	fixture.agentMgr.startAgentProcessFunc = func(_ context.Context, _ string) error {
		fixture.agentMgr.mu.Lock()
		for _, call := range fixture.agentMgr.setExecutionDescriptionCalls {
			if strings.Contains(call.Prompt, marker) {
				promptOnce.Do(func() { close(promptDelivered) })
				break
			}
		}
		fixture.agentMgr.mu.Unlock()
		return nil
	}
	target := &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, AgentProfileID: "profile-a",
		Prompt:                    marker,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
	source := &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}

	fixture.svc.processOnEnter(ctx, "t1", fixture.current, target, "task description", 0, source)
	select {
	case <-promptDelivered:
	case <-time.After(2 * time.Second):
		t.Fatal("same-profile new workflow entry did not deliver a prompt")
	}
	select {
	case <-processStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("same-profile new workflow entry did not start the destination agent")
	}
	fixture.agentMgr.mu.Lock()
	descriptionCalls := append([]promptCall(nil), fixture.agentMgr.setExecutionDescriptionCalls...)
	fixture.agentMgr.mu.Unlock()
	if launchedSessionID != "" {
		require.NotEqual(t, fixture.current.ID, launchedSessionID)
		require.True(t, strings.Contains(launchedPrompt, marker))
	} else {
		markerIdx := -1
		for i, call := range descriptionCalls {
			if strings.Contains(call.Prompt, marker) {
				markerIdx = i
				break
			}
		}
		require.GreaterOrEqual(t, markerIdx, 0, "marker not found in setExecutionDescriptionCalls")
		require.NotEqual(t, "execution-a", descriptionCalls[markerIdx].ExecutionID)
	}

	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	var destination *models.TaskSession
	for _, candidate := range sessions {
		if candidate.ID != fixture.current.ID && candidate.IsPrimary {
			destination = candidate
			break
		}
	}
	require.NotNil(t, destination)
	require.Equal(t, "profile-a", destination.AgentProfileID)
	require.NotEqual(t, fixture.current.ID, destination.ID)
}
