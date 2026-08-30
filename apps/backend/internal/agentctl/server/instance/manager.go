// Package instance provides utilities for managing multi-agent instances.
package instance

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/netutil"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

// ServerFactory creates an HTTP handler for an instance given its config and process manager.
type ServerFactory func(cfg *config.InstanceConfig, procMgr *process.Manager, log *logger.Logger) http.Handler

const instanceHTTPShutdownGrace = 250 * time.Millisecond

// ErrManagerShuttingDown is returned by CreateInstance once Shutdown has begun.
// Callers get a clear refusal instead of an instance that the shutdown sequence
// has already walked past and will never stop.
var ErrManagerShuttingDown = errors.New("instance manager is shutting down")

// Manager manages multiple agent instances.
// It handles creation, tracking, and removal of agent instances,
// each with their own HTTP server on a dedicated port.
type Manager struct {
	config        *config.Config
	logger        *logger.Logger
	instances     map[string]*Instance
	portAlloc     *PortAllocator
	serverFactory ServerFactory
	mu            sync.RWMutex

	// reaperStop is closed by Shutdown to signal the idle reaper to exit.
	// reaperStopOnce serializes the close so concurrent Shutdown calls can't
	// double-close; sync.Once is used instead of m.mu because m.mu guards
	// the instances map and shouldn't gate a one-shot lifecycle signal.
	// reaperWG waits for the reaper goroutine to finish before Shutdown returns.
	reaperStop     chan struct{}
	reaperStopOnce sync.Once
	reaperWG       sync.WaitGroup

	// abandonWG tracks the teardown goroutines CreateInstance spawns when a
	// caller vanishes mid-creation. They run off the creation mutex so a slow
	// tracker stop cannot block the queue; Shutdown drains them so a process
	// exit does not leave half-torn-down trackers behind.
	//
	// shuttingDown, guarded by mu, closes the window where Shutdown could
	// observe the counter at zero and a CreateInstance already past its own
	// checks could then Add(1) behind it. Setting the flag under the same mutex
	// CreateInstance holds means every creation either completes before the
	// flag is set — so its Add is visible to the Wait below — or sees the flag
	// and refuses.
	abandonWG    sync.WaitGroup
	shuttingDown bool

	// afterTrackerStart runs immediately after StartAllWorkspaceTrackers and is
	// nil in production. It exists so a test can cancel the caller's context at
	// exactly that point and assert the abandonment branch ran, rather than
	// racing cancellation against tracker startup and accepting whichever
	// outcome it happens to get.
	afterTrackerStart func()
}

// NewManager creates a new instance manager.
// If cfg.IdleTimeout > 0, a background goroutine periodically reaps
// instances that have been idle (no in-flight HTTP requests and no
// activity) for the configured duration.
func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	// Clean up code-server processes orphaned by a previous session (safety net).
	process.CleanupOrphanedCodeServers(log)

	m := &Manager{
		config:     cfg,
		logger:     log.WithFields(zap.String("component", "instance-manager")),
		instances:  make(map[string]*Instance),
		portAlloc:  NewPortAllocator(cfg.Ports.Base, cfg.Ports.Max),
		reaperStop: make(chan struct{}),
	}

	if cfg.IdleTimeout > 0 {
		m.reaperWG.Add(1)
		go m.runIdleReaper(cfg.IdleTimeout, cfg.IdleReaperInterval)
	}

	return m
}

// SetServerFactory sets the factory function for creating HTTP handlers for instances.
// This must be called before creating any instances.
func (m *Manager) SetServerFactory(factory ServerFactory) {
	m.serverFactory = factory
}

