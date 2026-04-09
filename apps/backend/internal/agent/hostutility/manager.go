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
	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"
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
	log           *logger.Logger

	parentTmpDir string
	cache        *cache

	mu          sync.RWMutex
	instances   map[string]*instance // keyed by agent type
	createGroup singleflight.Group
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

// Start boots one warm instance per ACP-capable inference agent and runs an
// initial probe against each in parallel. Individual agent failures are
// captured in the cache but do not abort the other agents.
func (m *Manager) Start(ctx context.Context) error {
	// Create a process-scoped parent dir so concurrent kandev processes do not
	// share state, and so Stop only removes dirs owned by this process.
	parent, err := os.MkdirTemp("", fmt.Sprintf("kandev-host-utility-%d-*", os.Getpid()))
	if err != nil {
		return fmt.Errorf("create host utility tmp dir: %w", err)
	}
	m.parentTmpDir = parent
	m.log.Info("host utility parent tmp dir created", zap.String("path", parent))

	targets := m.eligibleAgents()
	if len(targets) == 0 {
		m.log.Info("no ACP-capable inference agents enabled; host utility idle")
		return nil
	}

	g, gctx := errgroup.WithContext(ctx)
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
	instances := make([]*instance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.instances = make(map[string]*instance)
	m.mu.Unlock()

	for _, inst := range instances {
		if err := m.controlClient.DeleteInstance(ctx, inst.instanceID); err != nil {
			m.log.Warn("failed to delete host utility instance",
				zap.String("agent_type", inst.agentType),
				zap.String("instance_id", inst.instanceID),
				zap.Error(err))
		}
	}

	if m.parentTmpDir != "" {
		if err := os.RemoveAll(m.parentTmpDir); err != nil {
			m.log.Warn("failed to remove host utility parent tmp dir",
				zap.String("path", m.parentTmpDir), zap.Error(err))
		}
		m.parentTmpDir = ""
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

	m.mu.Lock()
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
	workDir := filepath.Join(m.parentTmpDir, agentType)
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

	client := agentctlclient.NewClient(m.controlHost, resp.Port, m.log)

	// Wait a moment for the instance HTTP server to come up.
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := waitForClientHealthy(healthCtx, client); err != nil {
		_ = m.controlClient.DeleteInstance(context.Background(), resp.ID)
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
	inst := m.instances[agentType]
	m.mu.RUnlock()
	if inst != nil {
		return inst, ia, nil
	}

	if m.parentTmpDir == "" {
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
			return existing, nil
		}
		created, cerr := m.createInstance(ctx, agentType)
		if cerr != nil {
			return nil, cerr
		}
		m.mu.Lock()
		m.instances[agentType] = created
		m.mu.Unlock()
		return created, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return v.(*instance), ia, nil
}

// probe runs an ACP probe against the given instance and translates the result
// into an AgentCapabilities record suitable for the cache.
func (m *Manager) probe(ctx context.Context, inst *instance, ia agents.InferenceAgent) AgentCapabilities {
	cfg := ia.InferenceConfig()
	req := &agentctlutil.ProbeRequest{
		AgentID: inst.agentType,
		InferenceConfig: &agentctlutil.InferenceConfigDTO{
			Command:   cfg.Command.Args(),
			ModelFlag: cfg.ModelFlag.Args(),
			WorkDir:   inst.workDir,
		},
	}
	resp, err := inst.client.Probe(ctx, req)
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
		caps.AuthMethods = append(caps.AuthMethods, AuthMethod(m))
	}
	for _, m := range resp.Models {
		caps.Models = append(caps.Models, Model(m))
	}
	for _, m := range resp.Modes {
		caps.Modes = append(caps.Modes, Mode(m))
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
