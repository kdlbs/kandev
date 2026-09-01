package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		UserID: "owner-1", Role: authn.RoleAdmin,
	}))
	router.ServeHTTP(member, memberRequest)
	if member.Code != http.StatusCreated {
		t.Fatalf("owner status = %d body=%s", member.Code, member.Body.String())
	}

	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/github/ci-run-grants?workspace_id=workspace-1", nil)
	listRequest = listRequest.WithContext(authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin}))
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

func TestCIRunGrantEndpointsRejectMembersForEveryOperation(t *testing.T) {
	service, _, _ := setupCIRunServiceTest(t, false)
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewController(service, nil).RegisterHTTPRoutes(router)
	body, _ := json.Marshal(CreateCIRunGrantInput{
		WorkspaceID: "workspace-1", ActorTaskID: "coordinator-1", TargetTaskID: "target-1",
		WorkflowID: "workflow-1", WorkflowStepID: "ci-fixup", RepositoryID: "repository-1",
	})
	memberContext := func(request *http.Request) *http.Request {
		return request.WithContext(authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1", Role: authn.RoleMember}))
	}
	requests := []*http.Request{
		memberContext(httptest.NewRequest(http.MethodPost, "/api/v1/github/ci-run-grants", bytes.NewReader(body))),
		memberContext(httptest.NewRequest(http.MethodGet, "/api/v1/github/ci-run-grants?workspace_id=workspace-1", nil)),
		memberContext(httptest.NewRequest(http.MethodDelete, "/api/v1/github/ci-run-grants/grant-1?workspace_id=workspace-1", nil)),
	}
	for _, request := range requests {
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("member operation status: method=%s status=%d body=%s", request.Method, recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "authenticated workspace administrator authorization is required") {
			t.Fatalf("member operation message: method=%s body=%s", request.Method, recorder.Body.String())
		}
	}
}

func TestCIRunGrantEndpointsRejectSyntheticMissingAndForeignIdentities(t *testing.T) {
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

	identities := []struct {
		name string
		id   *authn.Identity
	}{
		{name: "synthetic", id: &authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin, Synthetic: true}},
		{name: "missing"},
		{name: "foreign", id: &authn.Identity{UserID: "foreign-1", Role: authn.RoleAdmin}},
	}
	for _, tc := range identities {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/github/ci-run-grants", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			if tc.id != nil {
				request = request.WithContext(authn.WithIdentity(context.Background(), *tc.id))
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusCreated {
				t.Fatalf("unauthorized create succeeded: body=%s", recorder.Body.String())
			}
		})
	}

	for _, tc := range identities {
		t.Run(tc.name+" list", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/github/ci-run-grants?workspace_id=workspace-1", nil)
			if tc.id != nil {
				request = request.WithContext(authn.WithIdentity(context.Background(), *tc.id))
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK {
				t.Fatalf("unauthorized list succeeded: body=%s", recorder.Body.String())
			}
		})
	}

	for _, tc := range identities {
		t.Run(tc.name+" revoke", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/github/ci-run-grants/grant-1?workspace_id=workspace-1", nil)
			if tc.id != nil {
				request = request.WithContext(authn.WithIdentity(context.Background(), *tc.id))
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK {
				t.Fatalf("unauthorized revoke succeeded: body=%s", recorder.Body.String())
			}
		})
	}
}
