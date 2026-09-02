package websocket

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/lsp/protocol"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	lspCloseStreamError    = 4006
	lspProxyWriteTimeout   = 10 * time.Second
	lspProxyIdleTimeout    = 90 * time.Second
	lspProxyPingInterval   = 30 * time.Second
	lspErrorResponseKey    = "error"
	lspUnavailableText     = "task language server attachment unavailable"
	lspHostUnavailableText = "task-host LSP attachment unavailable"
)

type lspMessageWriter interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
}

type LSPAttachmentResolver interface {
	ResolveAttachment(ctx context.Context, taskID, language string) (*sharedlsp.AttachmentTarget, error)
}

// LSPHandler is a thin, non-owning proxy. The task controller authorizes and
// resolves the current task/language generation; browser disconnect only
// closes its downstream attachment.
type LSPHandler struct {
	controller LSPAttachmentResolver
	logger     *logger.Logger
}

var lspUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     checkWebSocketOrigin,
}

func NewLSPHandler(controller LSPAttachmentResolver, log *logger.Logger) *LSPHandler {
	return &LSPHandler{
		controller: controller,
		logger:     log.WithFields(zap.String("component", "lsp_attachment_handler")),
	}
}

// HandleLSPConnection serves /lsp/tasks/:taskId/:language/attach.
func (h *LSPHandler) HandleLSPConnection(c *gin.Context) {
	taskID := c.Param("taskId")
	language := c.Param("language")
	target, err := h.controller.ResolveAttachment(c.Request.Context(), taskID, language)
	if err != nil {
		status := lspAttachmentHTTPStatus(err)
		message := lspUnavailableText
		if status == http.StatusBadRequest || status == http.StatusConflict ||
			status == http.StatusUnprocessableEntity {
			message = err.Error()
		}
		c.JSON(status, gin.H{lspErrorResponseKey: message})
		return
	}

	upstreamConn, response, err := target.Host.DialTaskLSPAttach(
		c.Request.Context(), target.Language, target.Generation,
	)
	if err != nil {
		status := http.StatusBadGateway
		if response != nil && response.StatusCode >= http.StatusBadRequest {
			status = response.StatusCode
		}
		c.JSON(status, gin.H{lspErrorResponseKey: lspHostUnavailableText})
		return
	}
	upstreamConn.SetReadLimit(protocol.MaxMessageBytes)

	browserConn, err := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = upstreamConn.Close()
		return
	}
	browserConn.SetReadLimit(protocol.MaxMessageBytes)
	h.proxyLSPConnections(browserConn, upstreamConn, taskID, language)
}

func lspAttachmentHTTPStatus(err error) int {
	switch {
	case errors.Is(err, sharedlsp.ErrUnsupportedLanguage):
		return http.StatusBadRequest
	case errors.Is(err, sharedlsp.ErrAttachmentNotReady), errors.Is(err, sharedlsp.ErrServerDisabled):
		return http.StatusConflict
	case errors.Is(err, sharedlsp.ErrTaskNotReady), errors.Is(err, sharedlsp.ErrExecutorUnsupported):
		return http.StatusUnprocessableEntity
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func (h *LSPHandler) proxyLSPConnections(
	browserConn, upstreamConn *gorillaws.Conn,
	taskID, language string,
) {
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = browserConn.Close()
			_ = upstreamConn.Close()
		})
	}
	if err := configureLSPReadKeepalive(browserConn); err != nil {
		closeBoth()
		return
	}
	if err := configureLSPReadKeepalive(upstreamConn); err != nil {
		closeBoth()
		return
	}
	stopPings := make(chan struct{})
	pingsDone := make(chan struct{})
	go func() {
		defer close(pingsDone)
		ticker := time.NewTicker(lspProxyPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deadline := time.Now().Add(lspProxyWriteTimeout)
				if err := browserConn.WriteControl(gorillaws.PingMessage, nil, deadline); err != nil {
					closeBoth()
					return
				}
				if err := upstreamConn.WriteControl(gorillaws.PingMessage, nil, deadline); err != nil {
					closeBoth()
					return
				}
			case <-stopPings:
				return
			}
		}
	}()
	done := make(chan struct{}, 2)
	go func() {
		h.copyLSPMessages("taskhost->browser", upstreamConn, browserConn, taskID, language)
		done <- struct{}{}
	}()
	go func() {
		h.copyLSPMessages("browser->taskhost", browserConn, upstreamConn, taskID, language)
		done <- struct{}{}
	}()
	<-done
	close(stopPings)
	closeBoth()
	<-done
	<-pingsDone
	h.logger.Info("LSP attachment closed", zap.String("task_id", taskID), zap.String("language", language))
}

func configureLSPReadKeepalive(conn *gorillaws.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(lspProxyIdleTimeout)); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(lspProxyIdleTimeout))
	})
	return nil
}

func (h *LSPHandler) copyLSPMessages(
	direction string,
	src *gorillaws.Conn,
	dst lspMessageWriter,
	taskID, language string,
) {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			h.forwardLSPClose(dst, err)
			if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				h.logger.Debug("LSP attachment read error",
					zap.String("direction", direction), zap.String("task_id", taskID),
					zap.String("language", language), zap.Error(err))
			}
			return
		}
		if err := writeLSPProxyMessage(dst, messageType, message); err != nil {
			h.logger.Debug("LSP attachment write error",
				zap.String("direction", direction), zap.String("task_id", taskID),
				zap.String("language", language), zap.Error(err))
			return
		}
	}
}

func (h *LSPHandler) forwardLSPClose(dst lspMessageWriter, err error) {
	if closeErr, ok := err.(*gorillaws.CloseError); ok {
		_ = writeLSPProxyMessage(dst, gorillaws.CloseMessage, gorillaws.FormatCloseMessage(closeErr.Code, closeErr.Text))
		return
	}
	_ = writeLSPProxyMessage(
		dst, gorillaws.CloseMessage,
		gorillaws.FormatCloseMessage(lspCloseStreamError, "LSP attachment closed"),
	)
}

func writeLSPProxyMessage(dst lspMessageWriter, messageType int, payload []byte) error {
	if err := dst.SetWriteDeadline(time.Now().Add(lspProxyWriteTimeout)); err != nil {
		return err
	}
	return dst.WriteMessage(messageType, payload)
}
