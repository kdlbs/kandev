package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/lsp/protocol"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"go.uber.org/zap"
)

const (
	lspCloseBinaryNotFound = 4001
	lspCloseInstallFailed  = 4003

	lspLanguageKey          = "language"
	lspStatusKey            = "status"
	lspStatusInstalling     = "installing"
	lspStatusInstalled      = "installed"
	lspStatusInstallFailed  = "install_failed"
	lspStatusReady          = "ready"
	lspWorkspacePathJSONKey = "workspacePath"
)

type lspServerProcess struct {
	id     string
	stdin  io.WriteCloser
	stdout io.ReadCloser
	done   <-chan struct{}
}

type lspInstallerRegistry interface {
	BinaryPath(language string) (string, error)
	StrategyFor(language string) (tools.Strategy, error)
}

type lspInstallGate struct {
	token chan struct{}
}

func newLSPInstallGate() *lspInstallGate {
	token := make(chan struct{}, 1)
	token <- struct{}{}
	return &lspInstallGate{token: token}
}

func (g *lspInstallGate) run(ctx context.Context, install func() (string, error)) (string, error) {
	select {
	case <-g.token:
		defer func() { g.token <- struct{}{} }()
		return install()
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

var sharedLSPInstallGate = newLSPInstallGate()

func (s *Server) handleLSPStreamWS(c *gin.Context) {
	language := c.Query("language")
	if language == "" {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "language query parameter is required"})
		return
	}
	if !installer.IsSupported(language) {
		c.JSON(http.StatusBadRequest, gin.H{errKey: fmt.Sprintf("unsupported language: %s", language)})
		return
	}

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("LSP: failed to upgrade WebSocket", zap.Error(err))
		return
	}
	conn.SetReadLimit(protocol.MaxMessageBytes)

	binaryPath, err := s.lspInstaller.BinaryPath(language)
	if err != nil {
		s.handleLSPBinaryNotFound(
			c.Request.Context(),
			conn,
			language,
			lspAutoInstallRequested(c) && installer.CanAutoInstall(language),
			err,
		)
		return
	}

	s.handleLSPBridge(conn, language, binaryPath)
}

func lspAutoInstallRequested(c *gin.Context) bool {
	value := c.Query("autoInstall")
	return value == "1" || strings.EqualFold(value, "true")
}

func (s *Server) handleLSPBinaryNotFound(ctx context.Context, conn *websocket.Conn, language string, autoInstall bool, binaryErr error) {
	if !autoInstall {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(lspCloseBinaryNotFound, binaryErr.Error()))
		_ = conn.Close()
		return
	}

	if err := writeLSPJSONMessage(conn, map[string]string{lspStatusKey: lspStatusInstalling, lspLanguageKey: language}); err != nil {
		s.logger.Warn("failed to send LSP installing status", zap.String("language", language), zap.Error(err))
	}

	binaryPath, err := s.awaitOrInstallLSP(ctx, language)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, process.ErrManagerStopping) {
			s.logger.Debug("LSP auto-install canceled during task teardown", zap.String("language", language))
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "task stopping"))
			_ = conn.Close()
			return
		}
		s.logger.Error("LSP auto-install failed", zap.String("language", language), zap.Error(err))
		if writeErr := writeLSPJSONMessage(conn, map[string]string{lspStatusKey: lspStatusInstallFailed, lspLanguageKey: language, errKey: err.Error()}); writeErr != nil {
			s.logger.Warn("failed to send LSP install failure status", zap.String("language", language), zap.Error(writeErr))
		}
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(lspCloseInstallFailed, "install failed"))
		_ = conn.Close()
		return
	}

	if err := writeLSPJSONMessage(conn, map[string]string{lspStatusKey: lspStatusInstalled, lspLanguageKey: language}); err != nil {
		s.logger.Warn("failed to send LSP installed status", zap.String("language", language), zap.Error(err))
	}

	s.handleLSPBridge(conn, language, binaryPath)
}

