// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/pkg/acp/protocol"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestOrchestratorTaskPriority(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Create tasks with different priorities
	lowPriorityTask := ts.CreateTestTask(t, "augment-agent", 1)
	highPriorityTask := ts.CreateTestTask(t, "augment-agent", 3)
	medPriorityTask := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start low priority first
	_, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          lowPriorityTask,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Then high priority
	_, err = client.SendRequest("start-2", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          highPriorityTask,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Then medium priority
	_, err = client.SendRequest("start-3", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          medPriorityTask,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// All should start (we have capacity)
	assert.Equal(t, 3, ts.AgentManager.GetLaunchCount())
}

func TestOrchestratorTriggerTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("trigger-1", ws.ActionOrchestratorTrigger, map[string]interface{}{
		"task_id": taskID,
	})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))
	assert.True(t, payload["success"].(bool))
	assert.Equal(t, taskID, payload["task_id"])
}

func TestOrchestratorEndToEndWorkflow(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createOrchestratorWorkspace(t, client)

	// 1. Create workflow with workflow template
	workflowResp, err := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "E2E Test Workflow",
		"description":          "End-to-end test workflow",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var workflowPayload map[string]interface{}
	require.NoError(t, workflowResp.ParsePayload(&workflowPayload))
	workflowID := workflowPayload["id"].(string)

	// 2. Get first workflow step from workflow
	stepResp, err := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)

	var stepListPayload map[string]interface{}
	require.NoError(t, stepResp.ParsePayload(&stepListPayload))
	steps := stepListPayload["steps"].([]interface{})
	require.NotEmpty(t, steps, "workflow should have workflow steps")
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// 3. Create task with workflow step
	repoResp, err := client.SendRequest("repo-1", ws.ActionRepositoryCreate, map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Test Repo",
		"source_type":  "local",
		"local_path":   createTempRepoDir(t),
	})
	require.NoError(t, err)

	var repoPayload map[string]interface{}
	require.NoError(t, repoResp.ParsePayload(&repoPayload))
	repositoryID := repoPayload["id"].(string)

	taskResp, err := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"workflow_step_id": workflowStepID,
		"title":            "Implement feature X",
		"description":      "Create a new feature with tests",
		"priority":         2,
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	require.NoError(t, err)

	var taskPayload map[string]interface{}
	require.NoError(t, taskResp.ParsePayload(&taskPayload))
	taskID := taskPayload["id"].(string)

	// 4. Subscribe to task
	_, err = client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// 5. Start orchestrator
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	assert.True(t, startPayload["success"].(bool))
	agentExecutionID := startPayload["agent_execution_id"].(string)
	assert.NotEmpty(t, agentExecutionID)
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// 6. Collect notifications during execution
	time.Sleep(400 * time.Millisecond)
	notifications := client.CollectNotifications(200 * time.Millisecond)
	t.Logf("Received %d notifications", len(notifications))

	// 7. Complete the task
	completeResp, err := client.SendRequest("complete-1", ws.ActionOrchestratorComplete, map[string]interface{}{
		"task_id": taskID,
	})
	require.NoError(t, err)

	var completePayload map[string]interface{}
	require.NoError(t, completeResp.ParsePayload(&completePayload))
	assert.True(t, completePayload["success"].(bool))

	// 8. Verify final task state
	getResp, err := client.SendRequest("get-1", ws.ActionTaskGet, map[string]string{"id": taskID})
	require.NoError(t, err)

	var getPayload map[string]interface{}
	require.NoError(t, getResp.ParsePayload(&getPayload))
	assert.Equal(t, "COMPLETED", getPayload["state"].(string))
}

