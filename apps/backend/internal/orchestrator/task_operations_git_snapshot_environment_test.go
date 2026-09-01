package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSaveGitStatusSnapshotCompletionSupersedesEnvironmentPeers(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSharedGitSnapshotEnvironment(t, repo, "task-git-completion", "env-git-completion", "session-completion-old", "session-completion-new")

	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:                "archive-before-completion",
		TaskEnvironmentID: "env-git-completion",
		SessionID:         "session-completion-old",
		SnapshotType:      models.SnapshotTypeArchive,
		CreatedAt:         base,
		Metadata:          map[string]interface{}{"repository_name": "backend"},
	}))
	require.NoError(t, repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:                "completion-before-completion",
		TaskEnvironmentID: "env-git-completion",
		SessionID:         "session-completion-old",
		SnapshotType:      models.SnapshotTypeStatusUpdate,
		TriggeredBy:       "agent_completed",
		CreatedAt:         base.Add(time.Minute),
		Metadata:          map[string]interface{}{"repository_name": "backend"},
	}))
	require.NoError(t, repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
		ID:                "live-before-completion",
		TaskEnvironmentID: "env-git-completion",
		SessionID:         "session-completion-new",
		CreatedAt:         base.Add(2 * time.Minute),
		Metadata:          map[string]interface{}{"repository_name": "backend"},
	}))

	agent := &mockAgentManager{
		getGitStatusFreshFunc: func(context.Context, string) (*client.GitStatusResult, error) {
			return &client.GitStatusResult{
				Success:         true,
				RepositoryName:  "backend",
				Branch:          "feature/current",
				HeadCommit:      "commit-current",
				BaseCommit:      "base",
				BranchAdditions: 4,
				Files:           map[string]interface{}{"main.go": map[string]interface{}{"status": "modified"}},
			}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	wrote, noExecution := svc.saveGitStatusSnapshot(ctx, "session-completion-new", true)
	require.True(t, wrote)
	require.False(t, noExecution)

	oldRows, err := repo.GetGitSnapshotsBySession(ctx, "session-completion-old", 0)
	require.NoError(t, err)
	require.Len(t, oldRows, 1)
	require.Equal(t, models.SnapshotTypeArchive, oldRows[0].SnapshotType)

	newRows, err := repo.GetGitSnapshotsBySession(ctx, "session-completion-new", 0)
	require.NoError(t, err)
	require.Len(t, newRows, 1)
	require.Equal(t, "agent_completed", newRows[0].TriggeredBy)
	require.Equal(t, "env-git-completion", newRows[0].TaskEnvironmentID)
}

func TestCaptureArchiveDiffRequiresEnvironmentIdentity(t *testing.T) {
	t.Run("writes environment-owned archive snapshot", func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		seedSharedGitSnapshotEnvironment(t, repo, "task-git-archive", "env-git-archive", "session-git-archive")

		agent := &mockAgentManager{
			getCumulativeDiffFunc: func(context.Context, string, string) (*client.CumulativeDiffResult, error) {
				return &client.CumulativeDiffResult{
					Success:      true,
					BaseCommit:   "base",
					HeadCommit:   "head",
					TotalCommits: 2,
					Files:        map[string]interface{}{"main.go": map[string]interface{}{"status": "modified"}},
				}, nil
			},
			getGitStatusFreshFunc: func(context.Context, string) (*client.GitStatusResult, error) {
				return nil, nil
			},
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
		svc.captureArchiveDiff(ctx, "session-git-archive", "base")

		rows, err := repo.GetGitSnapshotsBySession(ctx, "session-git-archive", 0)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, models.SnapshotTypeArchive, rows[0].SnapshotType)
		require.Equal(t, "env-git-archive", rows[0].TaskEnvironmentID)
	})

	t.Run("stops before agentctl when environment is missing", func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		seedSession(t, repo, "task-git-no-environment", "session-git-no-environment", "")
		var callsMu sync.Mutex
		calls := 0
		agent := &mockAgentManager{
			getCumulativeDiffFunc: func(context.Context, string, string) (*client.CumulativeDiffResult, error) {
				callsMu.Lock()
				calls++
				callsMu.Unlock()
				return nil, nil
			},
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
		svc.captureArchiveDiff(ctx, "session-git-no-environment", "base")

		callsMu.Lock()
		require.Zero(t, calls)
		callsMu.Unlock()
	})
}

func TestCaptureGitStatusSnapshotWithRetryStopsWithoutEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-git-no-environment", "session-git-no-environment", "")
	var callsMu sync.Mutex
	calls := 0
	agent := &mockAgentManager{
		getGitStatusFreshFunc: func(context.Context, string) (*client.GitStatusResult, error) {
			callsMu.Lock()
			calls++
			callsMu.Unlock()
			return nil, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	started := time.Now()
	svc.captureGitStatusSnapshotWithRetry(ctx, "session-git-no-environment")

	require.Less(t, time.Since(started), 500*time.Millisecond)
	callsMu.Lock()
	require.Zero(t, calls)
	callsMu.Unlock()
}

func TestSaveGitStatusSnapshotConcurrentCompletionWritesKeepOneCurrentRow(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSharedGitSnapshotEnvironment(t, repo, "task-git-concurrent", "env-git-concurrent", "session-concurrent-a", "session-concurrent-b")

	agent := &mockAgentManager{
		getGitStatusFreshFunc: func(context.Context, string) (*client.GitStatusResult, error) {
			return &client.GitStatusResult{
				Success:         true,
				RepositoryName:  "backend",
				Branch:          "feature/concurrent",
				HeadCommit:      "commit-concurrent",
				BaseCommit:      "base",
				BranchAdditions: 1,
			}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sessionID := "session-concurrent-a"
			if index%2 == 1 {
				sessionID = "session-concurrent-b"
			}
			wrote, noExecution := svc.saveGitStatusSnapshot(ctx, sessionID, true)
			if !wrote || noExecution {
				errs <- fmt.Errorf("write %d returned wrote=%t noExecution=%t", index, wrote, noExecution)
			}
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	current, err := repo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, []string{"env-git-concurrent"})
	require.NoError(t, err)
	require.Len(t, current, 1)
	require.Equal(t, "env-git-concurrent", current[0].TaskEnvironmentID)
	require.Equal(t, "backend", current[0].Metadata["repository_name"])
}
