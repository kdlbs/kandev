package hostutility

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// Manager owns the per-agent-type warm agentctl instances used for boot probes,
// capability refresh, and sessionless utility execution.
//
// Lifecycle:
//   - Start(ctx): creates a process-scoped tmp parent dir, iterates enabled
//     ACP-capable inference agents, and for each creates an agentctl instance
//     (tmp subdir, no agent subprocess) and runs ProbeAgent in parallel.
//   - ExecutePrompt / RawPrompt: picks the instance for the agent type and runs
//     an ephemeral ACP session (same path as task-scoped utility calls).
//   - RefreshAgent: re-runs the probe against the existing instance.
//   - Stop(ctx): deletes each instance from agentctl and removes the tmp parent.
type Manager struct {
	registry      *registry.Registry
	controlHost   string
	controlPort   int
	controlClient *agentctlclient.ControlClient
	authToken     string // per-launch auth token for instance clients
	log           *logger.Logger

	parentTmpDir string
	cache        *cache

	mu          sync.RWMutex
	instances   map[string]*instance // keyed by agent type
	createGroup singleflight.Group
	startCancel context.CancelFunc
	stopped     bool
}

// instance is a single warm agentctl instance bound to an agent type.
type instance struct {
	agentType  string
	instanceID string
	workDir    string
	client     *agentctlclient.Client
}

// NewManager constructs a HostUtilityManager.
func NewManager(
	reg *registry.Registry,
	controlHost string,
	controlPort int,
	controlClient *agentctlclient.ControlClient,
	log *logger.Logger,
) *Manager {
	return &Manager{
		registry:      reg,
		controlHost:   controlHost,
		controlPort:   controlPort,
		controlClient: controlClient,
		log:           log.WithFields(zap.String("component", "host-utility")),
		cache:         newCache(),
		instances:     make(map[string]*instance),
	}
}

// SetAuthToken sets the per-launch auth token for authenticating instance clients.
func (m *Manager) SetAuthToken(token string) {
	m.authToken = token
}

// Start boots one warm instance per ACP-capable inference agent and runs an
// initial probe against each in parallel. Individual agent failures are
// captured in the cache but do not abort the other agents.
func (m *Manager) Start(ctx context.Context) error {
	startCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		cancel()
		return nil
	}
	m.startCancel = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.startCancel = nil
		m.mu.Unlock()
		cancel()
	}()

	// Create a process-scoped parent dir so concurrent kandev processes do not
	// share state, and so Stop only removes dirs owned by this process.
	parent, err := os.MkdirTemp("", fmt.Sprintf("kandev-host-utility-%d-*", os.Getpid()))
	if err != nil {
		return fmt.Errorf("create host utility tmp dir: %w", err)
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		if err := os.RemoveAll(parent); err != nil {
			m.log.Warn("failed to remove unused host utility parent tmp dir",
				zap.String("path", parent), zap.Error(err))
		}
		return nil
	}
	m.parentTmpDir = parent
	m.mu.Unlock()
	m.log.Info("host utility parent tmp dir created", zap.String("path", parent))

	targets := m.eligibleAgents()
	if len(targets) == 0 {
		m.log.Info("no ACP-capable inference agents enabled; host utility idle")
		return nil
	}

	g, gctx := errgroup.WithContext(startCtx)
	for _, ag := range targets {
		ag := ag
		g.Go(func() error {
			m.bootstrapAgent(gctx, ag)
			return nil // Never fail the group; per-agent failures land in cache.
		})
	}
	_ = g.Wait()
	return nil
}

// Stop deletes all warm instances and removes the process-scoped tmp parent.
// Only dirs owned by this process are removed; other kandev processes' dirs
// are untouched.
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	m.stopped = true
	cancel := m.startCancel
	m.startCancel = nil
	instances := make([]*instance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.instances = make(map[string]*instance)
	parentTmpDir := m.parentTmpDir
	m.parentTmpDir = ""
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	for _, inst := range instances {
		deleteCtx, cancel := hostUtilityDeleteContext(ctx)
		m.deleteInstance(deleteCtx, inst)
		cancel()
	}

	if parentTmpDir != "" {
		if err := os.RemoveAll(parentTmpDir); err != nil {
			m.log.Warn("failed to remove host utility parent tmp dir",
				zap.String("path", parentTmpDir), zap.Error(err))
		}
	}
}

const hostUtilityDeleteTimeout = 2 * time.Second

func hostUtilityDeleteContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), hostUtilityDeleteTimeout)
}

