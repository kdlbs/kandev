package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/auth/authn"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/lsp/protocol"
	"github.com/kandev/kandev/internal/user/models"
)

// Custom WebSocket close codes for LSP connections.
const (
	lspCloseSessionNotFound      = 4002
	lspCloseUnsupportedExecutor  = 4004
	lspCloseCapacityExceeded     = 4005
	lspCloseRuntimeStopped       = 4010
	lspCloseUnsupportedCloseText = "LSP is only supported for local_pc and local_docker tasks in this release"
	lspCloseSessionNotFoundText  = "session not found"
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
	CheckSessionAccess(ctx context.Context, sessionID string) error
	ResolveSessionRuntime(ctx context.Context, sessionID string) (agentruntime.Runtime, error)
	GetOrEnsureExecution(ctx context.Context, sessionID string) (*lifecycle.AgentExecution, error)
}

var errLSPCapacityExceeded = errors.New("active LSP connection cap exceeded")

// LSPHandler handles browser-facing LSP WebSocket connections.
// Each browser attaches to a lease for a task-host language server.
type LSPHandler struct {
	lifecycleMgr lspLifecycleManager
	userService  LSPUserService
	logger       *logger.Logger
	leases       *lspLeaseManager
	sessionFence func(string) func()
	settingsSub  bus.Subscription
}

func newLSPHandler(
	lifecycleMgr lspLifecycleManager,
	userService LSPUserService,
	log *logger.Logger,
	maxConnections int,
	sessionFence func(string) func(),
	eventBus bus.EventBus,
) *LSPHandler {
	handler := &LSPHandler{
		lifecycleMgr: lifecycleMgr,
		userService:  userService,
		logger:       log.WithFields(zap.String("component", "lsp_handler")),
		sessionFence: sessionFence,
		leases:       newLSPLeaseManager(maxConnections, log),
	}
	if eventBus != nil {
		subscription, err := eventBus.Subscribe(events.UserSettingsUpdated, handler.handleUserSettingsUpdated)
		if err != nil {
			handler.logger.Warn("failed to subscribe LSP leases to user settings updates", zap.Error(err))
		} else {
			handler.settingsSub = subscription
		}
	}
	return handler
}

// Close releases retained language servers and the settings subscription.
func (h *LSPHandler) Close() error {
	if h.settingsSub != nil && h.settingsSub.IsValid() {
		_ = h.settingsSub.Unsubscribe()
	}
	if h.leases != nil {
		h.leases.Close()
	}
	return nil
}

var lspUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     checkWebSocketOrigin,
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
	h.handleContinuityConnection(c, sessionID, language)
}

