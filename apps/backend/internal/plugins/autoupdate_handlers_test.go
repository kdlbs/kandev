package plugins

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/store"
)

func newTestRouterWithSettings(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	svc.SetSecrets(newFakeSecretRevealer())

	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	ss, err := newSettingsStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	svc.SetSettings(ss)

	router := gin.New()
	RegisterRoutes(router, svc, nil, testLogger(t))
	return router, svc
}

func TestGetSettingsReturnsDefault(t *testing.T) {
	router, _ := newTestRouterWithSettings(t)
	rec := doRequest(router, http.MethodGet, "/api/plugins/settings", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AutoUpdateDefault {
		t.Fatal("fresh settings should report auto_update_default=false")
	}
}

func TestUpdateSettingsPersistsDefault(t *testing.T) {
	router, _ := newTestRouterWithSettings(t)
	rec := doRequest(router, http.MethodPut, "/api/plugins/settings",
		`{"auto_update_default":true}`, map[string]string{"Content-Type": "application/json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /settings status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	rec = doRequest(router, http.MethodGet, "/api/plugins/settings", "", nil)
	var got Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.AutoUpdateDefault {
		t.Fatal("auto_update_default did not persist across PUT then GET")
	}
}

func TestSetAutoUpdateEndpointSetsAndClearsOverride(t *testing.T) {
	router, svc := newTestRouterWithSettings(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	// Set the override on.
	rec := doRequest(router, http.MethodPut, "/api/plugins/kandev-plugin-slack/auto-update",
		`{"auto_update":true}`, map[string]string{"Content-Type": "application/json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT auto-update status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got store.Record
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AutoUpdate == nil || !*got.AutoUpdate {
		t.Fatalf("response record AutoUpdate = %v, want *true", got.AutoUpdate)
	}

	// Clear it with an explicit null.
	rec = doRequest(router, http.MethodPut, "/api/plugins/kandev-plugin-slack/auto-update",
		`{"auto_update":null}`, map[string]string{"Content-Type": "application/json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT auto-update(null) status = %d, want 200", rec.Code)
	}
	got = store.Record{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AutoUpdate != nil {
		t.Fatalf("response record AutoUpdate = %v, want nil after clear", got.AutoUpdate)
	}
}

func TestSetAutoUpdateEndpointUnknownPluginIs404(t *testing.T) {
	router, _ := newTestRouterWithSettings(t)
	rec := doRequest(router, http.MethodPut, "/api/plugins/does-not-exist/auto-update",
		`{"auto_update":true}`, map[string]string{"Content-Type": "application/json"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PUT auto-update on unknown plugin status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}
