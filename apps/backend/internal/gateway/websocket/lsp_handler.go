package websocket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/user/models"
)

// Custom WebSocket close codes for LSP connections.
const (
	lspCloseBinaryNotFound  = 4001
	lspCloseSessionNotFound = 4002
	lspCloseInstallFailed   = 4003
)

// LSPUserService provides user settings for the LSP handler.
type LSPUserService interface {
	GetUserSettings(ctx context.Context) (*models.UserSettings, error)
}

// LSPHandler handles WebSocket connections for LSP (Language Server Protocol) proxying.
// Each WebSocket connection gets its own dedicated LSP server process (1:1 mapping).
// This avoids LSP protocol issues with shared servers (duplicate initialize,
// request ID collisions between clients).
type LSPHandler struct {
	lifecycleMgr *lifecycle.Manager
	userService  LSPUserService
	installer    *installer.Registry
	logger       *logger.Logger
	installMu    sync.Mutex
	installing   map[string]chan struct{}
}

// lspServerProcess represents a single LSP server process for one WebSocket client.
type lspServerProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cancel context.CancelFunc
}

var lspUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     checkWebSocketOrigin,
}

// NewLSPHandler creates a new LSPHandler.
func NewLSPHandler(lifecycleMgr *lifecycle.Manager, userService LSPUserService, installerRegistry *installer.Registry, log *logger.Logger) *LSPHandler {
	return &LSPHandler{
		lifecycleMgr: lifecycleMgr,
		userService:  userService,
		installer:    installerRegistry,
		logger:       log.WithFields(zap.String("component", "lsp_handler")),
		installing:   make(map[string]chan struct{}),
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

	// Validate language against the registry
	if !installer.IsSupported(language) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported language: %s", language)})
		return
	}

	h.logger.Info("LSP WebSocket connection request",
		zap.String("session_id", sessionID),
		zap.String("language", language))

	// Get workspace path from execution
	execution, exists := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !exists {
		h.logger.Warn("LSP: session not found in lifecycle manager",
			zap.String("session_id", sessionID))
		conn, err := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			h.logger.Error("LSP: failed to upgrade WebSocket for close", zap.Error(err))
			return
		}
		_ = conn.WriteMessage(gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(lspCloseSessionNotFound, "session not found"))
		_ = conn.Close()
		return
	}

	workspacePath := execution.WorkspacePath
	h.logger.Debug("LSP: found execution",
		zap.String("session_id", sessionID),
		zap.String("workspace_path", workspacePath))

	// Check if binary is available
	binaryPath, err := h.installer.BinaryPath(language)
	if err != nil {
		h.logger.Info("LSP: binary not found",
			zap.String("language", language),
			zap.Error(err))
		// Binary not found — check auto-install setting
		settings, settingsErr := h.userService.GetUserSettings(c.Request.Context())
		autoInstall := false
		if settingsErr == nil && settings != nil {
			for _, lang := range settings.LspAutoInstallLanguages {
				if lang == language {
					autoInstall = true
					break
				}
			}
		}
		h.logger.Debug("LSP: auto-install check",
			zap.String("language", language),
			zap.Bool("auto_install", autoInstall),
			zap.Error(settingsErr))

		if !autoInstall {
			h.logger.Info("LSP: closing with binary not found (auto-install off)",
				zap.String("language", language),
				zap.String("error", err.Error()))
			conn, upgradeErr := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
			if upgradeErr != nil {
				h.logger.Error("LSP: failed to upgrade WebSocket for close", zap.Error(upgradeErr))
				return
			}
			_ = conn.WriteMessage(gorillaws.CloseMessage,
				gorillaws.FormatCloseMessage(lspCloseBinaryNotFound, err.Error()))
			_ = conn.Close()
			return
		}

		// Auto-install: upgrade WS, send status messages, install
		conn, upgradeErr := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
		if upgradeErr != nil {
			return
		}

		if err := writeJSONMessage(conn, map[string]string{"status": "installing", "language": language}); err != nil {
			h.logger.Warn("failed to send status message", zap.Error(err))
		}

		// Concurrency protection: wait if another install for the same language is in progress
		binaryPath, err = h.awaitOrInstall(c.Request.Context(), language)
		if err != nil {
			h.logger.Error("LSP auto-install failed",
				zap.String("language", language),
				zap.Error(err))
			if err := writeJSONMessage(conn, map[string]string{"status": "install_failed", "error": err.Error()}); err != nil {
				h.logger.Warn("failed to send status message", zap.Error(err))
			}
			_ = conn.WriteMessage(gorillaws.CloseMessage,
				gorillaws.FormatCloseMessage(lspCloseInstallFailed, "install failed"))
			_ = conn.Close()
			return
		}

		if err := writeJSONMessage(conn, map[string]string{"status": "installed"}); err != nil {
			h.logger.Warn("failed to send status message", zap.Error(err))
		}

		// Continue with the already-upgraded connection
		h.handleLSPBridge(conn, sessionID, language, binaryPath, workspacePath)
		return
	}

	// Binary found — upgrade and bridge
	h.logger.Info("LSP: binary found, upgrading WebSocket",
		zap.String("language", language),
		zap.String("binary_path", binaryPath))
	conn, upgradeErr := lspUpgrader.Upgrade(c.Writer, c.Request, nil)
	if upgradeErr != nil {
		h.logger.Error("LSP: failed to upgrade to WebSocket",
			zap.String("session_id", sessionID),
			zap.Error(upgradeErr))
		return
	}

	h.handleLSPBridge(conn, sessionID, language, binaryPath, workspacePath)
}

