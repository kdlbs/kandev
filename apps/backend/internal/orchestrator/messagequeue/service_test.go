package messagequeue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupService(t *testing.T) *Service {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stdout",
	})
	require.NoError(t, err)
	return NewService(log)
}

func TestQueueMessage(t *testing.T) {
	t.Run("successfully queues a message", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "test content", "model-1", "user-1", false, nil)

		require.NoError(t, err)
		assert.NotEmpty(t, msg.ID)
		assert.Equal(t, "session-1", msg.SessionID)
		assert.Equal(t, "task-1", msg.TaskID)
		assert.Equal(t, "test content", msg.Content)
		assert.Equal(t, "model-1", msg.Model)
		assert.Equal(t, "user-1", msg.QueuedBy)
		assert.False(t, msg.PlanMode)
		assert.NotZero(t, msg.QueuedAt)
	})

	t.Run("replaces existing queued message for same session", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue first message
		msg1, err := svc.QueueMessage(ctx, "session-1", "task-1", "first message", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		// Queue second message for same session
		msg2, err := svc.QueueMessage(ctx, "session-1", "task-1", "second message", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		// IDs should be different
		assert.NotEqual(t, msg1.ID, msg2.ID)

		// Only the second message should be queued
		status := svc.GetStatus(ctx, "session-1")
		assert.True(t, status.IsQueued)
		assert.Equal(t, msg2.ID, status.Message.ID)
		assert.Equal(t, "second message", status.Message.Content)
	})

	t.Run("queues messages with attachments", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		attachments := []MessageAttachment{
			{Type: "image", Data: "base64data", MimeType: "image/png"},
		}

		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "message with attachment", "model-1", "user-1", false, attachments)

		require.NoError(t, err)
		assert.Len(t, msg.Attachments, 1)
		assert.Equal(t, "image", msg.Attachments[0].Type)
	})

	t.Run("supports plan mode flag", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "plan mode message", "model-1", "user-1", true, nil)

		require.NoError(t, err)
		assert.True(t, msg.PlanMode)
	})
}

func TestTakeQueued(t *testing.T) {
	t.Run("retrieves and removes queued message", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue a message
		original, err := svc.QueueMessage(ctx, "session-1", "task-1", "test content", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		// Take the message
		msg, exists := svc.TakeQueued(ctx, "session-1")

		assert.True(t, exists)
		assert.Equal(t, original.ID, msg.ID)
		assert.Equal(t, "test content", msg.Content)

		// Should be removed after taking
		status := svc.GetStatus(ctx, "session-1")
		assert.False(t, status.IsQueued)
		assert.Nil(t, status.Message)
	})

	t.Run("returns false when no message queued", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, exists := svc.TakeQueued(ctx, "session-1")

		assert.False(t, exists)
		assert.Nil(t, msg)
	})

	t.Run("is idempotent - second take returns false", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue and take once
		_, err := svc.QueueMessage(ctx, "session-1", "task-1", "test", "model-1", "user-1", false, nil)
		require.NoError(t, err)
		_, exists := svc.TakeQueued(ctx, "session-1")
		require.True(t, exists)

		// Second take should return false
		_, exists = svc.TakeQueued(ctx, "session-1")
		assert.False(t, exists)
	})
}

func TestCancelQueued(t *testing.T) {
	t.Run("cancels and removes queued message", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue a message
		original, err := svc.QueueMessage(ctx, "session-1", "task-1", "test content", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		// Cancel it
		cancelled, err := svc.CancelQueued(ctx, "session-1")

		require.NoError(t, err)
		assert.Equal(t, original.ID, cancelled.ID)

		// Should be removed
		status := svc.GetStatus(ctx, "session-1")
		assert.False(t, status.IsQueued)
	})

	t.Run("returns error when no message queued", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		_, err := svc.CancelQueued(ctx, "session-1")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no queued message")
	})
}

func TestGetStatus(t *testing.T) {
	t.Run("returns queued status with message", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "test", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		status := svc.GetStatus(ctx, "session-1")

		assert.True(t, status.IsQueued)
		assert.NotNil(t, status.Message)
		assert.Equal(t, msg.ID, status.Message.ID)
	})

	t.Run("returns not queued status when no message", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		status := svc.GetStatus(ctx, "session-1")

		assert.False(t, status.IsQueued)
		assert.Nil(t, status.Message)
	})
}

