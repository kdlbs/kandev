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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/netutil"
	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
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
	provisional   map[string]*provisionalInstance
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
	// caller vanishes mid-creation. They run off m.mu so a slow tracker stop
	// cannot block other operations; Shutdown drains them so a process exit
	// does not leave half-torn-down trackers behind.
	//
	// activeCreations counts creations that passed admission and have not yet
	// been published or handed to abandonWG. Startup runs without m.mu, so
	// Shutdown waits on it before draining abandonWG and snapshotting the maps.
	//
	// shuttingDown, guarded by mu, closes the window where Shutdown could
	// observe activeCreations at zero and a creation could then Add(1) behind
	// it: admission checks the flag and adds under m.mu, so every creation
	// either is counted before the flag is set or sees the flag and refuses.
	abandonWG       sync.WaitGroup
	activeCreations sync.WaitGroup
	shuttingDown    bool

	// afterTrackerStart runs immediately after StartAllWorkspaceTrackers and is
	// nil in production. It exists so a test can cancel the caller's context at
	// exactly that point and assert the abandonment branch ran, rather than
	// racing cancellation against tracker startup and accepting whichever
	// outcome it happens to get.
	afterTrackerStart func()

	// turnIDSeq allocates the turn identifiers retained terminal outcomes
	// are keyed by (AC-EXECUTORS-SURVIVAL-004.1). It is shared across every
	// instance this Manager supervises and lives for this process's whole
	// lifetime, matching the AC's "unique across every turn of every
	// instance the control server supervises for as long as that control
	// server runs" -- deliberately NOT reset per instance.
	turnIDSeq atomic.Int64

	// diskSampler is shared by every instance so concurrent tasks on the same
	// proven filesystem reuse one advisory capacity read per interval.
	diskSampler *systemmetrics.DiskCapacitySampler
}

type provisionalInstance struct {
	id       string
	lease    PortLease
	listener net.Listener
	procMgr  processManager

	cleanupMu      sync.Mutex
	listenerClosed bool
	processStopped bool
	cleaned        bool
	lastCleanupErr error
}