// awaitOrInstall either waits for an in-progress installation to finish or
// performs the installation itself, broadcasting completion to other waiters.
func (h *LSPHandler) awaitOrInstall(ctx context.Context, language string) (string, error) {
	h.installMu.Lock()
	if ch, ok := h.installing[language]; ok {
		// Another goroutine is already installing — wait for it
		h.installMu.Unlock()
		select {
		case <-ch:
			// Install completed; look up the binary path
			return h.installer.BinaryPath(language)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	// Claim the install slot
	ch := make(chan struct{})
	h.installing[language] = ch
	h.installMu.Unlock()

	defer func() {
		h.installMu.Lock()
		delete(h.installing, language)
		close(ch)
		h.installMu.Unlock()
	}()

	strategy, err := h.installer.StrategyFor(language)
	if err != nil {
		return "", err
	}
	result, err := strategy.Install(ctx)
	if err != nil {
		return "", err
	}
	return result.BinaryPath, nil
}

func (h *LSPHandler) handleLSPBridge(conn *gorillaws.Conn, sessionID, language, binaryPath, workspacePath string) {
	server, err := h.startServer(language, binaryPath, workspacePath)
	if err != nil {
		h.logger.Error("LSP: failed to start language server",
			zap.String("session_id", sessionID),
			zap.String("language", language),
			zap.Error(err))
		_ = conn.WriteMessage(gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(gorillaws.CloseInternalServerErr, err.Error()))
		_ = conn.Close()
		return
	}

	h.logger.Info("LSP: server started, sending ready signal",
		zap.String("session_id", sessionID),
		zap.String("language", language),
		zap.Int("pid", server.cmd.Process.Pid))

	// Signal the frontend that the language server is ready.
	// Include workspace path so the frontend can set rootUri in the LSP initialize request.
	if err := writeJSONMessage(conn, map[string]string{"status": "ready", "workspacePath": workspacePath}); err != nil {
		h.logger.Error("LSP: failed to send ready message", zap.Error(err))
		h.stopServer(server)
		_ = conn.Close()
		return
	}

	h.runLSPBridge(conn, sessionID, language, server)
	h.stopServer(server)

	h.logger.Info("LSP: connection closed",
		zap.String("session_id", sessionID),
		zap.String("language", language))
}

func (h *LSPHandler) startServer(language, binaryPath, workspacePath string) (*lspServerProcess, error) {
	binary, args := installer.LspCommand(language)
	if binaryPath != "" {
		binary = binaryPath
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = workspacePath

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start %s: %w", binary, err)
	}

	return &lspServerProcess{cmd: cmd, stdin: stdin, stdout: stdout, cancel: cancel}, nil
}

func (h *LSPHandler) stopServer(server *lspServerProcess) {
	server.cancel()
	_ = server.stdin.Close()
	_ = server.cmd.Wait()
}

// runLSPBridge runs bidirectional proxying between a WebSocket and an LSP server.
// Blocks until the WebSocket closes or the server exits.
func (h *LSPHandler) runLSPBridge(conn *gorillaws.Conn, sessionID, language string, server *lspServerProcess) {
	done := make(chan struct{})

	// stdout → WebSocket
	go func() {
		defer close(done)
		reader := bufio.NewReader(server.stdout)
		for {
			msg, err := readLSPMessage(reader)
			if err != nil {
				if err != io.EOF {
					h.logger.Debug("LSP stdout read error",
						zap.String("session_id", sessionID),
						zap.String("language", language),
						zap.Error(err))
				}
				// Server exited — close WebSocket so WS→stdin loop exits
				_ = conn.WriteMessage(gorillaws.CloseMessage,
					gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "language server exited"))
				_ = conn.Close()
				return
			}
			if wErr := conn.WriteMessage(gorillaws.TextMessage, msg); wErr != nil {
				h.logger.Debug("LSP WebSocket write error",
					zap.String("session_id", sessionID),
					zap.String("language", language),
					zap.Error(wErr))
				return
			}
		}
	}()

	// WebSocket → stdin
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				h.logger.Debug("LSP WebSocket read error",
					zap.String("session_id", sessionID),
					zap.String("language", language),
					zap.Error(err))
			}
			break
		}

		// Wrap with Content-Length header for the language server
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
		if _, err := server.stdin.Write([]byte(header)); err != nil {
			h.logger.Debug("LSP stdin write error",
				zap.String("session_id", sessionID),
				zap.String("language", language),
				zap.Error(err))
			break
		}
		if _, err := server.stdin.Write(msg); err != nil {
			h.logger.Debug("LSP stdin write error",
				zap.String("session_id", sessionID),
				zap.String("language", language),
				zap.Error(err))
			break
		}
	}

	// Kill server so stdout goroutine exits, then wait for it
	server.cancel()
	_ = server.stdin.Close()
	<-done
}

// readLSPMessage reads a single LSP message from a reader, parsing Content-Length headers.
func readLSPMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// End of headers
			break
		}
		if after, found := strings.CutPrefix(line, "Content-Length: "); found {
			n, err := strconv.Atoi(after)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %s", after)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}

	return body, nil
}

func writeJSONMessage(conn *gorillaws.Conn, data any) error {
	msg, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return conn.WriteMessage(gorillaws.TextMessage, msg)
}