// CreateInstance creates a new agent instance.
func (m *Manager) CreateInstance(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	// createStart includes the m.mu queue wait deliberately: that wait is the
	// leak pathology described below, and the diagnostic agentctl_create_ready_ms
	// metric (api.handleSystemMetrics) exists to make it visible.
	createStart := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Refuse once Shutdown has begun, so no creation can register an
	// abandonment teardown after Shutdown drained the wait group.
	if m.shuttingDown {
		return nil, ErrManagerShuttingDown
	}

	// The caller may already be gone. Creation is serialised on m.mu, the
	// control client gives up after 30s, and callers that give up retry — so
	// under load this lock grows a queue of requests nobody is waiting for any
	// more. Building an instance for one of those leaks it: no caller holds its
	// ID, so nothing ever stops it, and it keeps a port, an HTTP server and a
	// full set of workspace trackers polling git. That extra polling is what
	// makes the next creation slower, which lengthens the queue, which leaks
	// more instances.
	//
	// Seen in the field as create latency climbing 6ms -> 5.5s -> 34s -> 192s
	// with 16 live instances and zero deletions, by which point plain
	// `git ls-files` was hitting its 10s timeout and session resume failed.
	if err := ctx.Err(); err != nil {
		m.logger.Warn("create instance abandoned while queued; caller had already given up",
			zap.String("workspace_path", req.WorkspacePath),
			zap.Error(err))
		return nil, fmt.Errorf("create instance abandoned while queued: %w", err)
	}

	agentEnv, err := config.CollectAgentEnvWithError(req.Env)
	if err != nil {
		return nil, fmt.Errorf("prepare agent environment: %w", err)
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}
	if _, exists := m.instances[id]; exists {
		return nil, fmt.Errorf("instance with ID %s already exists", id)
	}

	port, listener, err := m.allocatePortAndListener(id)
	if err != nil {
		return nil, err
	}

	agentCmd := m.resolveAgentCommand(req)
	autoStart := req.AutoStart
	mcpServers := m.buildMcpServerConfigs(req.McpServers)

	m.logger.Info("CreateInstance: received request",
		zap.String("req_protocol", req.Protocol),
		zap.String("workspace_path", req.WorkspacePath))

	overrides := &config.InstanceOverrides{
		InstanceID:               id,
		Protocol:                 agent.Protocol(req.Protocol),
		AgentCommand:             agentCmd,
		WorkDir:                  req.WorkspacePath,
		AutoStart:                &autoStart,
		Env:                      agentEnv,
		AutoApprovePermissions:   req.AutoApprovePermissions,
		AgentType:                req.AgentType,
		McpServers:               mcpServers,
		SessionID:                req.SessionID,
		TaskID:                   req.TaskID,
		DisableAskQuestion:       req.DisableAskQuestion,
		AssumeMcpSse:             req.AssumeMcpSse,
		AssumeMcpHttp:            req.AssumeMcpHttp,
		McpMode:                  req.McpMode,
		McpProviders:             req.McpProviders,
		McpProfile:               req.McpProfile,
		RequiresProcessKill:      req.RequiresProcessKill,
		StripEnv:                 req.StripEnv,
		BaseBranches:             req.BaseBranches,
		ComparisonTargets:        req.ComparisonTargets,
		RemoteContributions:      req.RemoteContributions,
		ContributionDestinations: req.ContributionDestinations,
		WorkspaceSourceRoots:     req.WorkspaceSourceRoots,
	}

	m.logger.Info("CreateInstance: applying overrides",
		zap.String("override_protocol", string(overrides.Protocol)))

	// Create instance config using the unified method
	instanceCfg := m.config.NewInstanceConfig(port, overrides)

	m.logger.Info("CreateInstance: instance config created",
		zap.String("config_protocol", string(instanceCfg.Protocol)))

	// Create process manager
	procMgr := process.NewManager(instanceCfg, m.logger)
	// Materialize provider-qualified comparison targets before any tracker
	// polling starts. Failures remain explicit unavailable tracker state.
	procMgr.PrepareComparisonTargets(ctx)

	// Start root + per-repo trackers so file-change events fire even in passthrough mode.
	procMgr.StartAllWorkspaceTrackers(context.Background())
	if m.afterTrackerStart != nil {
		m.afterTrackerStart()
	}

	// Starting the trackers is the slow part of creation, so re-check: a caller
	// that was still waiting when we took the lock can time out during it.
	if err := ctx.Err(); err != nil {
		// Teardown runs off this goroutine because it must not happen under
		// m.mu. stopWorkspaceTrackers stops trackers one at a time, and
		// WorkspaceTracker.Stop takes no context — it waits on its own
		// stopTimeout timer — so a workspace with a dozen repositories could
		// pin the creation mutex for the better part of a minute, lengthening
		// the very queue this abandonment exists to drain.
		//
		// Nothing can observe the instance meanwhile: it was never added to
		// m.instances, and PortAllocator carries its own mutex. The wait group
		// lets Shutdown drain these before it returns.
		m.abandonWG.Add(1)
		go func() {
			defer m.abandonWG.Done()
			m.abandonPartialInstance(id, port, listener, procMgr)
		}()
		return nil, fmt.Errorf("create instance abandoned during startup: %w", err)
	}

	// Create instance up-front so the activity middleware can reference it.
	inst := &Instance{
		ID:            id,
		Port:          port,
		Status:        "running",
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  agentCmd,
		Env:           req.Env,
		CreatedAt:     time.Now(),
		manager:       procMgr,
	}
	inst.MarkActivity()

	handler := activityMiddleware(inst)(m.buildHTTPHandler(instanceCfg, procMgr))
	httpServer := m.startHTTPServer(port, listener, handler, id)
	inst.server = httpServer
	m.instances[id] = inst

	// Clamp to a minimum of 1ms so a genuinely sub-millisecond creation can't
	// be stored as 0, which CreateReadyMillis's zero value reserves to mean
	// "not yet recorded".
	readyMillis := time.Since(createStart).Milliseconds()
	if readyMillis <= 0 {
		readyMillis = 1
	}
	instanceCfg.CreateReadyMillis.Store(readyMillis)

	m.logger.Info("created instance",
		zap.String("instance_id", id),
		zap.Int("port", port),
		zap.String("workspace", req.WorkspacePath))

	return &CreateResponse{
		ID:   id,
		Port: port,
	}, nil
}

