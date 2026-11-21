package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type builtinIntentProbeManager struct {
	mockAgentManager
	probeErr error
}

func (m *builtinIntentProbeManager) ProbeAgentRunningForSession(context.Context, string) (bool, error) {
	return m.isAgentRunning, m.probeErr
}

// @covers AC-AGENTS-BUILTIN-MCP-003.3 and .4
func TestBuiltinIntentFutureStartUsesDesiredPackages(t *testing.T) {
	for _, inventory := range []string{"missing", "stopped", "suspended", "retired", "probe-error"} {
		for _, selected := range []bool{false, true} {
			t.Run(inventory+map[bool]string{false: "/off", true: "/on"}[selected], func(t *testing.T) {
				ctx := context.Background()
				svc, manager := newBuiltinIntentResumeFixture(t, inventory, selected)
				view, err := svc.ReadBuiltinSessionMCP(ctx, "session")
				require.NoError(t, err)
				if inventory != "missing" {
					require.NotNil(t, view.Bound)
					require.Equal(t, "old-execution", view.Bound.ExecutionID)
				}
				prepared, admissionErr := svc.PrepareBuiltinIntent(ctx, "task", "session", "walkthrough")
				switch {
				case inventory == "probe-error":
					require.Equal(t, "unknown", view.WalkthroughAvailability)
					require.ErrorIs(t, admissionErr, manager.probeErr)
				case !selected:
					require.Equal(t, "unavailable", view.WalkthroughAvailability)
					require.ErrorIs(t, admissionErr, builtinintent.ErrUnavailable)
				default:
					require.Equal(t, "available", view.WalkthroughAvailability)
					require.NoError(t, admissionErr)
					binding, ok := builtinintent.FromContext(prepared)
					require.True(t, ok)
					require.Empty(t, binding.ExecutionID, "retired inventory must not authorize the next process")
					require.NoError(t, builtinintent.ValidateExecution(prepared, "task", "session", "new-execution", view.Desired))
					require.ErrorIs(t, builtinintent.ValidateExecution(prepared, "task", "session", "new-execution", nil), builtinintent.ErrUnavailable)
				}
			})
		}
	}
}

func newBuiltinIntentResumeFixture(t *testing.T, inventory string, selected bool) (*Service, *builtinIntentProbeManager) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(ctx, task))
	manager := &builtinIntentProbeManager{mockAgentManager: mockAgentManager{repoForExecutionLookup: repo}}
	if inventory == "probe-error" {
		manager.probeErr = errors.New("runtime status unavailable")
	}
	tasks := newMockTaskRepo()
	tasks.tasks["task"] = &v1.Task{ID: "task", Title: "Walkthrough task", State: v1.TaskStateInProgress}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), tasks, manager)
	svc.executor.SetBuiltinPolicyReader(builtinPromptReader{})
	if selected {
		svc.executor.SetBuiltinPolicyReader(defaultBuiltinIntentReader{})
	}
	manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "ws1", TaskID: "task", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	// The old execution has the opposite choice from the next start.
	if !selected {
		manifest.Packages = append(manifest.Packages, p.PackageWalkthroughs)
		manifest.Origins[p.PackageWalkthroughs] = "task"
	}
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = manifest
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	if inventory != "missing" {
		running := &models.ExecutorRunning{TaskID: "task", SessionID: "session", AgentExecutionID: "old-execution", Runtime: "standalone", Status: models.ExecutorRunningStatusStopped, Metadata: map[string]interface{}{p.BuiltinBindingMetadataKey: encoded}}
		if inventory == "suspended" {
			running.IdleSuspensionState = models.ExecutorIdleSuspensionSuspended
		}
		if inventory == "retired" || inventory == "probe-error" {
			running.Status = models.ExecutorRunningStatusRunning
		}
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	}
	history := streams.MCPAttachmentHistory{Version: 1, Current: streams.MCPAttachmentAttempt{AttemptID: "old-attempt", TaskID: "task", SessionID: "session", ExecutionID: "old-execution", BuiltinPackages: manifest}, BuiltinBinding: &streams.BuiltinMCPAcceptedBinding{Manifest: manifest, SessionID: "session", ExecutionID: "old-execution", AttemptID: "old-attempt", AcceptedAt: time.Now().UTC()}}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session", models.SessionMetaKeyMCPAttachmentState, history))
	return svc, manager
}

func TestBuiltinIntentColdResumeRebindsAtLaunch(t *testing.T) {
	svc, manager := newBuiltinIntentResumeFixture(t, "suspended", true)
	ctx, err := svc.PrepareBuiltinIntent(context.Background(), "task", "session", "walkthrough")
	require.NoError(t, err)
	launched := false
	manager.launchAgentFunc = func(dispatch context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launched = true
		require.NotNil(t, request.McpProfile)
		require.NoError(t, builtinintent.ValidateExecution(dispatch, request.TaskID, request.SessionID, "new-execution", request.McpProfile.BuiltinPackages))
		require.Contains(t, request.McpProfile.BuiltinPackages.Packages, p.PackageWalkthroughs)
		return &executor.LaunchAgentResponse{AgentExecutionID: "new-execution"}, nil
	}
	session, err := svc.repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	session.AgentProfileID = "profile1"
	require.NoError(t, svc.repo.UpdateTaskSession(ctx, session))
	execution, err := svc.executor.ResumeSession(ctx, session, true)
	require.NoError(t, err)
	require.True(t, launched)
	require.Equal(t, "new-execution", execution.AgentExecutionID)
}
