package messagequeue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
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

	t.Run("rejects deletion of agent-authored entries", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		agentEntry, err := svc.QueueMessageWithMetadata(ctx, "s", "t", "agent entry", "", QueuedByAgent, false, nil, nil)
		require.NoError(t, err)

		err = svc.RemoveEntry(ctx, "s", agentEntry.ID)
		assert.ErrorIs(t, err, ErrEntryNotFound)

		status := svc.GetStatus(ctx, "s")
		assert.Equal(t, 1, status.Count)
		assert.Equal(t, "agent entry", status.Entries[0].Content)
	})
}

func TestCancelAll(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		_, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
		require.NoError(t, err)
	}
	n, err := svc.CancelAll(ctx, "s")
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	status := svc.GetStatus(ctx, "s")
	assert.Equal(t, 0, status.Count)
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
