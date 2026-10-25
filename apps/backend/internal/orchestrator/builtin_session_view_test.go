package orchestrator

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type defaultBuiltinIntentReader struct{}

func (defaultBuiltinIntentReader) GetBuiltinPolicySnapshot(_ context.Context, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	return &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy(workspaceID), Preferences: &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: workspaceID, TaskID: taskID}}, nil
}

func TestBuiltinIntentAdmissionDoesNotFreezeTitleOwnership(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(ctx, task))
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.executor.SetBuiltinPolicyReader(defaultBuiltinIntentReader{})
	prepared, err := svc.PrepareBuiltinIntent(ctx, "task", "session", "walkthrough")
	require.NoError(t, err)
	binding, ok := builtinintent.FromContext(prepared)
	require.True(t, ok)
	require.Empty(t, binding.ExecutionID)
	require.Nil(t, executor.PreparedMCPProfile(prepared, "task", "session"), "intent admission precedes title ownership; actual launch preparation owns the final profile")
}

func TestBuiltinSessionViewSeparatesDesiredAndAcceptedPackages(t *testing.T) {
	for _, state := range []string{"accepted", "unknown", "retired", "created"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			sessionState := models.TaskSessionStateWaitingForInput
			if state == "created" {
				sessionState = models.TaskSessionStateCreated
			}
			seedTaskAndSession(t, repo, "task", "session", sessionState)
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			task.WorkspaceID = "ws1"
			require.NoError(t, repo.UpdateTask(ctx, task))
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			svc.executor.SetBuiltinPolicyReader(builtinPromptReader{})
			reader, ok := any(svc).(interface {
				ReadBuiltinSessionMCP(context.Context, string) (*mcpconfig.BuiltinSessionView, error)
			})
			require.True(t, ok, "management needs one authorized session snapshot")
			manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "ws1", TaskID: "task", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore, p.PackageWalkthroughs}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageWalkthroughs: "task"}}
			profile := p.New(p.SurfaceKanbanTask, nil, nil)
			profile.BuiltinPackages = manifest
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			if state != "created" {
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: "task", SessionID: "session", AgentExecutionID: "execution", Runtime: "standalone", Status: models.ExecutorRunningStatusRunning, Metadata: map[string]interface{}{p.BuiltinBindingMetadataKey: encoded}}))
			}
			history := streams.MCPAttachmentHistory{Version: 1, Current: streams.MCPAttachmentAttempt{AttemptID: "attempt", TaskID: "task", SessionID: "session", ExecutionID: "execution", StartedAt: time.Now().UTC(), BuiltinPackages: manifest}}
			if state == "accepted" || state == "retired" {
				history.BuiltinBinding = &streams.BuiltinMCPAcceptedBinding{Manifest: manifest, SessionID: "session", ExecutionID: "execution", AttemptID: "attempt", AcceptedAt: time.Now().UTC()}
				if state == "retired" {
					history.BuiltinBinding.ExecutionID = "old"
				}
			}
			require.NoError(t, repo.SetSessionMetadataKey(ctx, "session", models.SessionMetaKeyMCPAttachmentState, history))
			view, err := reader.ReadBuiltinSessionMCP(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, "task", view.TaskID)
			require.Equal(t, "ws1", view.WorkspaceID)
			require.NotContains(t, view.Desired.Packages, p.PackageWalkthroughs)
			if state == "accepted" {
				require.Equal(t, "accepted", view.BindingState)
				require.Equal(t, "available", view.WalkthroughAvailability)
				require.Equal(t, "execution", view.Bound.ExecutionID)
			} else {
				require.Nil(t, view.Bound)
				require.Equal(t, "unknown", view.BindingState)
				if state == "created" {
					require.Equal(t, "unavailable", view.WalkthroughAvailability)
				} else {
					require.Equal(t, "unknown", view.WalkthroughAvailability)
				}
			}
			_, err = reader.ReadBuiltinSessionMCP(ctx, "missing-session")
			require.Error(t, err)
		})
	}
}

func TestBuiltinSessionViewRequiresSessionAndTaskAccess(t *testing.T) {
	for _, checkTask := range []bool{false, true} {
		ctx := context.Background()
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateCreated)
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
		denied := errors.New("resource denied")
		if checkTask {
			svc.SetTaskAccessChecker(func(context.Context, string) error { return denied })
		} else {
			svc.SetSessionAccessChecker(func(context.Context, string) error { return denied })
		}
		view, err := svc.ReadBuiltinSessionMCP(ctx, "session")
		require.ErrorIs(t, err, denied)
		require.Nil(t, view)
	}
}
