package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// WorkspaceStreamCallbacks defines callbacks for workspace stream events
type WorkspaceStreamCallbacks struct {
	OnShellOutput   func(data string)
	OnShellExit     func(code int)
	OnGitStatus     func(update *GitStatusUpdate)
	OnGitCommit     func(notification *GitCommitNotification)
	OnGitReset      func(notification *GitResetNotification)
	OnFileChange    func(notification *FileChangeNotification)
	OnProcessOutput func(output *types.ProcessOutput)
	OnProcessStatus func(status *types.ProcessStatusUpdate)
	OnConnected     func()
	OnError         func(err string)
}

// WorkspaceStream represents an active workspace stream connection
type WorkspaceStream struct {
	conn      *websocket.Conn
	inputCh   chan types.WorkspaceStreamMessage
	closeCh   chan struct{}
	closeOnce sync.Once
	logger    *logger.Logger
}

// StreamWorkspace opens a unified WebSocket connection for all workspace events
func (c *Client) StreamWorkspace(ctx context.Context, callbacks WorkspaceStreamCallbacks) (*WorkspaceStream, error) {
	c.mu.Lock()
	if c.workspaceStreamConn != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("workspace stream already connected")
	}
	c.mu.Unlock()

	wsURL := "ws" + c.baseURL[4:] + "/api/v1/workspace/stream"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace stream: %w", err)
	}

	c.mu.Lock()
	c.workspaceStreamConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to workspace stream", zap.String("url", wsURL))

	stream := &WorkspaceStream{
		conn:    conn,
		inputCh: make(chan types.WorkspaceStreamMessage, 64),
		closeCh: make(chan struct{}),
		logger:  c.logger,
	}

	go c.readWorkspaceStream(conn, stream, callbacks)
	go stream.writeLoop(conn)

	return stream, nil
}

// workspaceTracedTypes contains message types that are low-volume and worth tracing.
// High-volume types (shell_output, process_output, shell_input, ping, pong) are excluded.
var workspaceTracedTypes = map[types.WorkspaceMessageType]bool{
	types.WorkspaceMessageTypeGitStatus:     true,
	types.WorkspaceMessageTypeGitCommit:     true,
	types.WorkspaceMessageTypeGitReset:      true,
	types.WorkspaceMessageTypeFileChange:    true,
	types.WorkspaceMessageTypeProcessStatus: true,
	types.WorkspaceMessageTypeConnected:     true,
	types.WorkspaceMessageTypeError:         true,
}

// readWorkspaceStream is the read loop for the workspace WebSocket stream.
func (c *Client) readWorkspaceStream(conn *websocket.Conn, stream *WorkspaceStream, callbacks WorkspaceStreamCallbacks) {
	defer func() {
		c.mu.Lock()
		c.workspaceStreamConn = nil
		c.mu.Unlock()
		stream.Close()
	}()

	for {
		var msg types.WorkspaceStreamMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Debug("workspace stream read error", zap.Error(err))
			}
			return
		}
		if workspaceTracedTypes[msg.Type] {
			tracing.TraceWorkspaceEvent(c.getTraceCtx(), string(msg.Type), c.executionID, c.sessionID)
		}
		dispatchWorkspaceMessage(msg, callbacks)
	}
}

// writeLoop reads from the inputCh and writes messages to the WebSocket connection.
func (ws *WorkspaceStream) writeLoop(conn *websocket.Conn) {
	for {
		select {
		case <-ws.closeCh:
			return
		case msg, ok := <-ws.inputCh:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				ws.logger.Debug("workspace stream write error", zap.Error(err))
				return
			}
		}
	}
}

// WriteShellInput sends input to the shell through the workspace stream
func (ws *WorkspaceStream) WriteShellInput(data string) error {
	msg := types.NewWorkspaceShellInput(data)
	select {
	case ws.inputCh <- msg:
		return nil
	case <-ws.closeCh:
		return fmt.Errorf("workspace stream closed")
	}
}

// ResizeShell sends a shell resize command through the workspace stream
func (ws *WorkspaceStream) ResizeShell(cols, rows int) error {
	msg := types.NewWorkspaceShellResize(cols, rows)
	select {
	case ws.inputCh <- msg:
		return nil
	case <-ws.closeCh:
		return fmt.Errorf("workspace stream closed")
	}
}

// Ping sends a ping message through the workspace stream
func (ws *WorkspaceStream) Ping() error {
	msg := types.NewWorkspacePing()
	select {
	case ws.inputCh <- msg:
		return nil
	case <-ws.closeCh:
		return fmt.Errorf("workspace stream closed")
	}
}

// Close closes the workspace stream
func (ws *WorkspaceStream) Close() {
	ws.closeOnce.Do(func() {
		close(ws.closeCh)
		if ws.conn != nil {
			if err := ws.conn.Close(); err != nil {
				ws.logger.Debug("failed to close workspace stream connection", zap.Error(err))
			}
		}
	})
}

// Done returns a channel that is closed when the stream is closed
func (ws *WorkspaceStream) Done() <-chan struct{} {
	return ws.closeCh
}

// CloseWorkspaceStream closes the workspace stream connection
func (c *Client) CloseWorkspaceStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.workspaceStreamConn != nil {
		if err := c.workspaceStreamConn.Close(); err != nil {
			c.logger.Debug("failed to close workspace stream", zap.Error(err))
		}
		c.workspaceStreamConn = nil
	}
}
