package loginpty

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
)

// LoginCommandLookup returns the agent's LoginCommand or nil. Implemented by
// the registry so we don't import agent/settings here.
type LoginCommandLookup interface {
	Get(id string) (agents.Agent, bool)
}

// ErrNoHostShellDescriptor is the sentinel a HostShellSessionBinder returns
// when the client_id has no durable descriptor to bind. It is not fatal: the
// host shell PTY still starts and streams, there is simply nothing persistent
// to reconcile against.
var ErrNoHostShellDescriptor = errors.New("no host shell descriptor for client")

type HostShellSessionBinder interface {
	BindHostShellSession(ctx context.Context, tabID, sessionID string) error
}

// Handlers wraps the manager + agent registry to expose HTTP/WS endpoints.
type Handlers struct {
	mgr             *Manager
	registry        *registry.Registry
	logger          *zap.Logger
	upgrader        gorillaws.Upgrader
	hostShellBinder HostShellSessionBinder
}

// NewHandlers constructs Handlers. If checkOrigin is nil, the upgrader uses
// the default localhost-friendly check below — needed because gorilla's
// stock CheckOrigin rejects cross-origin (e.g. Next dev on :37429 → backend
// on :38429).
func NewHandlers(mgr *Manager, reg *registry.Registry, log *zap.Logger, checkOrigin func(*http.Request) bool) *Handlers {
	if checkOrigin == nil {
		checkOrigin = defaultCheckOrigin
	}
	up := gorillaws.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     checkOrigin,
	}
	return &Handlers{
		mgr:      mgr,
		registry: reg,
		logger:   log,
		upgrader: up,
	}
}

func (h *Handlers) SetHostShellSessionBinder(binder HostShellSessionBinder) {
	h.hostShellBinder = binder
}

// defaultCheckOrigin mirrors the policy used by the existing terminal
// handler: allow no-origin requests (non-browser clients), allow loopback
// origins (dev), otherwise require Origin host to match Request host.
//
// Loopback is matched by exact hostname after parsing — a HasPrefix check
// against `http://localhost` would also accept `http://localhost.attacker.tld`,
// which a hostile page could use to slip through the dev exception.
func defaultCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch originURL.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	// Strip port for flexibility (but keep IPv6 brackets intact).
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		if !strings.Contains(host, "]") || colonIdx > strings.Index(host, "]") {
			host = host[:colonIdx]
		}
	}
	return originURL.Hostname() == host
}

// RegisterRoutes mounts the login-pty endpoints on the given router.
// Routes are split into agents/* (lookup by agent name) and sessions/* (lookup
// by session ID) so gin doesn't see two routes sharing a wildcard slot with
// different param names.
//
// The /host-shell/start route reuses the same session-manager infrastructure
// to spawn a plain user shell — handy for ad-hoc install/setup commands the
// install button doesn't cover.
func (h *Handlers) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	api.POST("/agent-login/agents/:agentName/start", h.httpStart)
	api.POST("/agent-login/sessions/:sessionID/stop", h.httpStop)
	api.POST("/agent-login/sessions/:sessionID/resize", h.httpResize)
	api.GET("/agent-login/sessions/:sessionID/status", h.httpStatus)
	api.GET("/agent-login/sessions/:sessionID/stream", h.handleStream)
	api.POST("/host-shell/start", h.httpStartHostShell)
}

// hostShellAgentID is the synthetic key the session manager uses for host
// shell sessions so they don't collide with agent login sessions.
const HostShellAgentID = "_host_shell"

const hostShellAgentID = HostShellAgentID

