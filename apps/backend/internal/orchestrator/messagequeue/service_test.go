package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupService(t *testing.T) *Service {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stderr",
	})
	require.NoError(t, err)
	return NewServiceMemory(log)
}

func TestQueueMessage(t *testing.T) {
	t.Run("appends new entries", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "test content", "model-1", "user-1", false, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, msg.ID)
		assert.Equal(t, "session-1", msg.SessionID)
		assert.Equal(t, "test content", msg.Content)
		assert.Equal(t, "user-1", msg.QueuedBy)
		assert.NotZero(t, msg.QueuedAt)
		assert.Equal(t, int64(1), msg.Position)
	})

	t.Run("multiple messages produce ordered list", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		for _, body := range []string{"first", "second", "third"} {
			_, err := svc.QueueMessage(ctx, "session-1", "task-1", body, "", "user-1", false, nil)
			require.NoError(t, err)
		}
		status := svc.GetStatus(ctx, "session-1")
		require.Equal(t, 3, status.Count)
		assert.Equal(t, "first", status.Entries[0].Content)
		assert.Equal(t, "second", status.Entries[1].Content)
		assert.Equal(t, "third", status.Entries[2].Content)
	})

	t.Run("rejects overflow with ErrQueueFull", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		for i := 0; i < DefaultMaxPerSession; i++ {
			_, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
			require.NoError(t, err)
		}
		_, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
		assert.ErrorIs(t, err, ErrQueueFull)
	})

	t.Run("queues messages with attachments", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		attachments := []MessageAttachment{
			{Type: "image", Data: "base64data", MimeType: "image/png"},
		}
		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "with attachment", "", "user-1", false, attachments)
		require.NoError(t, err)
		assert.Len(t, msg.Attachments, 1)
		assert.Equal(t, "image", msg.Attachments[0].Type)
	})
}

func TestLoweredQueueCapacityBlocksAdmissionsWithoutPruning(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 2, log)
	ctx := context.Background()

	for _, content := range []string{"first", "second"} {
		_, err := svc.QueueMessage(ctx, "s", "t", content, "", QueuedByUser, false, nil)
		require.NoError(t, err)
	}
	svc.SetMaxPerSession(1)

	assert.Equal(t, 2, svc.GetStatus(ctx, "s").Count)
	_, err = svc.QueueMessage(ctx, "s", "t", "blocked", "", QueuedByUser, false, nil)
	assert.ErrorIs(t, err, ErrQueueFull)
}

func TestRestoreMessageBypassesLoweredCapacity(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 2, log)
	ctx := context.Background()

	_, err = svc.QueueMessage(ctx, "s", "t", "first", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	_, err = svc.QueueMessage(ctx, "s", "t", "second", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	first, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)
	svc.SetMaxPerSession(1)

	_, err = svc.RestoreMessage(ctx, first)
	require.NoError(t, err)
	status := svc.GetStatus(ctx, "s")
	require.Equal(t, 2, status.Count)
	assert.Equal(t, "first", status.Entries[0].Content)
	assert.Equal(t, "second", status.Entries[1].Content)
}

func TestRequeueMessageBypassesLoweredCapacity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		coalesceKey string
	}{
		{name: "ordinary"},
		{name: "coalesced", coalesceKey: "retry-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
			require.NoError(t, err)
			svc := NewService(NewMemoryRepository(), 2, log)
			ctx := context.Background()

			var first *QueuedMessage
			if tc.coalesceKey == "" {
				first, err = svc.QueueMessage(ctx, "s", "t", "first", "", QueuedByUser, false, nil)
			} else {
				first, _, err = svc.QueueMessageWithCoalesceKey(
					ctx, "s", "t", "first", "", QueuedByWorkflow, false, nil, nil,
					tc.coalesceKey, true,
				)
			}
			require.NoError(t, err)
			_, err = svc.QueueMessage(ctx, "s", "t", "second", "", QueuedByUser, false, nil)
			require.NoError(t, err)
			dequeued, ok := svc.TakeQueued(ctx, "s")
			require.True(t, ok)
			require.Equal(t, first.ID, dequeued.ID)
			svc.SetMaxPerSession(1)

			requeued, replaced, err := svc.RequeueMessage(
				ctx, dequeued, dequeued.QueuedBy, tc.coalesceKey,
			)
			require.NoError(t, err)
			require.NotNil(t, requeued)
			assert.False(t, replaced)
			status := svc.GetStatus(ctx, "s")
			require.Equal(t, 2, status.Count)
			assert.Equal(t, "second", status.Entries[0].Content)
			assert.Equal(t, "first", status.Entries[1].Content)
		})
	}
}

