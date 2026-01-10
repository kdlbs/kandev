package acp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/acp/protocol"
)

// createTestMessage creates a test ACP message
func createTestMessage(taskID string, msgType protocol.MessageType, data map[string]interface{}) *protocol.Message {
	return &protocol.Message{
		Type:      msgType,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    taskID,
		Data:      data,
	}
}

func TestNewMemoryMessageStore(t *testing.T) {
	store := NewMemoryMessageStore(100)
	if store == nil {
		t.Fatal("NewMemoryMessageStore returned nil")
	}
	if store.maxPerTask != 100 {
		t.Errorf("expected maxPerTask = 100, got %d", store.maxPerTask)
	}
}

func TestNewMemoryMessageStoreDefaultMax(t *testing.T) {
	// When maxPerTask is 0 or negative, it should default to 1000
	store := NewMemoryMessageStore(0)
	if store.maxPerTask != 1000 {
		t.Errorf("expected default maxPerTask = 1000, got %d", store.maxPerTask)
	}

	store = NewMemoryMessageStore(-1)
	if store.maxPerTask != 1000 {
		t.Errorf("expected default maxPerTask = 1000, got %d", store.maxPerTask)
	}
}

func TestStore(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	msg := createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Test log message",
	})

	err := store.Store(ctx, msg)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify message was stored
	messages, err := store.GetMessages(ctx, "task-1", 10, time.Time{})
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}

func TestStoreMultipleMessages(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		msg := createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
			"message": "Log message",
			"index":   i,
		})
		_ = store.Store(ctx, msg)
	}

	messages, _ := store.GetMessages(ctx, "task-1", 10, time.Time{})
	if len(messages) != 5 {
		t.Errorf("expected 5 messages, got %d", len(messages))
	}
}

func TestStoreTrimExcess(t *testing.T) {
	store := NewMemoryMessageStore(3) // Max 3 messages per task
	ctx := context.Background()

	// Store 5 messages
	for i := 0; i < 5; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeLog,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			AgentID:   "test-agent",
			TaskID:    "task-1",
			Data:      map[string]interface{}{"index": i},
		}
		_ = store.Store(ctx, msg)
	}

	messages, _ := store.GetMessages(ctx, "task-1", 10, time.Time{})
	if len(messages) != 3 {
		t.Errorf("expected 3 messages after trimming, got %d", len(messages))
	}

	// Verify we kept the most recent messages (indices 2, 3, 4)
	for i, msg := range messages {
		expectedIndex := i + 2
		if idx, ok := msg.Data["index"].(int); ok {
			if idx != expectedIndex {
				t.Errorf("expected message index %d, got %d", expectedIndex, idx)
			}
		}
	}
}

func TestGetMessagesEmpty(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	messages, err := store.GetMessages(ctx, "non-existent-task", 10, time.Time{})
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for non-existent task, got %d", len(messages))
	}
}

func TestGetMessagesWithLimit(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		msg := createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
			"index": i,
		})
		_ = store.Store(ctx, msg)
	}

	messages, _ := store.GetMessages(ctx, "task-1", 3, time.Time{})
	if len(messages) != 3 {
		t.Errorf("expected 3 messages with limit, got %d", len(messages))
	}
}

func TestGetMessagesWithSince(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	baseTime := time.Now()

	// Store messages with different timestamps
	for i := 0; i < 5; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeLog,
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			AgentID:   "test-agent",
			TaskID:    "task-1",
			Data:      map[string]interface{}{"index": i},
		}
		_ = store.Store(ctx, msg)
	}

	// Get messages since 2 hours after base (should get indices 3 and 4)
	since := baseTime.Add(2 * time.Hour)
	messages, _ := store.GetMessages(ctx, "task-1", 10, since)
	if len(messages) != 2 {
		t.Errorf("expected 2 messages after since filter, got %d", len(messages))
	}
}