// httpStartHostShell spawns the user's interactive shell under a PTY. Returns
// the standard session snapshot — the client uses the same
// stop/resize/stream endpoints.
func (h *Handlers) httpStartHostShell(c *gin.Context) {
	var req startRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	managerKey := hostShellAgentID
	tabID := ""
	if len(req.ClientID) > 0 {
		var clientID string
		if err := json.Unmarshal(req.ClientID, &clientID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "client_id must be a UUID"})
			return
		}
		parsedClientID, err := uuid.Parse(clientID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "client_id must be a UUID"})
			return
		}
		tabID = parsedClientID.String()
		managerKey += ":" + tabID
	}

	shell, shellArgs := detectShell()
	command := append([]string{shell}, shellArgs...)
	sess, err := h.mgr.StartWithKey(managerKey, hostShellAgentID, command, req.Cols, req.Rows)
	reused := errors.Is(err, ErrSessionAlreadyRunning)
	if err != nil && !reused {
		h.logger.Warn("host shell start failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tabID != "" && h.hostShellBinder != nil {
		if handled := h.bindHostShellSession(c, tabID, sess, reused); handled {
			return
		}
	}
	c.JSON(http.StatusOK, sess.Status())
}

// bindHostShellSession reconciles a started host-shell PTY with its durable
// descriptor. It returns true when it has already written an error response, so
// the caller must stop processing.
//
// A missing descriptor (ErrNoHostShellDescriptor) is not fatal: the PTY is a
// generic host shell keyed by client_id and callers may attach without ever
// persisting a Quick Terminal tab, so the session keeps running. Any other bind
// error is a 500; a freshly created session is stopped to avoid leaking it,
// while a reused session belongs to an existing attachment and must survive.
func (h *Handlers) bindHostShellSession(c *gin.Context, tabID string, sess *Session, reused bool) bool {
	bindErr := h.hostShellBinder.BindHostShellSession(c.Request.Context(), tabID, sess.ID)
	if bindErr == nil {
		return false
	}
	if errors.Is(bindErr, ErrNoHostShellDescriptor) {
		h.logger.Debug("host shell started without durable descriptor", zap.String("tab_id", tabID))
		return false
	}
	if !reused {
		sess.stop()
	}
	h.logger.Warn("host shell bind failed", zap.String("tab_id", tabID), zap.Error(bindErr))
	c.JSON(http.StatusInternalServerError, gin.H{"error": bindErr.Error()})
	return true
}

// detectShell picks a sensible interactive shell, preferring $SHELL on Unix
// and PowerShell on Windows. The returned arguments are part of the shell
// command because PowerShell needs flags to remain an interactive PTY shell.
func detectShell() (string, []string) {
	return detectShellForOS(runtime.GOOS, os.Getenv, func(candidate string) bool {
		_, err := exec.LookPath(candidate)
		return err == nil
	})
}

func detectShellForOS(goos string, getenv func(string) string, exists func(string) bool) (string, []string) {
	if goos == "windows" {
		for _, candidate := range []string{"pwsh.exe", "powershell.exe"} {
			if exists(candidate) {
				return candidate, []string{"-NoLogo", "-NoExit"}
			}
		}
		if comspec := strings.TrimSpace(getenv("COMSPEC")); comspec != "" {
			return comspec, nil
		}
		return "cmd.exe", nil
	}

	if shell := strings.TrimSpace(getenv("SHELL")); shell != "" && exists(shell) {
		return shell, nil
	}
	for _, candidate := range []string{"/bin/bash", "/bin/zsh", "/bin/sh"} {
		if exists(candidate) {
			return candidate, nil
		}
	}
	return "/bin/sh", nil
}

type startRequest struct {
	Cols     uint16          `json:"cols"`
	Rows     uint16          `json:"rows"`
	ClientID json.RawMessage `json:"client_id"`
}

func (h *Handlers) httpStart(c *gin.Context) {
	name := strings.TrimSpace(c.Param("agentName"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}
	ag, ok := h.registry.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	loginAg, ok := ag.(agents.LoginAgent)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent has no login command"})
		return
	}
	lc := loginAg.LoginCommand()
	if lc == nil || len(lc.Cmd) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent has no login command"})
		return
	}

	var req startRequest
	_ = c.ShouldBindJSON(&req) // body is optional

	// Wrap the agent's login command so that when the login process exits
	// (Ctrl+C, /quit, success exit, anything) the PTY drops the user into
	// their normal shell instead of becoming a dead terminal. The user can
	// then re-run the login, install a missing dep, or just close the
	// dialog.
	//
	// `sh -c '"$@"; exec "${SHELL:-/bin/sh}"' wrapper <cmd...>` lets sh do
	// the argv parsing - no shell-escaping required on our side.
	//
	// `trap 'true' INT` stops the wrapper sh from dying when Ctrl+C is
	// pressed (non-interactive sh exits on SIGINT by default). The child
	// (codex/auggie/...) still receives SIGINT directly via the foreground
	// process group, and exec(2) resets the trap to SIG_DFL in the child
	// - so the agent dies as expected and sh survives to exec the shell.
	wrapped := append(
		[]string{"sh", "-c", `trap 'true' INT; "$@"; exec "${SHELL:-/bin/sh}"`, "kandev-login-wrapper"},
		lc.Cmd...,
	)
	sess, err := h.mgr.Start(name, wrapped, req.Cols, req.Rows)
	if err != nil && err != ErrSessionAlreadyRunning {
		h.logger.Warn("login session start failed",
			zap.String("agent", name),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sess.Status())
}