// abandonPartialInstanceTimeout bounds the admission wait inside
// StopForTeardown for an instance whose caller vanished mid-creation. It does
// not bound the tracker stops themselves — WorkspaceTracker.Stop honours no
// context and falls back to its own stopTimeout — which is exactly why this
// teardown runs off the creation mutex rather than under it.
const abandonPartialInstanceTimeout = 5 * time.Second

// abandonPartialInstance unwinds the pieces CreateInstance built before it
// noticed the caller had gone: the tracker goroutines, the bound listener and
// the allocated port. It must be called WITHOUT m.mu held — the tracker stops
// are slow and uninterruptible — and it does not route through StopInstance,
// which would take that lock. The instance was never registered in
// m.instances, so there is nothing there to remove.
func (m *Manager) abandonPartialInstance(id string, port int, listener net.Listener, procMgr *process.Manager) {
	// The request context is already cancelled, so teardown needs its own.
	ctx, cancel := context.WithTimeout(context.Background(), abandonPartialInstanceTimeout)
	defer cancel()

	// Give the port back first. Nothing was ever published on this listener, and
	// stopping the trackers below can take seconds — holding the port for that
	// long shrinks the pool exactly when creations are already piling up.
	if listener != nil {
		_ = listener.Close()
	}
	m.portAlloc.Release(port)

	if procMgr != nil {
		procMgr.CloseAdmission()
		if err := procMgr.StopForTeardown(ctx); err != nil {
			m.logger.Warn("error stopping process manager for abandoned instance",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}

	m.logger.Warn("abandoned partially created instance",
		zap.String("instance_id", id),
		zap.Int("port", port))
}

// allocatePortAndListener allocates a free port and binds a TCP listener to it.
func (m *Manager) allocatePortAndListener(id string) (int, net.Listener, error) {
	maxAttempts := m.config.Ports.Max - m.config.Ports.Base + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		allocated, err := m.portAlloc.Allocate(id)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to allocate port: %w", err)
		}
		// Bind loopback-only when auth is disabled (no token); otherwise bind
		// all interfaces so Docker/remote executors can reach the instance.
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", m.config.ListenHost(), allocated))
		if err != nil {
			if netutil.IsAddrInUse(err) {
				m.portAlloc.MarkUnavailable(allocated)
				m.logger.Warn("port already in use; retrying",
					zap.String("instance_id", id),
					zap.Int("port", allocated))
				continue
			}
			m.portAlloc.Release(allocated)
			return 0, nil, fmt.Errorf("failed to bind instance port %d: %w", allocated, err)
		}
		return allocated, ln, nil
	}
	return 0, nil, fmt.Errorf("failed to allocate an available port for instance %s", id)
}