func TestLifecycleRetryBypassesCurrentCapacity(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	repo := &maxRecordingRepository{Repository: NewMemoryRepository()}
	svc := NewService(repo, 3, log)
	ctx := context.Background()

	queued, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s", "t", "initial", "", QueuedByWorkflow, false, nil, nil, "lifecycle:1", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)
	svc.SetMaxPerSession(1)
	_, _, accepted, err = svc.RequeueLifecycleMessageWithCoalesceKey(
		ctx, "s", "t", "retry", "", QueuedByWorkflow, false, nil,
		queued.Metadata, "lifecycle:1", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)
	assert.Equal(t, []int{3, 0}, repo.lifecycleMaxima())
}

func TestQueueCapacityConcurrentReadWrite(t *testing.T) {
	svc := setupService(t)
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(offset int) {
			defer wait.Done()
			for i := 0; i < 500; i++ {
				svc.SetMaxPerSession((i + offset) % 20)
				_ = svc.MaxPerSession()
				_ = svc.GetStatus(context.Background(), "s")
			}
		}(worker)
	}
	wait.Wait()
}

func TestLiveCapacityCoversEveryNewAdmissionPath(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 1, log)
	ctx := context.Background()

	_, err = svc.QueueMessage(ctx, "s", "t", "first", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	_, appended, err := svc.AppendContent(ctx, "s", "t", "same sender", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	assert.True(t, appended)
	_, _, err = svc.AppendContent(ctx, "s", "t", "new sender", "", QueuedByAgent, false, nil)
	assert.ErrorIs(t, err, ErrQueueFull)
	_, _, err = svc.QueueMessageWithCoalesceKey(
		ctx, "s", "t", "coalesced", "", QueuedByWorkflow, false, nil, nil, "key", true,
	)
	assert.ErrorIs(t, err, ErrQueueFull)
	_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s", "t", "lifecycle", "", QueuedByWorkflow, false, nil, nil, "lifecycle", true,
	)
	assert.ErrorIs(t, err, ErrQueueFull)
	assert.False(t, accepted)

	svc.SetMaxPerSession(0)
	_, err = svc.QueueMessage(ctx, "s", "t", "unlimited", "", QueuedByServer, false, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, svc.GetStatus(ctx, "s").Max)
}

func TestAppendContent(t *testing.T) {
	t.Run("appends to tail when same sender", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, err := svc.QueueMessage(ctx, "s", "t", "first", "", "user", false, nil)
		require.NoError(t, err)

		_, appended, err := svc.AppendContent(ctx, "s", "t", "second", "", "user", false, nil)
		require.NoError(t, err)
		assert.True(t, appended)

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 1, status.Count)
		assert.Equal(t, "first\n\n---\n\nsecond", status.Entries[0].Content)
	})

	t.Run("inserts new entry when tail is different sender", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, err := svc.QueueMessage(ctx, "s", "t", "from user", "", "user", false, nil)
		require.NoError(t, err)

		_, appended, err := svc.AppendContent(ctx, "s", "t", "from agent", "", "agent", false, nil)
		require.NoError(t, err)
		assert.False(t, appended)

		status := svc.GetStatus(ctx, "s")
		assert.Equal(t, 2, status.Count)
	})

	t.Run("inserts new entry when queue empty", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, appended, err := svc.AppendContent(ctx, "s", "t", "fresh", "", "user", false, nil)
		require.NoError(t, err)
		assert.False(t, appended)

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 1, status.Count)
		assert.Equal(t, "fresh", status.Entries[0].Content)
	})
}

func TestQueueMessageWithCoalesceKey(t *testing.T) {
	t.Run("replaces matching entry without changing FIFO position", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		first, err := svc.QueueMessage(ctx, "s", "t", "first", "", "user", false, nil)
		require.NoError(t, err)
		ci, replaced, err := svc.QueueMessageWithCoalesceKey(ctx, "s", "t", "old ci", "", QueuedByWorkflow, false, nil, map[string]interface{}{"origin": "ci"}, "ci-key", true)
		require.NoError(t, err)
		require.False(t, replaced)
		_, err = svc.QueueMessage(ctx, "s", "t", "tail", "", "user", false, nil)
		require.NoError(t, err)

		updated, replaced, err := svc.QueueMessageWithCoalesceKey(ctx, "s", "t", "new ci", "", QueuedByWorkflow, false, nil, map[string]interface{}{"origin": "ci-new"}, "ci-key", true)
		require.NoError(t, err)
		require.True(t, replaced)
		require.Equal(t, ci.ID, updated.ID)

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 3, status.Count)
		assert.Equal(t, first.ID, status.Entries[0].ID)
		assert.Equal(t, ci.ID, status.Entries[1].ID)
		assert.Equal(t, "new ci", status.Entries[1].Content)
		assert.Equal(t, "ci-new", status.Entries[1].Metadata["origin"])
		assert.Equal(t, "tail", status.Entries[2].Content)
	})

	t.Run("does not mutate caller metadata or retag existing entries", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()
		metadata := map[string]interface{}{"origin": "ci"}

		first, replaced, err := svc.QueueMessageWithCoalesceKey(ctx, "s", "t", "first ci", "", QueuedByWorkflow, false, nil, metadata, "ci-key", true)
		require.NoError(t, err)
		require.False(t, replaced)
		second, replaced, err := svc.QueueMessageWithCoalesceKey(ctx, "s", "t", "second ci", "", QueuedByWorkflow, false, nil, metadata, "other-key", true)
		require.NoError(t, err)
		require.False(t, replaced)

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 2, status.Count)
		assert.Equal(t, first.ID, status.Entries[0].ID)
		assert.Equal(t, second.ID, status.Entries[1].ID)
		assert.Equal(t, "ci-key", status.Entries[0].Metadata[MetadataCoalesceKey])
		assert.Equal(t, "other-key", status.Entries[1].Metadata[MetadataCoalesceKey])
		assert.NotContains(t, metadata, MetadataCoalesceKey)
	})

	t.Run("returns entry not found when insert disabled and no match exists", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, _, err := svc.QueueMessageWithCoalesceKey(ctx, "s", "t", "ci", "", QueuedByWorkflow, false, nil, nil, "ci-key", false)
		assert.ErrorIs(t, err, ErrEntryNotFound)
		assert.Equal(t, 0, svc.GetStatus(ctx, "s").Count)
	})
}