func (m *Manager) deleteInstance(ctx context.Context, inst *instance) {
	if inst == nil {
		return
	}
	if m.controlClient != nil {
		if err := m.controlClient.DeleteInstance(ctx, inst.instanceID); err != nil {
			m.log.Warn("failed to delete host utility instance",
				zap.String("agent_type", inst.agentType),
				zap.String("instance_id", inst.instanceID),
				zap.Error(err))
		}
	}
	if inst.workDir == "" {
		return
	}
	if err := os.RemoveAll(inst.workDir); err != nil {
		m.log.Warn("failed to remove host utility work dir",
			zap.String("agent_type", inst.agentType),
			zap.String("path", inst.workDir),
			zap.Error(err))
	}
}

// eligibleAgents returns enabled agents that implement InferenceAgent AND whose
// runtime protocol is ACP. This is the v1 scope.
func (m *Manager) eligibleAgents() []agents.InferenceAgent {
	all := m.registry.ListInferenceAgents()
	out := make([]agents.InferenceAgent, 0, len(all))
	for _, ia := range all {
		ag, ok := ia.(agents.Agent)
		if !ok {
			continue
		}
		rt := ag.Runtime()
		if rt == nil || rt.Protocol != agent.ProtocolACP {
			continue
		}
		out = append(out, ia)
	}
	return out
}

// bootstrapAgent creates a warm instance for one agent type and runs the
// initial probe. Failures are recorded in the cache with the appropriate
// status so the UI can surface them.
func (m *Manager) bootstrapAgent(ctx context.Context, ia agents.InferenceAgent) {
	ag := ia.(agents.Agent)
	agentType := ag.ID()
	log := m.log.WithFields(zap.String("agent_type", agentType))

	// Publish "probing" synchronously so the UI can distinguish "not started"
	// (cache miss) from "in flight".
	m.cache.set(AgentCapabilities{
		AgentType:     agentType,
		Status:        StatusProbing,
		LastCheckedAt: time.Now(),
	})

	cfg := ia.InferenceConfig()
	if cfg == nil || !cfg.Supported {
		m.cache.set(AgentCapabilities{
			AgentType:     agentType,
			Status:        StatusNotConfigured,
			Error:         "inference config not available",
			LastCheckedAt: time.Now(),
		})
		return
	}

	// Pre-check installation so we can skip expensive probes.
	if disc, err := ag.IsInstalled(ctx); err != nil || disc == nil || !disc.Available {
		msg := "agent not installed"
		if err != nil {
			msg = err.Error()
		}
		log.Info("skipping host utility bootstrap: agent not installed")
		m.cache.set(AgentCapabilities{
			AgentType:     agentType,
			Status:        StatusNotInstalled,
			Error:         msg,
			LastCheckedAt: time.Now(),
		})
		return
	}

	inst, err := m.createInstance(ctx, agentType)
	if err != nil {
		log.Warn("failed to create host utility instance", zap.Error(err))
		m.cache.set(AgentCapabilities{
			AgentType:     agentType,
			Status:        StatusFailed,
			Error:         err.Error(),
			LastCheckedAt: time.Now(),
		})
		return
	}

	if ctx.Err() != nil {
		deleteCtx, cancel := hostUtilityDeleteContext(ctx)
		m.deleteInstance(deleteCtx, inst)
		cancel()
		return
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		deleteCtx, cancel := hostUtilityDeleteContext(ctx)
		m.deleteInstance(deleteCtx, inst)
		cancel()
		return
	}
	m.instances[agentType] = inst
	m.mu.Unlock()

	caps := m.probe(ctx, inst, ia)
	m.cache.set(caps)
	log.Info("host utility bootstrap completed",
		zap.String("status", string(caps.Status)),
		zap.Int("models", len(caps.Models)),
		zap.Int("modes", len(caps.Modes)))
}