// NewManager creates a new instance manager.
// If cfg.IdleTimeout > 0, a background goroutine periodically reaps
// instances that have been idle (no in-flight HTTP requests and no
// activity) for the configured duration.
func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	// Clean up code-server processes orphaned by a previous session (safety net).
	process.CleanupOrphanedCodeServers(log)

	m := &Manager{
		config:      cfg,
		logger:      log.WithFields(zap.String("component", "instance-manager")),
		instances:   make(map[string]*Instance),
		provisional: make(map[string]*provisionalInstance),
		portAlloc:   NewPortAllocator(cfg.Ports.Base, cfg.Ports.Max),
		reaperStop:  make(chan struct{}),
		diskSampler: systemmetrics.NewDiskCapacitySampler(),
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

// createAdmission is an admitted creation: its ID is reserved in m.provisional
// and its port and listener are bound. The caller must publish or abandon it.
type createAdmission struct {
	bundle   *provisionalInstance
	agentEnv []string
}

// CreateInstance creates a new agent instance.
//
// Admission and publication hold m.mu briefly. Startup (process manager,
// comparison targets, workspace trackers) runs without it, so concurrent
// creations overlap and lookups of other instances do not wait for them. The
// provisional bundle keeps the instance ID and port exclusive meanwhile.
func (m *Manager) CreateInstance(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	if req == nil {
		return nil, errors.New("create instance request is required")
	}
	protocol := req.Protocol
	if protocol == "" {
		protocol = string(m.config.Defaults.Protocol)
	}
	if protocol == string(agent.ProtocolCodexAppServer) && !req.CodexAppServerEnabled {
		return nil, errors.New("codex app-server feature is disabled")
	}
	// createStart spans admission, startup and publication, so the diagnostic
	// agentctl_create_ready_ms metric (api.handleSystemMetrics) reports the
	// whole creation, including any wait for m.mu.
	createStart := time.Now()

	admission, err := m.admitCreate(ctx, req)
	if err != nil {
		return nil, err
	}
	bundle := admission.bundle
	cleanupPending := true
	defer func() {
		// Register the teardown before leaving activeCreations, so Shutdown's
		// abandonWG.Wait, which follows its activeCreations.Wait, covers it.
		if cleanupPending {
			m.abandonWG.Add(1)
			go func() {
				defer m.abandonWG.Done()
				if err := m.abandonPartialInstance(bundle); err != nil {
					m.logger.Warn("error cleaning up abandoned instance",
						zap.String("instance_id", bundle.id),
						zap.Error(err))
				}
			}()
		}
		m.activeCreations.Done()
	}()

	inst, handler, instanceCfg, err := m.startInstance(ctx, admission, req)
	if err != nil {
		return nil, err
	}
	if err := m.publishInstance(ctx, inst, bundle, handler); err != nil {
		return nil, err
	}
	cleanupPending = false

	// Clamp to a minimum of 1ms so a genuinely sub-millisecond creation can't
	// be stored as 0, which CreateReadyMillis's zero value reserves to mean
	// "not yet recorded".
	readyMillis := time.Since(createStart).Milliseconds()
	if readyMillis <= 0 {
		readyMillis = 1
	}
	instanceCfg.CreateReadyMillis.Store(readyMillis)

	m.logger.Info("created instance",
		zap.String("instance_id", inst.ID),
		zap.Int("port", inst.Port),
		zap.String("workspace", req.WorkspacePath))

	return &CreateResponse{
		ID:   inst.ID,
		Port: inst.Port,
	}, nil
}

// admitCreate is the serialized part of creation. It refuses during shutdown
// or for a caller that already gave up, rejects an ID that is published or
// still provisional, binds a port, and registers the provisional bundle.
func (m *Manager) admitCreate(ctx context.Context, req *CreateRequest) (*createAdmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Refuse once Shutdown has begun, so no creation can join activeCreations
	// after Shutdown started waiting on it.
	if m.shuttingDown {
		return nil, ErrManagerShuttingDown
	}

	// The caller may already be gone: callers give up at their client deadline
	// and retry. Building an instance for a caller that left leaks it: no caller
	// holds its ID, so nothing ever stops it, and it keeps a port, an HTTP server
	// and a full set of workspace trackers polling git. That extra polling makes
	// the next creation slower.
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

	agentEnv, err := config.CollectInstanceAgentEnvWithError(req.Env)
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
	if _, exists := m.provisional[id]; exists {
		return nil, fmt.Errorf("instance with ID %s is still cleaning up", id)
	}

	lease, listener, err := m.allocatePortAndListener(id)
	if err != nil {
		return nil, err
	}
	bundle := &provisionalInstance{id: id, lease: lease, listener: listener}
	m.provisional[id] = bundle
	m.activeCreations.Add(1)
	return &createAdmission{bundle: bundle, agentEnv: agentEnv}, nil
}

// startInstance builds an admitted instance without holding m.mu: its process
// manager, comparison targets, workspace trackers and HTTP handler. It returns
// the unpublished instance; publishInstance starts serving it.
func (m *Manager) startInstance(ctx context.Context, admission *createAdmission, req *CreateRequest) (*Instance, http.Handler, *config.InstanceConfig, error) {
	bundle := admission.bundle
	id := bundle.id
	port := bundle.lease.Port
	agentCmd := m.resolveAgentCommand(req)

	m.logger.Info("CreateInstance: received request",
		zap.String("req_protocol", req.Protocol),
		zap.String("workspace_path", req.WorkspacePath))

	overrides := m.buildInstanceOverrides(id, req, agentCmd, admission.agentEnv)

	m.logger.Info("CreateInstance: applying overrides",
		zap.String("override_protocol", string(overrides.Protocol)))

	// Create instance config using the unified method
	instanceCfg := m.config.NewInstanceConfig(port, overrides)

	m.logger.Info("CreateInstance: instance config created",
		zap.String("config_protocol", string(instanceCfg.Protocol)))

	// Create process manager
	procMgr := process.NewManager(instanceCfg, m.logger)
	procMgr.SetDiskCapacitySampler(m.diskSampler)
	bundle.setProcessManager(procMgr)
	// Wire retained-outcome recording (AC-EXECUTORS-SURVIVAL-004) before
	// anything that could reach Start(): this manager satisfies
	// process.TurnOutcomeRecorder via RetainTurnOutcome, and nothing can start
	// the process manager until CreateInstance publishes the instance, so
	// setting it here happens-before any terminal event the instance could
	// ever produce.
	procMgr.SetTurnOutcomeRecorder(id, m)
	// Materialize provider-qualified comparison targets before any tracker
	// polling starts. Failures remain explicit unavailable tracker state.
	procMgr.PrepareComparisonTargets(ctx)

	// Start root + per-repo trackers so file-change events fire even in passthrough mode.
	procMgr.StartAllWorkspaceTrackers(context.Background())
	if m.afterTrackerStart != nil {
		m.afterTrackerStart()
	}

	// Starting the trackers is the slow part of creation, so re-check: a caller
	// that was still waiting at admission can time out during it.
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("create instance abandoned during startup: %w", err)
	}

	// Create instance up-front so the activity middleware can reference it.
	inst := &Instance{
		ID:            id,
		Port:          port,
		lease:         bundle.lease,
		Status:        "running",
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  agentCmd,
		Env:           req.Env,
		CreatedAt:     time.Now(),
		SessionID:     req.SessionID,
		TaskID:        req.TaskID,
		manager:       procMgr,
		listenerDone:  make(chan struct{}),
	}
	inst.MarkActivity()

	handler := activityMiddleware(inst)(m.buildHTTPHandler(instanceCfg, procMgr))
	return inst, handler, instanceCfg, nil
}

