package github

import (
	"context"
	"net/http"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1
func TestEnterpriseMockConnectionAndRegistrationHosts(t *testing.T) {
	router, service, store := setupWorkspaceAuthMockController(t)
	for _, step := range []struct{ path, body string }{
		{"/api/v1/github/mock/workspace-connections/workspace-1", `{"source":"pat","login":"alice","github_host":"git.example.com"}`},
		{"/api/v1/github/mock/app-registrations/enterprise", `{"display_name":"Enterprise App","app_id":42,"github_host":"git.example.com"}`},
	} {
		result := serveMockJSON(t, router, http.MethodPut, step.path, step.body)
		if result.Code != http.StatusOK {
			t.Fatalf("fixture setup %s: %d %s", step.path, result.Code, result.Body.String())
		}
	}
	connection, err := store.GetWorkspaceConnection(context.Background(), "workspace-1")
	if err != nil || connection.GitHubHost != "git.example.com" {
		t.Fatalf("mock connection host %+v, error %v", connection, err)
	}
	registration, err := store.GetAppRegistration(context.Background(), "enterprise")
	if err != nil || registration.GitHubHost != "git.example.com" {
		t.Fatalf("mock registration host %+v, error %v", registration, err)
	}
	result := serveMockJSON(t, router, http.MethodPut, "/api/v1/github/connections/workspace-1", `{"source":"pat","host":"git.example.com","token":"mock-pat:alice"}`)
	if result.Code != http.StatusOK {
		t.Fatalf("Enterprise mock Save: %d %s", result.Code, result.Body.String())
	}
	resolved, err := service.resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "workspace-1", Purpose: CredentialPurposeAutomation})
	if err != nil || resolved.GitHubHost != "git.example.com" {
		t.Fatalf("mock resolved host %+v, error %v", resolved, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3
func TestEnterpriseMockPersonalHostAndNonExpiringToken(t *testing.T) {
	router, service, _ := setupWorkspaceAuthMockController(t)
	for _, step := range []struct{ path, body string }{
		{"/api/v1/github/mock/app-registrations/enterprise-personal", `{"display_name":"Enterprise","app_id":43,"github_host":"git.example.com"}`},
		{"/api/v1/github/mock/workspace-connections/workspace-1", `{"source":"github_app_installation","app_registration_id":"enterprise-personal","installation_id":42,"installation_account_login":"acme","github_host":"git.example.com"}`},
		{"/api/v1/github/mock/personal-connections/workspace-1", `{"login":"alice","github_user_id":7,"status":"active"}`},
	} {
		response := serveMockJSON(t, router, http.MethodPut, step.path, step.body)
		if response.Code != http.StatusOK {
			t.Fatalf("setup %s: %d %s", step.path, response.Code, response.Body.String())
		}
	}
	resolved, err := service.resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "workspace-1", UserID: DefaultUserID, Purpose: CredentialPurposePersonalRead})
	if err != nil || resolved.GitHubHost != "git.example.com" || resolved.Principal.GitHubHost != "git.example.com" {
		t.Fatalf("mock personal identity: %+v %v", resolved, err)
	}
}
