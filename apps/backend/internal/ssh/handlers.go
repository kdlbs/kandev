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

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
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

// ExecutorFetcher looks up a single executor by id. Needed by the
// agent-readiness probe to translate executor.Config into an SSHTarget.
type ExecutorFetcher interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

// AgentLister is the narrow slice of *registry.Registry that the readiness
// probe needs: walk every enabled agent so we can probe each one's first
// command-line token. Defined as an interface so handler tests can pass a
// fake set of agents without bringing the full registry in.
type AgentLister interface {
	ListEnabled() []agents.Agent
}

// Handler exposes /api/v1/ssh routes used by the settings UI.
type Handler struct {
	repo            ExecutorRunningLister
	executorFetcher ExecutorFetcher
	agents          AgentLister
	resolver        *lifecycle.AgentctlResolver
	logger          *logger.Logger
}

// NewHandler builds an SSH HTTP/WS handler.
func NewHandler(
	repo ExecutorRunningLister,
	executorFetcher ExecutorFetcher,
	agents AgentLister,
	resolver *lifecycle.AgentctlResolver,
	log *logger.Logger,
) *Handler {
	return &Handler{
		repo:            repo,
		executorFetcher: executorFetcher,
		agents:          agents,
		resolver:        resolver,
		logger:          log.WithFields(zap.String("component", "ssh-handler")),
	}
}

