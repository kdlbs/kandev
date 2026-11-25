package backendapp

import (
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/webapp"
	"github.com/stretchr/testify/require"
)

func TestSidebarGroupingBootRequiresDiscoveryIdentity(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	enabled := newEnabledAuthService(t, cfg)
	disabled := newDisabledAuthService(t)
	registry := plugins.NewRegistry()
	registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "backend-only", SidebarGroupings: []manifest.SidebarGrouping{{ID: "labels", Label: "Private provider", Action: "snapshot"}}, Actions: []manifest.Action{{Key: "snapshot", ResourceScope: manifest.ActionScopeWorkspace}}}, Status: store.StatusActive})
	svc := plugins.NewService(store.NewFSStore(t.TempDir()), registry, nil, testPluginsLogger(t))
	t.Cleanup(func() { _ = svc.Close() })
	require.Len(t, svc.SidebarGroupingCatalog(), 1)
	for _, mode := range []string{"anonymous", "signed-in", "auth-off"} {
		t.Run(mode, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/app-state", nil)
			p := routeParams{authSvc: enabled, services: &Services{Plugins: svc}}
			if mode == "signed-in" {
				req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "reader", Role: authn.RoleMember}))
			}
			if mode == "auth-off" {
				p.authSvc = disabled
			}
			payload := bootPayload(req.Context(), req, p, webapp.ClassifyRoute("/"))
			if mode == "anonymous" {
				require.Empty(t, payload.SidebarGroupings)
			} else {
				require.Len(t, payload.SidebarGroupings, 1)
			}
		})
	}
}