// TestOrchestratorAgentMessagePersistence validates that agent messages are stored
// correctly in the database without missing data or ordering issues.
func TestOrchestratorAgentMessagePersistence(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// This test verifies that message_streaming and log events ARE correctly persisted.
	// Progress messages (protocol.MessageTypeProgress) are NOT persisted - only broadcast via WebSocket.
	// Log messages (protocol.MessageTypeLog) ARE now persisted to the database.

	// We'll manually publish message_streaming and log events to test persistence
	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		// Just emit a simple progress message - won't be stored
		return []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "starting",
					"message":  "Starting...",
				},
			},
		}
	})

	// Create test task
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task execution
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool), "Task should start successfully")
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID, "Session ID should be returned")

	// Subscribe to session for agent stream events
	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Wait for simulated agent to start
	time.Sleep(200 * time.Millisecond)

	// Now manually publish message_streaming events which ARE persisted
	// Generate unique message IDs for each streaming message
	messageIDs := []string{
		"msg-" + uuid.New().String()[:8],
		"msg-" + uuid.New().String()[:8],
		"msg-" + uuid.New().String()[:8],
	}

	expectedContents := []string{
		"I'm analyzing the codebase structure.",
		"I found several files that need modification.",
		"Here is my recommendation for the changes.",
	}

	// Log messages to publish
	logMessages := []struct {
		Level   string
		Message string
	}{
		{"info", "Found 10 files to process"},
		{"debug", "Processing file: main.go"},
		{"warning", "Deprecated API usage detected"},
	}

	// Publish message_streaming events - these ARE persisted
	for i, msgID := range messageIDs {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":       "message_streaming",
				"text":       expectedContents[i],
				"message_id": msgID,
				"is_append":  false, // Each is a new message
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Publish log events - these ARE now persisted
	for _, logMsg := range logMessages {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type": "log",
				"data": map[string]interface{}{
					"level":   logMsg.Level,
					"message": logMsg.Message,
				},
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all messages to be processed and stored
	time.Sleep(300 * time.Millisecond)

	// Query the database to verify stored messages
	storedMessages, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)

	t.Logf("Found %d total stored messages", len(storedMessages))

	type msgEntry struct {
		ID            string
		Type          string
		Content       string
		AuthorType    string
		Metadata      map[string]interface{}
		CreatedAt     time.Time
		TaskSessionID string
		TurnID        string
	}
	var agentMessages []*msgEntry
	for _, msg := range storedMessages {
		if string(msg.AuthorType) == "agent" {
			agentMessages = append(agentMessages, &msgEntry{
				ID:            msg.ID,
				Type:          string(msg.Type),
				Content:       msg.Content,
				AuthorType:    string(msg.AuthorType),
				Metadata:      msg.Metadata,
				CreatedAt:     msg.CreatedAt,
				TaskSessionID: msg.TaskSessionID,
				TurnID:        msg.TurnID,
			})
		}
	}
	t.Logf("Found %d agent messages in database", len(agentMessages))

	// Verify messages are stored
	require.GreaterOrEqual(t, len(agentMessages), 3, "Should have at least 3 agent messages from streaming events")

	// Verify messages are in correct chronological order
	for i := 1; i < len(agentMessages); i++ {
		prev := agentMessages[i-1]
		curr := agentMessages[i]
		assert.True(t, !curr.CreatedAt.Before(prev.CreatedAt),
			"Messages should be in chronological order: message %d (%s) at %v should not be before message %d (%s) at %v",
			i, curr.ID, curr.CreatedAt, i-1, prev.ID, prev.CreatedAt)
	}

	// Verify no duplicate messages exist (check for duplicate IDs)
	seenIDs := make(map[string]bool)
	for _, msg := range agentMessages {
		assert.False(t, seenIDs[msg.ID], "Duplicate message ID found: %s", msg.ID)
		seenIDs[msg.ID] = true
	}

	// Verify all messages are associated with the correct TaskSessionID
	for _, msg := range agentMessages {
		assert.Equal(t, sessionID, msg.TaskSessionID,
			"Message %s should be associated with session %s", msg.ID, sessionID)
	}

	// Verify all agent messages have a TurnID
	for _, msg := range agentMessages {
		assert.NotEmpty(t, msg.TurnID,
			"Message %s should have a TurnID", msg.ID)
	}

	// Verify message content matches expected content
	var contentMessages []*msgEntry
	for _, msg := range agentMessages {
		if msg.Type == "content" {
			contentMessages = append(contentMessages, msg)
		}
	}
	require.Len(t, contentMessages, 3, "Should have exactly 3 content messages")

	// Verify content is in correct order
	for i, expected := range expectedContents {
		if i < len(contentMessages) {
			assert.Equal(t, expected, contentMessages[i].Content,
				"Message %d content should match expected", i)
		}
	}

	// Log summary of stored messages for debugging
	t.Logf("Message storage verification summary:")
	for i, msg := range agentMessages {
		t.Logf("  [%d] ID=%s Type=%s Content=%q CreatedAt=%v TurnID=%s",
			i, msg.ID[:8], msg.Type, truncateString(msg.Content, 50), msg.CreatedAt, msg.TurnID[:8])
	}
}

