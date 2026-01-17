package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
)

// StandaloneRuntime implements Runtime for standalone agentctl execution.
// In this mode, a single agentctl control server manages multiple agent instances.
type StandaloneRuntime struct {
	ctl    *agentctl.StandaloneCtl
	host   string
	port   int
	logger *logger.Logger
}

// NewStandaloneRuntime creates a new standalone runtime.
func NewStandaloneRuntime(ctl *agentctl.StandaloneCtl, host string, port int, log *logger.Logger) *StandaloneRuntime {
	return &StandaloneRuntime{
		ctl:    ctl,
		host:   host,
		port:   port,
		logger: log.WithFields(zap.String("runtime", "standalone")),
	}
}

func (r *StandaloneRuntime) Name() string {
	return "standalone"
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

	// Create instance via control API
	// Agent command is NOT set - workspace access only. Agent is started explicitly via agentctl client.
	createReq := &agentctl.CreateInstanceRequest{
		ID:            req.InstanceID,
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  "", // Agent command set via Configure endpoint
		Protocol:      req.Protocol,
		Env:           env,
		AutoStart:     false,
	}

	resp, err := r.ctl.CreateInstance(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create standalone instance: %w", err)
	}

	// Create agentctl client pointing to the instance port
	client := agentctl.NewClient(r.host, resp.Port, r.logger)

	// Build metadata
	metadata := make(map[string]interface{})
	metadata["standalone_port"] = resp.Port
	if req.WorktreeID != "" {
		metadata["worktree_id"] = req.WorktreeID
		metadata["worktree_path"] = req.WorkspacePath
		metadata["worktree_branch"] = req.WorktreeBranch
	}

	r.logger.Info("standalone instance created",
		zap.String("instance_id", req.InstanceID),
		zap.Int("port", resp.Port),
		zap.String("workspace", req.WorkspacePath))

	return &RuntimeInstance{
		InstanceID:           req.InstanceID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
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
