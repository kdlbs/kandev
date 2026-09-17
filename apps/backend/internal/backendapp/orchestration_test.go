package backendapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/config"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrchestrationFlagGuardsRuntimeAndLegacyRoutes(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	database := sqlx.NewDb(adapter.taskRepo.DB(), "sqlite3")
	repo, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := database.Exec(`INSERT INTO agents (id,name,created_at,updated_at) VALUES ('test-agent','test-agent',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	a := &officemodels.AgentInstance{ID: "orchestrator", AgentID: "test-agent", WorkspaceID: "ws-1", Name: "Chief", Role: officemodels.AgentRoleAssistant}
	if err = repo.CreateAgentInstance(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err = repo.RegisterOrchestrator(ctx, a.ID, a.WorkspaceID, "chief-of-staff"); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		allowed, e := orchestrationRunGuard(config.FeaturesConfig{Orchestration: enabled, Office: !enabled}, repo)(ctx, a.ID)
		if e != nil || allowed != enabled {
			t.Fatalf("flag %v: %v %v", enabled, allowed, e)
		}
	}
	legacy, e := orchestrationRunGuard(config.FeaturesConfig{Orchestration: true}, repo)(ctx, "legacy")
	if e != nil || legacy {
		t.Fatalf("legacy Office run enabled by orchestration: %v %v", legacy, e)
	}
	channel, err := repo.EnsureAgentConversation(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		router := gin.New()
		p := routeParams{features: config.FeaturesConfig{Orchestration: enabled, Office: !enabled}, officeRepo: repo, taskSvc: svc}
		g := router.Group("/api/v1/office", orchestrationCompatibilityGate(p))
		g.GET("/tasks/:id", func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/"+channel.TaskID, nil))
		want := 404
		if w.Code != want {
			t.Fatalf("conversation flag %v: %d", enabled, w.Code)
		}
	}
}
