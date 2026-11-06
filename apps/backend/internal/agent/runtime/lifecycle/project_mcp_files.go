package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// projectMCPPlacement is the executor-local placement for one materialization.
// Remote placements are written and read back by the trusted helper operation;
// local placements use the backend file owner.
type projectMCPPlacement struct {
	remote     bool
	workspace  string
	coreURL    string
	generation uint64
	client     *agentctl.Client
}

func projectMCPUsesHelperFiles(execution *AgentExecution, executorType string) bool {
	if execution == nil {
		return false
	}
	runtimeName := execution.RuntimeName
	if runtimeName == agentruntime.RuntimeSSH || runtimeName.IsContainerized() {
		return true
	}
	if executorType == "" {
		return false
	}
	mapped := models.ExecutorType(executorType).Runtime()
	return mapped == agentruntime.RuntimeSSH || mapped.IsContainerized()
}

// resolveProjectMCPPlacement asks a remote helper for its own workspace and
// loopback MCP endpoint. A local placement preserves the existing standalone
// path, including the "skip when no port is available" behavior.
func (m *Manager) resolveProjectMCPPlacement(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	executorType string,
) (projectMCPPlacement, bool, error) {
	if !projectMCPUsesHelperFiles(execution, executorType) {
		port := passthroughMCPConfigPort(execution)
		if port <= 0 {
			return projectMCPPlacement{}, false, nil
		}
		return projectMCPPlacement{
			workspace: execution.WorkspacePath,
			coreURL:   m.agentMCPURL(execution, port, builtinCoreMCPPath),
			client:    client,
		}, true, nil
	}
	if client == nil {
		return projectMCPPlacement{}, false, errors.New("agent helper is unavailable for remote project MCP delivery")
	}
	if err := client.RequireCapability(ctx, agentctltypes.ProjectMCPDeliveryCapability); err != nil {
		return projectMCPPlacement{}, false, err
	}
	target, err := client.GetProjectMCP(ctx)
	if err != nil {
		return projectMCPPlacement{}, false, fmt.Errorf("resolve executor-local project MCP target: %w", err)
	}
	if target.InstanceID != client.InstanceID() || target.SessionID != execution.SessionID ||
		target.WorkspacePath == "" || target.MCPURL == "" {
		return projectMCPPlacement{}, false, errors.New("agent helper returned a mismatched project MCP target")
	}
	return projectMCPPlacement{
		remote:     true,
		workspace:  target.WorkspacePath,
		coreURL:    target.MCPURL,
		generation: target.Generation,
		client:     client,
	}, true, nil
}

func (m *Manager) projectMCPPaths(
	execution *AgentExecution,
	placement projectMCPPlacement,
) mcpconfig.PassthroughPaths {
	paths := m.passthroughMCPPaths(execution)
	paths.WorkspaceDir = placement.workspace
	paths.Unix = placement.remote
	return paths
}

// writeProjectMCPFiles materializes generated files on the placement target.
// Remote materialization is accepted only after the helper confirms its own
// identity, workspace, endpoint, generation, and read-back file fingerprints.
func (m *Manager) writeProjectMCPFiles(
	ctx context.Context,
	execution *AgentExecution,
	placement projectMCPPlacement,
	files []mcpconfig.PassthroughConfigFile,
) (bool, error) {
	if !placement.remote {
		return m.writePassthroughMCPFiles(ctx, execution, files)
	}
	requestFiles := make([]passthroughconfig.File, 0, len(files))
	for _, file := range files {
		if file.Path == "" {
			continue
		}
		requestFiles = append(requestFiles, passthroughconfig.File{
			Path: file.Path, Content: file.Content, MergeKey: file.MergeKey,
		})
	}
	if len(requestFiles) == 0 {
		return false, nil
	}
	request := agentctltypes.ProjectMCPPrepareRequest{
		Generation: placement.generation + 1,
		Files:      requestFiles,
	}
	response, err := placement.client.PrepareProjectMCP(ctx, request)
	if err != nil {
		if placement.generation == 0 {
			m.reconcileUncertainProjectMCPPreparation(ctx, execution, placement.client)
		}
		return false, fmt.Errorf("prepare project MCP files on the execution runtime: %w", err)
	}
	if err := validateProjectMCPPrepareResponse(execution, placement, response, request); err != nil {
		if placement.generation == 0 {
			m.reconcileUncertainProjectMCPPreparation(ctx, execution, placement.client)
		}
		return false, err
	}
	execution.setProjectMCPRemotePrepared(placement.client, response.Generation)
	return true, nil
}

func validateProjectMCPPrepareResponse(
	execution *AgentExecution,
	placement projectMCPPlacement,
	response *agentctltypes.ProjectMCPPrepareResponse,
	request agentctltypes.ProjectMCPPrepareRequest,
) error {
	if response == nil || response.Generation != request.Generation {
		return errors.New("execution runtime did not confirm the project MCP generation")
	}
	if response.InstanceID != placement.client.InstanceID() || response.SessionID != execution.SessionID {
		return errors.New("execution runtime returned a mismatched project MCP owner")
	}
	workspace := agentctltypes.CanonicalProjectMCPDeliveryWorkspace(placement.workspace)
	if agentctltypes.CanonicalProjectMCPDeliveryWorkspace(response.WorkspacePath) != workspace || response.MCPURL != placement.coreURL {
		return errors.New("execution runtime returned a mismatched project MCP placement")
	}
	evidence := make(map[string]string, len(response.Files))
	for _, file := range response.Files {
		if file.Path == "" || len(file.Fingerprint) != 64 {
			return errors.New("execution runtime returned invalid project MCP file evidence")
		}
		evidence[file.Path] = file.Fingerprint
	}
	for _, file := range request.Files {
		if evidence[file.Path] == "" {
			return fmt.Errorf("execution runtime did not confirm project MCP file %q", file.Path)
		}
	}
	return nil
}

// reconcileUncertainProjectMCPPreparation releases a first preparation whose
// response was lost or rejected. Later generations keep their prior claims and
// are replaced by the next successful preparation.
func (m *Manager) reconcileUncertainProjectMCPPreparation(ctx context.Context, execution *AgentExecution, client *agentctl.Client) {
	if client == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := client.CleanupProjectMCP(cleanupCtx); err != nil {
		m.logger.Warn("failed to reconcile uncertain project MCP preparation",
			zap.String("execution_id", execution.ID), zap.Error(err))
	}
}

// cleanupProjectMCPFiles releases remote project-file claims through the
// helper that prepared them. Callers must supply a client when the execution no
// longer holds one (for example while its lifecycle lock is already held).
func (m *Manager) cleanupProjectMCPFiles(ctx context.Context, execution *AgentExecution, fallback *agentctl.Client) error {
	if execution == nil {
		return nil
	}
	preparedClient, generation := execution.projectMCPRemotePrepared()
	if generation == 0 {
		return nil
	}
	client := preparedClient
	if client == nil {
		client = fallback
	}
	if client == nil {
		return errors.New("project MCP helper is unavailable for cleanup")
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := client.CleanupProjectMCP(cleanupCtx); err != nil {
		m.logger.Warn("failed to clean remote project MCP files",
			zap.String("execution_id", execution.ID), zap.Error(err))
		return err
	}
	execution.clearProjectMCPRemotePrepared()
	return nil
}