func (s *Server) handleLSPBridge(conn *websocket.Conn, language, binaryPath string) {
	server, err := s.startLSPServer(language, binaryPath)
	if err != nil {
		s.logger.Error("LSP: failed to start language server", zap.String("language", language), zap.Error(err))
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
		_ = conn.Close()
		return
	}

	if err := writeLSPJSONMessage(conn, map[string]string{lspStatusKey: lspStatusReady, lspWorkspacePathJSONKey: s.cfg.WorkDir}); err != nil {
		s.stopLSPServer(server)
		_ = conn.Close()
		return
	}

	s.runLSPBridge(conn, language, server)
}

func (s *Server) startLSPServer(language, binaryPath string) (*lspServerProcess, error) {
	binary, args := installer.LspCommand(language)
	if binaryPath != "" {
		binary = binaryPath
	}

	sessionID := s.cfg.SessionID
	if sessionID == "" {
		sessionID = s.cfg.InstanceID
	}
	if sessionID == "" {
		sessionID = "lsp"
	}
	proc, err := s.procMgr.StartPipedProcess(process.PipedStartRequest{
		SessionID:  sessionID,
		Kind:       types.ProcessKindCustom,
		ScriptName: "lsp-" + language,
		Command:    binary,
		Args:       args,
		WorkingDir: s.cfg.WorkDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", binary, err)
	}

	return &lspServerProcess{id: proc.ID, stdin: proc.Stdin, stdout: proc.Stdout, done: proc.Done}, nil
}

func (s *Server) stopLSPServer(server *lspServerProcess) {
	_ = server.stdin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.procMgr.StopProcess(ctx, process.StopProcessRequest{ProcessID: server.id}); err != nil {
		s.logger.Debug("LSP process already stopped", zap.String("process_id", server.id), zap.Error(err))
	}
	select {
	case <-server.done:
	case <-ctx.Done():
		s.logger.Warn("timed out waiting for LSP process teardown", zap.String("process_id", server.id))
	}
}

func (s *Server) runLSPBridge(conn *websocket.Conn, language string, server *lspServerProcess) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		reader := bufio.NewReader(server.stdout)
		for {
			msg, err := protocol.ReadMessage(reader)
			if err != nil {
				if err != io.EOF {
					s.logger.Debug("LSP stdout read error", zap.String("language", language), zap.Error(err))
				}
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "language server exited"))
				_ = conn.Close()
				return
			}
			if wErr := conn.WriteMessage(websocket.TextMessage, msg); wErr != nil {
				s.logger.Debug("LSP WebSocket write error", zap.String("language", language), zap.Error(wErr))
				return
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.logger.Debug("LSP WebSocket read error", zap.String("language", language), zap.Error(err))
			}
			break
		}

		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
		if _, err := server.stdin.Write([]byte(header)); err != nil {
			s.logger.Debug("LSP stdin write error", zap.String("language", language), zap.Error(err))
			break
		}
		if _, err := server.stdin.Write(msg); err != nil {
			s.logger.Debug("LSP stdin write error", zap.String("language", language), zap.Error(err))
			break
		}
	}

	s.stopLSPServer(server)
	<-done
}

func (s *Server) awaitOrInstallLSP(ctx context.Context, language string) (string, error) {
	installCtx, release, err := s.procMgr.BeginOwnedOperation(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	return sharedLSPInstallGate.run(installCtx, func() (string, error) {
		if binaryPath, err := s.lspInstaller.BinaryPath(language); err == nil {
			return binaryPath, nil
		}
		strategy, err := s.lspInstaller.StrategyFor(language)
		if err != nil {
			return "", err
		}
		result, err := strategy.Install(installCtx)
		if err != nil {
			return "", err
		}
		return result.BinaryPath, nil
	})
}

func writeLSPJSONMessage(conn *websocket.Conn, data any) error {
	msg, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, msg)
}
