// Package integration provides end-to-end integration tests for the Kandev backend.
// This file tests that agent thinking/reasoning events are properly saved to the database.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// TestThinkingEventsPersistToDatabase verifies that agent thinking/reasoning events
// are properly saved to the database through the full orchestrator pipeline.
//
// This tests the flow:
// 1. Lifecycle manager receives "reasoning" event from agentctl
// 2. Manager buffers content and publishes "thinking_streaming" event
// 3. Orchestrator handles event and calls messageCreator.CreateThinkingMessageStreaming
// 4. Task service creates message with type="thinking" in database
func TestThinkingEventsPersistToDatabase(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Create a task with session
	taskID := ts.CreateTestTask(t, "test-agent", 1)

	// Create a session for the task using the repository directly
	sessionID := uuid.New().String()
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "test-agent",
		State:          models.TaskSessionStateRunning,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := ts.TaskRepo.CreateTaskSession(ctx, session)
	require.NoError(t, err, "failed to create session")

	// Generate IDs for the test
	thinkingID := uuid.New().String()
	thinkingContent := "The user wants to list the files in the current directory.\nI'll use the view tool with type \"directory\" to show the contents of the workspace."

	// Simulate a thinking_streaming event being published
	// This is what the lifecycle manager would publish after receiving a "reasoning" event
	eventData := &lifecycle.AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:        "thinking_streaming",
			Text:        thinkingContent,
			MessageID:   thinkingID,
			IsAppend:    false, // First chunk - creates new message
			MessageType: "thinking",
		},
	}

	// Publish the event to the event bus (simulating what lifecycle manager does)
	subject := events.BuildAgentStreamSubject(taskID)
	event := bus.NewEvent(events.AgentStream, "test", eventData)
	err = ts.EventBus.Publish(ctx, subject, event)
	require.NoError(t, err, "failed to publish thinking event")

	// Wait for event to be processed
	time.Sleep(200 * time.Millisecond)

	// Check that the thinking message was saved to the database
	messages, err := ts.TaskSvc.ListMessages(ctx, sessionID)
	require.NoError(t, err, "failed to get messages")

	// Find the thinking message
	var foundThinking bool
	for _, msg := range messages {
		t.Logf("Message: ID=%s, Type=%s, AuthorType=%s", msg.ID, msg.Type, msg.AuthorType)
		if msg.Type == "thinking" {
			foundThinking = true
			assert.Equal(t, thinkingID, msg.ID, "thinking message ID should match")
			assert.Equal(t, models.MessageAuthorAgent, msg.AuthorType, "thinking message author should be agent")
			// Check metadata contains thinking content
			thinking, hasThinking := msg.Metadata["thinking"]
			assert.True(t, hasThinking, "thinking message should have 'thinking' in metadata")
			assert.Equal(t, thinkingContent, thinking, "thinking content should match")
			break
		}
	}

	assert.True(t, foundThinking, "should find a thinking message in the database")
}

// TestThinkingEventAppend verifies that appending to thinking messages works correctly.
func TestThinkingEventAppend(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Create a task with session
	taskID := ts.CreateTestTask(t, "test-agent", 1)

	// Create a session for the task using the repository directly
	sessionID := uuid.New().String()
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "test-agent",
		State:          models.TaskSessionStateRunning,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := ts.TaskRepo.CreateTaskSession(ctx, session)
	require.NoError(t, err)

	thinkingID := uuid.New().String()
	chunk1 := "First chunk of thinking content.\n"
	chunk2 := "Second chunk with more reasoning."

	// First chunk - creates the message
	event1 := &lifecycle.AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:        "thinking_streaming",
			Text:        chunk1,
			MessageID:   thinkingID,
			IsAppend:    false,
			MessageType: "thinking",
		},
	}

	subject := events.BuildAgentStreamSubject(taskID)
	err = ts.EventBus.Publish(ctx, subject, bus.NewEvent(events.AgentStream, "test", event1))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Second chunk - appends to existing message
	event2 := &lifecycle.AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:        "thinking_streaming",
			Text:        chunk2,
			MessageID:   thinkingID,
			IsAppend:    true,
			MessageType: "thinking",
		},
	}

	err = ts.EventBus.Publish(ctx, subject, bus.NewEvent(events.AgentStream, "test", event2))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Verify the message contains both chunks
	messages, err := ts.TaskSvc.ListMessages(ctx, sessionID)
	require.NoError(t, err)

	var foundThinking bool
	for _, msg := range messages {
		if msg.Type == "thinking" && msg.ID == thinkingID {
			foundThinking = true
			thinking := msg.Metadata["thinking"].(string)
			assert.Contains(t, thinking, chunk1, "should contain first chunk")
			assert.Contains(t, thinking, chunk2, "should contain second chunk")
			break
		}
	}

	assert.True(t, foundThinking, "should find thinking message with both chunks")
}

