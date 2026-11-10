package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type coalescedRecoveryWaitContext struct {
	context.Context
	onWait func()
	once   sync.Once
}

func (c *coalescedRecoveryWaitContext) Done() <-chan struct{} {
	c.once.Do(c.onWait)
	return c.Context.Done()
}

func TestRecoveryLaunchDoesNotInheritConcurrentWorkspaceAdmissionFailure(t *testing.T) {
	unrelatedFailure := errors.New("workspace backend unavailable")
	for _, tc := range []struct {
		name           string
		action         string
		candidateID    string
		peerError      error
		alreadyRunning bool
		wantError      error
	}{
		{name: "native resume", action: "resume"},
		{name: "explicit continuation", action: "continue_from_history"},
		{name: "silent native restore", candidateID: "workspace-execution"},
		{name: "silent restore unrelated peer error", candidateID: "workspace-execution", peerError: unrelatedFailure, wantError: unrelatedFailure},
		{name: "silent restore own launch failure", candidateID: "workspace-execution", alreadyRunning: true, wantError: ErrAgentAlreadyRunning},
		{name: "normal launch remains blocked", wantError: ErrSessionRecoveryRequired},
		{name: "unrelated peer error", action: "resume", peerError: unrelatedFailure, wantError: unrelatedFailure},
		{name: "own launch failure", action: "resume", alreadyRunning: true, wantError: ErrAgentAlreadyRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{
				ProfileID: "profile-1", AgentName: "auggie",
			}}
			mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
				session: &models.TaskSession{
					ID: workspaceRecoverySessionID, TaskID: workspaceRecoveryTaskID,
					State:    models.TaskSessionStateStarting,
					Metadata: activeWorkspaceRecoveryMetadata(models.AgentDeliveryRecoveryUncertain, 1),
				},
			})
			// The successful launch reuses existing infrastructure. Workspace access
			// must still reject the unresolved notice; recovery owns its settlement.
			existing := &AgentExecution{
				ID: "workspace-execution", SessionID: workspaceRecoverySessionID,
				TaskID: workspaceRecoveryTaskID, AgentProfileID: "profile-1",
			}
			if tc.alreadyRunning {
				existing.AgentCommand = "auggie"
			}
			require.NoError(t, mgr.executionStore.Add(existing))
			entered, release := make(chan struct{}), make(chan struct{})
			peer := mgr.ensureExecutionGroup.DoChan(workspaceRecoverySessionID, func() (interface{}, error) {
				close(entered)
				<-release
				if tc.peerError != nil {
					return nil, tc.peerError
				}
				return nil, mgr.ensureWorkspaceRecoveryResolved(context.Background(), workspaceRecoverySessionID, "")
			})
			<-entered
			ctx := &coalescedRecoveryWaitContext{Context: context.Background(), onWait: func() { close(release) }}
			got, err := mgr.Launch(ctx, &LaunchRequest{
				TaskID: workspaceRecoveryTaskID, SessionID: workspaceRecoverySessionID,
				AgentProfileID: "profile-1", ACPSessionID: "original-native-session",
				RecoveryAction: tc.action, CandidateExecutionID: tc.candidateID,
			})
			peerResult := <-peer
			if tc.peerError != nil {
				require.ErrorIs(t, peerResult.Err, tc.peerError)
			} else {
				require.ErrorIs(t, peerResult.Err, ErrSessionRecoveryRequired)
			}
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				require.Nil(t, got)
				retained, ok := mgr.executionStore.GetBySessionID(workspaceRecoverySessionID)
				require.True(t, ok)
				require.Same(t, existing, retained)
				return
			}
			require.NoError(t, err, "the recovery launch must use its own admission after the workspace peer fails")
			require.Same(t, existing, got)
			require.NotEmpty(t, got.AgentCommand)
			require.Equal(t, "original-native-session", got.ACPSessionID)
			require.ErrorIs(t, mgr.ensureWorkspaceRecoveryResolved(context.Background(), workspaceRecoverySessionID, ""), ErrSessionRecoveryRequired)
		})
	}
}

func TestCancelledRecoveryLaunchDoesNotStartSharedOperation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		action      string
		candidateID string
	}{
		{name: "explicit resume", action: "resume"},
		{name: "silent restore", candidateID: "cancelled-candidate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			resolver := &countingProfileResolver{err: errors.New("profile unavailable")}
			mgr.profileResolver = resolver
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := mgr.coalesceSessionLaunch(ctx, &LaunchRequest{
				SessionID: "cancelled-session", AgentProfileID: "profile",
				RecoveryAction: tc.action, CandidateExecutionID: tc.candidateID,
			})

			require.ErrorIs(t, err, context.Canceled)
			// Joining the same singleflight key waits for any operation admitted above.
			_, err = mgr.doCoalescedExecution(context.Background(), "cancelled-session", func(context.Context) (interface{}, error) { return nil, nil })
			if err != nil {
				require.ErrorContains(t, err, "profile unavailable")
			}
			require.Zero(t, resolver.calls.Load(), "an already cancelled recovery must not launch caller-independent work")
		})
	}
}
