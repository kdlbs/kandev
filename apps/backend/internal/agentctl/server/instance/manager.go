// Package instance provides utilities for managing multi-agent instances.
package instance

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

// ServerFactory creates an HTTP handler for an instance given its config and process manager.
type ServerFactory func(cfg *config.InstanceConfig, procMgr *process.Manager, log *logger.Logger) http.Handler

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
}

// NewManager creates a new instance manager.
func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	return &Manager{
		config:    cfg,
		logger:    log.WithFields(zap.String("component", "instance-manager")),
		instances: make(map[string]*Instance),
		portAlloc: NewPortAllocator(cfg.Ports.Base, cfg.Ports.Max),
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
	mcpServers := buildMcpServerConfigs(req.McpServers)

	m.logger.Info("CreateInstance: received request",
		zap.String("req_protocol", req.Protocol),
		zap.String("workspace_path", req.WorkspacePath))

	overrides := &config.InstanceOverrides{
		Protocol:     agent.Protocol(req.Protocol),
		AgentCommand: agentCmd,
		WorkDir:      req.WorkspacePath,
		AutoStart:    &autoStart,
		Env:          config.CollectAgentEnv(req.Env),
		AgentType:    req.AgentType,
		McpServers:   mcpServers,
		SessionID:    req.SessionID,
	}

	m.logger.Info("CreateInstance: applying overrides",
		zap.String("override_protocol", string(overrides.Protocol)))

	// Create instance config using the unified method
	instanceCfg := m.config.NewInstanceConfig(port, overrides)

	m.logger.Info("CreateInstance: instance config created",
		zap.String("config_protocol", string(instanceCfg.Protocol)))

	// Create process manager
	procMgr := process.NewManager(instanceCfg, m.logger)

	// Start workspace tracker immediately so process output can be streamed
	// even without an agent running (for dev server, etc.)
	procMgr.GetWorkspaceTracker().Start(context.Background())

	handler := m.buildHTTPHandler(instanceCfg, procMgr)
	httpServer := m.startHTTPServer(port, listener, handler, id)

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

// allocatePortAndListener allocates a free port and binds a TCP listener to it.
func (m *Manager) allocatePortAndListener(id string) (int, net.Listener, error) {
	maxAttempts := m.config.Ports.Max - m.config.Ports.Base + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		allocated, err := m.portAlloc.Allocate(id)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to allocate port: %w", err)
		}
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", allocated))
		if err != nil {
			if errors.Is(err, syscall.EADDRINUSE) || strings.Contains(err.Error(), "address already in use") {
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

// buildMcpServerConfigs converts instance McpServerConfig entries to config.McpServerConfig.
func buildMcpServerConfigs(mcpServers []McpServerConfig) []config.McpServerConfig {
	result := make([]config.McpServerConfig, 0, len(mcpServers))
	for _, mcp := range mcpServers {
		result = append(result, config.McpServerConfig{
			Name:    mcp.Name,
			URL:     mcp.URL,
			Type:    mcp.Type,
			Command: mcp.Command,
			Args:    mcp.Args,
		})
	}
	return result
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
	// Get instance and remove from map under lock (quick operation)
	m.mu.Lock()
	inst, ok := m.instances[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("instance %s not found", id)
	}
	// Remove from map immediately so new instances aren't blocked
	delete(m.instances, id)
	m.mu.Unlock()

	m.logger.Debug("stopping instance", zap.String("instance_id", id))

	// Stop the process manager (potentially slow, done without lock)
	if inst.manager != nil {
		if err := inst.manager.Stop(ctx); err != nil {
			m.logger.Warn("error stopping process manager",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}

	m.logger.Debug("StopInstance: shutting down HTTP server",
		zap.String("instance_id", id),
		zap.Int("port", inst.Port))

	// Shutdown HTTP server (potentially slow, done without lock)
	if inst.server != nil {
		if err := inst.server.Shutdown(ctx); err != nil {
			m.logger.Warn("error shutting down HTTP server",
				zap.String("instance_id", id),
				zap.Error(err))
		}
	}

	m.logger.Debug("StopInstance: releasing port",
		zap.String("instance_id", id),
		zap.Int("port", inst.Port))

	// Release port (quick operation, re-acquire lock)
	m.mu.Lock()
	m.portAlloc.Release(inst.Port)
	m.mu.Unlock()

	m.logger.Info("StopInstance completed",
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