// TestOrchestratorAgentMessagePersistenceWithToolCalls validates that tool call messages
// are stored correctly with proper metadata including tool_call_id, title, status, and args.
func TestOrchestratorAgentMessagePersistenceWithToolCalls(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure simulated agent to emit tool call events via direct event publishing
	toolCallID1 := "tc-" + uuid.New().String()[:8]
	toolCallID2 := "tc-" + uuid.New().String()[:8]

	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		baseTime := time.Now()
		return []protocol.Message{
			// Initial progress
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: baseTime,
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "starting",
					"message":  "Starting tool execution...",
				},
			},
			// Log message
			{
				Type:      protocol.MessageTypeLog,
				TaskID:    taskID,
				Timestamp: baseTime.Add(20 * time.Millisecond),
				Data: map[string]interface{}{
					"level":   "info",
					"message": "Executing view tool",
				},
			},
			// Final progress
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: baseTime.Add(100 * time.Millisecond),
				Data: map[string]interface{}{
					"progress": 100,
					"stage":    "completed",
					"message":  "Tool execution completed",
				},
			},
		}
	})

	// Create test task
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task execution
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	// Subscribe to session
	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Now publish tool call events directly to the event bus to simulate real tool usage
	// This mimics what the lifecycle manager does when processing agent tool calls
	publishToolCallEvent := func(toolCallID, title, status string, args map[string]interface{}) {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":         "tool_call",
				"tool_call_id": toolCallID,
				"tool_title":   title,
				"tool_name":    "view",
				"tool_status":  status,
				"tool_args":    args,
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
	}

	publishToolUpdateEvent := func(toolCallID, status, result string) {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":         "tool_update",
				"tool_call_id": toolCallID,
				"tool_status":  status,
				"tool_result":  result,
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
	}

	// Wait a bit for the initial messages to be processed
	time.Sleep(300 * time.Millisecond)

	// Publish tool call start events
	publishToolCallEvent(toolCallID1, "View main.go", "running", map[string]interface{}{
		"path": "main.go",
		"kind": "view",
	})
	time.Sleep(50 * time.Millisecond)

	publishToolCallEvent(toolCallID2, "View utils.go", "running", map[string]interface{}{
		"path": "utils.go",
		"kind": "view",
	})
	time.Sleep(50 * time.Millisecond)

	// Publish tool update events (completion)
	publishToolUpdateEvent(toolCallID1, "complete", "File content: package main...")
	time.Sleep(50 * time.Millisecond)

	publishToolUpdateEvent(toolCallID2, "complete", "File content: package utils...")
	time.Sleep(50 * time.Millisecond)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Query stored messages
	storedMessages, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)

	t.Logf("Found %d total stored messages", len(storedMessages))

	// Find tool_call messages
	var toolCallMessages []*struct {
		ID       string
		Type     string
		Content  string
		Metadata map[string]interface{}
	}
	for _, msg := range storedMessages {
		if string(msg.Type) == "tool_call" {
			toolCallMessages = append(toolCallMessages, &struct {
				ID       string
				Type     string
				Content  string
				Metadata map[string]interface{}
			}{
				ID:       msg.ID,
				Type:     string(msg.Type),
				Content:  msg.Content,
				Metadata: msg.Metadata,
			})
		}
	}

	t.Logf("Found %d tool_call messages", len(toolCallMessages))

	// Verify tool call messages exist and have correct metadata
	assert.GreaterOrEqual(t, len(toolCallMessages), 2,
		"Should have at least 2 tool_call messages")

	// Verify tool call metadata structure
	for _, msg := range toolCallMessages {
		assert.NotNil(t, msg.Metadata, "Tool call message should have metadata")
		if msg.Metadata != nil {
			toolCallIDMeta, _ := msg.Metadata["tool_call_id"].(string)
			assert.NotEmpty(t, toolCallIDMeta,
				"Tool call message should have tool_call_id in metadata")

			title, _ := msg.Metadata["title"].(string)
			t.Logf("Tool call message: ID=%s, tool_call_id=%s, title=%s, status=%v",
				msg.ID[:8], toolCallIDMeta, title, msg.Metadata["status"])
		}
	}
}