func TestTakeQueued(t *testing.T) {
	t.Run("returns entries in FIFO order", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		for _, body := range []string{"first", "second", "third"} {
			_, err := svc.QueueMessage(ctx, "s", "t", body, "", "u", false, nil)
			require.NoError(t, err)
		}
		for _, want := range []string{"first", "second", "third"} {
			msg, ok := svc.TakeQueued(ctx, "s")
			require.True(t, ok, "queue empty before %q", want)
			assert.Equal(t, want, msg.Content)
		}
		_, ok := svc.TakeQueued(ctx, "s")
		assert.False(t, ok)
	})

	t.Run("returns false when empty", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, ok := svc.TakeQueued(ctx, "s")
		assert.False(t, ok)
		assert.Nil(t, msg)
	})
}

// TestTakeQueuedEntry covers TakeQueuedEntry: out-of-FIFO-order removal,
// takeability of agent-authored entries, the
// not-found (nil, false, nil) shape for a missing or foreign-session id,
// and — distinctly — a genuine repository error propagating as a non-nil
// error rather than being collapsed into the not-found shape.
func TestTakeQueuedEntry(t *testing.T) {
	t.Run("removes and returns the targeted entry regardless of FIFO position", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, err := svc.QueueMessage(ctx, "s", "t", "first", "", "u", false, nil)
		require.NoError(t, err)
		second, err := svc.QueueMessage(ctx, "s", "t", "second", "", "u", false, nil)
		require.NoError(t, err)
		_, err = svc.QueueMessage(ctx, "s", "t", "third", "", "u", false, nil)
		require.NoError(t, err)

		msg, ok, err := svc.TakeQueuedEntry(ctx, "s", second.ID)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "second", msg.Content)

		// The other two entries are untouched and keep their relative order.
		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 2, status.Count)
		assert.Equal(t, "first", status.Entries[0].Content)
		assert.Equal(t, "third", status.Entries[1].Content)

		// Taking the same id again finds nothing — it's already gone.
		_, ok, err = svc.TakeQueuedEntry(ctx, "s", second.ID)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("takes agent-authored entries", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		agentEntry, err := svc.QueueMessageWithMetadata(ctx, "s", "t", "agent entry", "", QueuedByAgent, false, nil, nil)
		require.NoError(t, err)

		msg, ok, err := svc.TakeQueuedEntry(ctx, "s", agentEntry.ID)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "agent entry", msg.Content)
	})

	t.Run("returns false for a missing or foreign-session id", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		victim, err := svc.QueueMessage(ctx, "s-victim", "t", "victim entry", "", "u", false, nil)
		require.NoError(t, err)

		_, ok, err := svc.TakeQueuedEntry(ctx, "s-victim", "missing-id")
		require.NoError(t, err)
		assert.False(t, ok)

		_, ok, err = svc.TakeQueuedEntry(ctx, "s-attacker", victim.ID)
		require.NoError(t, err)
		assert.False(t, ok)

		status := svc.GetStatus(ctx, "s-victim")
		assert.Equal(t, 1, status.Count)
	})

	t.Run("propagates a genuine repository error instead of reporting not-found", func(t *testing.T) {
		// A repository error (e.g. a transient DB failure) must not be
		// collapsed into the same (nil, false) shape as a legitimate
		// not-found — InterruptForPeerMessage treats the two differently
		// (not-found falls back to a FIFO-head drain; an error propagates
		// without a fallback, since the error says nothing about what is
		// actually at the FIFO head).
		log, err := logger.NewLogger(logger.LoggingConfig{
			Level:      "error",
			Format:     "console",
			OutputPath: "stderr",
		})
		require.NoError(t, err)
		wantErr := errors.New("db unavailable")
		repo := &errInjectingRepository{Repository: NewMemoryRepository(), takeByIDErr: wantErr}
		svc := NewService(repo, DefaultMaxPerSession, log)
		ctx := context.Background()

		msg, ok, err := svc.TakeQueuedEntry(ctx, "s", "some-id")
		assert.Nil(t, msg)
		assert.False(t, ok)
		assert.ErrorIs(t, err, wantErr)
	})
}