// RegisterRoutes wires the SSH routes onto the given gin engine + WS dispatcher.
func RegisterRoutes(
	router *gin.Engine,
	dispatcher *ws.Dispatcher,
	repo ExecutorRunningLister,
	executorFetcher ExecutorFetcher,
	registry *registry.Registry,
	resolver *lifecycle.AgentctlResolver,
	log *logger.Logger,
) {
	var agentLister AgentLister
	if registry != nil {
		agentLister = registry
	}
	h := NewHandler(repo, executorFetcher, agentLister, resolver, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1/ssh")
	api.POST("/test", h.httpTest)
	api.GET("/executors/:id/sessions", h.httpListSessions)
	api.POST("/executors/:id/probe-agents", h.httpProbeAgents)
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
	AgentctlAction  string     `json:"agentctl_action,omitempty"` // "cached" | "needs_upload" | "skipped"
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

// AgentReadinessRow reports the readiness of one agent on the remote host:
// whether the first token of agent.BuildCommand(...).Args() resolves on the
// remote's $PATH, and the install hint sourced from agent.InstallScript() so
// the UI can render a copy-button instead of forcing the user to look up
// per-agent install commands.
type AgentReadinessRow struct {
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name"`
	Binary      string `json:"binary"`
	Available   bool   `json:"available"`
	ResolvedAt  string `json:"resolved_at,omitempty"` // command -v stdout, when found
	InstallHint string `json:"install_hint,omitempty"`
	Error       string `json:"error,omitempty"`
}

// AgentReadinessResponse is the body of POST /api/v1/ssh/executors/:id/probe-agents.
type AgentReadinessResponse struct {
	Host       string              `json:"host"`
	DurationMs int64               `json:"duration_ms"`
	Rows       []AgentReadinessRow `json:"rows"`
}

// httpProbeAgents SSHs to the given executor's host and runs `command -v`
// for each enabled agent's first command-line token. Drives the "Available
// agents on this host" card on /settings/executors/ssh/:id.
func (h *Handler) httpProbeAgents(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: "executor id required"})
		return
	}
	if h.executorFetcher == nil || h.agents == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorJSONKey: "agent readiness not wired"})
		return
	}
	resp, status, err := h.probeAgents(c.Request.Context(), id)
	if err != nil {
		c.JSON(status, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// probeAgents is the testable core of httpProbeAgents.
func (h *Handler) probeAgents(ctx context.Context, executorID string) (*AgentReadinessResponse, int, error) {
	started := time.Now()
	executor, err := h.executorFetcher.GetExecutor(ctx, executorID)
	if err != nil {
		return nil, http.StatusNotFound, fmt.Errorf("executor %q not found: %w", executorID, err)
	}
	if executor == nil || executor.Type != models.ExecutorTypeSSH {
		return nil, http.StatusBadRequest, fmt.Errorf("executor %q is not an SSH executor", executorID)
	}
	target, err := sshTargetFromExecutorConfig(executor.Config)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	client, err := lifecycle.DialSSH(ctx, target)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("ssh: dial %s: %w", target.Host, err)
	}
	defer func() { _ = client.Close() }()

	rows := h.probeAgentsOnClient(ctx, client)
	return &AgentReadinessResponse{
		Host:       target.Host,
		DurationMs: time.Since(started).Milliseconds(),
		Rows:       rows,
	}, http.StatusOK, nil
}

// probeAgentsOnClient walks every enabled agent and runs command -v over
// the existing client. Public seam for a unit test that fakes the SSH side.
func (h *Handler) probeAgentsOnClient(ctx context.Context, client *ssh.Client) []AgentReadinessRow {
	enabled := h.agents.ListEnabled()
	rows := make([]AgentReadinessRow, 0, len(enabled))
	for _, ag := range enabled {
		rows = append(rows, probeOneAgent(ctx, client, ag))
	}
	return rows
}

func probeOneAgent(ctx context.Context, client *ssh.Client, ag agents.Agent) AgentReadinessRow {
	row := AgentReadinessRow{
		AgentID:     ag.ID(),
		AgentName:   ag.Name(),
		InstallHint: strings.TrimSpace(ag.InstallScript()),
	}
	cmd := ag.BuildCommand(agents.CommandOptions{Runtime: agentruntime.RuntimeSSH})
	args := cmd.Args()
	if len(args) == 0 {
		row.Error = "agent did not emit a command"
		return row
	}
	row.Binary = args[0]
	resolved, err := lifecycle.ProbeRemoteBinary(ctx, client, row.Binary)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	if resolved == "" {
		return row
	}
	row.Available = true
	row.ResolvedAt = resolved
	return row
}

// sshTargetFromExecutorConfig projects an executor.Config into the
// SSHConnConfig the dialer expects. Keeps the handler decoupled from
// lifecycle's internal metadata-vs-config representation.
func sshTargetFromExecutorConfig(cfg map[string]string) (*lifecycle.SSHTarget, error) {
	if cfg == nil {
		return nil, fmt.Errorf("ssh executor has no config")
	}
	port := 0
	if p := strings.TrimSpace(cfg["ssh_port"]); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid ssh_port %q", p)
		}
		port = n
	}
	return lifecycle.ResolveSSHTarget(lifecycle.SSHConnConfig{
		HostAlias:         cfg["ssh_host_alias"],
		Host:              cfg["ssh_host"],
		Port:              port,
		User:              cfg["ssh_user"],
		IdentitySource:    lifecycle.SSHIdentitySource(cfg["ssh_identity_source"]),
		IdentityFile:      cfg["ssh_identity_file"],
		ProxyJump:         cfg["ssh_proxy_jump"],
		PinnedFingerprint: cfg["ssh_host_fingerprint"],
	})
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
		// "needs_upload": the binary is absent on the remote and will be
		// uploaded the first time a session launches; the test endpoint
		// only probes for the sha256 sidecar, it doesn't push the binary.
		// Anything that wants to assert an upload actually happened needs
		// to drive a real launch.
		result.AgentctlAction = "needs_upload"
	}
	result.Steps = append(result.Steps, upStep)
}

// listSessions returns the active SSH-runtime sessions for the given executor.
// Backed by the ExecutorRunning table; each row carries the SSH metadata we
// persisted in CreateInstance. The executor binding is resolved by looking up
// the TaskSession associated with each row — the row itself has an
// ExecutorID column but the lifecycle manager does not populate it for fresh
// executions, so it's unreliable as a filter.
//
// The runtime filter runs before the session lookup, so the per-row
// GetTaskSession call only fires for SSH rows — typically <10 per executor.
// If that count ever grows materially we should add a batch List by
// session-ID method to the repo and resolve in one query.
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
		if run.Runtime != agentruntime.RuntimeSSH {
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
