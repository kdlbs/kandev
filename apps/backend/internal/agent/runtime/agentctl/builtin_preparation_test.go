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

func TestBuiltinWorkspaceBindingRequiresHelperAgreement(t *testing.T) {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	for _, result := range []string{"match", "different", "old"} {
		t.Run(result, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodPut, r.Method)
				require.Equal(t, "/api/v1/mcp/builtin-binding", r.URL.Path)
				if result == "old" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var received p.Context
				require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
				require.Equal(t, profile.BuiltinPackages, received.BuiltinPackages)
				if result == "different" {
					received.BuiltinPackages.TaskID = "other"
				}
				_ = json.NewEncoder(w).Encode(received)
			}))
			t.Cleanup(server.Close)
			err := newHTTPOnlyClient(server.URL).PrepareBuiltinMCP(context.Background(), profile)
			if result == "match" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestBuiltinVerificationRejectsMissingOrDifferentHelperBinding(t *testing.T) {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	for _, result := range []string{"match", "different", "old"} {
		t.Run(result, func(t *testing.T) {
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/mcp/builtin-binding", r.URL.Path)
				if result == "old" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				actual := p.Normalize(profile)
				if result == "different" {
					actual.BuiltinPackages.TaskID = "other"
				}
				_ = json.NewEncoder(w).Encode(actual)
			}))
			t.Cleanup(helper.Close)
			err := newHTTPOnlyClient(helper.URL).VerifyBuiltinMCP(context.Background(), profile)
			if result == "match" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
