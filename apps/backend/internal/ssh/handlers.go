// Package ssh provides HTTP and WebSocket handlers for SSH executor
// management: test-connection-and-fingerprint, list active SSH sessions for
// an executor.
package ssh

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const errorJSONKey = "error"

// ExecutorRunningLister is the narrow repository slice we need to surface
// active SSH sessions. We list all running executors and resolve each row's
// executor_id by looking up the task_session — the lifecycle manager doesn't
// populate ExecutorRunning.ExecutorID directly (it's only carried forward from
// prior rows on resume), so joining through the session is the source of
// truth.
type ExecutorRunningLister interface {
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
}

// Handler exposes /api/v1/ssh routes used by the settings UI.
type Handler struct {
	repo     ExecutorRunningLister
	resolver *lifecycle.AgentctlResolver
	logger   *logger.Logger
}

// NewHandler builds an SSH HTTP/WS handler.
func NewHandler(repo ExecutorRunningLister, resolver *lifecycle.AgentctlResolver, log *logger.Logger) *Handler {
	return &Handler{
		repo:     repo,
		resolver: resolver,
		logger:   log.WithFields(zap.String("component", "ssh-handler")),
	}
}

// RegisterRoutes wires the SSH routes onto the given gin engine + WS dispatcher.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, repo ExecutorRunningLister, resolver *lifecycle.AgentctlResolver, log *logger.Logger) {
	h := NewHandler(repo, resolver, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1/ssh")
	api.POST("/test", h.httpTest)
	api.GET("/executors/:id/sessions", h.httpListSessions)
}

func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc("ssh.test", h.wsTest)
	dispatcher.RegisterFunc("ssh.sessions.list", h.wsListSessions)
}

// --- Request / response shapes ---

// TestRequest is the body of POST /api/v1/ssh/test.
type TestRequest struct {
	Name         string `json:"name"`
	HostAlias    string `json:"host_alias,omitempty"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	User         string `json:"user,omitempty"`
	IdentitySrc  string `json:"identity_source,omitempty"` // "agent" or "file"
	IdentityFile string `json:"identity_file,omitempty"`
	ProxyJump    string `json:"proxy_jump,omitempty"`
}

// TestStep is one entry in the test result step list.
type TestStep struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// TestResult is the response of POST /api/v1/ssh/test. The UI requires
// `success` and `fingerprint` before letting the user tick "Trust this host".
type TestResult struct {
	Success         bool       `json:"success"`
	Fingerprint     string     `json:"fingerprint,omitempty"`
	UnameAll        string     `json:"uname_all,omitempty"`
	Arch            string     `json:"arch,omitempty"`
	GitVersion      string     `json:"git_version,omitempty"`
	AgentctlAction  string     `json:"agentctl_action,omitempty"` // "cached" | "uploaded" | "skipped"
	Steps           []TestStep `json:"steps"`
	TotalDurationMs int64      `json:"total_duration_ms"`
	Error           string     `json:"error,omitempty"`
}

// SessionRow is one entry in the active-sessions table.
type SessionRow struct {
	SessionID        string `json:"session_id"`
	TaskID           string `json:"task_id"`
	TaskTitle        string `json:"task_title,omitempty"`
	Host             string `json:"host"`
	User             string `json:"user,omitempty"`
	RemoteTaskDir    string `json:"remote_task_dir,omitempty"`
	RemoteAgentPort  int    `json:"remote_agentctl_port,omitempty"`
	LocalForwardPort int    `json:"local_forward_port,omitempty"`
	Status           string `json:"status"`
	UptimeSeconds    int64  `json:"uptime_seconds"`
	CreatedAt        string `json:"created_at"`
}

// --- HTTP handlers ---

func (h *Handler) httpTest(c *gin.Context) {
	var req TestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: "invalid request body: " + err.Error()})
		return
	}
	result := h.runTest(c.Request.Context(), req)
	c.JSON(http.StatusOK, result)
}

func (h *Handler) httpListSessions(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: "executor id required"})
		return
	}
	rows, err := h.listSessions(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// --- WS handlers ---

func (h *Handler) wsTest(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req TestRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, h.runTest(ctx, req))
}

func (h *Handler) wsListSessions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ExecutorID string `json:"executor_id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	if payload.ExecutorID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "executor_id required", nil)
	}
	rows, err := h.listSessions(ctx, payload.ExecutorID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, rows)
}