func TestUpdateMessage(t *testing.T) {
	t.Run("updates existing queued message content", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue a message
		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "original content", "model-1", "user-1", false, nil)
		require.NoError(t, err)
		originalID := msg.ID

		// Update it
		err = svc.UpdateMessage(ctx, "session-1", "updated content")

		require.NoError(t, err)

		// Verify update
		status := svc.GetStatus(ctx, "session-1")
		assert.True(t, status.IsQueued)
		assert.Equal(t, originalID, status.Message.ID)
		assert.Equal(t, "updated content", status.Message.Content)
	})

	t.Run("returns error when no message queued", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		err := svc.UpdateMessage(ctx, "session-1", "new content")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no queued message")
	})

	t.Run("allows empty content updates", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue a message
		_, err := svc.QueueMessage(ctx, "session-1", "task-1", "original", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		// Update to empty (validation happens at handler level)
		err = svc.UpdateMessage(ctx, "session-1", "")

		require.NoError(t, err)
		status := svc.GetStatus(ctx, "session-1")
		assert.Equal(t, "", status.Message.Content)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent queue operations safely", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		var wg sync.WaitGroup
		numGoroutines := 100

		// Concurrently queue messages to different sessions
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sessionID := "session-" + string(rune('A'+idx%26))
				_, err := svc.QueueMessage(ctx, sessionID, "task-1", "content", "model-1", "user-1", false, nil)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()

		// Verify all sessions have messages
		sessionIDs := make(map[string]bool)
		for i := 0; i < numGoroutines; i++ {
			sessionID := "session-" + string(rune('A'+i%26))
			sessionIDs[sessionID] = true
		}

		for sessionID := range sessionIDs {
			status := svc.GetStatus(ctx, sessionID)
			assert.True(t, status.IsQueued, "session %s should be queued", sessionID)
		}
	})

	t.Run("handles concurrent take operations safely", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue a message
		_, err := svc.QueueMessage(ctx, "session-1", "task-1", "test", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		// Try to take the message from multiple goroutines
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, exists := svc.TakeQueued(ctx, "session-1")
				if exists {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		// Only one should succeed
		assert.Equal(t, 1, successCount, "only one goroutine should successfully take the message")

		// Message should be gone
		status := svc.GetStatus(ctx, "session-1")
		assert.False(t, status.IsQueued)
	})

	t.Run("handles concurrent updates and reads safely", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue initial message
		_, err := svc.QueueMessage(ctx, "session-1", "task-1", "initial", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numUpdates := 50
		numReads := 50

		// Concurrent updates
		for i := 0; i < numUpdates; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_ = svc.UpdateMessage(ctx, "session-1", "update-"+string(rune('0'+idx%10)))
			}(i)
		}

		// Concurrent reads
		for i := 0; i < numReads; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				status := svc.GetStatus(ctx, "session-1")
				assert.True(t, status.IsQueued)
			}()
		}

		wg.Wait()

		// Final state should be consistent
		status := svc.GetStatus(ctx, "session-1")
		assert.True(t, status.IsQueued)
		assert.NotEmpty(t, status.Message.Content)
	})
}

func TestMultipleSessionsIsolation(t *testing.T) {
	t.Run("sessions are isolated from each other", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		// Queue messages for multiple sessions
		_, err := svc.QueueMessage(ctx, "session-1", "task-1", "message-1", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		_, err = svc.QueueMessage(ctx, "session-2", "task-2", "message-2", "model-1", "user-1", false, nil)
		require.NoError(t, err)

		// Take from session-1
		msg1, exists := svc.TakeQueued(ctx, "session-1")
		require.True(t, exists)
		assert.Equal(t, "message-1", msg1.Content)

		// session-2 should still have its message
		status2 := svc.GetStatus(ctx, "session-2")
		assert.True(t, status2.IsQueued)
		assert.Equal(t, "message-2", status2.Message.Content)

		// session-1 should be empty
		status1 := svc.GetStatus(ctx, "session-1")
		assert.False(t, status1.IsQueued)
	})
}

func TestQueuedMessageTimestamp(t *testing.T) {
	t.Run("sets queued timestamp correctly", func(t *testing.T) {
		svc := setupService(t)
		ctx := context.Background()

		before := time.Now()
		msg, err := svc.QueueMessage(ctx, "session-1", "task-1", "test", "model-1", "user-1", false, nil)
		after := time.Now()

		require.NoError(t, err)
		assert.True(t, msg.QueuedAt.After(before) || msg.QueuedAt.Equal(before))
		assert.True(t, msg.QueuedAt.Before(after) || msg.QueuedAt.Equal(after))
	})
}