// safeAgentTypeName validates that the string is safe for use as a single
// filesystem path segment: only letters, digits, dash, and underscore. The
// ACP agent IDs registered in the agent registry always satisfy this (they
// are hardcoded Go identifiers), but we enforce it explicitly so CodeQL's
// taint analysis can see that the value cannot escape the parent tmp dir.
var safeAgentTypeName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// createInstance asks the control client to create a new workspace-only
// agentctl instance in a tmp subdirectory dedicated to this agent type.
func (m *Manager) createInstance(ctx context.Context, agentType string) (*instance, error) {
	if !safeAgentTypeName.MatchString(agentType) {
		return nil, fmt.Errorf("invalid agent type %q: must match %s", agentType, safeAgentTypeName.String())
	}
	m.mu.RLock()
	parentTmpDir := m.parentTmpDir
	stopped := m.stopped
	m.mu.RUnlock()
	if stopped {
		return nil, errManagerStopped
	}
	if parentTmpDir == "" {
		return nil, errors.New("host utility manager not started")
	}
	workDir := filepath.Join(parentTmpDir, agentType)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", workDir, err)
	}

	req := &agentctlclient.CreateInstanceRequest{
		WorkspacePath: workDir,
		AgentType:     agentType,
		// No AgentCommand / Protocol / AutoStart: the instance is workspace-only
		// and never runs a persistent agent subprocess. Probe/Prompt calls
		// spawn their own ephemeral ACP subprocesses via InferencePrompt/Probe.
	}
	resp, err := m.controlClient.CreateInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	client := agentctlclient.NewClient(m.controlHost, resp.Port, m.log,
		agentctlclient.WithExecutionID(resp.ID),
		agentctlclient.WithAuthToken(m.authToken))

	// Wait a moment for the instance HTTP server to come up.
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := waitForClientHealthy(healthCtx, client); err != nil {
		deleteCtx, deleteCancel := hostUtilityDeleteContext(ctx)
		m.deleteInstance(deleteCtx, &instance{
			agentType:  agentType,
			instanceID: resp.ID,
			workDir:    workDir,
			client:     client,
		})
		deleteCancel()
		return nil, fmt.Errorf("instance %s not healthy: %w", resp.ID, err)
	}

	return &instance{
		agentType:  agentType,
		instanceID: resp.ID,
		workDir:    workDir,
		client:     client,
	}, nil
}