func TestGetMessagesReturnsCopy(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	msg := createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Original",
	})
	_ = store.Store(ctx, msg)

	messages1, _ := store.GetMessages(ctx, "task-1", 10, time.Time{})
	messages2, _ := store.GetMessages(ctx, "task-1", 10, time.Time{})

	// Modifying returned slice should not affect the store
	if len(messages1) > 0 {
		messages1[0] = nil
	}
	if messages2[0] == nil {
		t.Error("GetMessages should return a copy of messages")
	}
}

func TestGetLatestProgress(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	// Store some non-progress messages
	_ = store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Log message",
	}))

	// Store progress messages
	_ = store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress": 50,
			"message":  "Halfway done",
		},
	})

	_ = store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress":        75,
			"message":         "Almost done",
			"current_file":    "main.go",
			"files_processed": 3,
			"total_files":     4,
		},
	})

	progress, err := store.GetLatestProgress(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetLatestProgress failed: %v", err)
	}
	if progress == nil {
		t.Fatal("expected progress data, got nil")
	}
	if progress.Progress != 75 {
		t.Errorf("expected progress = 75, got %d", progress.Progress)
	}
	if progress.Message != "Almost done" {
		t.Errorf("expected message = 'Almost done', got %s", progress.Message)
	}
	if progress.CurrentFile != "main.go" {
		t.Errorf("expected current_file = 'main.go', got %s", progress.CurrentFile)
	}
}

func TestGetLatestProgressNoProgress(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	// Store only non-progress messages
	_ = store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Log message",
	}))

	progress, err := store.GetLatestProgress(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetLatestProgress failed: %v", err)
	}
	if progress != nil {
		t.Error("expected nil progress when no progress messages exist")
	}
}

func TestGetLatestProgressNonExistentTask(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	progress, err := store.GetLatestProgress(ctx, "non-existent")
	if err != nil {
		t.Fatalf("GetLatestProgress failed: %v", err)
	}
	if progress != nil {
		t.Error("expected nil progress for non-existent task")
	}
}

func TestDelete(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	_ = store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, nil))
	_ = store.Store(ctx, createTestMessage("task-2", protocol.MessageTypeLog, nil))

	err := store.Delete(ctx, "task-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// task-1 messages should be gone
	messages, _ := store.GetMessages(ctx, "task-1", 10, time.Time{})
	if len(messages) != 0 {
		t.Error("expected no messages after delete")
	}

	// task-2 messages should still exist
	messages, _ = store.GetMessages(ctx, "task-2", 10, time.Time{})
	if len(messages) != 1 {
		t.Error("delete should not affect other tasks")
	}
}

func TestMultipleTasks(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	// Store messages for multiple tasks
	_ = store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, nil))
	_ = store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, nil))
	_ = store.Store(ctx, createTestMessage("task-2", protocol.MessageTypeLog, nil))

	messages1, _ := store.GetMessages(ctx, "task-1", 10, time.Time{})
	messages2, _ := store.GetMessages(ctx, "task-2", 10, time.Time{})

	if len(messages1) != 2 {
		t.Errorf("expected 2 messages for task-1, got %d", len(messages1))
	}
	if len(messages2) != 1 {
		t.Errorf("expected 1 message for task-2, got %d", len(messages2))
	}
}

func TestProgressDataExtraction(t *testing.T) {
	store := NewMemoryMessageStore(100)
	ctx := context.Background()

	// Test with float64 values (as would come from JSON)
	_ = store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress":        float64(50),
			"files_processed": float64(10),
			"total_files":     float64(20),
		},
	})

	progress, _ := store.GetLatestProgress(ctx, "task-1")
	if progress.Progress != 50 {
		t.Errorf("expected progress = 50, got %d", progress.Progress)
	}
	if progress.FilesProcessed != 10 {
		t.Errorf("expected files_processed = 10, got %d", progress.FilesProcessed)
	}
	if progress.TotalFiles != 20 {
		t.Errorf("expected total_files = 20, got %d", progress.TotalFiles)
	}
}

