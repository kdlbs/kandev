package websocket

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/lsp/protocol"
)

const (
	lspCloseStreamError  = 4006
	lspProxyWriteTimeout = 10 * time.Second
	lspErrorResponseKey  = "error"
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
		message := "task language server attachment unavailable"
		if status == http.StatusBadRequest || status == http.StatusConflict {
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
		c.JSON(status, gin.H{lspErrorResponseKey: "task-host LSP attachment unavailable"})
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
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
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
	closeBoth()
	<-done
	h.logger.Info("LSP attachment closed", zap.String("task_id", taskID), zap.String("language", language))
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
