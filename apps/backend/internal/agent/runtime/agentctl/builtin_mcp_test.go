package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinInstanceCreationRequiresCompatibleHelper(t *testing.T) {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface,
		Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	for _, tc := range []struct {
		name       string
		capability bool
		version    int
		want       bool
	}{
		{"old helper", false, 0, false}, {"no catalog agreement", true, 0, false}, {"future catalog", true, 2, false}, {"supported", true, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					capabilities := []string{}
					if tc.capability {
						capabilities = append(capabilities, p.BuiltinPackagesCapability)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities, "builtin_mcp_catalog_version": tc.version})
					return
				}
				created++
				var request CreateInstanceRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, profile.BuiltinPackages, request.McpProfile.BuiltinPackages)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"instance","port":12345}`))
			}))
			t.Cleanup(server.Close)
			_, err := newTestControlClient(t, server).CreateInstance(context.Background(), &CreateInstanceRequest{TaskID: "task", McpProfile: &profile})
			if tc.want {
				require.NoError(t, err)
				require.Equal(t, 1, created)
			} else {
				require.Error(t, err)
				require.Zero(t, created)
			}
		})
	}
}