// errInjectingRepository wraps a Repository and returns a configured error
// from TakeByID, letting tests exercise TakeQueuedEntry's error-propagation
// path without needing a real repository failure.
type errInjectingRepository struct {
	Repository
	takeByIDErr error
}

type maxRecordingRepository struct {
	Repository
	mu               sync.Mutex
	lifecycleMaxSeen []int
}

func (r *maxRecordingRepository) InsertOrReplaceLifecycleByCoalesceKey(
	ctx context.Context,
	msg *QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	r.lifecycleMaxSeen = append(r.lifecycleMaxSeen, maxPerSession)
	r.mu.Unlock()
	return r.Repository.InsertOrReplaceLifecycleByCoalesceKey(
		ctx, msg, coalesceKey, maxPerSession, allowInsert,
	)
}

func (r *maxRecordingRepository) lifecycleMaxima() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.lifecycleMaxSeen...)
}

// TakeByID returns the configured error if set, otherwise delegates to the
// embedded Repository so the remaining repository operations (Insert,
// ListBySession, CountBySession, ...) still work against the real
// underlying store.
func (r *errInjectingRepository) TakeByID(ctx context.Context, sessionID, entryID string) (*QueuedMessage, error) {
	if r.takeByIDErr != nil {
		return nil, r.takeByIDErr
	}
	return r.Repository.TakeByID(ctx, sessionID, entryID)
}

func TestUpdateMessage(t *testing.T) {
	t.Run("updates content and survives in list", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "s", "t", "original", "", "user-1", false, nil)
		require.NoError(t, err)

		require.NoError(t, svc.UpdateMessage(ctx, "s", msg.ID, "edited", nil, "user-1"))

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 1, status.Count)
		assert.Equal(t, "edited", status.Entries[0].Content)
		assert.Equal(t, msg.ID, status.Entries[0].ID)
	})

	t.Run("rejects update from foreign sender", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "s", "t", "x", "", "user-1", false, nil)
		require.NoError(t, err)

		err = svc.UpdateMessage(ctx, "s", msg.ID, "intruder", nil, "user-2")
		assert.ErrorIs(t, err, ErrEntryNotFound)
	})

	t.Run("rejects update from foreign session", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "s-victim", "t", "x", "", "user-1", false, nil)
		require.NoError(t, err)

		err = svc.UpdateMessage(ctx, "s-attacker", msg.ID, "hijack", nil, "user-1")
		assert.ErrorIs(t, err, ErrEntryNotFound)
	})

	t.Run("returns ErrEntryNotFound for missing id", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()
		err := svc.UpdateMessage(ctx, "s", "missing", "x", nil, "")
		assert.ErrorIs(t, err, ErrEntryNotFound)
	})
}

func TestMemoryRepository_UpdateContentAndMetadataPreservesUnrelatedKeys(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	msg := &QueuedMessage{
		SessionID: "s",
		TaskID:    "t",
		Content:   "original",
		QueuedBy:  "user-1",
		Metadata: map[string]interface{}{
			"entity_references": []interface{}{"old"},
			"origin":            "inter-task",
		},
	}
	require.NoError(t, repo.Insert(ctx, msg, 0))

	require.NoError(t, repo.UpdateContentAndMetadata(
		ctx, "s", msg.ID, "edited", nil,
		map[string]interface{}{"entity_references": []interface{}{"new"}},
		"user-1",
	))

	entries, err := repo.ListBySession(ctx, "s")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "edited", entries[0].Content)
	assert.Equal(t, "inter-task", entries[0].Metadata["origin"])
	assert.Equal(t, []interface{}{"new"}, entries[0].Metadata["entity_references"])
}

func TestUpdateMessageWithMetadataClearsEntityReferences(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	msg, err := svc.QueueMessageWithMetadata(
		ctx, "s", "t", "original", "", "user-1", false, nil,
		map[string]interface{}{
			"entity_references": []interface{}{"old"},
			"origin":            "inter-task",
		},
	)
	require.NoError(t, err)

	require.NoError(t, svc.UpdateMessageWithMetadata(
		ctx, "s", msg.ID, "edited", nil,
		map[string]interface{}{"entity_references": nil},
		"user-1",
	))

	entries := svc.GetStatus(ctx, "s").Entries
	require.Len(t, entries, 1)
	assert.Equal(t, "edited", entries[0].Content)
	assert.Equal(t, "inter-task", entries[0].Metadata["origin"])
	_, exists := entries[0].Metadata["entity_references"]
	assert.False(t, exists, "empty replacement must remove stale references")
}