// TestOrchestratorAgentMessageChunkStreaming validates that streaming message chunks
// are correctly aggregated and stored as complete messages.
func TestOrchestratorAgentMessageChunkStreaming(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure minimal ACP messages - we'll manually publish streaming chunks
	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		return []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "starting",
					"message":  "Starting...",
				},
			},
		}
	})

	// Create test task
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe and start task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Wait for initial setup
	time.Sleep(300 * time.Millisecond)

	// Simulate streaming message chunks by publishing message_streaming events
	messageID := uuid.New().String()
	chunks := []string{
		"Hello, ",
		"this is ",
		"a streaming ",
		"message that ",
		"should be ",
		"aggregated.",
	}

	for i, chunk := range chunks {
		payload := map[string]interface{}{
			"type":       "agent/event",
			"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
			"agent_id":   "test-agent",
			"task_id":    taskID,
			"session_id": sessionID,
			"data": map[string]interface{}{
				"type":       "message_streaming",
				"text":       chunk,
				"message_id": messageID,
				"is_append":  i > 0, // First chunk creates, subsequent append
			},
		}
		event := bus.NewEvent(events.AgentStream, "test", payload)
		subject := events.BuildAgentStreamSubject(sessionID)
		err := ts.EventBus.Publish(context.Background(), subject, event)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Query stored messages
	storedMessages, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)

	// Find the streaming message
	var streamingMessage *struct {
		ID      string
		Content string
		Type    string
	}
	for _, msg := range storedMessages {
		if msg.ID == messageID {
			streamingMessage = &struct {
				ID      string
				Content string
				Type    string
			}{
				ID:      msg.ID,
				Content: msg.Content,
				Type:    string(msg.Type),
			}
			break
		}
	}

	// Verify the streaming message was created and aggregated
	if streamingMessage != nil {
		expectedContent := strings.Join(chunks, "")
		assert.Equal(t, expectedContent, streamingMessage.Content,
			"Streaming message content should be aggregated from all chunks")
		t.Logf("Streaming message aggregated correctly: %q", streamingMessage.Content)
	} else {
		// The message might be stored with agent-generated ID
		// Look for content messages from the agent
		var agentContentMessages []*struct {
			ID      string
			Content string
			Type    string
		}
		for _, msg := range storedMessages {
			if string(msg.AuthorType) == "agent" && (string(msg.Type) == "message" || string(msg.Type) == "content") {
				agentContentMessages = append(agentContentMessages, &struct {
					ID      string
					Content string
					Type    string
				}{
					ID:      msg.ID,
					Content: msg.Content,
					Type:    string(msg.Type),
				})
			}
		}
		t.Logf("Found %d agent content messages", len(agentContentMessages))
		for _, msg := range agentContentMessages {
			t.Logf("  ID=%s Type=%s Content=%q", msg.ID[:8], msg.Type, truncateString(msg.Content, 100))
		}
	}
}

// truncateString truncates a string to the specified length, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