func (h *Handlers) httpStop(c *gin.Context) {
	id := c.Param("sessionID")
	sess := h.mgr.GetByID(id)
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	sess.stop()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type resizeRequest struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (h *Handlers) httpResize(c *gin.Context) {
	id := c.Param("sessionID")
	sess := h.mgr.GetByID(id)
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	var req resizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := sess.Resize(req.Cols, req.Rows); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) httpStatus(c *gin.Context) {
	id := c.Param("sessionID")
	sess := h.mgr.GetByID(id)
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, sess.Status())
}

// handleStream upgrades to WS and proxies bytes both directions:
//   - server → client: PTY output as BinaryMessage frames
//   - client → server:
//   - BinaryMessage frames = raw input to PTY (keystrokes)
//   - TextMessage frames = JSON control msg: {"type":"resize","cols":N,"rows":M}
//
// On session exit the subscriber channel is closed; the writer goroutine
// detects this, sends a final TextMessage {"type":"exit","exit_code":N},
// and the connection is closed.
func (h *Handlers) handleStream(c *gin.Context) {
	id := c.Param("sessionID")
	sess := h.mgr.GetByID(id)
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn("login pty ws upgrade failed",
			zap.String("session_id", id),
			zap.Error(err),
		)
		return
	}
	defer func() { _ = conn.Close() }()

	// Send buffered output up-front so a late-joining UI catches up.
	if buf := sess.BufferedOutput(); len(buf) > 0 {
		_ = conn.WriteMessage(gorillaws.BinaryMessage, buf)
	}

	out := make(chan []byte, 128)
	sess.Subscribe(out)
	defer sess.Unsubscribe(out)

	done := make(chan struct{})
	// disconnect is closed by the reader on client disconnect so the writer
	// can exit even when the PTY is silent (e.g. idle browser-OAuth flows).
	// Without this, an abrupt client disconnect during an idle session leaks
	// the writer goroutine for up to IdleTimeout.
	disconnect := make(chan struct{})

	// Writer: forward PTY output → client.
	go func() {
		defer close(done)
		for {
			select {
			case data, ok := <-out:
				if !ok {
					// Channel closed: session ended. Send a final exit notice.
					status := sess.Status()
					payload := map[string]any{"type": "exit"}
					if status.ExitCode != nil {
						payload["exit_code"] = *status.ExitCode
					}
					b, _ := json.Marshal(payload)
					_ = conn.WriteMessage(gorillaws.TextMessage, b)
					return
				}
				if err := conn.WriteMessage(gorillaws.BinaryMessage, data); err != nil {
					return
				}
			case <-disconnect:
				return
			}
		}
	}()

	// Reader: forward client → PTY (binary = input, text = JSON control).
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch mt {
		case gorillaws.BinaryMessage:
			if _, err := sess.Write(data); err != nil {
				h.logger.Debug("pty write error", zap.Error(err))
			}
		case gorillaws.TextMessage:
			h.handleControlMessage(sess, data)
		}
	}
	close(disconnect)
	<-done
}

func (h *Handlers) handleControlMessage(sess *Session, data []byte) {
	var msg struct {
		Type string `json:"type"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	if msg.Type == "resize" {
		_ = sess.Resize(msg.Cols, msg.Rows)
	}
}