// resolveAgentCommand returns the effective agent command for a create request.
func (m *Manager) resolveAgentCommand(req *CreateRequest) string {
	agentCmd := req.AgentCommand
	if agentCmd == "" {
		agentCmd = m.config.Defaults.AgentCommand
	}
	if req.WorkspacePath != "" && req.WorkspaceFlag != "" && !strings.Contains(agentCmd, req.WorkspaceFlag) {
		agentCmd = agentCmd + " " + req.WorkspaceFlag + " " + req.WorkspacePath
	}
	return agentCmd
}

// buildMcpServerConfigs converts instance McpServerConfig entries to
// config.McpServerConfig. Stdio entries whose Command can't be resolved
// (binary missing from PATH, no longer installed, etc.) are dropped with
// a warning so the agent doesn't spawn a permanently-broken child for an
// MCP it can never invoke. See GH issue #1247 — the `/snap/bin/brave`
// stale-MCP repro.
func (m *Manager) buildMcpServerConfigs(mcpServers []McpServerConfig) []config.McpServerConfig {
	result := make([]config.McpServerConfig, 0, len(mcpServers))
	for _, mcp := range mcpServers {
		if reason := mcpStdioValidationError(mcp); reason != "" {
			m.logger.Warn("dropping MCP server: stdio command unavailable",
				zap.String("mcp_name", mcp.Name),
				zap.String("command", mcp.Command),
				zap.String("reason", reason))
			continue
		}
		result = append(result, config.McpServerConfig{
			Name:    mcp.Name,
			URL:     mcp.URL,
			Type:    mcp.Type,
			Command: mcp.Command,
			Args:    mcp.Args,
			Env:     mcp.Env,
			Headers: mcp.Headers,
		})
	}
	return result
}

// mcpStdioValidationError returns an empty string when the MCP entry is
// either non-stdio (URL transport) or its stdio Command resolves on PATH.
// Otherwise it returns a human-readable reason the entry should be dropped.
//
// Caveat: PATH is resolved against the agentctl process's environment.
// In Docker and SSH executor modes the agent may launch in a different
// environment than agentctl, so a binary that lives only inside the
// container/remote host but not on the agentctl host will be dropped
// here even though it would have worked at agent runtime. For Standalone
// and Sprites this is unambiguously correct; for Docker/SSH it's an
// acceptable false positive — surfacing the warn log is better than
// spawning a permanently broken child every session (the `/snap/bin/brave`
// repro in GH issue #1247).
func mcpStdioValidationError(mcp McpServerConfig) string {
	// Non-stdio transports (sse, http, streamable_http) carry their endpoint
	// in URL — nothing to validate locally.
	if mcp.URL != "" {
		return ""
	}
	if mcp.Command == "" {
		return "stdio MCP entry has neither URL nor Command"
	}
	// Tolerate a compound `Command` string like "python3 -m mcp_server"
	// where the user collapsed Command+Args into Command. The first token
	// is the actual binary to look up; everything else is argv that the
	// agent will splice in later. This is more permissive than the
	// schema strictly allows, but it matches what real configs look like.
	bin := mcp.Command
	if i := strings.IndexAny(bin, " \t"); i > 0 {
		bin = bin[:i]
	}
	if _, err := exec.LookPath(bin); err != nil {
		return err.Error()
	}
	return ""
}

// buildHTTPHandler creates the HTTP handler for an instance.
func (m *Manager) buildHTTPHandler(instanceCfg *config.InstanceConfig, procMgr *process.Manager) http.Handler {
	if m.serverFactory != nil {
		return m.serverFactory(instanceCfg, procMgr, m.logger)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("server factory not configured")); err != nil {
			m.logger.Debug("failed to write default response", zap.Error(err))
		}
	})
}

// startHTTPServer creates and starts an HTTP server on the given listener.
func (m *Manager) startHTTPServer(port int, listener net.Listener, handler http.Handler, id string) *http.Server {
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			m.logger.Error("instance server error",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}()
	return httpServer
}

// GetInstance returns an instance by ID.
func (m *Manager) GetInstance(id string) (*Instance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[id]
	return inst, ok
}

// ListInstances returns info for all instances.
func (m *Manager) ListInstances() []*InstanceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*InstanceInfo, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst.Info())
	}
	return result
}