func (h *LSPHandler) handleContinuityConnection(c *gin.Context, sessionID, language string) {
	releaseFence := func() {}
	fenceHeld := true
	if h.sessionFence != nil {
		releaseFence = h.sessionFence(sessionID)
	}
	defer func() {
		if fenceHeld {
			releaseFence()
		}
	}()
	execution, closeCode, closeMessage := h.resolveContinuityExecution(c.Request.Context(), sessionID)
	if closeCode != 0 {
		h.closeWithCode(c, closeCode, closeMessage)
		return
	}
	userID, configuration, autoInstall := h.continuityUserSettings(c.Request.Context(), language)

	browserConn, err := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("LSP: failed to upgrade to WebSocket", zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	browserConn.SetReadLimit(protocol.MaxMessageBytes)
	lease, generation, _, err := h.leases.attachOrCreate(
		c.Request.Context(),
		lspLeaseExecutionFromAgentExecution(execution),
		language,
		userID,
		c.Query("leaseId"),
		configuration,
		autoInstall,
		browserConn,
	)
	if err != nil {
		code, message := lspLeaseAdmissionClose(err)
		closeLSPConnWithCode(browserConn, code, message)
		return
	}
	// Admission is now represented by the live lease pin. Do not hold the
	// session lifecycle fence for the lifetime of the browser socket.
	releaseFence()
	fenceHeld = false

	h.serveContinuityConnection(browserConn, lease, generation, sessionID, language)
}

func (h *LSPHandler) resolveContinuityExecution(
	ctx context.Context,
	sessionID string,
) (*lifecycle.AgentExecution, int, string) {
	if h.lifecycleMgr == nil || h.leases == nil {
		return nil, lspCloseSessionNotFound, "LSP lease manager unavailable"
	}
	if h.lifecycleMgr.CheckSessionAccess(ctx, sessionID) != nil {
		return nil, lspCloseSessionNotFound, lspCloseSessionNotFoundText
	}
	runtimeName, err := h.lifecycleMgr.ResolveSessionRuntime(ctx, sessionID)
	if err != nil {
		return nil, lspCloseSessionNotFound, lspCloseSessionNotFoundText
	}
	if !lspRuntimeSupported(runtimeName) {
		return nil, lspCloseUnsupportedExecutor, lspCloseUnsupportedCloseText
	}
	execution, err := h.lifecycleMgr.GetOrEnsureExecution(ctx, sessionID)
	if err != nil || execution == nil || execution.SessionID != sessionID {
		return nil, lspCloseSessionNotFound, "agentctl unavailable"
	}
	if !lspRuntimeSupported(execution.RuntimeName) {
		return nil, lspCloseUnsupportedExecutor, lspCloseUnsupportedCloseText
	}
	return execution, 0, ""
}

func (h *LSPHandler) continuityUserSettings(
	ctx context.Context,
	language string,
) (string, map[string]any, bool) {
	userID := ""
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		userID = identity.UserID
	}
	if h.userService == nil {
		return userID, make(map[string]any), false
	}
	settings, err := h.userService.GetUserSettings(ctx)
	if err != nil || settings == nil {
		if err != nil {
			h.logger.Debug("LSP: failed to load settings for a retained lease", zap.Error(err))
		}
		return userID, make(map[string]any), false
	}
	if userID == "" {
		userID = settings.UserID
	}
	configuration := make(map[string]any)
	for key, value := range settings.LspServerConfigs[language] {
		configuration[key] = value
	}
	return userID, configuration, h.settingsAllowAutoInstall(settings, language)
}

func lspLeaseAdmissionClose(err error) (int, string) {
	if errors.Is(err, errLSPCapacityExceeded) {
		return lspCloseCapacityExceeded, errLSPCapacityExceeded.Error()
	}
	return lspCloseSessionNotFound, "failed to connect to task host LSP stream"
}

func (h *LSPHandler) serveContinuityConnection(
	browserConn *gorillaws.Conn,
	lease *lspLease,
	generation uint64,
	sessionID, language string,
) {
	defer lease.detach(generation)
	for {
		messageType, message, readErr := browserConn.ReadMessage()
		if readErr != nil {
			return
		}
		if messageType != gorillaws.TextMessage {
			continue
		}
		if err := lease.handleBrowserMessage(generation, message); err != nil {
			h.logger.Debug("LSP browser frame failed", zap.String("session_id", sessionID), zap.String("language", language), zap.Error(err))
			return
		}
	}
}

func lspLeaseExecutionFromAgentExecution(execution *lifecycle.AgentExecution) lspLeaseExecution {
	if execution == nil {
		return lspLeaseExecution{}
	}
	return lspLeaseExecution{
		ID:        execution.ID,
		SessionID: execution.SessionID,
		TaskID:    execution.TaskID,
		acquireAgentCtlClient: func() (lspLeaseAgentCtlClient, func()) {
			client, release := execution.AcquireAgentCtlClient()
			if client == nil {
				return nil, release
			}
			return client, release
		},
	}
}

func lspRuntimeSupported(runtimeName agentruntime.Runtime) bool {
	return runtimeName == agentruntime.RuntimeStandalone || runtimeName == agentruntime.RuntimeDocker
}

func (h *LSPHandler) settingsAllowAutoInstall(settings *models.UserSettings, language string) bool {
	if settings == nil || !installer.SupportsAutoInstall(language) {
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
