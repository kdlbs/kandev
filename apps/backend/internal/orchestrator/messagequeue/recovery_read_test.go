package messagequeue

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recoveryObserver interface {
	RecoveryRead(context.Context, QueueSessionIdentity) (RepositorySnapshot, error)
}

type recoveryFactory = struct {
	name string
	new  func(*testing.T) Repository
}

func recoveryFactories() []recoveryFactory {
	return append(append([]recoveryFactory{}, autoRunRepositoryFactories...), recoveryFactory{"postgres", newTestPostgresRepo})
}

func requireRecoveryObserver(t *testing.T, repo Repository) recoveryObserver {
	t.Helper()
	observer, ok := repo.(recoveryObserver)
	require.True(t, ok, "repository must expose bounded nonmutating recovery observation")
	return observer
}

// @covers AC-TASKS-QUEUE-RECOVERY-001.6
func TestRecoveryReadPreservesQueueAndGeneration(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			entry := &QueuedMessage{ID: "entry", SessionID: "session", TaskID: "task", Content: "original", Model: "model", QueuedBy: QueuedByUser, QueuedAt: time.Now().UTC()}
			entry.Metadata = map[string]interface{}{"nested": map[string]interface{}{"value": "original"}}
			require.NoError(t, repo.InsertForSession(ctx, identity, entry, 0))
			before, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			generation := recoveryWriteCounter(t, repo)
			observer := requireRecoveryObserver(t, repo)
			for range 2 {
				snapshot, err := observer.RecoveryRead(ctx, identity)
				require.NoError(t, err)
				require.Equal(t, before, snapshot.Entries)
				require.Zero(t, snapshot.StatusGeneration)
				snapshot.Entries[0].Content = "caller edit"
				snapshot.Entries[0].Metadata["nested"].(map[string]interface{})["value"] = "caller edit"
			}
			after, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, generation, recoveryWriteCounter(t, repo))
		})
	}
}

func recoveryWriteCounter(t *testing.T, repo Repository) interface{} {
	t.Helper()
	switch r := repo.(type) {
	case *memoryRepository:
		return []interface{}{reflect.ValueOf(r.statusGeneration).Len(), r.statusGeneration["session"], len(r.identities)}
	case *sqliteRepository:
		if r.db.DriverName() != "pgx" {
			var n int64
			require.NoError(t, r.db.Get(&n, "SELECT total_changes()"))
			return n
		}
		var n int64
		require.NoError(t, r.db.Get(&n, "SELECT COALESCE(SUM(status_generation), 0) FROM queue_session_state"))
		return n
	}
	t.Fatal("unsupported recovery fixture")
	return nil
}

// @covers AC-TASKS-QUEUE-RECOVERY-001.5
func TestRecoveryReadRejectsWrongIncarnationWithoutBinding(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			before := recoveryWriteCounter(t, repo)
			identity.SessionIncarnationID = "foreign"
			_, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)
			require.Equal(t, before, recoveryWriteCounter(t, repo))
		})
	}
}

// @covers AC-TASKS-QUEUE-RECOVERY-001.7
func TestRecoveryReadRejectsOversizeSource(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			entry := &QueuedMessage{ID: "entry", SessionID: "session", TaskID: "task", Content: strings.Repeat("x", 16*1024*1024+1), QueuedBy: QueuedByUser, QueuedAt: time.Now().UTC()}
			require.NoError(t, repo.InsertForSession(context.Background(), identity, entry, 0))
			before := recoveryWriteCounter(t, repo)
			got, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
			require.Error(t, err)
			require.Empty(t, got.Entries)
			require.Equal(t, before, recoveryWriteCounter(t, repo))
		})
	}
}

func TestRecoveryReadLegacyPolicyDoesNotBind(t *testing.T) {
	repo := NewMemoryRepository().(*memoryRepository)
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	repo.autoRun[identity.SessionID] = false
	snapshot, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
	require.NoError(t, err)
	require.False(t, snapshot.AutoRun)
	require.Empty(t, repo.autoRunIncarnation)
}

func TestRecoveryReadEmptyAuthoritativeSessionDoesNotBind(t *testing.T) {
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	repo := NewMemoryRepositoryWithAuthority(func(context.Context, string, string) (QueueSessionIdentity, error) { return identity, nil }).(*memoryRepository)
	repo.autoRun[identity.SessionID] = false
	snapshot, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
	require.NoError(t, err)
	require.False(t, snapshot.AutoRun)
	require.Empty(t, snapshot.Entries)
	require.Empty(t, repo.identities)
}

func TestRecoveryReadRejectsEntryCap(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			for i := range 1025 {
				entry := &QueuedMessage{ID: fmt.Sprintf("entry-%d", i), SessionID: "session", TaskID: "task", QueuedBy: QueuedByUser, QueuedAt: time.Now().UTC()}
				require.NoError(t, repo.InsertForSession(context.Background(), identity, entry, 0))
			}
			before := recoveryWriteCounter(t, repo)
			snapshot, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
			require.Error(t, err)
			require.Empty(t, snapshot.Entries)
			require.Equal(t, before, recoveryWriteCounter(t, repo))
		})
	}
}

func TestRecoveryReadBatchRejectsStaleWithoutPartialResult(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			reader, ok := repo.(interface {
				RecoveryReadBatch(context.Context, []QueueSessionIdentity) ([]RepositorySnapshot, error)
			})
			require.True(t, ok, "atomic bounded inventory observer required")
			before := recoveryWriteCounter(t, repo)
			stale := identity
			stale.SessionIncarnationID = "foreign"
			got, err := reader.RecoveryReadBatch(context.Background(), []QueueSessionIdentity{identity, stale})
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, recoveryWriteCounter(t, repo))
		})
	}
}

type recoveryMarshalProbe struct{ called *bool }

func (p recoveryMarshalProbe) MarshalJSON() ([]byte, error) {
	*p.called = true
	return []byte(`"probe"`), nil
}

func TestRecoveryReadPreflightsMetadataBeforeEncoding(t *testing.T) {
	repo := NewMemoryRepository().(*memoryRepository)
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	called := false
	repo.entries["session"] = []*QueuedMessage{{ID: "entry", TaskID: "task", SessionID: "session", Metadata: map[string]interface{}{"huge": strings.Repeat("x", 16*1024*1024+1), "probe": recoveryMarshalProbe{called: &called}}}}
	_, err := repo.RecoveryRead(context.Background(), identity)
	require.Error(t, err)
	require.False(t, called, "reject oversized source before allocating its JSON encoding")
}
