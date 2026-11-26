package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// An identity-free admission must fold under the captured policy: a leased
// sorted target is not merged into, and an unleased one is.
func TestMemoryIdentityFreeFoldHonorsLeasesAndSortedFolds(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	agentEntry := func(id, content string) *QueuedMessage {
		return &QueuedMessage{
			ID: id, SessionID: "session", TaskID: "task", Content: content, QueuedBy: QueuedByAgent,
			Metadata: map[string]interface{}{MetadataSenderTaskID: "sub-a"},
		}
	}
	require.NoError(t, repo.InsertForSession(ctx, identity, agentEntry("sender", "first"), 10))
	require.NoError(t, repo.InsertForSession(ctx, identity, &QueuedMessage{
		ID: "user", SessionID: "session", TaskID: "task", Content: "between", QueuedBy: QueuedByUser,
	}, 10))
	policy := func(leased map[string]struct{}) *AdmissionPolicy {
		return &AdmissionPolicy{AutoMerge: AutoMergePolicy{Enabled: true, Source: AutoMergeSourceGlobal}, leased: leased}
	}

	_, didMerge, err := repo.(*memoryRepository).autoMergeCandidateIntoAbove(ctx, nil,
		agentEntry("incoming", "second"), nil, policy(map[string]struct{}{"sender": {}}), nil)
	require.NoError(t, err)
	require.False(t, didMerge, "a leased sorted target must not receive the fold")

	merged, didMerge, err := repo.(*memoryRepository).autoMergeCandidateIntoAbove(ctx, nil,
		agentEntry("incoming", "second"), nil, policy(nil), nil)
	require.NoError(t, err)
	require.True(t, didMerge)
	require.Equal(t, "sender", merged.ID)
}

// Disabling a session, or reading an existing override, must not depend on
// the global settings read. Enabling an inheriting session still does.
func TestSessionSwitchReadsGlobalPolicyOnlyWhenInheriting(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	inheriting := QueueSessionIdentity{TaskID: "task", SessionID: "inheriting", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, inheriting)
	service := NewService(repo, 10, logger.Default())
	service.SetGlobalAutoMergePolicyLoader(func(context.Context) (AutoMergePolicy, error) {
		return AutoMergePolicy{}, errors.New("settings unavailable")
	})

	_, err := service.SetSessionAutoMerge(ctx, inheriting, false)
	require.NoError(t, err, "disabling an inheriting session must not need global settings")
	_, err = service.SetSessionAutoMerge(ctx, inheriting, true)
	require.NoError(t, err, "re-enabling an explicit override must not need global settings")

	fresh := QueueSessionIdentity{TaskID: "task", SessionID: "fresh", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, fresh)
	_, err = service.SetSessionAutoMerge(ctx, fresh, true)
	require.Error(t, err, "enabling an inheriting session needs the global value to decide compaction")
}
