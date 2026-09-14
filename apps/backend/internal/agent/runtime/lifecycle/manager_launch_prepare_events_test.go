package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestLaunch_WorktreeResumePublishesPrepareCompleted(t *testing.T) {
	mgr, eventBus := newPrepareEventsTestManager(t, "profile-worktree-resume")
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(models.ExecutorTypeWorktree, &progressPreparer{})

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-worktree-resume",
		SessionID:      "session-worktree-resume",
		ACPSessionID:   "acp-session-worktree-resume",
		AgentProfileID: "profile-worktree-resume",
		ExecutorType:   string(models.ExecutorTypeWorktree),
		RepositoryPath: "/tmp/repo",
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
