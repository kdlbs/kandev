package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"

	"github.com/kandev/kandev/internal/orchestration/personas"
	"github.com/kandev/kandev/internal/orchestration/repository/sqlite"

	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testHandler(t *testing.T) (*gin.Engine, *sqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	profiles, _, err := settingsstore.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.New(db, db)
	err = repo.Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY); INSERT INTO workspaces(id) VALUES ('ws'),('other')`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = profiles.CreateAgent(ctx, &settingsmodels.Agent{ID: "claude", Name: "claude-acp"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*settingsmodels.AgentProfile{{ID: "personal", AgentID: "claude", Name: "Personal", Enabled: true, Model: "default"}, {ID: "work", AgentID: "claude", Name: "Work", Enabled: true, Model: "default"}, {ID: "disabled", AgentID: "claude", Name: "Disabled", Enabled: false, Model: "default"}, {ID: "foreign", AgentID: "claude", Name: "Foreign", Enabled: true, Model: "default", WorkspaceID: "other"}} {
		if err = profiles.CreateAgentProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := profiles.UpdateAgentProfileEnabled(ctx, "disabled", false); err != nil {
		t.Fatal(err)
	}
	svc := &personas.Service{Profiles: profiles, Repo: repo}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration"), &Handler{Registry: repo, Repo: repo, Agents: svc})
	return router, repo
}
func request(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func TestOrchestratorsUseProfilesAndScopeConfiguration(t *testing.T) {
	r, repo := testHandler(t)
	path := "/api/v1/orchestration/workspaces/ws/orchestrators"
	cfg := configuration{Name: "Chief", RoleID: "chief-of-staff", ProfileID: "personal", Instructions: "Coordinate work", Context: "Use task agents"}
	created := request(t, r, http.MethodPost, path, cfg)
	if created.Code != 201 {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	var first map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	id := first["id"].(string)
	cfg.Name = "Second"
	cfg.ProfileID = "work"
	second := request(t, r, http.MethodPost, path, cfg)
	if second.Code != 201 {
		t.Fatal(second.Body.String())
	}
	ids, err := repo.ListOrchestratorIDs(context.Background(), "ws")
	if err != nil || len(ids) != 2 {
		t.Fatalf("instances: %v %v", ids, err)
	}
	for _, profile := range []string{"foreign", "disabled", "missing"} {
		cfg.ProfileID = profile
		bad := request(t, r, http.MethodPost, path, cfg)
		if bad.Code != 400 {
			t.Fatalf("accepted %s: %d", profile, bad.Code)
		}
	}
	foreign := request(t, r, http.MethodGet, "/api/v1/orchestration/workspaces/other/orchestrators/"+id, nil)
	if foreign.Code != 404 {
		t.Fatalf("foreign access: %d", foreign.Code)
	}
	role := request(t, r, http.MethodDelete, "/api/v1/orchestration/roles/chief-of-staff", nil)
	if role.Code != 400 {
		t.Fatal("deleted role in use")
	}
	paused := request(t, r, http.MethodPost, path+"/"+id+"/status", map[string]string{"status": "paused"})
	if paused.Code != 200 {
		t.Fatal(paused.Body.String())
	}
	got := request(t, r, http.MethodGet, path+"/"+id, nil)
	if !bytes.Contains(got.Body.Bytes(), []byte(`"status":"paused"`)) {
		t.Fatal(got.Body.String())
	}
}

func TestImportPreservesAssistantIdentityAndRejectsOtherWorkspaces(t *testing.T) {
	r, repo := testHandler(t)
	ctx := context.Background()
	cfg := configuration{Name: "Existing chief", RoleID: "chief-of-staff", ProfileID: "personal", Instructions: "Coordinate existing tasks"}
	path := "/api/v1/orchestration/workspaces/ws/orchestrators"
	created := request(t, r, http.MethodPost, path, cfg)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var row struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnregisterOrchestrator(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	foreign := request(t, r, http.MethodPost, "/api/v1/orchestration/workspaces/other/import/"+row.ID, cfg)
	if foreign.Code != 404 {
		t.Fatalf("foreign import: %d", foreign.Code)
	}
	imported := request(t, r, http.MethodPost, "/api/v1/orchestration/workspaces/ws/import/"+row.ID, cfg)
	if imported.Code != 200 {
		t.Fatal(imported.Body.String())
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(imported.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != row.ID {
		t.Fatalf("identity changed: %s", got.ID)
	}
	duplicate := request(t, r, http.MethodPost, "/api/v1/orchestration/workspaces/ws/import/"+row.ID, cfg)
	if duplicate.Code != 409 {
		t.Fatalf("duplicate import: %d", duplicate.Code)
	}
}