func TestRemoveEntry(t *testing.T) {
	t.Run("removes the targeted entry", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, _ = svc.QueueMessage(ctx, "s", "t", "a", "", "u", false, nil)
		b, _ := svc.QueueMessage(ctx, "s", "t", "b", "", "u", false, nil)
		_, _ = svc.QueueMessage(ctx, "s", "t", "c", "", "u", false, nil)

		require.NoError(t, svc.RemoveEntry(ctx, "s", b.ID))

		status := svc.GetStatus(ctx, "s")
		assert.Equal(t, 2, status.Count)
		assert.Equal(t, "a", status.Entries[0].Content)
		assert.Equal(t, "c", status.Entries[1].Content)

		err := svc.RemoveEntry(ctx, "s", b.ID)
		assert.ErrorIs(t, err, ErrEntryNotFound)
	})

	t.Run("rejects deletion from a foreign session", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		victim, _ := svc.QueueMessage(ctx, "s-victim", "t", "victim entry", "", "u", false, nil)

		// Attacker knows the entry id (e.g. leaked via queue_full payload from
		// a sibling task) and tries to remove it scoped to a different session.
		err := svc.RemoveEntry(ctx, "s-attacker", victim.ID)
		assert.ErrorIs(t, err, ErrEntryNotFound)

		// Victim entry must still be present.
		status := svc.GetStatus(ctx, "s-victim")
		assert.Equal(t, 1, status.Count)
	})

	t.Run("removes visible entries from every origin", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		for _, queuedBy := range []string{QueuedByUser, QueuedByAgent, QueuedByWorkflow, QueuedByServer} {
			entry, err := svc.QueueMessageWithMetadata(
				ctx, "s", "t", queuedBy+" entry", "", queuedBy, false, nil, nil,
			)
			require.NoError(t, err)
			require.NoError(t, svc.RemoveEntry(ctx, "s", entry.ID), queuedBy)
		}

		assert.Equal(t, 0, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("preserves a durable entry already reserved in flight", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
			ctx, "s", "t", "lifecycle", "", QueuedByWorkflow, false, nil,
			map[string]interface{}{"origin": "github_pr_automation"}, "lifecycle:1", true,
		)
		require.NoError(t, err)
		require.True(t, accepted)

		reserved, ok := svc.ReserveQueued(ctx, "s")
		require.True(t, ok)
		assert.ErrorIs(t, svc.RemoveEntry(ctx, "s", reserved.ID), ErrEntryNotFound)
		require.NoError(t, svc.AcknowledgeQueued(ctx, "s", reserved.ID))
	})
}

func TestCancelAll(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	for _, queuedBy := range []string{QueuedByUser, QueuedByAgent, QueuedByWorkflow, QueuedByServer} {
		_, err := svc.QueueMessage(ctx, "s", "t", "x", "", queuedBy, false, nil)
		require.NoError(t, err)
	}
	n, err := svc.CancelAll(ctx, "s")
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	status := svc.GetStatus(ctx, "s")
	assert.Equal(t, 0, status.Count)
}

func TestCancelAllPreservesDurableEntryReservedInFlight(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s", "t", "lifecycle", "", QueuedByWorkflow, false, nil,
		map[string]interface{}{"origin": "github_pr_automation"}, "lifecycle:1", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)
	reserved, ok := svc.ReserveQueued(ctx, "s")
	require.True(t, ok)

	for _, queuedBy := range []string{QueuedByAgent, QueuedByWorkflow, QueuedByServer} {
		_, err := svc.QueueMessage(ctx, "s", "t", "visible", "", queuedBy, false, nil)
		require.NoError(t, err)
	}

	removed, err := svc.CancelAll(ctx, "s")
	require.NoError(t, err)
	assert.Equal(t, 3, removed)
	assert.Equal(t, 0, svc.GetStatus(ctx, "s").Count)
	require.NoError(t, svc.AcknowledgeQueued(ctx, "s", reserved.ID))
}