// --- Business logic ---

// runTest performs the SSH-test sequence: dial without a pinned fingerprint
// (so the host key is observed but not yet trusted), probe remote OS/arch,
// then dry-run the agentctl-upload-or-skip step. The UI gates the "Trust this
// host" checkbox on result.Success && result.Fingerprint != "".
func (h *Handler) runTest(ctx context.Context, req TestRequest) *TestResult {
	start := time.Now()
	result := &TestResult{}
	finalize := func() *TestResult {
		result.TotalDurationMs = time.Since(start).Milliseconds()
		return result
	}

	target, err := h.testResolveTarget(req, result)
	if err != nil {
		return finalize()
	}

	client, err := h.testHandshake(ctx, target, result)
	if err != nil {
		return finalize()
	}
	defer func() { _ = client.Close() }()

	infoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := h.testProbeAndArch(infoCtx, client, result); err != nil {
		return finalize()
	}
	h.testAgentctlCache(infoCtx, client, result)

	result.Success = true
	return finalize()
}

func (h *Handler) testResolveTarget(req TestRequest, result *TestResult) (*lifecycle.SSHTarget, error) {
	target, err := lifecycle.ResolveSSHTarget(lifecycle.SSHConnConfig{
		HostAlias:      req.HostAlias,
		Host:           req.Host,
		Port:           req.Port,
		User:           req.User,
		IdentitySource: lifecycle.SSHIdentitySource(req.IdentitySrc),
		IdentityFile:   req.IdentityFile,
		ProxyJump:      req.ProxyJump,
		// Deliberately empty PinnedFingerprint — first-connect / test mode.
	})
	if err != nil {
		result.Steps = append(result.Steps, TestStep{Name: "Resolve target", Success: false, Error: err.Error()})
		result.Error = err.Error()
		return nil, err
	}
	result.Steps = append(result.Steps, TestStep{
		Name:    "Resolve target",
		Success: true,
		Output:  fmt.Sprintf("%s@%s:%d (identity=%s)", target.User, target.Host, target.Port, target.IdentitySource),
	})
	return target, nil
}

func (h *Handler) testHandshake(
	ctx context.Context, target *lifecycle.SSHTarget, result *TestResult,
) (*ssh.Client, error) {
	stepStart := time.Now()
	client, err := lifecycle.DialSSH(ctx, target)
	step := TestStep{Name: "SSH handshake", DurationMs: time.Since(stepStart).Milliseconds()}
	if err != nil {
		step.Success = false
		step.Error = err.Error()
		result.Steps = append(result.Steps, step)
		result.Error = err.Error()
		return nil, err
	}
	step.Success = true
	result.Fingerprint = target.ObservedFingerprint
	step.Output = "fingerprint=" + result.Fingerprint
	result.Steps = append(result.Steps, step)
	return client, nil
}

func (h *Handler) testProbeAndArch(ctx context.Context, client *ssh.Client, result *TestResult) error {
	info, err := lifecycle.SSHProbeRemote(ctx, client)
	if err != nil {
		result.Steps = append(result.Steps, TestStep{Name: "Probe remote", Success: false, Error: err.Error()})
		result.Error = err.Error()
		return err
	}
	result.UnameAll = info.UnameAll
	result.Arch = info.Arch
	result.GitVersion = info.GitVer
	result.Steps = append(result.Steps, TestStep{Name: "Probe remote", Success: true, Output: info.UnameAll})

	if err := lifecycle.SSHRequireSupportedArch(info.Arch); err != nil {
		result.Steps = append(result.Steps, TestStep{Name: "Verify arch", Success: false, Error: err.Error()})
		result.Error = err.Error()
		return err
	}
	result.Steps = append(result.Steps, TestStep{Name: "Verify arch", Success: true, Output: info.Arch})
	return nil
}

