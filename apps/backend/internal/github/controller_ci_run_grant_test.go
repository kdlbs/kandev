package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestCIRunGrantEndpointRequiresAdminAndCreatesExactScope(t *testing.T) {
	service, _, _ := setupCIRunServiceTest(t, false)
	if _, err := service.store.db.Exec(`DELETE FROM github_ci_run_grants`); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewController(service, nil).RegisterHTTPRoutes(router)
	body, _ := json.Marshal(CreateCIRunGrantInput{
		WorkspaceID: "workspace-1", ActorTaskID: "coordinator-1", TargetTaskID: "target-1",
		WorkflowID: "workflow-1", WorkflowStepID: "ci-fixup", RepositoryID: "repository-1",
	})

	member := httptest.NewRecorder()
	memberRequest := httptest.NewRequest(http.MethodPost, "/api/v1/github/ci-run-grants", bytes.NewReader(body))
	memberRequest.Header.Set("Content-Type", "application/json")
	memberRequest = memberRequest.WithContext(authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1", Role: authn.RoleMember,
	}))
	router.ServeHTTP(member, memberRequest)
	if member.Code != http.StatusForbidden {
		t.Fatalf("member status = %d", member.Code)
	}

	admin := httptest.NewRecorder()
	adminRequest := httptest.NewRequest(http.MethodPost, "/api/v1/github/ci-run-grants", bytes.NewReader(body))
	adminRequest.Header.Set("Content-Type", "application/json")
	adminRequest = adminRequest.WithContext(authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1", Role: authn.RoleAdmin,
	}))
	router.ServeHTTP(admin, adminRequest)
	if admin.Code != http.StatusCreated {
		t.Fatalf("admin status = %d body=%s", admin.Code, admin.Body.String())
	}
	var grant CIRunGrant
	if err := json.Unmarshal(admin.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if grant.ActorTaskID != "coordinator-1" || grant.TargetTaskID != "target-1" ||
		grant.WorkflowStepID != "ci-fixup" || grant.RepositoryID != "repository-1" {
		t.Fatalf("grant = %+v", grant)
	}
}
