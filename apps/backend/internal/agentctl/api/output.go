package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/process"
	"go.uber.org/zap"
)

// OutputResponse is the response for getting output
type OutputResponse struct {
	Lines []process.OutputLine `json:"lines"`
	Total int                  `json:"total"`
}

func (s *Server) handleGetOutput(c *gin.Context) {
	// Parse optional 'last' query param
	lastStr := c.DefaultQuery("last", "100")
	last, err := strconv.Atoi(lastStr)
	if err != nil || last < 1 {
		last = 100
	}

	lines := s.procMgr.GetOutputBuffer().GetLast(last)
	c.JSON(http.StatusOK, OutputResponse{
		Lines: lines,
		Total: s.procMgr.GetOutputBuffer().Count(),
	})
}

// handleOutputStreamWS streams output via WebSocket
func (s *Server) handleOutputStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	s.logger.Info("Output stream WebSocket connected")

	// Check if client wants history first
	sendHistory := c.DefaultQuery("history", "false") == "true"
	historyCount, _ := strconv.Atoi(c.DefaultQuery("history_count", "100"))

	if sendHistory {
		lines := s.procMgr.GetOutputBuffer().GetLast(historyCount)
		for _, line := range lines {
			data, err := json.Marshal(line)
			if err != nil {
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				s.logger.Debug("WebSocket write error (history)", zap.Error(err))
				return
			}
		}
	}

	// Subscribe to real-time output
	sub := s.procMgr.GetOutputBuffer().Subscribe()
	defer s.procMgr.GetOutputBuffer().Unsubscribe(sub)

	// Handle WebSocket close
	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	// Stream output to client
	for {
		select {
		case line, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(line)
			if err != nil {
				s.logger.Error("failed to marshal output line", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				s.logger.Debug("WebSocket write error", zap.Error(err))
				return
			}
		case <-closeCh:
			s.logger.Info("Output stream WebSocket closed by client")
			return
		}
	}
}

