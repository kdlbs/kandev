package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestCIRunGrantEndpointAllowsWorkspaceOwnerAndListsGrants(t *testing.T) {
	service, _, _ := setupCIRunServiceTest(t, false)
	service.SetWorkspaceAuthorizer(func(ctx context.Context, workspaceID string) error {
		identity, _ := authn.IdentityFromContext(ctx)
		if workspaceID == "workspace-1" && identity.UserID == "owner-1" {
			return nil
		}
		return errors.New("workspace is not visible")
	})
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
		UserID: "owner-1", Role: authn.RoleMember,
	}))
	router.ServeHTTP(member, memberRequest)
	if member.Code != http.StatusCreated {
		t.Fatalf("owner status = %d body=%s", member.Code, member.Body.String())
	}

	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/github/ci-run-grants?workspace_id=workspace-1", nil)
	listRequest = listRequest.WithContext(authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember}))
	router.ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var grants []CIRunGrant
	if err := json.Unmarshal(list.Body.Bytes(), &grants); err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 {
		t.Fatalf("grants = %+v", grants)
	}
	grant := grants[0]
	if grant.ActorTaskID != "coordinator-1" || grant.TargetTaskID != "target-1" ||
		grant.WorkflowStepID != "ci-fixup" || grant.RepositoryID != "repository-1" {
		t.Fatalf("grant = %+v", grant)
	}
}