// publishInstance starts serving an admitted instance and makes it visible,
// both under m.mu, so a published instance always has a running server and a
// creation refused during shutdown, or whose caller gave up while it waited
// for m.mu, never starts one.
func (m *Manager) publishInstance(ctx context.Context, inst *Instance, bundle *provisionalInstance, handler http.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shuttingDown {
		return ErrManagerShuttingDown
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create instance abandoned before publication: %w", err)
	}
	inst.server = m.startHTTPServer(inst, bundle.listener, handler)
	m.instances[inst.ID] = inst
	delete(m.provisional, inst.ID)
	return nil
}

func (m *Manager) buildInstanceOverrides(id string, req *CreateRequest, agentCmd string, agentEnv []string) *config.InstanceOverrides {
	autoStart := req.AutoStart
	return &config.InstanceOverrides{
		InstanceID:                 id,
		Protocol:                   agent.Protocol(req.Protocol),
		AgentCommand:               agentCmd,
		WorkDir:                    req.WorkspacePath,
		AutoStart:                  &autoStart,
		Env:                        agentEnv,
		AutoApprovePermissions:     req.AutoApprovePermissions,
		AgentType:                  req.AgentType,
		McpServers:                 m.buildMcpServerConfigs(req.McpServers),
		SessionID:                  req.SessionID,
		TaskID:                     req.TaskID,
		DisableAskQuestion:         req.DisableAskQuestion,
		PromptSuggestions:          req.PromptSuggestions,
		AssumeMcpSse:               req.AssumeMcpSse,
		AssumeMcpHttp:              req.AssumeMcpHttp,
		McpMode:                    req.McpMode,
		McpProviders:               req.McpProviders,
		McpProfile:                 req.McpProfile,
		NamespacesMCPToolsByServer: req.NamespacesMCPToolsByServer,
		RequiresProcessKill:        req.RequiresProcessKill,
		StripEnv:                   req.StripEnv,
		ProviderGatewayAuth:        req.ProviderGatewayAuth,
		BaseBranches:               req.BaseBranches,
		ComparisonTargets:          req.ComparisonTargets,
		RemoteContributions:        req.RemoteContributions,
		ContributionDestinations:   req.ContributionDestinations,
		WorkspaceSourceRoots:       req.WorkspaceSourceRoots,
		DurableJournalPath:         req.DurableJournalPath,
		DeliveryStreamID:           req.DeliveryStreamID,
		DeliveryIncarnationID:      req.DeliveryIncarnationID,
		DeliveryHarnessGeneration:  req.DeliveryHarnessGeneration,
	}
}

// setProcessManager stores the process manager under cleanupMu, the lock
// cleanup reads it under.
func (b *provisionalInstance) setProcessManager(mgr processManager) {
	b.cleanupMu.Lock()
	defer b.cleanupMu.Unlock()
	b.procMgr = mgr
}

// abandonPartialInstanceTimeout bounds the admission wait inside
// StopForTeardown for an instance whose caller vanished mid-creation. It does
// not bound the tracker stops themselves — WorkspaceTracker.Stop honours no
// context and falls back to its own stopTimeout — which is exactly why this
// teardown runs off the creation mutex rather than under it.
const abandonPartialInstanceTimeout = 5 * time.Second

// abandonPartialInstance unwinds one unregistered instance. Failed cleanup
// keeps the bundle and its lease available for a later retry.
func (m *Manager) abandonPartialInstance(bundle *provisionalInstance) error {
	ctx, cancel := context.WithTimeout(context.Background(), abandonPartialInstanceTimeout)
	defer cancel()
	return m.cleanupProvisionalInstance(ctx, bundle)
}

