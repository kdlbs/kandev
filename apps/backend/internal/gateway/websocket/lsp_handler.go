package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/lsp/protocol"
	"github.com/kandev/kandev/internal/user/models"
)

// Custom WebSocket close codes for LSP connections.
const (
	lspCloseBinaryNotFound       = 4001
	lspCloseSessionNotFound      = 4002
	lspCloseInstallFailed        = 4003
	lspCloseUnsupportedExecutor  = 4004
	lspCloseCapacityExceeded     = 4005
	lspCloseStreamError          = 4006
	lspCloseUnsupportedCloseText = "LSP is only supported for local_pc and local_docker tasks in this release"
	lspProxyWriteTimeout         = 10 * time.Second
)

type lspMessageWriter interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
}

// LSPUserService provides user settings for the LSP handler.
type LSPUserService interface {
	GetUserSettings(ctx context.Context) (*models.UserSettings, error)
}

type lspLifecycleManager interface {
	ResolveSessionRuntime(ctx context.Context, sessionID string) (agentruntime.Runtime, error)
	GetOrEnsureExecution(ctx context.Context, sessionID string) (*lifecycle.AgentExecution, error)
}

var (
	errLSPUnsupportedExecutor = errors.New("unsupported LSP executor")
	errLSPCapacityExceeded    = errors.New("active LSP connection cap exceeded")
)

// LSPHandler handles browser-facing LSP WebSocket connections.
// The backend owns session/runtime policy and proxies raw LSP traffic to the
// task host's agentctl instance, where the language server process runs.
type LSPHandler struct {
	lifecycleMgr lspLifecycleManager
	userService  LSPUserService
	capacity     *lspCapacityLimiter
	logger       *logger.Logger
}

var lspUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     checkWebSocketOrigin,
}

// NewLSPHandler creates a new LSPHandler.
func NewLSPHandler(lifecycleMgr *lifecycle.Manager, userService LSPUserService, log *logger.Logger, configuredMax ...int) *LSPHandler {
	capacity := newLSPCapacityLimiterFromEnv()
	if len(configuredMax) > 0 {
		capacity = newLSPCapacityLimiter(configuredMax[0])
	}
	return &LSPHandler{
		lifecycleMgr: lifecycleMgr,
		userService:  userService,
		capacity:     capacity,
		logger:       log.WithFields(zap.String("component", "lsp_handler")),
	}
}

// HandleLSPConnection handles WebSocket connections at /lsp/:sessionId?language=...
func (h *LSPHandler) HandleLSPConnection(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	language := c.Query("language")
	if language == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "language query parameter is required"})
		return
	}
	if !installer.IsSupported(language) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported language: %s", language)})
		return
	}

	h.logger.Info("LSP WebSocket connection request",
		zap.String("session_id", sessionID),
		zap.String("language", language))

	execution, runtimeName, err := h.resolveLSPExecution(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, errLSPCapacityExceeded) {
			h.logger.Info("LSP: capacity exceeded",
				zap.String("session_id", sessionID),
				zap.String("language", language))
			h.closeWithCode(c, lspCloseCapacityExceeded, errLSPCapacityExceeded.Error())
			return
		}
		if errors.Is(err, errLSPUnsupportedExecutor) {
			h.logger.Info("LSP: unsupported runtime",
				zap.String("session_id", sessionID),
				zap.Stringer("runtime", runtimeName))
			h.closeWithCode(c, lspCloseUnsupportedExecutor, lspCloseUnsupportedCloseText)
			return
		}
		h.logger.Warn("LSP: session not found in lifecycle manager",
			zap.String("session_id", sessionID),
			zap.Error(err))
		h.closeWithCode(c, lspCloseSessionNotFound, "session not found")
		return
	}
	defer h.capacity.Release()
	agentctlClient, releaseClient := execution.AcquireAgentCtlClient()
	if agentctlClient == nil {
		releaseClient()
		h.logger.Warn("LSP: execution has no agentctl client", zap.String("session_id", sessionID))
		h.closeWithCode(c, lspCloseSessionNotFound, "agentctl unavailable")
		return
	}
	browserConn, upgradeErr := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
	if upgradeErr != nil {
		releaseClient()
		h.logger.Error("LSP: failed to upgrade to WebSocket",
			zap.String("session_id", sessionID),
			zap.Error(upgradeErr))
		return
	}
	browserConn.SetReadLimit(protocol.MaxMessageBytes)

	autoInstall := h.shouldAutoInstall(c.Request.Context(), language)
	upstreamConn, resp, dialErr := agentctlClient.DialLSP(c.Request.Context(), language, autoInstall)
	releaseClient()
	if dialErr != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		h.logger.Warn("LSP: failed to connect to agentctl LSP stream",
			zap.String("session_id", sessionID),
			zap.String("language", language),
			zap.Int("status", status),
			zap.Error(dialErr))
		closeLSPConnWithCode(browserConn, lspCloseSessionNotFound, "failed to connect to task host LSP stream")
		return
	}
	upstreamConn.SetReadLimit(protocol.MaxMessageBytes)
	defer func() { _ = upstreamConn.Close() }()

	h.proxyLSPConnections(browserConn, upstreamConn, sessionID, language)
}

