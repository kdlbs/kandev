package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/marketplace"
)

// attachMarketplaceWithSource wires an in-memory marketplace onto svc whose
// built-in official source points at officialURL.
func attachMarketplaceWithSource(t *testing.T, svc *Service, officialURL string) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := marketplace.NewSourceStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("source store: %v", err)
	}
	if err := store.EnsureBuiltin(marketplace.OfficialSourceName, officialURL); err != nil {
		t.Fatalf("ensure builtin: %v", err)
	}
	svc.SetMarketplace(marketplace.NewService(store, testLogger(t)))
}

func fixtureIndexServer(t *testing.T) *httptest.Server {
	t.Helper()
	body := `{"schema_version":1,"generated_at":"2026-07-18T00:00:00Z",` +
		`"source":{"name":"Kandev Official","url":""},"plugins":[` +
		`{"id":"agent-stats","name":"Agent Stats","categories":["analytics"],` +
		`"version":"1.0.0","stars":42,"package_url":"https://ex/agent-stats-1.0.0.tar.gz"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMarketplaceCatalogUnavailableReturns503(t *testing.T) {
	router, _ := newTestRouter(t) // no marketplace attached
	rec := doRequest(router, http.MethodGet, "/api/plugins/marketplace", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestMarketplaceCatalogReturnsAnnotatedEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	attachMarketplaceWithSource(t, svc, fixtureIndexServer(t).URL)
	router := gin.New()
	RegisterRoutes(router, svc, nil, testLogger(t))

	rec := doRequest(router, http.MethodGet, "/api/plugins/marketplace?category=analytics", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var body marketplace.CatalogResult
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Plugins) != 1 || body.Plugins[0].ID != "agent-stats" {
		t.Fatalf("unexpected catalog plugins: %+v", body.Plugins)
	}
	if body.Plugins[0].InstallState != marketplace.StateAvailable {
		t.Fatalf("want available, got %q", body.Plugins[0].InstallState)
	}
	if len(body.Sources) != 1 || !body.Sources[0].Healthy {
		t.Fatalf("want 1 healthy source, got %+v", body.Sources)
	}
}

func TestMarketplaceSourceCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	attachMarketplaceWithSource(t, svc, "https://official.example/index.json")
	router := gin.New()
	RegisterRoutes(router, svc, nil, testLogger(t))

	// List returns the built-in source.
	list := doRequest(router, http.MethodGet, "/api/plugins/marketplace/sources", "", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", list.Code)
	}

	// Add a corporate source.
	add := doRequest(router, http.MethodPost, "/api/plugins/marketplace/sources",
		`{"name":"Acme","url":"https://acme.example/index.json"}`,
		map[string]string{"Content-Type": "application/json"})
	if add.Code != http.StatusCreated {
		t.Fatalf("add: want 201, got %d (%s)", add.Code, add.Body.String())
	}
	var added marketplace.SourceRecord
	if err := json.Unmarshal(add.Body.Bytes(), &added); err != nil {
		t.Fatalf("decode add: %v", err)
	}

	// Disable it.
	patch := doRequest(router, http.MethodPatch, "/api/plugins/marketplace/sources/"+added.ID,
		`{"enabled":false}`, map[string]string{"Content-Type": "application/json"})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d", patch.Code)
	}

	// Delete it.
	del := doRequest(router, http.MethodDelete, "/api/plugins/marketplace/sources/"+added.ID, "", nil)
	if del.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", del.Code)
	}
}

func TestMarketplaceDeleteBuiltinReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	attachMarketplaceWithSource(t, svc, "https://official.example/index.json")
	router := gin.New()
	RegisterRoutes(router, svc, nil, testLogger(t))

	sources, err := svc.Marketplace().Sources()
	if err != nil {
		t.Fatalf("sources: %v", err)
	}
	rec := doRequest(router, http.MethodDelete, "/api/plugins/marketplace/sources/"+sources[0].ID, "", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete builtin: want 409, got %d", rec.Code)
	}
}
