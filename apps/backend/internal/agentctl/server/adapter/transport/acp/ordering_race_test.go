package acp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeACPSessionUpdateNotification creates a valid ACP session/update JSON-RPC notification
// with an agent_message_chunk update containing the given text.
func makeACPSessionUpdateNotification(sessionID, text string) []byte {
	// ACP uses discriminator-based unmarshaling:
	// - SessionUpdate.sessionUpdate = "agent_message_chunk"
	// - ContentBlock.type = "text"
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": text,
				},
			},
		},
	}
	data, _ := json.Marshal(notification)
	return append(data, '\n')
}

// TestACPMessageChunkOrdering demonstrates the race condition in ACP SDK
// where message chunks can be processed out of order due to goroutine scheduling.
//
// The ACP SDK's connection.go spawns a new goroutine for each incoming notification:
//
//	case msg.Method != "":
//	    go c.handleInbound(&msg)  // Each notification in separate goroutine!
//
// This test sends multiple message chunks in quick succession and verifies
// whether they arrive in the expected order.
func TestACPMessageChunkOrdering(t *testing.T) {
	const numChunks = 100 // Send enough chunks to trigger race condition

	// Create pipes for stdin/stdout simulation
	agentStdinReader, agentStdinWriter := io.Pipe()
	agentStdoutReader, agentStdoutWriter := io.Pipe()

	// Track received chunks with ordering
	var mu sync.Mutex
	var receivedChunks []string

	// Create ACP client with update handler that records chunk order
	client := acpclient.NewClient(
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				mu.Lock()
				receivedChunks = append(receivedChunks, n.Update.AgentMessageChunk.Content.Text.Text)
				mu.Unlock()
			}
		}),
	)

	// Create ACP connection (this starts the receive goroutine)
	conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

	// Simulate agent sending message chunks rapidly
	go func() {
		for i := 0; i < numChunks; i++ {
			data := makeACPSessionUpdateNotification("test-session", fmt.Sprintf("chunk_%03d", i))
			_, _ = agentStdoutWriter.Write(data)
		}
		// Give time for processing then close
		time.Sleep(100 * time.Millisecond)
		_ = agentStdoutWriter.Close()
	}()

	// Wait for connection to close
	<-conn.Done()

	// Allow some buffer time for any remaining processing
	time.Sleep(50 * time.Millisecond)

	// Check results
	mu.Lock()
	defer mu.Unlock()

	if len(receivedChunks) != numChunks {
		t.Errorf("Expected %d chunks, got %d", numChunks, len(receivedChunks))
	}

	// Check if chunks arrived in order
	outOfOrder := 0
	for i, chunk := range receivedChunks {
		expected := fmt.Sprintf("chunk_%03d", i)
		if chunk != expected {
			outOfOrder++
			if outOfOrder <= 5 { // Log first 5 out-of-order
				t.Logf("Out of order at position %d: expected %q, got %q", i, expected, chunk)
			}
		}
	}

	if outOfOrder > 0 {
		t.Errorf("RACE CONDITION DETECTED: %d/%d chunks arrived out of order!", outOfOrder, numChunks)
		t.Log("This confirms the ACP SDK processes notifications in parallel goroutines")
	} else {
		t.Log("All chunks arrived in order (race condition not triggered this run)")
		t.Log("Note: Race conditions are non-deterministic - try running with -count=10")
	}

	// Clean up
	_ = agentStdinReader.Close()
	_ = agentStdinWriter.Close()
}

// TestACPUpdateHandlerOrdering tests if adding serialization in our handler fixes ordering.
// Spoiler: It doesn't fully fix it because the ordering is already lost when goroutines
// are spawned in the ACP SDK before our handler is even called.
func TestACPUpdateHandlerOrdering(t *testing.T) {
	const numChunks = 100

	agentStdoutReader, agentStdoutWriter := io.Pipe()
	agentStdinReader, agentStdinWriter := io.Pipe()

	var mu sync.Mutex
	var receivedChunks []string

	// Add a serialization mutex to test the fix
	var handlerMu sync.Mutex

	client := acpclient.NewClient(
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			// PROPOSED FIX: Serialize processing with a mutex
			handlerMu.Lock()
			defer handlerMu.Unlock()

			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				mu.Lock()
				receivedChunks = append(receivedChunks, n.Update.AgentMessageChunk.Content.Text.Text)
				mu.Unlock()
			}
		}),
	)

	conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

	go func() {
		for i := 0; i < numChunks; i++ {
			data := makeACPSessionUpdateNotification("test-session", fmt.Sprintf("chunk_%03d", i))
			_, _ = agentStdoutWriter.Write(data)
		}
		time.Sleep(100 * time.Millisecond)
		_ = agentStdoutWriter.Close()
	}()

	<-conn.Done()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	outOfOrder := 0
	for i, chunk := range receivedChunks {
		expected := fmt.Sprintf("chunk_%03d", i)
		if chunk != expected {
			outOfOrder++
		}
	}

	// With the mutex in the handler, we might STILL see out-of-order
	// because the mutex is acquired AFTER the goroutine is spawned
	t.Logf("With handler mutex: %d/%d chunks out of order", outOfOrder, len(receivedChunks))
	if outOfOrder > 0 {
		t.Log("Handler mutex doesn't fully fix the issue - ordering lost before handler is called")
	}

	// Clean up
	_ = agentStdinReader.Close()
	_ = agentStdinWriter.Close()
}

