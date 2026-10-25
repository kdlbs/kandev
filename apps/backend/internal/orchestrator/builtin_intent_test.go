package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

type builtinIntentReadFailure struct {
	sessionExecutorStore
	err error
}

func (r builtinIntentReadFailure) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.err
}

func TestBuiltinIntentQueueReadFailurePreservesRequest(t *testing.T) {
	f := newLifecycleDeliveryFixture(t, false)
	failure := errors.New("temporary session read failure")
	f.service.repo = builtinIntentReadFailure{sessionExecutorStore: f.service.repo, err: failure}
	_, err := f.service.prepareQueuedBuiltinIntent(context.Background(), &messagequeue.QueuedMessage{
		TaskID: "t1", SessionID: "s1", Metadata: map[string]interface{}{"builtin_intent": "walkthrough"},
	})
	require.ErrorIs(t, err, failure)
	require.True(t, isBuiltinIntentError(err), "an unknown availability result must restore the queue claim")
}

func TestBuiltinIntentUsesAcceptedExecutionRatherThanDesiredPackages(t *testing.T) {
	for _, state := range []string{"selected", "excluded", "unknown", "foreign-execution", "created", "legacy-observed"} {
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
			svc.executor.SetBuiltinPolicyReader(builtinPromptReader{}) // Desired Walkthroughs is Off.
			validator, ok := any(svc).(interface {
				ValidateBuiltinIntent(context.Context, string, string, string) error
			})
			require.True(t, ok, "typed request admission must use the production authority boundary")
			if state != "created" {
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: "task", SessionID: "session", AgentExecutionID: "execution", Runtime: "standalone", Status: models.ExecutorRunningStatusRunning}))
			}
			manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "ws1", TaskID: "task", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			if state != "excluded" {
				manifest.Packages = append(manifest.Packages, p.PackageWalkthroughs)
				manifest.Origins[p.PackageWalkthroughs] = "task"
			}
			history := streams.MCPAttachmentHistory{Version: 1, Current: streams.MCPAttachmentAttempt{AttemptID: "attempt", TaskID: "task", SessionID: "session", ExecutionID: "execution", BuiltinPackages: manifest}}
			if state == "selected" || state == "excluded" || state == "foreign-execution" {
				history.BuiltinBinding = &streams.BuiltinMCPAcceptedBinding{Manifest: manifest, SessionID: "session", ExecutionID: "execution", AttemptID: "attempt", AcceptedAt: time.Now().UTC()}
				if state == "foreign-execution" {
					history.BuiltinBinding.ExecutionID = "old-execution"
				}
			}
			if state == "legacy-observed" {
				history.Current.BuiltinPackages = nil
				now := time.Now().UTC()
				history.Current.Servers = []streams.MCPServerAttachment{{Name: "kandev", Source: streams.MCPServerSourceKandev, Status: streams.MCPAttachmentStatusActive, ToolsListedAt: &now, Tools: []streams.MCPToolSummary{{Name: "show_walkthrough_kandev"}}}}
			}
			require.NoError(t, repo.SetSessionMetadataKey(ctx, "session", models.SessionMetaKeyMCPAttachmentState, history))
			err = validator.ValidateBuiltinIntent(ctx, "task", "session", "walkthrough")
			if state == "selected" || state == "legacy-observed" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Error(t, validator.ValidateBuiltinIntent(ctx, "foreign-task", "session", "walkthrough"))
			require.Error(t, validator.ValidateBuiltinIntent(ctx, "task", "session", "unknown-tool"))
		})
	}
}

func TestBuiltinIntentPreparedDispatchContext(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	preparer, ok := any(svc).(interface {
		PrepareBuiltinIntent(context.Context, string, string, string) (context.Context, error)
	})
	require.True(t, ok, "dispatch must carry server-validated execution authority")
	ctx, err := preparer.PrepareBuiltinIntent(context.Background(), "task", "session", "")
	require.NoError(t, err)
	_, bound := builtinintent.FromContext(ctx)
	require.False(t, bound)
	_, err = preparer.PrepareBuiltinIntent(ctx, "task", "session", "unknown")
	require.Error(t, err)
}

func TestBuiltinIntentQueueDispatchRetainsUnavailableRequest(t *testing.T) {
	for _, sendNow := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "send-now"}[sendNow], func(t *testing.T) {
			f := newLifecycleDeliveryFixture(t, false)
			queued := f.reserve(t, map[string]interface{}{"builtin_intent": "walkthrough"})
			if sendNow {
				attempted, err := f.service.promptSendNowClaim(context.Background(), &messagequeue.SendNowClaim{Dispatch: *queued, Sources: []messagequeue.QueuedMessage{*queued}})
				require.ErrorIs(t, err, builtinintent.ErrUnavailable)
				require.False(t, attempted)
			} else {
				f.execute(t, queued)
				require.Equal(t, 1, f.service.messageQueue.GetStatus(context.Background(), "s1").Count)
			}
			require.Zero(t, f.calls)
			messages, listErr := f.repo.ListMessages(context.Background(), "s1")
			require.NoError(t, listErr)
			require.Empty(t, messages)
		})
	}
}

func TestBuiltinIntentWorkflowHandoffCannotBypassAvailability(t *testing.T) {
	f := newLifecycleDeliveryFixture(t, false)
	ctx := context.Background()
	_, err := f.service.messageQueue.QueueMessageWithMetadata(ctx, "s1", "t1", "walkthrough request", "", messagequeue.QueuedByUser, false, nil, map[string]interface{}{"builtin_intent": "walkthrough"})
	require.NoError(t, err)
	require.NoError(t, f.service.messageQueue.SetAutoRun(ctx, "s1", true))
	session, err := f.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	_, actionable, err := f.service.prepareAutoStartStepPrompt(ctx, "t1", session, &wfmodels.WorkflowStep{ID: "step1"}, "step instructions", false, true, nil, "")
	require.ErrorIs(t, err, builtinintent.ErrUnavailable)
	require.False(t, actionable)
	require.Equal(t, 1, f.service.messageQueue.GetStatus(ctx, "s1").Count)
	require.Zero(t, f.calls)
}

func TestBuiltinIntentFallbackQueuesPreserveRequirement(t *testing.T) {
	for _, workflow := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct-fallback", true: "workflow-fallback"}[workflow], func(t *testing.T) {
			f := newLifecycleDeliveryFixture(t, false)
			ctx := builtinintent.Bind(context.Background(), builtinintent.Binding{Intent: builtinintent.Walkthrough, TaskID: "t1", SessionID: "s1", ExecutionID: "exec-1"})
			f.service.markQueuedDispatchInFlightWithSourceLocked(ctx, "s1", "in-flight", nil)
			if workflow {
				_, err := f.service.persistAutoStartPrompt(ctx, "t1", "s1", "request", false, nil, workflowMessageOrigin{}, true, nil, "")
				require.NoError(t, err)
			} else {
				require.NoError(t, f.service.QueueUserPrompt(ctx, "t1", "s1", "request", "", false, nil, nil, true))
			}
			entries := f.service.messageQueue.GetStatus(ctx, "s1").Entries
			require.Len(t, entries, 1)
			require.Equal(t, "walkthrough", entries[0].Metadata["builtin_intent"])
		})
	}
}
