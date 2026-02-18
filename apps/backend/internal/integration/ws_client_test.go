// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// OrchestratorWSClient is a WebSocket client for orchestrator tests
type OrchestratorWSClient struct {
	conn          *websocket.Conn
	t             *testing.T
	notifications chan *ws.Message
	done          chan struct{}
	// pending tracks in-flight requests: request ID -> response channel
	pending map[string]chan *ws.Message
	// send is the channel for outgoing messages (serialized through writePump)
	send chan []byte
	mu   sync.Mutex
}

// NewOrchestratorWSClient creates a WebSocket connection to the test server
func NewOrchestratorWSClient(t *testing.T, serverURL string) *OrchestratorWSClient {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	client := &OrchestratorWSClient{
		conn:          conn,
		t:             t,
		notifications: make(chan *ws.Message, 100),
		done:          make(chan struct{}),
		pending:       make(map[string]chan *ws.Message),
		send:          make(chan []byte, 256),
	}

	go client.readPump()
	go client.writePump()

	return client
}

func createOrchestratorWorkspace(t *testing.T, client *OrchestratorWSClient) string {
	t.Helper()

	resp, err := client.SendRequest("workspace-1", ws.ActionWorkspaceCreate, map[string]interface{}{
		"name": "Test Workspace",
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	return payload["id"].(string)
}

// readPump reads messages from the WebSocket connection
func (c *OrchestratorWSClient) readPump() {
	defer close(c.done)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg ws.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.Type == ws.MessageTypeNotification {
			select {
			case c.notifications <- &msg:
			default:
			}
		} else {
			// Route response to the pending request by ID
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			c.mu.Unlock()
			if ok {
				select {
				case ch <- &msg:
				default:
				}
			}
		}
	}
}

// writePump serializes all writes to the WebSocket connection
func (c *OrchestratorWSClient) writePump() {
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// Close closes the WebSocket connection
func (c *OrchestratorWSClient) Close() {
	close(c.send)
	if err := c.conn.Close(); err != nil {
		c.t.Logf("failed to close websocket: %v", err)
	}
	<-c.done
}

// SendRequest sends a request and waits for a response
func (c *OrchestratorWSClient) SendRequest(id, action string, payload interface{}) (*ws.Message, error) {
	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// Create a response channel for this request
	respCh := make(chan *ws.Message, 1)

	// Register the pending request BEFORE sending (so we don't miss the response)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	// Ensure we clean up when done
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send the request through the write pump (serialized)
	select {
	case c.send <- data:
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("send buffer full")
	}

	// Wait for response on our dedicated channel
	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, context.DeadlineExceeded
	}
}

// WaitForNotification waits for a notification with the given action prefix
func (c *OrchestratorWSClient) WaitForNotification(actionPrefix string, timeout time.Duration) (*ws.Message, error) {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-c.notifications:
			if strings.HasPrefix(msg.Action, actionPrefix) {
				return msg, nil
			}
		case <-deadline:
			return nil, context.DeadlineExceeded
		}
	}
}

// CollectNotifications collects all notifications for a duration
func (c *OrchestratorWSClient) CollectNotifications(duration time.Duration) []*ws.Message {
	var msgs []*ws.Message
	deadline := time.After(duration)
	for {
		select {
		case msg := <-c.notifications:
			msgs = append(msgs, msg)
		case <-deadline:
			return msgs
		}
	}
}