// Helper to verify AdapterEvent type (unused variable fix)
var _ = streams.EventTypeMessageChunk

// TestNotificationOrderingFix verifies that the ACP SDK fork (github.com/kdlbs/acp-go-sdk)
// correctly preserves notification ordering by processing them synchronously.
//
// This test is the primary verification that the fix works. It should:
// - Pass 100% of the time with the patched SDK
// - Fail frequently (40-80% out of order) with the original SDK
func TestNotificationOrderingFix(t *testing.T) {
	const numChunks = 200 // Use more chunks to increase confidence

	agentStdoutReader, agentStdoutWriter := io.Pipe()
	agentStdinReader, agentStdinWriter := io.Pipe()

	var mu sync.Mutex
	var receivedChunks []string

	client := acpclient.NewClient(
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				mu.Lock()
				receivedChunks = append(receivedChunks, n.Update.AgentMessageChunk.Content.Text.Text)
				mu.Unlock()
			}
		}),
	)

	conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

	// Send chunks as fast as possible to stress test ordering
	go func() {
		for i := 0; i < numChunks; i++ {
			data := makeACPSessionUpdateNotification("test-session", fmt.Sprintf("chunk_%03d", i))
			_, _ = agentStdoutWriter.Write(data)
		}
		time.Sleep(100 * time.Millisecond)
		_ = agentStdoutWriter.Close()
	}()

	<-conn.Done()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, numChunks, len(receivedChunks), "All chunks should be received")

	// Verify strict ordering - this is the key assertion
	for i, chunk := range receivedChunks {
		expected := fmt.Sprintf("chunk_%03d", i)
		assert.Equal(t, expected, chunk, "Chunk at position %d should be in order", i)
	}

	_ = agentStdinReader.Close()
	_ = agentStdinWriter.Close()
}

// TestNotificationOrderingWithMultipleSessions verifies ordering is maintained
// per-session when multiple sessions send interleaved notifications.
func TestNotificationOrderingWithMultipleSessions(t *testing.T) {
	const chunksPerSession = 50
	sessions := []string{"session-a", "session-b", "session-c"}

	agentStdoutReader, agentStdoutWriter := io.Pipe()
	agentStdinReader, agentStdinWriter := io.Pipe()

	var mu sync.Mutex
	receivedBySession := make(map[string][]string)

	client := acpclient.NewClient(
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				mu.Lock()
				sessionID := string(n.SessionId)
				receivedBySession[sessionID] = append(receivedBySession[sessionID], n.Update.AgentMessageChunk.Content.Text.Text)
				mu.Unlock()
			}
		}),
	)

	conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

	// Send interleaved chunks from multiple sessions
	go func() {
		for i := 0; i < chunksPerSession; i++ {
			for _, session := range sessions {
				data := makeACPSessionUpdateNotification(session, fmt.Sprintf("%s_chunk_%03d", session, i))
				_, _ = agentStdoutWriter.Write(data)
			}
		}
		time.Sleep(100 * time.Millisecond)
		_ = agentStdoutWriter.Close()
	}()

	<-conn.Done()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify each session received all chunks in order
	for _, session := range sessions {
		chunks := receivedBySession[session]
		require.Equal(t, chunksPerSession, len(chunks), "Session %s should receive all chunks", session)

		for i, chunk := range chunks {
			expected := fmt.Sprintf("%s_chunk_%03d", session, i)
			assert.Equal(t, expected, chunk, "Session %s chunk at position %d should be in order", session, i)
		}
	}

	_ = agentStdinReader.Close()
	_ = agentStdinWriter.Close()
}

// TestNotificationOrderingWithLargePayloads verifies ordering with varying payload sizes.
// Larger payloads take longer to process, which could expose ordering issues.
func TestNotificationOrderingWithLargePayloads(t *testing.T) {
	const numChunks = 50

	agentStdoutReader, agentStdoutWriter := io.Pipe()
	agentStdinReader, agentStdinWriter := io.Pipe()

	var mu sync.Mutex
	var receivedChunks []string

	client := acpclient.NewClient(
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				mu.Lock()
				// Extract just the chunk ID from the payload
				text := n.Update.AgentMessageChunk.Content.Text.Text
				if idx := strings.Index(text, ":"); idx != -1 {
					receivedChunks = append(receivedChunks, text[:idx])
				}
				mu.Unlock()
			}
		}),
	)

	conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

	// Send chunks with varying payload sizes
	go func() {
		for i := 0; i < numChunks; i++ {
			// Alternate between small and large payloads
			payloadSize := 100
			if i%2 == 0 {
				payloadSize = 10000
			}
			padding := strings.Repeat("x", payloadSize)
			text := fmt.Sprintf("chunk_%03d:%s", i, padding)
			data := makeACPSessionUpdateNotification("test-session", text)
			_, _ = agentStdoutWriter.Write(data)
		}
		time.Sleep(150 * time.Millisecond)
		_ = agentStdoutWriter.Close()
	}()

	<-conn.Done()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, numChunks, len(receivedChunks), "All chunks should be received")

	for i, chunk := range receivedChunks {
		expected := fmt.Sprintf("chunk_%03d", i)
		assert.Equal(t, expected, chunk, "Chunk at position %d should be in order", i)
	}

	_ = agentStdinReader.Close()
	_ = agentStdinWriter.Close()
}

