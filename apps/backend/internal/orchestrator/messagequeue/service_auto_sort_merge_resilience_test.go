package messagequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

var errGlobalSettingsUnavailable = errors.New("global settings unavailable")

func resilienceRepos() map[string]func(*testing.T) Repository {
	return map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() },
		"sqlite": newTestSQLiteRepo,
	}
}

// A fold runs inside the repository critical section. It must never wait on
// the service-wide edit-lease mutex, which an edit in another session holds
// across its own repository write.
func TestAutoFoldInsideRepositoryDoesNotAcquireEditLeaseMutex(t *testing.T) {
	for name, makeRepo := range resilienceRepos() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			service := NewService(repo, 10, logger.Default())
			require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("A1", "A"), 0))
			source := sortTestEntry("A2", "A")
			require.NoError(t, repo.InsertForSession(ctx, identity, source, 0))
			policy := service.resolveAdmissionAutoMergePolicy(ctx, &identity, identity.SessionID)
			require.NotNil(t, policy)

			service.editLeaseMu.Lock()
			done := make(chan error, 1)
			go func() {
				_, merged, err := repo.(autoMergeAdmissionRepository).AutoMergeIntoAboveForSessionWithPolicy(ctx, identity, source.ID, *policy)
				if err == nil && !merged {
					err = errors.New("adjacent same-sender rows did not fold")
				}
				done <- err
			}()
			select {
			case err := <-done:
				service.editLeaseMu.Unlock()
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				service.editLeaseMu.Unlock()
				t.Fatal("repository fold waited on the service edit-lease mutex")
			}
		})
	}
}

func TestAdmissionUsesExplicitSessionPoliciesWhenGlobalLoaderFails(t *testing.T) {
	failingLoader := func(context.Context) (AutoMergePolicy, error) {
		return AutoMergePolicy{}, errGlobalSettingsUnavailable
	}
	for name, makeRepo := range resilienceRepos() {
		t.Run(name+"/adjacent with explicit auto-merge", func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			service := NewService(repo, 1, logger.Default())
			_, err := service.SetSessionAutoMerge(ctx, identity, true)
			require.NoError(t, err)
			first := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "first"}
			require.NoError(t, repo.InsertForSession(ctx, identity, first, 0))
			service.SetGlobalAutoMergePolicyLoader(failingLoader)

			_, err = service.QueueMessageWithMetadataForSession(ctx, identity, "second", "", QueuedByUser, false, nil, nil)
			require.NoError(t, err)
			rows, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{"first\n\nsecond"}, sortTestContents(rows))
		})
		t.Run(name+"/sorted with explicit auto-merge", func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			service := NewService(repo, 2, logger.Default())
			_, err := service.SetSessionAutoMerge(ctx, identity, true)
			require.NoError(t, err)
			require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("A1", "A"), 0))
			require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("B1", "B"), 0))
			service.SetGlobalAutoMergePolicyLoader(failingLoader)

			_, err = service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
			require.NoError(t, err)
			rows, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{"A1\n\nA2", "B1"}, sortTestContents(rows))
		})
		t.Run(name+"/inherited auto-merge fails closed", func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			service := NewService(repo, 1, logger.Default())
			first := &QueuedMessage{TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "first"}
			require.NoError(t, repo.InsertForSession(ctx, identity, first, 0))
			service.SetGlobalAutoMergePolicyLoader(failingLoader)

			_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "second", "", QueuedByUser, false, nil, nil)
			require.ErrorIs(t, err, ErrQueueFull)
		})
	}
}
