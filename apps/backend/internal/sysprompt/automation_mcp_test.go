package sysprompt

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPGuidance(t *testing.T) {
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
	guidance := builtinPackageGuidance(manifest)
	require.Contains(t, guidance, "private automation")
	require.Contains(t, guidance, "cannot change MCP access grants")
	require.NotContains(t, guidance, "A regular task keeps")
}