func TestGetStatus(t *testing.T) {
	t.Run("empty queue returns zero count and configured max", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		status := svc.GetStatus(ctx, "s")
		assert.Equal(t, 0, status.Count)
		assert.Empty(t, status.Entries)
		assert.Equal(t, DefaultMaxPerSession, status.Max)
	})

	t.Run("returns ordered entries", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		for _, body := range []string{"a", "b", "c"} {
			_, err := svc.QueueMessage(ctx, "s", "t", body, "", "u", false, nil)
			require.NoError(t, err)
		}
		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 3, status.Count)
		assert.Equal(t, "a", status.Entries[0].Content)
		assert.Equal(t, "c", status.Entries[2].Content)
	})

	t.Run("hides a lifecycle row already reserved for delivery", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
			ctx, "s", "t", "pr merged", "", QueuedByWorkflow, false, nil,
			map[string]interface{}{"origin": "github_pr_automation"},
			"github-pr:repo:1:merged", true,
		)
		require.NoError(t, err)
		require.True(t, accepted)
		require.Equal(t, 1, svc.GetStatus(ctx, "s").Count)

		reserved, ok := svc.ReserveQueued(ctx, "s")
		require.True(t, ok)
		// The reservation stays in storage for crash recovery, but it is no
		// longer pending: listing it duplicates the prompt being delivered.
		assert.False(t, reserved.IsReservedInFlight())
		assert.True(t, reserved.IsReservedLifecycleDelivery())
		assert.Equal(t, 0, svc.GetStatus(ctx, "s").Count)

		// A failed delivery requeues the same entry and it becomes visible again.
		_, _, accepted, err = svc.RequeueLifecycleMessageWithCoalesceKey(
			ctx, "s", "t", reserved.Content, "", QueuedByWorkflow, false, nil,
			reserved.Metadata, "github-pr:repo:1:merged", true,
		)
		require.NoError(t, err)
		require.True(t, accepted)
		assert.Equal(t, 1, svc.GetStatus(ctx, "s").Count)

		require.NoError(t, svc.AcknowledgeQueued(ctx, "s", reserved.ID))
		assert.Equal(t, 0, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("does not label a destructive lifecycle take as a reservation", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
			ctx, "s", "t", "pr merged", "", QueuedByWorkflow, false, nil,
			map[string]interface{}{"origin": "github_pr_automation"},
			"github-pr:repo:1:merged", true,
		)
		require.NoError(t, err)
		require.True(t, accepted)

		taken, ok := svc.TakeQueued(ctx, "s")
		require.True(t, ok)
		assert.False(t, taken.IsReservedLifecycleDelivery())
	})
}

func TestTransferSession(t *testing.T) {
	t.Run("moves entries and pending move", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, err := svc.QueueMessage(ctx, "old", "task-1", "hand-off", "", "u", false, nil)
		require.NoError(t, err)
		svc.SetPendingMove(ctx, "old", &PendingMove{TaskID: "task-1", WorkflowStepID: "step-b"})

		require.NoError(t, svc.TransferSession(ctx, "old", "new"))

		_, ok := svc.TakeQueued(ctx, "old")
		assert.False(t, ok)
		_, ok = svc.TakePendingMove(ctx, "old")
		assert.False(t, ok)

		msg, ok := svc.TakeQueued(ctx, "new")
		require.True(t, ok)
		assert.Equal(t, "hand-off", msg.Content)
		assert.Equal(t, "new", msg.SessionID)

		move, ok := svc.TakePendingMove(ctx, "new")
		require.True(t, ok)
		assert.Equal(t, "step-b", move.WorkflowStepID)
	})

	t.Run("no-op when source empty", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()
		require.NoError(t, svc.TransferSession(ctx, "empty", "new"))
		_, ok := svc.TakeQueued(ctx, "new")
		assert.False(t, ok)
	})
}

func TestRestoreSession(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	original, err := svc.QueueMessageWithMetadata(ctx, "s", "task-1", "original", "model-a", "agent", true, []MessageAttachment{
		{Type: "image", Data: "abc", MimeType: "image/png"},
	}, map[string]interface{}{"sender": "task-a"})
	require.NoError(t, err)
	svc.SetPendingMove(ctx, "s", &PendingMove{TaskID: "task-1", WorkflowStepID: "step-a"})

	_, err = svc.QueueMessage(ctx, "s", "task-1", "mutated", "", "user", false, nil)
	require.NoError(t, err)
	svc.SetPendingMove(ctx, "s", &PendingMove{TaskID: "task-1", WorkflowStepID: "step-b"})

	require.NoError(t, svc.RestoreSession(ctx, "s", []QueuedMessage{*original}, &PendingMove{
		TaskID:         "task-1",
		WorkflowStepID: "step-a",
		QueuedAt:       original.QueuedAt,
	}))

	status := svc.GetStatus(ctx, "s")
	require.Equal(t, 1, status.Count)
	restored := status.Entries[0]
	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.Position, restored.Position)
	assert.Equal(t, original.QueuedAt, restored.QueuedAt)
	assert.Equal(t, original.Content, restored.Content)
	assert.Equal(t, original.Metadata, restored.Metadata)
	move, ok := svc.TakePendingMove(ctx, "s")
	require.True(t, ok)
	assert.Equal(t, "step-a", move.WorkflowStepID)
}

