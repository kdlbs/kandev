package process

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// validateProjectMCPDelivery proves the record belongs to this helper, task,
// canonical workspace, and bound package manifest before configuration accepts
// it. It returns a private clone so request bytes cannot mutate accepted state.
func validateProjectMCPDelivery(
	cfg *config.InstanceConfig,
	delivery *types.ProjectMCPDelivery,
) (*types.ProjectMCPDelivery, error) {
	if cfg == nil || delivery == nil {
		return nil, errors.New("project MCP delivery evidence is missing")
	}
	if err := delivery.Validate(); err != nil {
		return nil, err
	}
	if delivery.Strategy == types.ProjectMCPDeliveryStrategyPi && cfg.AgentType != types.ProjectMCPDeliveryAgentPi {
		return nil, errors.New("project MCP delivery evidence does not match this agent")
	}
	if err := validateProjectMCPDeliveryOwner(cfg, delivery); err != nil {
		return nil, err
	}
	if cfg.McpProfile == nil || cfg.McpProfile.BuiltinPackages == nil {
		return nil, errors.New("project MCP delivery evidence has no bound package manifest")
	}
	digest, err := p.BuiltinBindingDigest(*cfg.McpProfile)
	if err != nil {
		return nil, fmt.Errorf("read bound built-in package manifest: %w", err)
	}
	if delivery.BindingDigest != digest {
		return nil, errors.New("project MCP delivery evidence has another package binding")
	}
	for _, raw := range delivery.Packages {
		if !slices.Contains(cfg.McpProfile.BuiltinPackages.Packages, p.PackageID(raw)) {
			return nil, fmt.Errorf("project MCP delivery evidence names unbound package %q", raw)
		}
	}
	return delivery.Clone(), nil
}

func validateProjectMCPDeliveryOwner(cfg *config.InstanceConfig, delivery *types.ProjectMCPDelivery) error {
	if cfg.InstanceID == "" || delivery.InstanceID != cfg.InstanceID {
		return errors.New("project MCP delivery evidence belongs to another agent helper")
	}
	if cfg.TaskID == "" || delivery.TaskID != cfg.TaskID {
		return errors.New("project MCP delivery evidence belongs to another task")
	}
	workspace, err := projectMCPDeliveryWorkspace(cfg.WorkDir)
	if err != nil {
		return err
	}
	if projectMCPDeliveryWorkspaceForCompare(delivery.Workspace) != projectMCPDeliveryWorkspaceForCompare(workspace) {
		return errors.New("project MCP delivery evidence belongs to another workspace")
	}
	return nil
}

// projectMCPDeliveryWorkspaceForCompare resolves an existing workspace so a
// backend-recorded path and the helper's resolved WorkDir agree across symlinks.
func projectMCPDeliveryWorkspaceForCompare(path string) string {
	canonical := types.CanonicalProjectMCPDeliveryWorkspace(path)
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		return types.CanonicalProjectMCPDeliveryWorkspace(resolved)
	}
	return canonical
}

func projectMCPDeliveryWorkspace(path string) (string, error) {
	workspace := types.CanonicalProjectMCPDeliveryWorkspace(path)
	if workspace == "" {
		return "", errors.New("project MCP delivery workspace is unavailable")
	}
	return workspace, nil
}

// acceptProjectMCPDelivery validates explicit evidence before accepted
// configuration state changes. An absent record clears previous evidence.
func (m *Manager) acceptProjectMCPDelivery(delivery *types.ProjectMCPDelivery) (*types.ProjectMCPDelivery, error) {
	if delivery == nil {
		return nil, nil
	}
	return validateProjectMCPDelivery(m.cfg, delivery)
}
