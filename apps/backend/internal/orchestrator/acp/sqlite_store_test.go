package acp

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/acp/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory SQLite database with the required schema
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS task_agent_execution_logs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_instance_id TEXT,
			log_level TEXT NOT NULL,
			message_type TEXT NOT NULL,
			message TEXT,
			metadata TEXT,
			timestamp DATETIME NOT NULL
		)
	`)
	require.NoError(t, err)

	return db
}

func TestNewSQLiteMessageStore(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewSQLiteMessageStore(db)
	require.NotNil(t, store)
	assert.Equal(t, db, store.db)
}

// ============================================================================
// Store() method tests
// ============================================================================

func TestSQLiteStore_StoreSingleMessage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	msg := createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Test log message",
		"level":   "info",
	})

	err := store.Store(ctx, msg)
	require.NoError(t, err)

	// Verify message was stored
	messages, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, protocol.MessageTypeLog, messages[0].Type)
}

func TestSQLiteStore_StoreMultipleMessagesForDifferentTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store messages for task-1
	for i := 0; i < 3; i++ {
		err := store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
			"message": "Task 1 log",
			"index":   i,
		}))
		require.NoError(t, err)
	}

	// Store messages for task-2
	for i := 0; i < 2; i++ {
		err := store.Store(ctx, createTestMessage("task-2", protocol.MessageTypeLog, map[string]interface{}{
			"message": "Task 2 log",
			"index":   i,
		}))
		require.NoError(t, err)
	}

	messages1, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages1, 3)

	messages2, err := store.GetAllMessages(ctx, "task-2")
	require.NoError(t, err)
	assert.Len(t, messages2, 2)
}

func TestSQLiteStore_StoreAllMessageTypes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	testCases := []struct {
		name     string
		msgType  protocol.MessageType
		data     map[string]interface{}
	}{
		{
			name:    "progress",
			msgType: protocol.MessageTypeProgress,
			data:    map[string]interface{}{"progress": 50, "message": "Processing"},
		},
		{
			name:    "log",
			msgType: protocol.MessageTypeLog,
			data:    map[string]interface{}{"level": "info", "message": "Log message"},
		},
		{
			name:    "result",
			msgType: protocol.MessageTypeResult,
			data:    map[string]interface{}{"status": "completed", "summary": "Task done"},
		},
		{
			name:    "error",
			msgType: protocol.MessageTypeError,
			data:    map[string]interface{}{"error": "Something went wrong"},
		},
		{
			name:    "status",
			msgType: protocol.MessageTypeStatus,
			data:    map[string]interface{}{"status": "running", "message": "In progress"},
		},
		{
			name:    "heartbeat",
			msgType: protocol.MessageTypeHeartbeat,
			data:    map[string]interface{}{},
		},
		{
			name:    "session_info",
			msgType: protocol.MessageTypeSessionInfo,
			data:    map[string]interface{}{"session_id": "sess-123", "resumable": true},
		},
		{
			name:    "input_required",
			msgType: protocol.MessageTypeInputRequired,
			data:    map[string]interface{}{"prompt_id": "p1", "prompt": "Enter name"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			taskID := "task-" + tc.name
			msg := createTestMessage(taskID, tc.msgType, tc.data)
			err := store.Store(ctx, msg)
			require.NoError(t, err)

			// Verify message was stored and can be retrieved
			messages, err := store.GetAllMessages(ctx, taskID)
			require.NoError(t, err)
			assert.Len(t, messages, 1)
			assert.Equal(t, tc.msgType, messages[0].Type)
		})
	}
}

func TestSQLiteStore_StoreMetadataSerialization(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	complexData := map[string]interface{}{
		"string_field":  "test value",
		"int_field":     float64(42), // JSON unmarshals numbers as float64
		"bool_field":    true,
		"nested_object": map[string]interface{}{"key": "value"},
		"array_field":   []interface{}{"a", "b", "c"},
	}

	msg := createTestMessage("task-1", protocol.MessageTypeLog, complexData)
	err := store.Store(ctx, msg)
	require.NoError(t, err)

	// Verify by querying database directly
	var metadataStr string
	err = db.QueryRow(`SELECT metadata FROM task_agent_execution_logs WHERE task_id = ?`, "task-1").Scan(&metadataStr)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(metadataStr), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "test value", parsed["string_field"])
	assert.Equal(t, float64(42), parsed["int_field"])
	assert.Equal(t, true, parsed["bool_field"])
}

func TestSQLiteStore_StoreLogLevelExtraction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	testCases := []struct {
		name          string
		msgType       protocol.MessageType
		data          map[string]interface{}
		expectedLevel string
	}{
		{
			name:          "log with level",
			msgType:       protocol.MessageTypeLog,
			data:          map[string]interface{}{"level": "warn", "message": "Warning"},
			expectedLevel: "warn",
		},
		{
			name:          "log with debug level",
			msgType:       protocol.MessageTypeLog,
			data:          map[string]interface{}{"level": "debug", "message": "Debug"},
			expectedLevel: "debug",
		},
		{
			name:          "error message",
			msgType:       protocol.MessageTypeError,
			data:          map[string]interface{}{"error": "Error occurred"},
			expectedLevel: "error",
		},
		{
			name:          "progress message",
			msgType:       protocol.MessageTypeProgress,
			data:          map[string]interface{}{"progress": 50},
			expectedLevel: "info",
		},
		{
			name:          "status message",
			msgType:       protocol.MessageTypeStatus,
			data:          map[string]interface{}{"status": "running"},
			expectedLevel: "info",
		},
		{
			name:          "heartbeat message",
			msgType:       protocol.MessageTypeHeartbeat,
			data:          map[string]interface{}{},
			expectedLevel: "debug",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			taskID := "task-level-" + string(rune('a'+i))
			msg := createTestMessage(taskID, tc.msgType, tc.data)
			err := store.Store(ctx, msg)
			require.NoError(t, err)

			var logLevel string
			err = db.QueryRow(`SELECT log_level FROM task_agent_execution_logs WHERE task_id = ?`, taskID).Scan(&logLevel)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedLevel, logLevel)
		})
	}
}

// ============================================================================
// GetAllMessages() method tests
// ============================================================================

func TestSQLiteStore_GetAllMessages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store multiple messages
	for i := 0; i < 5; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeLog,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			AgentID:   "test-agent",
			TaskID:    "task-1",
			Data:      map[string]interface{}{"index": i},
		}
		err := store.Store(ctx, msg)
		require.NoError(t, err)
	}

	messages, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages, 5)
}

func TestSQLiteStore_GetAllMessagesEmptyForNonExistentTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	messages, err := store.GetAllMessages(ctx, "non-existent-task")
	require.NoError(t, err)
	assert.Len(t, messages, 0)
}


func TestSQLiteStore_GetAllMessagesChronologicalOrder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	baseTime := time.Now()

	// Store messages in reverse order
	for i := 4; i >= 0; i-- {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeLog,
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			AgentID:   "test-agent",
			TaskID:    "task-1",
			Data:      map[string]interface{}{"index": i},
		}
		err := store.Store(ctx, msg)
		require.NoError(t, err)
	}

	messages, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	require.Len(t, messages, 5)

	// Verify chronological order (earliest first)
	for i, msg := range messages {
		idx, ok := msg.Data["index"].(float64)
		require.True(t, ok)
		assert.Equal(t, float64(i), idx, "Messages should be in chronological order")
	}
}

func TestSQLiteStore_GetAllMessagesTaskIsolation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store messages for task-1
	for i := 0; i < 3; i++ {
		err := store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
			"task": "1",
			"idx":  i,
		}))
		require.NoError(t, err)
	}

	// Store messages for task-2
	for i := 0; i < 2; i++ {
		err := store.Store(ctx, createTestMessage("task-2", protocol.MessageTypeLog, map[string]interface{}{
			"task": "2",
			"idx":  i,
		}))
		require.NoError(t, err)
	}

	// Verify task-1 only gets its own messages
	messages1, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages1, 3)
	for _, msg := range messages1 {
		assert.Equal(t, "1", msg.Data["task"])
	}

	// Verify task-2 only gets its own messages
	messages2, err := store.GetAllMessages(ctx, "task-2")
	require.NoError(t, err)
	assert.Len(t, messages2, 2)
	for _, msg := range messages2 {
		assert.Equal(t, "2", msg.Data["task"])
	}
}

func TestSQLiteStore_GetAllMessagesMetadataDeserialization(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	originalData := map[string]interface{}{
		"string":  "value",
		"number":  float64(123),
		"boolean": true,
		"nested":  map[string]interface{}{"inner": "data"},
	}

	msg := createTestMessage("task-1", protocol.MessageTypeLog, originalData)
	err := store.Store(ctx, msg)
	require.NoError(t, err)

	messages, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, "value", messages[0].Data["string"])
	assert.Equal(t, float64(123), messages[0].Data["number"])
	assert.Equal(t, true, messages[0].Data["boolean"])
	nested, ok := messages[0].Data["nested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "data", nested["inner"])
}

// ============================================================================
// GetMessages() method tests
// ============================================================================

func TestSQLiteStore_GetMessagesWithLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store 10 messages
	for i := 0; i < 10; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeLog,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			AgentID:   "test-agent",
			TaskID:    "task-1",
			Data:      map[string]interface{}{"index": i},
		}
		err := store.Store(ctx, msg)
		require.NoError(t, err)
	}

	messages, err := store.GetMessages(ctx, "task-1", 3, time.Time{})
	require.NoError(t, err)
	assert.Len(t, messages, 3)
}

func TestSQLiteStore_GetMessagesSinceTime(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
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
		err := store.Store(ctx, msg)
		require.NoError(t, err)
	}

	// Get messages since 2 hours after base (should get indices 3 and 4)
	since := baseTime.Add(2 * time.Hour)
	messages, err := store.GetMessages(ctx, "task-1", 0, since)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestSQLiteStore_GetMessagesCombineLimitAndSince(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	baseTime := time.Now()

	// Store 10 messages
	for i := 0; i < 10; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeLog,
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			AgentID:   "test-agent",
			TaskID:    "task-1",
			Data:      map[string]interface{}{"index": i},
		}
		err := store.Store(ctx, msg)
		require.NoError(t, err)
	}

	// Get at most 2 messages since hour 3 (should get indices 4, 5 only due to limit)
	since := baseTime.Add(3 * time.Hour)
	messages, err := store.GetMessages(ctx, "task-1", 2, since)
	require.NoError(t, err)
	assert.Len(t, messages, 2)

	// Verify we got the earliest matching messages
	idx1, _ := messages[0].Data["index"].(float64)
	idx2, _ := messages[1].Data["index"].(float64)
	assert.Equal(t, float64(4), idx1)
	assert.Equal(t, float64(5), idx2)
}

func TestSQLiteStore_GetMessagesEmptyWhenNoMatch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	baseTime := time.Now()

	// Store a message
	msg := &protocol.Message{
		Type:      protocol.MessageTypeLog,
		Timestamp: baseTime,
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data:      map[string]interface{}{"message": "test"},
	}
	err := store.Store(ctx, msg)
	require.NoError(t, err)

	// Query with a since time in the future
	futureSince := baseTime.Add(1 * time.Hour)
	messages, err := store.GetMessages(ctx, "task-1", 10, futureSince)
	require.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestSQLiteStore_GetMessagesEmptyForNonExistentTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	messages, err := store.GetMessages(ctx, "non-existent", 10, time.Time{})
	require.NoError(t, err)
	assert.Len(t, messages, 0)
}

// ============================================================================
// GetLatestProgress() method tests
// ============================================================================

func TestSQLiteStore_GetLatestProgress(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store some non-progress messages
	err := store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Log message",
	}))
	require.NoError(t, err)

	// Store first progress message
	err = store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress": float64(50),
			"message":  "Halfway done",
		},
	})
	require.NoError(t, err)

	// Store second progress message (more recent)
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	err = store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress":        float64(75),
			"message":         "Almost done",
			"current_file":    "main.go",
			"files_processed": float64(3),
			"total_files":     float64(4),
		},
	})
	require.NoError(t, err)

	progress, err := store.GetLatestProgress(ctx, "task-1")
	require.NoError(t, err)
	require.NotNil(t, progress)
	assert.Equal(t, 75, progress.Progress)
	assert.Equal(t, "Almost done", progress.Message)
	assert.Equal(t, "main.go", progress.CurrentFile)
	assert.Equal(t, 3, progress.FilesProcessed)
	assert.Equal(t, 4, progress.TotalFiles)
}

func TestSQLiteStore_GetLatestProgressNoProgressMessages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store only non-progress messages
	err := store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, map[string]interface{}{
		"message": "Log message",
	}))
	require.NoError(t, err)

	progress, err := store.GetLatestProgress(ctx, "task-1")
	require.NoError(t, err)
	assert.Nil(t, progress)
}

func TestSQLiteStore_GetLatestProgressNonExistentTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	progress, err := store.GetLatestProgress(ctx, "non-existent-task")
	require.NoError(t, err)
	assert.Nil(t, progress)
}

func TestSQLiteStore_GetLatestProgressParseData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Test with float64 values (as they come from JSON/database)
	err := store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress":        float64(50),
			"message":         "Processing files",
			"current_file":    "test.go",
			"files_processed": float64(10),
			"total_files":     float64(20),
		},
	})
	require.NoError(t, err)

	progress, err := store.GetLatestProgress(ctx, "task-1")
	require.NoError(t, err)
	require.NotNil(t, progress)
	assert.Equal(t, 50, progress.Progress)
	assert.Equal(t, "Processing files", progress.Message)
	assert.Equal(t, "test.go", progress.CurrentFile)
	assert.Equal(t, 10, progress.FilesProcessed)
	assert.Equal(t, 20, progress.TotalFiles)
}

func TestSQLiteStore_GetLatestProgressPartialData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Test with only progress field
	err := store.Store(ctx, &protocol.Message{
		Type:      protocol.MessageTypeProgress,
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		TaskID:    "task-1",
		Data: map[string]interface{}{
			"progress": float64(25),
		},
	})
	require.NoError(t, err)

	progress, err := store.GetLatestProgress(ctx, "task-1")
	require.NoError(t, err)
	require.NotNil(t, progress)
	assert.Equal(t, 25, progress.Progress)
	assert.Equal(t, "", progress.Message)
	assert.Equal(t, "", progress.CurrentFile)
	assert.Equal(t, 0, progress.FilesProcessed)
	assert.Equal(t, 0, progress.TotalFiles)
}

// ============================================================================
// Delete() method tests
// ============================================================================

func TestSQLiteStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store messages for task-1
	for i := 0; i < 3; i++ {
		err := store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, nil))
		require.NoError(t, err)
	}

	// Verify messages exist
	messages, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages, 3)

	// Delete messages
	err = store.Delete(ctx, "task-1")
	require.NoError(t, err)

	// Verify messages are gone
	messages, err = store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestSQLiteStore_DeletePreservesOtherTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Store messages for both tasks
	err := store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, nil))
	require.NoError(t, err)
	err = store.Store(ctx, createTestMessage("task-1", protocol.MessageTypeLog, nil))
	require.NoError(t, err)
	err = store.Store(ctx, createTestMessage("task-2", protocol.MessageTypeLog, nil))
	require.NoError(t, err)

	// Delete task-1 messages
	err = store.Delete(ctx, "task-1")
	require.NoError(t, err)

	// task-1 messages should be gone
	messages1, err := store.GetAllMessages(ctx, "task-1")
	require.NoError(t, err)
	assert.Len(t, messages1, 0)

	// task-2 messages should still exist
	messages2, err := store.GetAllMessages(ctx, "task-2")
	require.NoError(t, err)
	assert.Len(t, messages2, 1)
}

func TestSQLiteStore_DeleteNonExistentTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Deleting non-existent task should not return an error
	err := store.Delete(ctx, "non-existent-task")
	require.NoError(t, err)
}

// ============================================================================
// Error handling tests
// ============================================================================

func TestSQLiteStore_StoreWithClosedDB(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Close the database
	db.Close()

	// Attempt to store should return an error
	msg := createTestMessage("task-1", protocol.MessageTypeLog, nil)
	err := store.Store(ctx, msg)
	assert.Error(t, err)
}

func TestSQLiteStore_GetMessagesWithClosedDB(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Close the database
	db.Close()

	// Attempt to get messages should return an error
	_, err := store.GetMessages(ctx, "task-1", 10, time.Time{})
	assert.Error(t, err)
}

func TestSQLiteStore_GetAllMessagesWithClosedDB(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Close the database
	db.Close()

	// Attempt to get all messages should return an error
	_, err := store.GetAllMessages(ctx, "task-1")
	assert.Error(t, err)
}

func TestSQLiteStore_GetLatestProgressWithClosedDB(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Close the database
	db.Close()

	// Attempt to get latest progress should return an error
	_, err := store.GetLatestProgress(ctx, "task-1")
	assert.Error(t, err)
}

func TestSQLiteStore_DeleteWithClosedDB(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteMessageStore(db)
	ctx := context.Background()

	// Close the database
	db.Close()

	// Attempt to delete should return an error
	err := store.Delete(ctx, "task-1")
	assert.Error(t, err)
}