func (h *LSPHandler) resolveLSPExecution(
	ctx context.Context,
	sessionID string,
) (*lifecycle.AgentExecution, agentruntime.Runtime, error) {
	runtimeName, err := h.lifecycleMgr.ResolveSessionRuntime(ctx, sessionID)
	if err != nil {
		return nil, runtimeName, err
	}
	if !lspRuntimeSupported(runtimeName) {
		return nil, runtimeName, errLSPUnsupportedExecutor
	}
	if !h.capacity.TryAcquire() {
		return nil, runtimeName, errLSPCapacityExceeded
	}
	releaseCapacity := true
	defer func() {
		if releaseCapacity {
			h.capacity.Release()
		}
	}()
	execution, err := h.lifecycleMgr.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		return nil, runtimeName, err
	}
	if execution == nil {
		return nil, runtimeName, errors.New("lifecycle manager returned no execution")
	}
	if !lspRuntimeSupported(execution.RuntimeName) {
		return nil, execution.RuntimeName, errLSPUnsupportedExecutor
	}
	releaseCapacity = false
	return execution, execution.RuntimeName, nil
}

func lspRuntimeSupported(runtimeName agentruntime.Runtime) bool {
	return runtimeName == agentruntime.RuntimeStandalone || runtimeName == agentruntime.RuntimeDocker
}

func (h *LSPHandler) shouldAutoInstall(ctx context.Context, language string) bool {
	if h.userService == nil || !installer.SupportsAutoInstall(language) {
		return false
	}
	settings, err := h.userService.GetUserSettings(ctx)
	if err != nil || settings == nil {
		if err != nil {
			h.logger.Debug("LSP: failed to load user settings for auto-install", zap.Error(err))
		}
		return false
	}
	for _, lang := range settings.LspAutoInstallLanguages {
		if lang == language {
			return true
		}
	}
	return false
}

// closeWithCode upgrades the WebSocket and immediately closes it with the given code.
func (h *LSPHandler) closeWithCode(c *gin.Context, code int, text string) {
	conn, err := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("LSP: failed to upgrade WebSocket for close", zap.Error(err))
		return
	}
	closeLSPConnWithCode(conn, code, text)
}

func (h *LSPHandler) proxyLSPConnections(browserConn, upstreamConn *gorillaws.Conn, sessionID, language string) {
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = browserConn.Close()
			_ = upstreamConn.Close()
		})
	}

	done := make(chan struct{}, 2)
	go func() {
		h.copyLSPMessages("agentctl->browser", upstreamConn, browserConn, sessionID, language)
		done <- struct{}{}
	}()
	go func() {
		h.copyLSPMessages("browser->agentctl", browserConn, upstreamConn, sessionID, language)
		done <- struct{}{}
	}()

	<-done
	closeBoth()
	<-done

	h.logger.Info("LSP: connection closed",
		zap.String("session_id", sessionID),
		zap.String("language", language))
}

func (h *LSPHandler) copyLSPMessages(
	direction string,
	src *gorillaws.Conn,
	dst lspMessageWriter,
	sessionID, language string,
) {
	for {
		messageType, msg, err := src.ReadMessage()
		if err != nil {
			h.forwardLSPClose(dst, err)
			if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				h.logger.Debug("LSP proxy read error",
					zap.String("direction", direction),
					zap.String("session_id", sessionID),
					zap.String("language", language),
					zap.Error(err))
			}
			return
		}
		if err := writeLSPProxyMessage(dst, messageType, msg); err != nil {
			h.logger.Debug("LSP proxy write error",
				zap.String("direction", direction),
				zap.String("session_id", sessionID),
				zap.String("language", language),
				zap.Error(err))
			return
		}
	}
}

func (h *LSPHandler) forwardLSPClose(dst lspMessageWriter, err error) {
	if closeErr, ok := err.(*gorillaws.CloseError); ok {
		_ = writeLSPProxyMessage(
			dst,
			gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(closeErr.Code, closeErr.Text),
		)
		return
	}
	_ = writeLSPProxyMessage(
		dst,
		gorillaws.CloseMessage,
		gorillaws.FormatCloseMessage(lspCloseStreamError, "LSP stream closed"),
	)
}

func writeLSPProxyMessage(dst lspMessageWriter, messageType int, payload []byte) error {
	if err := dst.SetWriteDeadline(time.Now().Add(lspProxyWriteTimeout)); err != nil {
		return err
	}
	return dst.WriteMessage(messageType, payload)
}

func closeLSPConnWithCode(conn *gorillaws.Conn, code int, text string) {
	_ = writeLSPProxyMessage(conn, gorillaws.CloseMessage, gorillaws.FormatCloseMessage(code, text))
	_ = conn.Close()
}