func waitForClientHealthy(ctx context.Context, c *agentctlclient.Client) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := c.Health(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// errAgentNotInstalled is returned from getInstance when the agent CLI is not
// installed on the host. The Refresh path uses it to classify Status
// accurately instead of lumping every error into StatusFailed.
var errAgentNotInstalled = errors.New("agent not installed")
var errManagerStopped = errors.New("host utility manager stopped")

// getInstance returns the warm instance for the agent type, lazily recreating
// it if missing (e.g. after a previous failure or crash).
func (m *Manager) getInstance(ctx context.Context, agentType string) (*instance, agents.InferenceAgent, error) {
	ia, ok := m.registry.GetInferenceAgent(agentType)
	if !ok {
		return nil, nil, fmt.Errorf("agent %q not found or not inference-capable", agentType)
	}
	ag, ok := ia.(agents.Agent)
	if !ok {
		return nil, nil, fmt.Errorf("agent %q is not a full agent type", agentType)
	}
	if rt := ag.Runtime(); rt == nil || rt.Protocol != agent.ProtocolACP {
		return nil, nil, fmt.Errorf("agent %q is not ACP-capable", agentType)
	}

	m.mu.RLock()
	if m.stopped {
		m.mu.RUnlock()
		return nil, nil, errManagerStopped
	}
	inst := m.instances[agentType]
	parentTmpDir := m.parentTmpDir
	m.mu.RUnlock()
	if inst != nil {
		if err := m.staleInstanceCause(ctx, inst); err == nil {
			return inst, ia, nil
		} else if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
	}

	if parentTmpDir == "" {
		return nil, nil, errors.New("host utility manager not started")
	}

	// Serialize instance creation per agent type via singleflight so two
	// concurrent callers don't both spawn a process and then race to cache
	// the result.
	v, err, _ := m.createGroup.Do(agentType, func() (interface{}, error) {
		// Double-check inside the singleflight window.
		m.mu.RLock()
		existing := m.instances[agentType]
		m.mu.RUnlock()
		if existing != nil {
			if err := m.staleInstanceCause(ctx, existing); err == nil {
				return existing, nil
			} else if ctx.Err() != nil {
				return nil, ctx.Err()
			} else {
				m.dropStaleInstance(ctx, agentType, existing, err)
			}
		}
		// Pre-check installation so Refresh surfaces `not_installed`
		// instead of collapsing it into `failed` via createInstance errors.
		if disc, derr := ag.IsInstalled(ctx); derr != nil || disc == nil || !disc.Available {
			return nil, errAgentNotInstalled
		}
		created, cerr := m.createInstance(ctx, agentType)
		if cerr != nil {
			return nil, cerr
		}
		m.mu.Lock()
		if m.stopped {
			m.mu.Unlock()
			deleteCtx, cancel := hostUtilityDeleteContext(ctx)
			m.deleteInstance(deleteCtx, created)
			cancel()
			return nil, errManagerStopped
		}
		m.instances[agentType] = created
		m.mu.Unlock()
		return created, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return v.(*instance), ia, nil
}

const instanceHealthCheckTimeout = 500 * time.Millisecond

func (m *Manager) staleInstanceCause(ctx context.Context, inst *instance) error {
	if inst == nil || inst.client == nil {
		return errors.New("host utility instance client missing")
	}
	healthCtx, cancel := context.WithTimeout(ctx, instanceHealthCheckTimeout)
	defer cancel()
	err := inst.client.Health(healthCtx)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (m *Manager) dropStaleInstance(ctx context.Context, agentType string, inst *instance, cause error) {
	m.log.Warn("host utility instance unhealthy; recreating",
		zap.String("agent_type", agentType),
		zap.String("instance_id", inst.instanceID),
		zap.Error(cause))
	deleteCtx, cancel := hostUtilityDeleteContext(ctx)
	m.deleteInstance(deleteCtx, inst)
	cancel()

	m.mu.Lock()
	if current := m.instances[agentType]; current == inst {
		delete(m.instances, agentType)
	}
	m.mu.Unlock()
}

// probeTimeout caps each ACP probe so an agent that hangs (e.g. one that
// blocks waiting for interactive auth) doesn't keep its UI badge stuck on
// "Probing". 60s is generous enough for cold npm fetches and ACP handshake,
// short enough that the user sees a definitive Error/AuthRequired status
// quickly. Without this bound the only ceiling is the 5-minute HTTP client
// timeout on the agentctl call.
const probeTimeout = 60 * time.Second

// probe runs an ACP probe against the given instance and translates the result
// into an AgentCapabilities record suitable for the cache.
func (m *Manager) probe(ctx context.Context, inst *instance, ia agents.InferenceAgent) AgentCapabilities {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cfg := ia.InferenceConfig()
	req := &agentctlutil.ProbeRequest{
		AgentID: inst.agentType,
		InferenceConfig: &agentctlutil.InferenceConfigDTO{
			Command:   cfg.Command.Args(),
			ModelFlag: cfg.ModelFlag.Args(),
			WorkDir:   inst.workDir,
			StripEnv:  agents.StripEnvFor(ia),
		},
	}
	resp, err := inst.client.Probe(probeCtx, req)
	now := time.Now()
	if err != nil {
		return AgentCapabilities{
			AgentType:     inst.agentType,
			Status:        StatusFailed,
			Error:         err.Error(),
			LastCheckedAt: now,
		}
	}
	if !resp.Success {
		status := StatusFailed
		if isAuthError(resp.Error) {
			status = StatusAuthRequired
		}
		return AgentCapabilities{
			AgentType:     inst.agentType,
			Status:        status,
			Error:         resp.Error,
			DurationMs:    resp.DurationMs,
			LastCheckedAt: now,
		}
	}
	caps := AgentCapabilities{
		AgentType:       inst.agentType,
		AgentName:       resp.AgentName,
		AgentVersion:    resp.AgentVersion,
		Status:          StatusOK,
		ProtocolVersion: resp.ProtocolVersion,
		LoadSession:     resp.LoadSession,
		PromptCapabilities: PromptCapabilities{
			Image:           resp.PromptCapabilities.Image,
			Audio:           resp.PromptCapabilities.Audio,
			EmbeddedContext: resp.PromptCapabilities.EmbeddedContext,
		},
		CurrentModelID: resp.CurrentModelID,
		CurrentModeID:  resp.CurrentModeID,
		DurationMs:     resp.DurationMs,
		LastCheckedAt:  now,
	}
	for _, m := range resp.AuthMethods {
		caps.AuthMethods = append(caps.AuthMethods, AuthMethod{
			ID: m.ID, Name: m.Name, Description: m.Description, Meta: m.Meta,
		})
	}
	for _, m := range resp.Models {
		caps.Models = append(caps.Models, Model{
			ID: m.ID, Name: m.Name, Description: m.Description, Meta: m.Meta,
		})
	}
	for _, m := range resp.Modes {
		caps.Modes = append(caps.Modes, Mode{
			ID: m.ID, Name: m.Name, Description: m.Description, Meta: m.Meta,
		})
	}
	for _, opt := range resp.ConfigOptions {
		choices := make([]ConfigOptionChoice, 0, len(opt.Options))
		for _, choice := range opt.Options {
			choices = append(choices, ConfigOptionChoice{
				Value:       choice.Value,
				Name:        choice.Name,
				Description: choice.Description,
			})
		}
		caps.ConfigOptions = append(caps.ConfigOptions, ConfigOption{
			Type:         opt.Type,
			ID:           opt.ID,
			Name:         opt.Name,
			Description:  opt.Description,
			CurrentValue: opt.CurrentValue,
			Category:     opt.Category,
			Options:      choices,
		})
	}
	for _, c := range resp.Commands {
		caps.Commands = append(caps.Commands, Command{Name: c.Name, Description: c.Description})
	}
	return caps
}

// isAuthError is a coarse heuristic — ACP auth failures bubble up as string
// errors from the SDK without a distinct code. We match obvious markers so
// the UI can route the user to the auth flow; anything unmatched stays as
// StatusFailed.
func isAuthError(s string) bool {
	lower := strings.ToLower(s)
	for _, needle := range []string{"auth", "login", "unauthorized", "credential", "api key", "token"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