// populateSessionMetadata copies SSH-specific keys from an ExecutorRunning row
// into a SessionRow. Pure mapping; no I/O. Lives here (not on the row) so the
// metadata key names stay co-located with their consumer.
func populateSessionMetadata(row *SessionRow, md map[string]interface{}) {
	if md == nil {
		return
	}
	row.Host, _ = md[lifecycle.MetadataKeySSHHost].(string)
	row.User, _ = md[lifecycle.MetadataKeySSHUser].(string)
	row.RemoteTaskDir, _ = md[lifecycle.MetadataKeySSHRemoteTaskDir].(string)
	row.RemoteAgentPort = intFromMetadata(md, lifecycle.MetadataKeySSHRemoteAgentctlPort)
	row.LocalForwardPort = intFromMetadata(md, lifecycle.MetadataKeySSHLocalForwardPort)
}

func intFromMetadata(md map[string]interface{}, key string) int {
	v, ok := md[key].(string)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func (h *Handler) testAgentctlCache(ctx context.Context, client *ssh.Client, result *TestResult) {
	upStepStart := time.Now()
	cached, err := lifecycle.SSHCheckAgentctlCached(ctx, client, h.resolver)
	upStep := TestStep{Name: "Verify agentctl cache", DurationMs: time.Since(upStepStart).Milliseconds()}
	if err != nil {
		upStep.Success = false
		upStep.Error = err.Error()
		result.Steps = append(result.Steps, upStep)
		result.AgentctlAction = "skipped"
		return
	}
	upStep.Success = true
	if cached {
		upStep.Output = "cached"
		result.AgentctlAction = "cached"
	} else {
		upStep.Output = "will upload on first launch"
		result.AgentctlAction = "uploaded"
	}
	result.Steps = append(result.Steps, upStep)
}

// listSessions returns the active SSH-runtime sessions for the given executor.
// Backed by the ExecutorRunning table; each row carries the SSH metadata we
// persisted in CreateInstance. The executor binding is resolved by looking up
// the TaskSession associated with each row — the row itself has an
// ExecutorID column but the lifecycle manager does not populate it for fresh
// executions, so it's unreliable as a filter.
func (h *Handler) listSessions(ctx context.Context, executorID string) ([]SessionRow, error) {
	rows, err := h.repo.ListExecutorsRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ssh sessions: %w", err)
	}
	out := make([]SessionRow, 0, len(rows))
	now := time.Now()
	for _, run := range rows {
		if run == nil {
			continue
		}
		if run.Runtime != string(models.ExecutorTypeSSH) {
			continue
		}
		if !h.sessionBelongsToExecutor(ctx, run, executorID) {
			continue
		}
		row := SessionRow{
			SessionID:     run.SessionID,
			TaskID:        run.TaskID,
			Status:        run.Status,
			CreatedAt:     run.CreatedAt.Format(time.RFC3339),
			UptimeSeconds: int64(now.Sub(run.CreatedAt).Seconds()),
		}
		populateSessionMetadata(&row, run.Metadata)
		out = append(out, row)
	}
	return out, nil
}

// sessionBelongsToExecutor returns true when the given ExecutorRunning row was
// launched under the given executor config. Resolves via the task_session
// row because ExecutorRunning.ExecutorID is unpopulated on fresh executions.
// Falls back to the row's own ExecutorID when the session lookup fails, so
// resume / recovery paths (where the row has it) still match.
func (h *Handler) sessionBelongsToExecutor(ctx context.Context, run *models.ExecutorRunning, executorID string) bool {
	if run.ExecutorID == executorID && executorID != "" {
		return true
	}
	if run.SessionID == "" {
		return false
	}
	session, err := h.repo.GetTaskSession(ctx, run.SessionID)
	if err != nil || session == nil {
		return false
	}
	return session.ExecutorID == executorID
}