func TestPendingMove(t *testing.T) {
	t.Run("set then take returns and clears", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		svc.SetPendingMove(ctx, "s", &PendingMove{TaskID: "t1", WorkflowID: "w1", WorkflowStepID: "step-2", Position: 3})

		got, ok := svc.TakePendingMove(ctx, "s")
		require.True(t, ok)
		assert.Equal(t, "t1", got.TaskID)
		assert.Equal(t, "step-2", got.WorkflowStepID)
		assert.Equal(t, 3, got.Position)
		assert.NotZero(t, got.QueuedAt)

		_, ok = svc.TakePendingMove(ctx, "s")
		assert.False(t, ok)
	})

	t.Run("setting twice replaces previous", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		svc.SetPendingMove(ctx, "s", &PendingMove{TaskID: "t1", WorkflowStepID: "a"})
		svc.SetPendingMove(ctx, "s", &PendingMove{TaskID: "t1", WorkflowStepID: "b"})

		got, ok := svc.TakePendingMove(ctx, "s")
		require.True(t, ok)
		assert.Equal(t, "b", got.WorkflowStepID)
	})
}

func TestConcurrentInsertCap(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	const goroutines = 50
	var (
		wg   sync.WaitGroup
		ok   atomic.Int32
		full atomic.Int32
		bad  atomic.Int32
	)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrQueueFull):
				full.Add(1)
			default:
				bad.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(0), bad.Load())
	assert.Equal(t, int32(DefaultMaxPerSession), ok.Load())
	assert.Equal(t, int32(goroutines-DefaultMaxPerSession), full.Load())
}

func TestConcurrentTakeIdempotent(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.QueueMessage(ctx, "s", "t", "single", "", "u", false, nil)
	require.NoError(t, err)

	var (
		wg   sync.WaitGroup
		hits atomic.Int32
	)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := svc.TakeQueued(ctx, "s"); ok {
				hits.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), hits.Load())
}

func TestQueuedTimestamp(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	msg, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
	after := time.Now().Add(time.Second)

	require.NoError(t, err)
	assert.True(t, msg.QueuedAt.After(before))
	assert.True(t, msg.QueuedAt.Before(after))
}

// TestMemoryRepository_MergeIntoAbove exercises the in-memory repository merge
// with Go-struct entity references (the form the memory repo stores without a
// JSON round-trip) alongside the shared merge rules.
func TestMemoryRepository_MergeIntoAbove(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      entityrefs.CanonicalRef("github", "issue", "acme/repo", "1"),
		Provider: "github",
		Kind:     "issue",
		ID:       "1",
		Title:    "Issue 1",
		URL:      "https://github.com/acme/repo/issues/1",
		Scope:    "acme/repo",
	}

	target := &QueuedMessage{
		SessionID: "s1", TaskID: "t1", Content: "first", QueuedBy: "alice",
		Attachments: []MessageAttachment{{Type: "image", Data: "a", MimeType: "image/png"}},
		Metadata:    map[string]interface{}{MetadataEntityReferences: []apiv1.EntityReference{ref}},
	}
	require.NoError(t, repo.Insert(ctx, target, 0))
	source := &QueuedMessage{
		SessionID: "s1", TaskID: "t1", Content: "second", QueuedBy: "alice",
		Attachments: []MessageAttachment{{Type: "file", Data: "b", MimeType: "text/plain"}},
		Metadata:    map[string]interface{}{MetadataEntityReferences: []apiv1.EntityReference{ref}},
	}
	require.NoError(t, repo.Insert(ctx, source, 0))

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "alice")
	require.NoError(t, err)
	assert.Equal(t, target.ID, merged.ID)
	assert.Equal(t, "first\n\nsecond", merged.Content)
	assert.Len(t, merged.Attachments, 2)
	assert.Len(t, entityrefs.NormalizePersisted(merged.Metadata[MetadataEntityReferences]), 1)

	entries, err := repo.ListBySession(ctx, "s1")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "first\n\nsecond", entries[0].Content)
}

// TestMemoryRepository_MergeIntoAbove_MixedKindsRejected covers the memory repo
// kind gate and missing-source error mapping.
func TestMemoryRepository_MergeIntoAbove_MixedKindsRejected(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	require.NoError(t, repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "agent", QueuedBy: QueuedByAgent, Metadata: map[string]interface{}{MetadataSenderTaskID: "task-1"}}, 0))
	source := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "user", QueuedBy: "alice"}
	require.NoError(t, repo.Insert(ctx, source, 0))

	_, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "alice")
	assert.ErrorIs(t, err, ErrNoMergeTarget)

	_, err = repo.MergeIntoAbove(ctx, "s1", "missing", "alice")
	assert.ErrorIs(t, err, ErrEntryNotFound)
}