// StopInstance stops and removes an instance by ID.
func (m *Manager) StopInstance(ctx context.Context, id string) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("instance %s not found", id)
	}
	inst.stopMu.Lock()
	defer inst.stopMu.Unlock()

	// A concurrent successful stop may have removed the instance while this
	// caller waited for the per-instance teardown lock.
	m.mu.Lock()
	if current, exists := m.instances[id]; !exists || current != inst {
		m.mu.Unlock()
		return fmt.Errorf("instance %s not found", id)
	}
	inst.setStatus("stopping")
	m.mu.Unlock()

	m.logger.Debug("stopping instance", zap.String("instance_id", id))
	if inst.manager != nil {
		inst.manager.CloseAdmission()
	}

	// Quiesce HTTP before process teardown. CloseAdmission has already closed every
	// process-start admission path, including handlers already in flight.
	var httpStopErr error
	if inst.server != nil {
		httpStopErr = m.stopHTTPServer(ctx, id, inst.Port, inst.server)
	}
	// Stop the process manager (potentially slow, done without lock)
	var processStopErr error
	if inst.manager != nil {
		if err := inst.manager.StopForTeardown(ctx); err != nil {
			processStopErr = fmt.Errorf("stop process manager for instance %s: %w", id, err)
			m.logger.Warn("error stopping process manager",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}

	stopErr := errors.Join(httpStopErr, processStopErr)
	if httpStopErr != nil || processStopErr != nil {
		return stopErr
	}

	m.logger.Debug("StopInstance: releasing port",
		zap.String("instance_id", id),
		zap.Int("port", inst.Port))
	m.mu.Lock()
	if !inst.portReleased {
		m.portAlloc.Release(inst.Port)
		inst.portReleased = true
	}
	if stopErr == nil {
		delete(m.instances, id)
	}
	m.mu.Unlock()

	m.logger.Info("StopInstance completed",
		zap.String("instance_id", id),
		zap.Int("port", inst.Port))

	return stopErr
}

type instanceHTTPServer interface {
	Shutdown(context.Context) error
	Close() error
}

func (m *Manager) stopHTTPServer(ctx context.Context, id string, port int, server instanceHTTPServer) error {
	serverCtx, cancel := context.WithTimeout(ctx, instanceHTTPShutdownGrace)
	err := server.Shutdown(serverCtx)
	cancel()
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	m.logger.Debug("StopInstance: HTTP server graceful shutdown expired, closing active connections",
		zap.String("instance_id", id),
		zap.Int("port", port),
		zap.Duration("grace", instanceHTTPShutdownGrace),
		zap.Error(err))
	if closeErr := server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
		m.logger.Warn("error closing HTTP server",
			zap.String("instance_id", id),
			zap.Error(closeErr))
		return fmt.Errorf("close HTTP server for instance %s: %w", id, closeErr)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("shutdown HTTP server for instance %s: %w", id, err)
	}
	return nil
}

// Shutdown stops all instances gracefully.
func (m *Manager) Shutdown(ctx context.Context) error {
	// Stop the idle reaper first so it doesn't fire StopInstance concurrently
	// with our explicit per-instance shutdown loop.
	m.stopReaperOnce()

	// Refuse new creations before draining, so nothing can register another
	// teardown goroutine behind the Wait below.
	m.mu.Lock()
	m.shuttingDown = true
	m.mu.Unlock()

	// Drain any in-flight abandonment teardowns. They own trackers and a port
	// that are no longer reachable through m.instances, so nothing below would
	// wait for them otherwise.
	m.abandonWG.Wait()

	m.mu.Lock()
	ids := make([]string, 0, len(m.instances))
	for id := range m.instances {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var lastErr error
	for _, id := range ids {
		if err := m.StopInstance(ctx, id); err != nil {
			m.logger.Error("error stopping instance during shutdown",
				zap.String("instance_id", id),
				zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

// stopReaperOnce closes the reaper stop channel exactly once and waits for
// the reaper goroutine to drain. Safe to call multiple times.
//
// Note: reaperWG.Wait() blocks until the reaper finishes whatever sweep
// is currently in flight. Each instance in that sweep gets its own bounded
// context (idleReaperShutdownTimeout) independent of any caller deadline,
// so a Shutdown(ctx) caller with a tight deadline can find that the
// reaper drain consumed it before the main shutdown loop starts. The
// reaper polls reaperStop between instances, so the worst-case drain is
// one StopInstance round (15s), not N×timeout.
func (m *Manager) stopReaperOnce() {
	m.reaperStopOnce.Do(func() {
		close(m.reaperStop)
	})
	m.reaperWG.Wait()
}
