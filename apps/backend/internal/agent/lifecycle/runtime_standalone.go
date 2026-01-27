package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// StandaloneRuntime implements Runtime for standalone agentctl execution.
// In this mode, a single agentctl control server manages multiple agent instances.
type StandaloneRuntime struct {
	ctl               *agentctl.ControlClient
	host              string
	port              int
	logger            *logger.Logger
	interactiveRunner *process.InteractiveRunner
}

// NewStandaloneRuntime creates a new standalone runtime.
func NewStandaloneRuntime(ctl *agentctl.ControlClient, host string, port int, log *logger.Logger) *StandaloneRuntime {
	return &StandaloneRuntime{
		ctl:    ctl,
		host:   host,
		port:   port,
		logger: log.WithFields(zap.String("runtime", "standalone")),
	}
}

func (r *StandaloneRuntime) Name() runtime.Name {
	return runtime.NameStandalone
}

func (r *StandaloneRuntime) HealthCheck(ctx context.Context) error {
	return r.ctl.Health(ctx)
}

func (r *StandaloneRuntime) waitForReady(ctx context.Context) error {
	if err := r.ctl.Health(ctx); err == nil {
		return nil
	}

	waitCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("agentctl not ready: %w", waitCtx.Err())
		case <-ticker.C:
			if err := r.ctl.Health(waitCtx); err == nil {
				return nil
			}
		}
	}
}

func (r *StandaloneRuntime) CreateInstance(ctx context.Context, req *RuntimeCreateRequest) (*RuntimeInstance, error) {
	if err := r.waitForReady(ctx); err != nil {
		return nil, err
	}

	// Build environment variables
	env := req.Env
	if env == nil {
		env = make(map[string]string)
	}
	env["KANDEV_TASK_ID"] = req.TaskID
	env["KANDEV_SESSION_ID"] = req.SessionID

	// Convert MCP server configs
	var mcpServers []agentctl.McpServerConfig
	for _, mcp := range req.McpServers {
		mcpServers = append(mcpServers, agentctl.McpServerConfig{
			Name:    mcp.Name,
			URL:     mcp.URL,
			Type:    mcp.Type,
			Command: mcp.Command,
			Args:    mcp.Args,
		})
	}

	// Create instance via control API
	// Agent command is NOT set - workspace access only. Agent is started explicitly via agentctl client.
	createReq := &agentctl.CreateInstanceRequest{
		ID:            req.InstanceID,
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  "", // Agent command set via Configure endpoint
		Protocol:      req.Protocol,
		Env:           env,
		AutoStart:     false,
		McpServers:    mcpServers,
	}

	resp, err := r.ctl.CreateInstance(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create standalone instance: %w", err)
	}

	// Create agentctl client pointing to the instance port
	client := agentctl.NewClient(r.host, resp.Port, r.logger)

	// Extract runtime-specific values from metadata
	worktreeID := getMetadataString(req.Metadata, MetadataKeyWorktreeID)
	worktreeBranch := getMetadataString(req.Metadata, MetadataKeyWorktreeBranch)

	// Build metadata
	metadata := make(map[string]interface{})
	metadata["standalone_port"] = resp.Port
	if worktreeID != "" {
		metadata["worktree_id"] = worktreeID
		metadata["worktree_path"] = req.WorkspacePath
		metadata["worktree_branch"] = worktreeBranch
	}

	r.logger.Debug("standalone instance created",
		zap.String("instance_id", req.InstanceID),
		zap.Int("port", resp.Port),
		zap.String("workspace", req.WorkspacePath))

	return &RuntimeInstance{
		InstanceID:           req.InstanceID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		RuntimeName:          string(r.Name()),
		Client:               client,
		StandaloneInstanceID: resp.ID,
		StandalonePort:       resp.Port,
		WorkspacePath:        req.WorkspacePath,
		Metadata:             metadata,
	}, nil
}

func (r *StandaloneRuntime) StopInstance(ctx context.Context, instance *RuntimeInstance, force bool) error {
	if instance.StandaloneInstanceID == "" {
		return nil // No standalone instance to stop
	}

	if err := r.ctl.DeleteInstance(ctx, instance.StandaloneInstanceID); err != nil {
		return fmt.Errorf("failed to stop standalone instance: %w", err)
	}

	return nil
}

func (r *StandaloneRuntime) RecoverInstances(ctx context.Context) ([]*RuntimeInstance, error) {
	// Standalone instances are not persisted - they are transient processes
	// managed by agentctl. Session resume will restart them as needed.
	return nil, nil
}

// SetInteractiveRunner sets the interactive runner for passthrough mode.
func (r *StandaloneRuntime) SetInteractiveRunner(runner *process.InteractiveRunner) {
	r.interactiveRunner = runner
}

// GetInteractiveRunner returns the interactive runner for passthrough mode.
func (r *StandaloneRuntime) GetInteractiveRunner() *process.InteractiveRunner {
	return r.interactiveRunner
}
