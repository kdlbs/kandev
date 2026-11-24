package plugins

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
)

func TestIdentityLinkRoutesScopeTheCallerAndRequireAdminForAllLinks(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "alice", Role: authn.RoleMember})
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	svc.registry.Add(&pluginstore.Record{
		Manifest:       manifest.Manifest{ID: "chat", Capabilities: manifest.Capabilities{APIWrite: []string{resourceHumanResponses}}},
		InstallationID: "chat-installation",
	})
	svc.registry.Add(&pluginstore.Record{Manifest: manifest.Manifest{ID: "plain"}, InstallationID: "plain-installation"})

	created := doRequest(router, http.MethodPost, "/api/plugins/chat/identity-links/codes", "", jsonHeaders())
	var code identityLinkCodeResponse
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &code) != nil || code.Code == "" || created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create code status=%d body=%s", created.Code, created.Body.String())
	}
	if all := doRequest(router, http.MethodGet, "/api/plugins/chat/identity-links?scope=all", "", nil); all.Code != http.StatusForbidden {
		t.Fatalf("member listing all links status = %d, want 403", all.Code)
	}
	if own := doRequest(router, http.MethodGet, "/api/plugins/chat/identity-links", "", nil); own.Code != http.StatusOK {
		t.Fatalf("own links status = %d body=%s", own.Code, own.Body.String())
	}
	if plain := doRequest(router, http.MethodPost, "/api/plugins/plain/identity-links/codes", "", jsonHeaders()); plain.Code != http.StatusConflict {
		t.Fatalf("code for a plugin without linked accounts status = %d, want 409", plain.Code)
	}
	if missing := doRequest(router, http.MethodDelete, "/api/plugins/chat/identity-links/unknown", "", nil); missing.Code != http.StatusNotFound {
		t.Fatalf("revoke unknown link status = %d, want 404", missing.Code)
	}
}
