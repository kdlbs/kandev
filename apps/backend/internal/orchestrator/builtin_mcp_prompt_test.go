package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

type builtinPromptReader struct{}

func (builtinPromptReader) GetBuiltinPolicySnapshot(_ context.Context, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy(workspaceID)
	for id := range policy.Packages {
		policy.Packages[id] = mcpconfig.BuiltinPolicyAvailable
	}
	return &mcpconfig.BuiltinPolicySnapshot{Policy: policy, Preferences: &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: workspaceID, TaskID: taskID, Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: mcpconfig.BuiltinPreferenceOn}}}, nil
}

func TestBuiltinPromptProducersUsePreparedSelection(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateCreated)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.executor.SetBuiltinPolicyReader(builtinPromptReader{})
	session, err := repo.GetTaskSession(context.Background(), "session")
	require.NoError(t, err)
	task, err := repo.GetTask(context.Background(), "task")
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(context.Background(), task))
	ctx, err := svc.executor.PrepareMCPLaunch(context.Background(), "task", session, true)
	require.NoError(t, err)
	prompts := []string{
		svc.wrapCreatedSessionPrompt(ctx, "request", "task", "session", session, task, false, false, false, false, nil, ""),
		svc.applyLaunchPromptContext(ctx, launchPromptContext{prompt: "request", taskID: "task", sessionID: "session"}),
	}
	for _, prompt := range prompts {
		require.NotContains(t, prompt, "call show_rich_output_kandev")
		require.NotContains(t, prompt, "- show_walkthrough_kandev")
		require.Contains(t, prompt, "kandev-configuration")
		require.Contains(t, prompt, "PLAN EDITING:")
	}
}

type countedBuiltinPromptReader struct{ reads int }

func (r *countedBuiltinPromptReader) GetBuiltinPolicySnapshot(ctx context.Context, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	r.reads++
	return (builtinPromptReader{}).GetBuiltinPolicySnapshot(ctx, workspaceID, taskID)
}

func TestBuiltinFreshLaunchSharesPromptAndRuntimeSelection(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "prepared"}[prepared], func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateCreated)
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			task.WorkspaceID = "ws1"
			require.NoError(t, repo.UpdateTask(ctx, task))
			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task"] = &v1.Task{ID: "task", WorkspaceID: "ws1", Title: "Task", State: v1.TaskStateInProgress}
			var launched *executor.LaunchAgentRequest
			manager := &mockAgentManager{launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched = req
				return &executor.LaunchAgentResponse{AgentExecutionID: "execution"}, nil
			}}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
			svc.messageCreator = &mockMessageCreator{}
			reader := &countedBuiltinPromptReader{}
			svc.executor.SetBuiltinPolicyReader(reader)
			if prepared {
				_, err = svc.StartCreatedSession(ctx, "task", "session", "profile-1", "request", false, false, false, nil, nil)
			} else {
				_, err = svc.StartTask(ctx, "task", "profile-1", "", "", "", "request", "", false, false, nil)
			}
			require.NoError(t, err)
			require.NotNil(t, launched)
			require.NotNil(t, launched.McpProfile)
			require.NotNil(t, launched.McpProfile.BuiltinPackages)
			require.ElementsMatch(t, []p.PackageID{p.PackageCore, p.PackageConfiguration}, launched.McpProfile.BuiltinPackages.Packages)
			require.Contains(t, launched.TaskDescription, "kandev-configuration")
			require.NotContains(t, launched.TaskDescription, "call show_rich_output_kandev")
			require.Equal(t, 1, reader.reads)
		})
	}
}

func TestBuiltinAutoStartUsesFreshOrRetainedSelection(t *testing.T) {
	for _, fresh := range []bool{true, false} {
		t.Run(map[bool]string{true: "fresh", false: "reset"}[fresh], func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			state := models.TaskSessionStateCreated
			if !fresh {
				state = models.TaskSessionStateWaitingForInput
			}
			seedTaskAndSession(t, repo, "task", "session", state)
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			task.WorkspaceID = "ws1"
			require.NoError(t, repo.UpdateTask(ctx, task))
			session, err := repo.GetTaskSession(ctx, "session")
			require.NoError(t, err)
			profile := p.New(p.SurfaceKanbanTask, nil, nil)
			profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "ws1", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			if !fresh {
				encoded, encodeErr := p.MarshalBuiltinBinding(profile)
				require.NoError(t, encodeErr)
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: "task", SessionID: "session", AgentExecutionID: "execution", Runtime: "standalone", Status: models.ExecutorRunningStatusRunning, Metadata: map[string]any{p.BuiltinBindingMetadataKey: encoded}}))
			}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			reader := &countedBuiltinPromptReader{}
			svc.executor.SetBuiltinPolicyReader(reader)
			step := &wfmodels.WorkflowStep{ID: "step"}
			step.Events = wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterResetAgentContext}}}
			promptState := &autoStartStepPromptState{ctx: ctx, taskID: "task", sessionID: "session", session: session, step: step, agentPrompt: "request", effectiveAgentPrompt: "request"}
			recorded, dispatched, err := svc.composeAutoStartStepPrompts(promptState)
			require.NoError(t, err)
			for _, prompt := range []string{recorded, dispatched} {
				require.NotContains(t, prompt, "call show_rich_output_kandev")
				if fresh {
					require.Contains(t, prompt, "kandev-configuration")
				} else {
					require.NotContains(t, prompt, "kandev-configuration")
				}
			}
			if fresh {
				require.Equal(t, 1, reader.reads)
			} else {
				require.Zero(t, reader.reads)
			}
			require.NotNil(t, executor.PreparedMCPProfile(promptState.ctx, "task", "session"))
		})
	}
}
