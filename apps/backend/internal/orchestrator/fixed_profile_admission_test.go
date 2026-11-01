package orchestrator

import (
	"context"
	"testing"

	dynamic "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestFixedProfileAdmissionUsesDynamicRoutingFlag(t *testing.T) {
	resolver := newExplicitOnlyProfileExecutionResolver(t, "profile")
	resolver.RecordFixedProfileFailure(context.Background(), "profile", dynamic.ResourceKey(dynamic.ScopeProfile, "profile"), &routingerr.Error{Code: routingerr.CodeAccountBlocked})
	resolver.SetEnabled(false)
	svc := &Service{profileExecutionResolver: resolver}
	if err := svc.admitFixedProfileDispatch(context.Background(), "", "profile"); err != nil {
		t.Fatalf("disabled admission refused dispatch: %v", err)
	}
	resolver.SetEnabled(true)
	if err := svc.admitFixedProfileDispatch(context.Background(), "", "profile"); !isFixedProfileSuspendedError(err) {
		t.Fatalf("enabled admission did not refuse dispatch: %v", err)
	}
}

func TestFixedProfileWaitResumesWithoutLosingQueuedPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-fixed", "session-fixed", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	resolver := newExplicitOnlyProfileExecutionResolver(t, "profile")
	svc.SetProfileExecutionResolver(resolver)
	session, err := repo.GetTaskSession(ctx, "session-fixed")
	require.NoError(t, err)
	session.AgentProfileID = "profile"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRunForSession(ctx, identity, false))
	_, err = svc.messageQueue.QueueMessage(ctx, session.ID, session.TaskID, "keep this prompt", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	resolver.RecordFixedProfileFailure(ctx, "profile", "profile:profile", &routingerr.Error{Code: routingerr.CodeAccountBlocked})
	require.True(t, svc.queueDrainBlockedByFixedProfile(ctx, session.ID))
	svc.resumeFixedProfileWaits(ctx)
	blocked, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotEmpty(t, blocked.ErrorMessage)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, session.ID).Count)
	resolver.SetEnabled(false)
	svc.resumeFixedProfileWaits(ctx)
	resumed, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Empty(t, resumed.ErrorMessage)
	require.NotContains(t, resumed.Metadata, fixedProfileWaitMetadataKey)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, session.ID).Count, "auto-run off must preserve the prompt")
}

func TestFixedProfileWaitDoesNotClearNewerSessionDiagnostic(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-stale", "session-stale", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	session, err := repo.GetTaskSession(ctx, "session-stale")
	require.NoError(t, err)
	svc.surfaceFixedProfileSuspension(ctx, session.ID, dynamic.Suspension{Scope: dynamic.ScopeProfile, Code: routingerr.CodeRateLimited})
	observed, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, session.State, "newer diagnostic"))
	require.False(t, svc.clearFixedProfileQueueWait(ctx, observed))
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, "newer diagnostic", current.ErrorMessage)
	require.Contains(t, current.Metadata, fixedProfileWaitMetadataKey)
}