// TestNotificationOrderingStressTest runs multiple iterations to ensure
// the fix is reliable under repeated stress.
func TestNotificationOrderingStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	const iterations = 10
	const chunksPerIteration = 100

	for iter := 0; iter < iterations; iter++ {
		t.Run(fmt.Sprintf("iteration_%d", iter), func(t *testing.T) {
			agentStdoutReader, agentStdoutWriter := io.Pipe()
			agentStdinReader, agentStdinWriter := io.Pipe()

			var mu sync.Mutex
			var receivedChunks []string

			client := acpclient.NewClient(
				acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
					if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
						mu.Lock()
						receivedChunks = append(receivedChunks, n.Update.AgentMessageChunk.Content.Text.Text)
						mu.Unlock()
					}
				}),
			)

			conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

			go func() {
				for i := 0; i < chunksPerIteration; i++ {
					data := makeACPSessionUpdateNotification("test-session", fmt.Sprintf("chunk_%03d", i))
					_, _ = agentStdoutWriter.Write(data)
				}
				time.Sleep(100 * time.Millisecond)
				_ = agentStdoutWriter.Close()
			}()

			<-conn.Done()
			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			defer mu.Unlock()

			require.Equal(t, chunksPerIteration, len(receivedChunks))

			outOfOrder := 0
			for i, chunk := range receivedChunks {
				expected := fmt.Sprintf("chunk_%03d", i)
				if chunk != expected {
					outOfOrder++
				}
			}

			assert.Zero(t, outOfOrder, "No chunks should be out of order")

			_ = agentStdinReader.Close()
			_ = agentStdinWriter.Close()
		})
	}
}

// TestMixedNotificationTypes verifies ordering is preserved across different
// notification types (message chunks, tool calls, etc.)
func TestMixedNotificationTypes(t *testing.T) {
	agentStdoutReader, agentStdoutWriter := io.Pipe()
	agentStdinReader, agentStdinWriter := io.Pipe()

	var mu sync.Mutex
	var receivedEvents []string

	client := acpclient.NewClient(
		acpclient.WithUpdateHandler(func(n acp.SessionNotification) {
			mu.Lock()
			defer mu.Unlock()

			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				receivedEvents = append(receivedEvents, "msg:"+n.Update.AgentMessageChunk.Content.Text.Text)
			}
			if n.Update.ToolCall != nil {
				receivedEvents = append(receivedEvents, "tool:"+n.Update.ToolCall.Title)
			}
		}),
	)

	conn := acp.NewClientSideConnection(client, agentStdinWriter, agentStdoutReader)

	// Send interleaved message chunks and tool calls
	go func() {
		for i := 0; i < 20; i++ {
			// Message chunk
			msgData := makeACPSessionUpdateNotification("test-session", fmt.Sprintf("event_%02d", i*2))
			_, _ = agentStdoutWriter.Write(msgData)

			// Tool call notification
			toolData := makeACPToolCallNotification("test-session", fmt.Sprintf("event_%02d", i*2+1))
			_, _ = agentStdoutWriter.Write(toolData)
		}
		time.Sleep(100 * time.Millisecond)
		_ = agentStdoutWriter.Close()
	}()

	<-conn.Done()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, 40, len(receivedEvents), "Should receive all events")

	// Verify ordering across event types
	for i, event := range receivedEvents {
		var expected string
		if i%2 == 0 {
			expected = fmt.Sprintf("msg:event_%02d", i)
		} else {
			expected = fmt.Sprintf("tool:event_%02d", i)
		}
		assert.Equal(t, expected, event, "Event at position %d should be in order", i)
	}

	_ = agentStdinReader.Close()
	_ = agentStdinWriter.Close()
}

// makeACPToolCallNotification creates a valid ACP session/update JSON-RPC notification
// with a tool_call update.
func makeACPToolCallNotification(sessionID, title string) []byte {
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"title":         title,
				"toolCallId":    fmt.Sprintf("tc_%s", title),
				"status":        "started",
			},
		},
	}
	data, _ := json.Marshal(notification)
	return append(data, '\n')
}