// TestMemoryRepository_MergeIntoAbove_ReferenceOverflow asserts an over-cap
// reference union is rejected atomically in the in-memory repository: the
// target's content, attachments, and references must stay untouched so a
// failed merge does not leave a partially-folded queue behind.
func TestMemoryRepository_MergeIntoAbove_ReferenceOverflow(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	refs := make([]apiv1.EntityReference, 0, entityrefs.MaxReferencesPerMessage)
	for i := 0; i < entityrefs.MaxReferencesPerMessage; i++ {
		refs = append(refs, apiv1.EntityReference{
			Version:  apiv1.EntityReferenceVersion,
			Ref:      entityrefs.CanonicalRef("github", "issue", "acme/repo", fmt.Sprintf("%d", i)),
			Provider: "github",
			Kind:     "issue",
			ID:       fmt.Sprintf("%d", i),
			Title:    "Issue " + fmt.Sprintf("%d", i),
			URL:      "https://github.com/acme/repo/issues/" + fmt.Sprintf("%d", i),
			Scope:    "acme/repo",
		})
	}
	target := &QueuedMessage{
		SessionID: "s1", TaskID: "t1", Content: "first", QueuedBy: "alice",
		Attachments: []MessageAttachment{{Type: "image", Data: "a", MimeType: "image/png"}},
		Metadata:    map[string]interface{}{MetadataEntityReferences: refs},
	}
	require.NoError(t, repo.Insert(ctx, target, 0))
	source := &QueuedMessage{
		SessionID: "s1", TaskID: "t1", Content: "second", QueuedBy: "alice",
		Attachments: []MessageAttachment{{Type: "file", Data: "b", MimeType: "text/plain"}},
		Metadata: map[string]interface{}{MetadataEntityReferences: []apiv1.EntityReference{
			{Version: apiv1.EntityReferenceVersion, Ref: entityrefs.CanonicalRef("github", "issue", "acme/repo", "x"), Provider: "github", Kind: "issue", ID: "x", Title: "Issue x", URL: "https://github.com/acme/repo/issues/x", Scope: "acme/repo"},
		}},
	}
	require.NoError(t, repo.Insert(ctx, source, 0))

	_, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "alice")
	assert.ErrorIs(t, err, ErrMergeReferenceOverflow)

	entries, err := repo.ListBySession(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	byID := make(map[string]*QueuedMessage, len(entries))
	for i := range entries {
		byID[entries[i].ID] = &entries[i]
	}
	assert.Equal(t, "first", byID[target.ID].Content, "target content changed after rejected overflow merge")
	assert.Len(t, byID[target.ID].Attachments, 1, "target attachments changed after rejected overflow merge")
	assert.Len(t, entityrefs.NormalizePersisted(byID[target.ID].Metadata[MetadataEntityReferences]), entityrefs.MaxReferencesPerMessage, "target refs changed after rejected overflow merge")
	assert.Equal(t, "second", byID[source.ID].Content, "source content changed after rejected overflow merge")
}

// TestService_MergeIntoAbove exercises the service delegation path over the
// memory repository and the mapped errors.
func TestService_MergeIntoAbove(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	above, err := svc.QueueMessage(ctx, "s", "t", "above", "", "alice", false, nil)
	require.NoError(t, err)
	source, err := svc.QueueMessage(ctx, "s", "t", "source", "", "alice", false, nil)
	require.NoError(t, err)

	merged, err := svc.MergeIntoAbove(ctx, "s", source.ID, "alice")
	require.NoError(t, err)
	assert.Equal(t, above.ID, merged.ID)
	assert.Equal(t, "above\n\nsource", merged.Content)
	assert.Equal(t, 1, svc.GetStatus(ctx, "s").Count)

	_, err = svc.MergeIntoAbove(ctx, "s", "missing", "alice")
	assert.ErrorIs(t, err, ErrEntryNotFound)
}

// TestMemoryRepository_MergeIntoAbove_UnsortedSlice proves the memory repo
// selects the merge target by greatest position strictly below the source's,
// not by slice order. ReplaceSession can persist an unsorted snapshot, so a
// slice-adjacent target would be wrong: here the source (position 3) sits
// between an unrelated entry (position 1) and the true target (position 2).
func TestMemoryRepository_MergeIntoAbove_UnsortedSlice(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	err := repo.ReplaceSession(ctx, "s1", []QueuedMessage{
		{ID: "unrelated", SessionID: "s1", TaskID: "t1", Position: 1, Content: "other", QueuedBy: "alice"},
		{ID: "source", SessionID: "s1", TaskID: "t1", Position: 3, Content: "src", QueuedBy: "alice"},
		{ID: "target", SessionID: "s1", TaskID: "t1", Position: 2, Content: "tgt", QueuedBy: "alice"},
	}, nil)
	require.NoError(t, err)

	merged, err := repo.MergeIntoAbove(ctx, "s1", "source", "alice")
	require.NoError(t, err)
	assert.Equal(t, "target", merged.ID)
	assert.Equal(t, "tgt\n\nsrc", merged.Content)

	entries, err := repo.ListBySession(ctx, "s1")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}
