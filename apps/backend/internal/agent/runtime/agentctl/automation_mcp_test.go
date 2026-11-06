package client

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPHelperCapability(t *testing.T) {
	profile := p.NewAutomation()
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	identity := &IdentityInfo{Capabilities: []string{p.BuiltinPackagesCapability}, BuiltinMCPCatalogVersion: 1}
	require.Error(t, ValidateBuiltinHelper(&profile, "task", identity), "package capability alone does not prove automation support")
	identity.Capabilities = append(identity.Capabilities, p.AutomationMCPSelectionCapability)
	require.NoError(t, ValidateBuiltinHelper(&profile, "task", identity))
}
