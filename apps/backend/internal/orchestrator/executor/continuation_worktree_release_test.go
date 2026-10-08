package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

type failingContinuationReleaseStore struct {
	*worktree.SQLiteStore
	failure error
}

func (s *failingContinuationReleaseStore) ReleaseTaskEnvironmentRecoveryClaim(ctx context.Context, claim *models.TaskEnvironmentRecoveryClaim) error {
	return errors.Join(s.SQLiteStore.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim), s.failure)
}

func TestContinuationNativeStartupFailureRetainsAdmissionReleaseFailure(t *testing.T) {
	fixture := newExecutorMissingCheckoutRecoveryFixture(t, "resume")
	releaseFailure := errors.New("release admission failed")
	store := &failingContinuationReleaseStore{SQLiteStore: fixture.store, failure: releaseFailure}
	manager, err := worktree.NewManager(fixture.config, store, logger.Default())
	require.NoError(t, err)
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, fixture.taskID, fixture.sessionID, fixture.sessionState)
	configureExecutorMissingCheckoutEnvironment(repo, fixture)
	repo.tasks[fixture.taskID] = &models.Task{ID: fixture.taskID, WorkspaceID: fixture.workspaceID}
	session := repo.sessions[fixture.sessionID]
	session.DownstreamACPSessionID = "saved-conversation"
	session.Metadata = map[string]any{"acp": map[string]any{"session_id": "saved-conversation"}}
	startupFailure := errors.New("dial tcp: network is unreachable")
	mgr := &mockAgentManager{startAgentProcessFunc: func(context.Context, string) error { return startupFailure }}
	exec := newTestExecutor(t, mgr, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(manager.AdmitRecovery)
	execution, err := exec.ResumeSessionWithOptions(context.Background(), session, true, ResumeOptions{RequiredNativeConversationID: "saved-conversation"})
	require.NotNil(t, execution, "restore result: %v", err)
	require.ErrorIs(t, err, startupFailure)
	require.ErrorIs(t, err, releaseFailure)
}