func (m *Manager) cleanupProvisionalInstance(ctx context.Context, bundle *provisionalInstance) error {
	bundle.cleanupMu.Lock()
	defer bundle.cleanupMu.Unlock()
	if bundle.cleaned {
		return nil
	}

	var cleanupErr error
	if bundle.procMgr != nil {
		bundle.procMgr.CloseAdmission()
	}
	if bundle.listener == nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("listener missing for abandoned instance %s", bundle.id))
	} else if !bundle.listenerClosed {
		if err := bundle.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("close listener for abandoned instance %s: %w", bundle.id, err))
		} else {
			bundle.listenerClosed = true
		}
	}
	if bundle.procMgr != nil && !bundle.processStopped {
		if err := bundle.procMgr.StopForTeardown(ctx); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("stop process manager for abandoned instance %s: %w", bundle.id, err))
		} else {
			bundle.processStopped = true
		}
	}
	if cleanupErr != nil {
		bundle.lastCleanupErr = cleanupErr
		return cleanupErr
	}

	bundle.lastCleanupErr = nil
	m.portAlloc.Release(bundle.lease)
	bundle.cleaned = true
	m.mu.Lock()
	if m.provisional[bundle.id] == bundle {
		delete(m.provisional, bundle.id)
	}
	m.mu.Unlock()

	m.logger.Warn("abandoned partially created instance",
		zap.String("instance_id", bundle.id),
		zap.Int("port", bundle.lease.Port))
	return nil
}

// allocatePortAndListener allocates a free port and binds a TCP listener to it.
func (m *Manager) allocatePortAndListener(id string) (PortLease, net.Listener, error) {
	maxAttempts := m.config.Ports.Max - m.config.Ports.Base + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lease, err := m.portAlloc.Allocate(id)
		if err != nil {
			return PortLease{}, nil, fmt.Errorf("failed to allocate port: %w", err)
		}
		// Bind loopback-only when auth is disabled (no token); otherwise bind
		// all interfaces so Docker/remote executors can reach the instance.
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", m.config.ListenHost(), lease.Port))
		if err != nil {
			if netutil.IsAddrInUse(err) {
				m.portAlloc.MarkUnavailable(lease)
				m.logger.Warn("port already in use; retrying",
					zap.String("instance_id", id),
					zap.Int("port", lease.Port))
				continue
			}
			m.portAlloc.Release(lease)
			return PortLease{}, nil, fmt.Errorf("failed to bind instance port %d: %w", lease.Port, err)
		}
		return lease, ln, nil
	}
	return PortLease{}, nil, fmt.Errorf("failed to allocate an available port for instance %s", id)
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
			Name:         mcp.Name,
			URL:          mcp.URL,
			Type:         mcp.Type,
			Command:      mcp.Command,
			Args:         mcp.Args,
			Env:          mcp.Env,
			Headers:      mcp.Headers,
			DefinitionID: mcp.DefinitionID, DefinitionRevision: mcp.DefinitionRevision,
			Origins: append([]agentctltypes.McpServerOrigin(nil), mcp.Origins...),
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
func (m *Manager) startHTTPServer(inst *Instance, listener net.Listener, handler http.Handler) *http.Server {
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", inst.Port),
		Handler: handler,
	}
	inst.listenerActive.Store(true)
	go func() {
		defer close(inst.listenerDone)
		defer inst.listenerActive.Store(false)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			m.logger.Error("instance server error",
				zap.String("instance_id", inst.ID),
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
	return m.stopInstance(ctx, id, inst)
}

func (m *Manager) stopInstance(ctx context.Context, id string, inst *Instance) error {
	inst.stopMu.Lock()
	defer inst.stopMu.Unlock()

	// A concurrent successful stop may have removed the instance while this
	// caller waited for the per-instance teardown lock.
	m.mu.Lock()
	current, exists := m.instances[id]
	if !exists {
		alreadyStopped := inst.portReleased
		m.mu.Unlock()
		if !alreadyStopped {
			return fmt.Errorf("instance %s not found", id)
		}
		m.logger.Debug("StopInstance already completed", zap.String("instance_id", id))
		return nil
	}
	if current != inst {
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
		m.portAlloc.Release(inst.lease)
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

	// Wait for admitted creations. Each one either publishes before the
	// snapshot below or is refused and registers its teardown on abandonWG
	// before it finishes, so the drain that follows covers it.
	m.activeCreations.Wait()

	// Drain any in-flight abandonment teardowns. They own trackers and a port
	// that are no longer reachable through m.instances, so nothing below would
	// wait for them otherwise.
	m.abandonWG.Wait()

	m.mu.Lock()
	ids := make([]string, 0, len(m.instances))
	for id := range m.instances {
		ids = append(ids, id)
	}
	provisional := make([]*provisionalInstance, 0, len(m.provisional))
	for _, bundle := range m.provisional {
		provisional = append(provisional, bundle)
	}
	m.mu.Unlock()

	var shutdownErr error
	for _, bundle := range provisional {
		if err := m.cleanupProvisionalInstance(ctx, bundle); err != nil {
			m.logger.Error("error retrying abandoned instance cleanup during shutdown",
				zap.String("instance_id", bundle.id),
				zap.Error(err))
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	for _, id := range ids {
		if err := m.StopInstance(ctx, id); err != nil {
			m.logger.Error("error stopping instance during shutdown",
				zap.String("instance_id", id),
				zap.Error(err))
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}

	return shutdownErr
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
