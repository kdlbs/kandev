package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestLaunch_WorktreeResumePublishesPrepareCompleted(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(&gitMetadataAwareCreateInstanceExecutor{
		createInstanceExecutor: createInstanceExecutor{
			MockExecutor: MockExecutor{name: executor.NameStandalone},
			client:       newReadyAgentctlClient(t, log),
		},
	})
	eventBus := &MockEventBusWithTracking{}
	mgr := NewManager(
		newTestRegistry(), eventBus, execRegistry,
		&MockCredentialsManager{}, &countingProfileResolver{info: &AgentProfileInfo{
			ProfileID: "profile-worktree-resume",
			AgentName: "auggie",
		}}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)

	// A resumed worktree session must present real, validated Git metadata
	// (ADR 2026-08-19): build an actual repository with a linked worktree so
	// gitMetadataProjectionForResumedWorktree resolves successfully instead of
	// failing closed against a fabricated path.
	repo := filepath.Join(t.TempDir(), "source")
	runContainerGit(t, "", "init", "-b", "main", repo)
	runContainerGit(t, repo, "config", "user.email", "test@example.com")
	runContainerGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, repo, "add", "file")
	runContainerGit(t, repo, "commit", "-m", "initial")
	checkout := filepath.Join(t.TempDir(), "checkout")
	runContainerGit(t, repo, "worktree", "add", "-b", "task", checkout)
	mgr.preparerRegistry.Register(models.ExecutorTypeWorktree, &progressPreparer{workspacePath: checkout})

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-worktree-resume",
		SessionID:      "session-worktree-resume",
		ACPSessionID:   "acp-session-worktree-resume",
		AgentProfileID: "profile-worktree-resume",
		ExecutorType:   string(models.ExecutorTypeWorktree),
		RepositoryPath: repo,
		UseWorktree:    true,
		BaseBranch:     "main",
	})
	require.NoError(t, err)
	require.NotEmpty(t, prepareProgressPayloads(eventBus))

	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.True(t, completed[0].Success)
	requirePrepareStep(t, completed[0].Steps, "Validate Docker")
}

// gitMetadataAwareCreateInstanceExecutor extends createInstanceExecutor with a
// no-op GitMetadataProjectionEnforcer attestation so tests that exercise a
// full resumed-worktree Launch (which now requires an executor capable of
// enforcing the task Git metadata policy) do not fail closed.
type gitMetadataAwareCreateInstanceExecutor struct {
	createInstanceExecutor
}

func (e *gitMetadataAwareCreateInstanceExecutor) PrepareGitMetadataProjection(_ context.Context, _ *ExecutorCreateRequest) error {
	return nil
}

func TestLaunch_ResumeWithoutPreparationPublishesNoPrepareEvents(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-resume-without-preparation")

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-resume-without-preparation",
		SessionID:      "session-resume-without-preparation",
		ACPSessionID:   "acp-session-resume-without-preparation",
		AgentProfileID: "profile-resume-without-preparation",
		ExecutorType:   string(models.ExecutorTypeLocal),
		IsEphemeral:    true,
	})
	require.NoError(t, err)
	require.Empty(t, prepareProgressPayloads(eventBus))
	require.Empty(t, prepareCompletedPayloads(eventBus))
}

func newPrepareEventsTestManager(t *testing.T, profileID string) (*Manager, *MockEventBusWithTracking) {
	t.Helper()
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(&createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, log),
	})
	eventBus := &MockEventBusWithTracking{}
	mgr := NewManager(
		newTestRegistry(), eventBus, execRegistry,
		&MockCredentialsManager{}, &countingProfileResolver{info: &AgentProfileInfo{
			ProfileID: profileID,
			AgentName: "auggie",
		}}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	return mgr, eventBus
}

func prepareProgressPayloads(eventBus *MockEventBusWithTracking) []*PrepareProgressEventPayload {
	eventBus.mu.Lock()
	defer eventBus.mu.Unlock()
	var out []*PrepareProgressEventPayload
	for _, tracked := range eventBus.PublishedEvents {
		payload, ok := tracked.Event.Data.(*PrepareProgressEventPayload)
		if ok {
			out = append(out, payload)
		}
	}
	return out
}
