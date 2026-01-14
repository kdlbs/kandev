// Package instance provides utilities for managing multi-agent instances.
package instance

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/config"
	"github.com/kandev/kandev/internal/agentctl/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

// ServerFactory creates an HTTP handler for an instance given its config and process manager.
type ServerFactory func(cfg *config.Config, procMgr *process.Manager, log *logger.Logger) http.Handler

// Manager manages multiple agent instances.
// It handles creation, tracking, and removal of agent instances,
// each with their own HTTP server on a dedicated port.
type Manager struct {
	config        *config.MultiConfig
	logger        *logger.Logger
	instances     map[string]*Instance
	portAlloc     *PortAllocator
	serverFactory ServerFactory
	mu            sync.RWMutex
}

// NewManager creates a new instance manager.
func NewManager(cfg *config.MultiConfig, log *logger.Logger) *Manager {
	return &Manager{
		config:    cfg,
		logger:    log.WithFields(zap.String("component", "instance-manager")),
		instances: make(map[string]*Instance),
		portAlloc: NewPortAllocator(cfg.InstancePortBase, cfg.InstancePortMax),
	}
}

// SetServerFactory sets the factory function for creating HTTP handlers for instances.
// This must be called before creating any instances.
func (m *Manager) SetServerFactory(factory ServerFactory) {
	m.serverFactory = factory
}

// CreateInstance creates a new agent instance.
func (m *Manager) CreateInstance(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if max instances reached
	if len(m.instances) >= m.config.MaxInstances {
		return nil, fmt.Errorf("maximum number of instances (%d) reached", m.config.MaxInstances)
	}

	// Generate ID if not provided
	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}

	// Check if ID already exists
	if _, exists := m.instances[id]; exists {
		return nil, fmt.Errorf("instance with ID %s already exists", id)
	}

	// Allocate a port
	port, err := m.portAlloc.Allocate(id)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate port: %w", err)
	}

	// Determine agent command
	agentCmd := req.AgentCommand
	if agentCmd == "" {
		agentCmd = m.config.DefaultAgentCommand
	}

	// Determine protocol
	protocol := agent.Protocol(req.Protocol)
	if protocol == "" {
		protocol = m.config.DefaultProtocol
	}

	// Add workspace path flag based on protocol
	// Each agent/protocol has different flags for workspace path
	// Note: We also set cmd.Dir to the workspace path, so this is mainly for
	// agents that need an explicit flag rather than relying on cwd
	if req.WorkspacePath != "" {
		switch protocol {
		case agent.ProtocolACP:
			// Auggie uses --workspace-root
			if !strings.Contains(agentCmd, "--workspace-root") {
				agentCmd = agentCmd + " --workspace-root " + req.WorkspacePath
			}
		case agent.ProtocolCodex:
			// Codex app-server uses the current working directory (cmd.Dir)
			// No additional flags needed
		}
	}

	// Create config for the process manager
	instanceCfg := &config.Config{
		Port:                   port,
		Protocol:               protocol,
		AgentCommand:           agentCmd,
		WorkDir:                req.WorkspacePath,
		AutoStart:              req.AutoStart,
		AutoApprovePermissions: m.config.AutoApprovePermissions,
		LogLevel:               m.config.LogLevel,
		LogFormat:              m.config.LogFormat,
	}
	// Parse agent args
	instanceCfg.AgentArgs = strings.Fields(instanceCfg.AgentCommand)
	// Collect environment
	instanceCfg.AgentEnv = collectEnvForInstance(req.Env)

	// Create process manager
	procMgr := process.NewManager(instanceCfg, m.logger)

	// Create HTTP handler using factory
	var handler http.Handler
	if m.serverFactory != nil {
		handler = m.serverFactory(instanceCfg, procMgr, m.logger)
	} else {
		// Default to a simple handler if no factory is set
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("server factory not configured"))
		})
	}

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	// Start HTTP server in background
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("instance server error", zap.String("instance_id", id), zap.Error(err))
		}
	}()

	// Create and store instance
	inst := &Instance{
		ID:            id,
		Port:          port,
		Status:        "running",
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  agentCmd,
		Env:           req.Env,
		CreatedAt:     time.Now(),
		manager:       procMgr,
		server:        httpServer,
	}
	m.instances[id] = inst

	m.logger.Info("created instance",
		zap.String("instance_id", id),
		zap.Int("port", port),
		zap.String("workspace", req.WorkspacePath))

	return &CreateResponse{
		ID:   id,
		Port: port,
	}, nil
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
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[id]
	if !ok {
		return fmt.Errorf("instance %s not found", id)
	}

	// Stop the process manager
	if inst.manager != nil {
		if err := inst.manager.Stop(ctx); err != nil {
			m.logger.Warn("error stopping process manager",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}

	// Shutdown HTTP server
	if inst.server != nil {
		if err := inst.server.Shutdown(ctx); err != nil {
			m.logger.Warn("error shutting down HTTP server",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}

	// Release port
	m.portAlloc.Release(inst.Port)

	// Remove from instances map
	delete(m.instances, id)

	m.logger.Info("stopped instance",
		zap.String("instance_id", id),
		zap.Int("port", inst.Port))

	return nil
}

// Shutdown stops all instances gracefully.
func (m *Manager) Shutdown(ctx context.Context) error {
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

// collectEnvForInstance collects environment variables for an instance.
// It starts with os.Environ() (excluding AGENTCTL_* vars), then adds/overrides
// with the provided env map.
func collectEnvForInstance(env map[string]string) []string {
	// Start with current environment, excluding AGENTCTL_* vars
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			if !strings.HasPrefix(key, "AGENTCTL_") {
				envMap[key] = parts[1]
			}
		}
	}

	// Add/override with provided env
	for k, v := range env {
		envMap[k] = v
	}

	// Convert back to []string
	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

