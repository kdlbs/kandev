package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) handlePassthroughStream(c *gin.Context) {
	target, err := passthroughTarget(c)
	if err != nil {
		passthroughError(c, err)
		return
	}
	buffer, output, release, err := s.procMgr.Passthrough().SubscribeTerminal(target)
	if err != nil {
		passthroughError(c, err)
		return
	}
	defer release()
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	releaseCredentials := closeOnCredentialInvalidation(c, conn)
	defer releaseCredentials()
	conn.SetReadLimit(1024 * 1024)
	if len(buffer) > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, buffer); err != nil {
			return
		}
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		s.readPassthroughInput(ctx, conn)
	}()
	defer func() { _ = conn.Close(); <-done }()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-output:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}
}

func (s *Server) readPassthroughInput(ctx context.Context, conn *websocket.Conn) {
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if len(data) == 0 || (kind != websocket.BinaryMessage && kind != websocket.TextMessage) {
			continue
		}
		// The authenticated session stream follows controller-owned replacements.
		target := s.procMgr.Passthrough().Snapshot().PassthroughTarget
		if len(data) > 1 && data[0] == shellResizeByte && data[1] == '{' {
			request := types.PassthroughResizeRequest{PassthroughTarget: target}
			if err := json.Unmarshal(data[1:], &request); err != nil {
				return
			}
			// Stream identity is immutable, even if the resize payload supplies fields.
			request.PassthroughTarget = target
			if err := s.procMgr.Passthrough().Resize(request); err != nil {
				return
			}
			continue
		}
		result, err := s.procMgr.Passthrough().Input(ctx, types.PassthroughInputRequest{PassthroughTarget: target, Data: string(data)})
		if err != nil || result.Outcome != "written" {
			return
		}
	}
}

func (s *Server) streamPassthroughEvents(c *gin.Context, sequence uint64) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	releaseCredentials := closeOnCredentialInvalidation(c, conn)
	defer releaseCredentials()
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	done := make(chan struct{})
	conn.SetReadLimit(128)
	go func() {
		defer close(done)
		defer cancel()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	defer func() { _ = conn.Close(); <-done }()
	for {
		page, err := s.procMgr.Passthrough().Events(ctx, sequence, true)
		if err != nil {
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteJSON(page); err != nil {
			return
		}
		sequence = page.Sequence
	}
}
