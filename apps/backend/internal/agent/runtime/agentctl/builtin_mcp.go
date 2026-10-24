package client

import (
	"context"
	"errors"
	"fmt"
	"slices"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (c *ControlClient) requireBuiltinPackages(ctx context.Context, request *CreateInstanceRequest) error {
	if request == nil || request.McpProfile == nil || request.McpProfile.BuiltinPackages == nil {
		return nil
	}
	identity, err := c.GetIdentity(ctx)
	if err != nil {
		return fmt.Errorf("check built-in MCP helper support: %w", err)
	}
	return ValidateBuiltinHelper(request.McpProfile, request.TaskID, identity)
}

// ValidateBuiltinHelper checks the shared contract for HTTP, SSH, and command transports.
func ValidateBuiltinHelper(profile *p.Context, taskID string, identity *IdentityInfo) error {
	if profile == nil || profile.BuiltinPackages == nil {
		return nil
	}
	if _, err := p.MarshalBuiltinBinding(*profile); err != nil {
		return err
	}
	if profile.BuiltinPackages.TaskID != taskID {
		return errors.New("built-in MCP binding belongs to another task")
	}
	if identity == nil || !slices.Contains(identity.Capabilities, p.BuiltinPackagesCapability) || identity.BuiltinMCPCatalogVersion != profile.BuiltinPackages.CatalogVersion {
		return errors.New("agentctl does not support the selected built-in MCP packages; upgrade the helper before starting this session")
	}
	return nil
}
