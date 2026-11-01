package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
			generation := recoveryStateFingerprint(t, repo)
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
			require.Equal(t, generation, recoveryStateFingerprint(t, repo))
		})
	}
}

func recoveryStateFingerprint(t *testing.T, repo Repository) interface{} {
	t.Helper()
	switch r := repo.(type) {
	case *memoryRepository:
		encoded, err := json.Marshal([]interface{}{r.entries, r.statusGeneration, r.identities, r.autoRun, r.autoRunIncarnation})
		require.NoError(t, err)
		digest := sha256.Sum256(encoded)
		return hex.EncodeToString(digest[:])
	case *sqliteRepository:
		hash := sha256.New()
		encoder := json.NewEncoder(hash)
		for _, table := range []string{"queued_messages", "queue_session_state", "tasks", "task_sessions"} {
			require.NoError(t, encoder.Encode(table))
			rows, err := r.db.Queryx("SELECT * FROM " + table + " ORDER BY 1")
			require.NoError(t, err)
			for rows.Next() {
				row := map[string]interface{}{}
				require.NoError(t, rows.MapScan(row))
				require.NoError(t, encoder.Encode(row))
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
		}
		return hex.EncodeToString(hash.Sum(nil))
	}
	t.Fatal("unsupported recovery fixture")
	return nil
}

func TestRecoveryStateFingerprintDetectsQueueRowMutation(t *testing.T) {
	for _, factory := range []recoveryFactory{{"sqlite", newTestSQLiteRepo}, {"postgres", newTestPostgresRepo}} {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: "entry", TaskID: "task", SessionID: "session", Content: "before", QueuedAt: time.Now().UTC()}, 0))
			before := recoveryStateFingerprint(t, repo)
			sqlRepo := repo.(*sqliteRepository)
			_, err := sqlRepo.db.Exec(`UPDATE queued_messages SET content = 'after' WHERE id = 'entry'`)
			require.NoError(t, err)
			require.NotEqual(t, before, recoveryStateFingerprint(t, repo))
			for _, statement := range []string{
				`UPDATE queue_session_state SET auto_run = 0 WHERE session_id = 'session'`,
				`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = 'task'`,
				`UPDATE task_sessions SET task_id = 'other' WHERE id = 'session'`,
			} {
				before = recoveryStateFingerprint(t, repo)
				_, err = sqlRepo.db.Exec(statement)
				require.NoError(t, err)
				require.NotEqual(t, before, recoveryStateFingerprint(t, repo), statement)
			}
		})
	}
}

// @covers AC-TASKS-QUEUE-RECOVERY-001.5
func TestRecoveryReadRejectsWrongIncarnationWithoutBinding(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			before := recoveryStateFingerprint(t, repo)
			identity.SessionIncarnationID = "foreign"
			_, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
			require.ErrorIs(t, err, ErrSessionIdentityMismatch)
			require.Equal(t, before, recoveryStateFingerprint(t, repo))
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
			before := recoveryStateFingerprint(t, repo)
			got, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
			require.Error(t, err)
			require.Empty(t, got.Entries)
			require.Equal(t, before, recoveryStateFingerprint(t, repo))
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
			before := recoveryStateFingerprint(t, repo)
			snapshot, err := requireRecoveryObserver(t, repo).RecoveryRead(context.Background(), identity)
			require.Error(t, err)
			require.Empty(t, snapshot.Entries)
			require.Equal(t, before, recoveryStateFingerprint(t, repo))
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
			before := recoveryStateFingerprint(t, repo)
			stale := identity
			stale.SessionIncarnationID = "foreign"
			got, err := reader.RecoveryReadBatch(context.Background(), []QueueSessionIdentity{identity, stale})
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, recoveryStateFingerprint(t, repo))
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
