package backendapp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	taskh "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestManagedSourceAttachmentReimportRegistration(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	log := newTestLogger()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "Workspace", OwnerID: "owner"}); err != nil {
		t.Fatal(err)
	}
	old := &models.Repository{ID: "legacy", WorkspaceID: "ws", Name: "Legacy", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "team", ProviderName: "repo", LocalPath: filepath.Join(t.TempDir(), "missing-legacy")}
	if err := repo.CreateRepository(ctx, old); err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: repo, RepoEntities: repo}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	router := gin.New()
	taskh.RegisterRepositoryRoutes(router, ws.NewDispatcher(), svc, log)
	body := `{"name":"Reimported","source_type":"provider","provider":"gitlab","provider_host":"https://gitlab.example","provider_owner":"team","provider_name":"repo"}`
	req := httptest.NewRequest("POST", "/api/v1/workspaces/ws/repositories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(authn.WithIdentity(ctx, authn.Identity{UserID: "owner", Role: authn.RoleMember}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != 201 {
		t.Fatalf("new registration: %d %s", recorder.Code, recorder.Body.String())
	}
	var created models.Repository
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == old.ID || created.ID == "" || created.LocalPath != "" || created.SourceType != "provider" {
		t.Fatalf("new current registration=%+v", created)
	}
	stored, err := repo.GetRepository(ctx, old.ID)
	if err != nil || stored.LocalPath != old.LocalPath {
		t.Fatalf("old registration changed: %+v %v", stored, err)
	}
}
