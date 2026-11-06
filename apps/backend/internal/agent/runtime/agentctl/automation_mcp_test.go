package client

import (
	"context"
	"encoding/json"
	"fmt"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
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

func TestAutomationMCPPrepareRequiresCapability(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(fmt.Sprint(supported), func(t *testing.T) {
			profile := p.NewAutomation()
			profile.BuiltinPackages = &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					capabilities := []string{p.BuiltinPackagesCapability}
					if supported {
						capabilities = append(capabilities, p.AutomationMCPSelectionCapability)
					}
					_ = json.NewEncoder(w).Encode(IdentityInfo{Capabilities: capabilities, BuiltinMCPCatalogVersion: 1})
					return
				}
				writes++
				_ = json.NewEncoder(w).Encode(profile)
			}))
			t.Cleanup(server.Close)
			err := newHTTPOnlyClient(server.URL).PrepareBuiltinMCP(context.Background(), profile)
			if supported {
				require.NoError(t, err)
				require.Equal(t, 1, writes)
			} else {
				require.ErrorContains(t, err, "upgrade")
				require.Zero(t, writes)
			}
		})
	}
}
