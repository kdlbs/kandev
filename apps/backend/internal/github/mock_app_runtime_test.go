package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestEnterpriseMockAppRuntimeAuthorization(t *testing.T) {
	router, service, store := setupWorkspaceAuthMockController(t)
	for _, seed := range []struct{ path, body string }{
		{"/api/v1/github/mock/app-registrations/enterprise", `{"display_name":"Enterprise App","app_id":702,"github_host":"git.company.example"}`},
		{"/api/v1/github/mock/workspace-connections/workspace-1", `{"source":"github_app_installation","status":"active","github_host":"git.company.example","app_registration_id":"enterprise","installation_id":702,"installation_account_login":"acme","installation_account_type":"Organization"}`},
	} {
		response := serveMockJSON(t, router, http.MethodPut, seed.path, seed.body)
		if response.Code != http.StatusOK {
			t.Fatalf("seed: %d %s", response.Code, response.Body.String())
		}
	}
	response := serveMockJSON(t, router, http.MethodPost, "/api/v1/github/app/install/start", `{"workspace_id":"workspace-1","app_registration_id":"enterprise"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("installation start: %d %s", response.Code, response.Body.String())
	}
	var installation AppInstallationStart
	if err := json.Unmarshal(response.Body.Bytes(), &installation); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(installation.URL, "https://git.company.example/apps/mock-enterprise/installations/new?") {
		t.Fatal(installation.URL)
	}
	if _, err := service.CompleteAppInstallation(context.Background(), "enterprise", AppInstallationCallback{WorkspaceID: "workspace-1", UserID: DefaultUserID, State: installation.State, Code: "mock-code", InstallationID: 702}); err != nil {
		t.Fatal(err)
	}
	response = serveMockJSON(t, router, http.MethodPost, "/api/v1/github/personal-connection/start?workspace_id=workspace-1", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("personal start: %d %s", response.Code, response.Body.String())
	}
	var personal PersonalAuthStart
	if err := json.Unmarshal(response.Body.Bytes(), &personal); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(personal.URL, "https://git.company.example/login/oauth/authorize?") {
		t.Fatal(personal.URL)
	}
	if _, err := service.CompletePersonalAuth(context.Background(), "enterprise", PersonalAuthCallback{WorkspaceID: "workspace-1", UserID: DefaultUserID, State: personal.State, Code: "mock-code"}); err != nil {
		t.Fatal(err)
	}
	connection, err := store.GetUserConnection(context.Background(), "workspace-1", DefaultUserID)
	if err != nil || connection == nil || connection.AppRegistrationID != "enterprise" {
		t.Fatalf("personal connection = %#v, %v", connection, err)
	}
}
