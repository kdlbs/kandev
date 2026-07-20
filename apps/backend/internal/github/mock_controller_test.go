package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupWorkspaceAuthMockController(t *testing.T) (*gin.Engine, *Service, *Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1", "workspace-2")
	mock := NewMockClient()
	svc := NewService(mock, AuthMethodPAT, nil, store, nil, newControllerTestLogger())
	svc.SetConnectionSecretStore(&fakeConnectionSecrets{values: make(map[string]string)})
	router := gin.New()
	NewController(svc, newControllerTestLogger()).RegisterHTTPRoutes(router)
	RegisterMockRoutes(router, svc, newControllerTestLogger())
	return router, svc, store
}

func serveMockJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestMockControllerWorkspaceConnectionsResolveIsolatedPrincipals(t *testing.T) {
	router, svc, _ := setupWorkspaceAuthMockController(t)
	first := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/mock/workspace-connections/workspace-1",
		`{"source":"github_app_installation","status":"active","installation_id":42,"installation_account_login":"acme","installation_account_type":"Organization","capabilities":{"pull_request_read":true}}`)
	if first.Code != http.StatusOK {
		t.Fatalf("seed app connection: %d %s", first.Code, first.Body.String())
	}
	second := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/mock/workspace-connections/workspace-2",
		`{"source":"gh_cli","status":"active","login":"alice","capabilities":{"pull_request_write":true}}`)
	if second.Code != http.StatusOK {
		t.Fatalf("seed CLI connection: %d %s", second.Code, second.Body.String())
	}

	app, err := svc.GetWorkspaceAuthStatus(context.Background(), "workspace-1", DefaultUserID)
	if err != nil || app.Automation == nil || app.Automation.Actor == nil {
		t.Fatalf("app status = %+v, err = %v", app, err)
	}
	if app.Automation.Actor.Kind != AuthPrincipalApp || app.Automation.Actor.Login != "acme" ||
		!app.Automation.Capabilities[CapabilityPullRequestRead] || app.Automation.Capabilities[CapabilityPullRequestWrite] {
		t.Fatalf("app automation = %+v", app.Automation)
	}
	cli, err := svc.GetWorkspaceAuthStatus(context.Background(), "workspace-2", DefaultUserID)
	if err != nil || cli.Automation == nil || cli.Automation.Actor == nil ||
		cli.Automation.Actor.Kind != AuthPrincipalHuman || cli.Automation.Actor.Login != "alice" {
		t.Fatalf("CLI status = %+v, err = %v", cli, err)
	}
}

func TestMockControllerPersonalCLIAvailabilityTransitionsAndReset(t *testing.T) {
	router, svc, store := setupWorkspaceAuthMockController(t)
	seed := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/mock/workspace-connections/workspace-1",
		`{"source":"github_app_installation","status":"active","installation_id":42,"installation_account_login":"acme"}`)
	if seed.Code != http.StatusOK {
		t.Fatalf("seed workspace: %d %s", seed.Code, seed.Body.String())
	}
	personal := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/mock/personal-connections/workspace-1",
		`{"login":"bob","status":"active","github_user_id":7,"access_expires_at":"2030-01-01T00:00:00Z"}`)
	if personal.Code != http.StatusOK {
		t.Fatalf("seed personal connection: %d %s", personal.Code, personal.Body.String())
	}
	accounts := serveMockJSON(t, router, http.MethodPut, "/api/v1/github/mock/cli-accounts",
		`{"accounts":[{"host":"github.com","login":"alice","active":true,"state":"success"}]}`)
	if accounts.Code != http.StatusOK {
		t.Fatalf("seed CLI accounts: %d %s", accounts.Code, accounts.Body.String())
	}
	available := serveMockJSON(t, router, http.MethodPut, "/api/v1/github/mock/app-available", `{"available":true}`)
	if available.Code != http.StatusOK {
		t.Fatalf("set App availability: %d %s", available.Code, available.Body.String())
	}

	status, err := svc.GetWorkspaceAuthStatus(context.Background(), "workspace-1", DefaultUserID)
	if err != nil || status.EffectivePersonalActor == nil || status.EffectivePersonalActor.Login != "bob" ||
		!status.GitHubAppAvailable {
		t.Fatalf("workspace status = %+v, err = %v", status, err)
	}
	listed := serveMockJSON(t, router, http.MethodGet, "/api/v1/github/auth/gh-cli/accounts", "")
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"login":"alice"`)) {
		t.Fatalf("list CLI accounts: %d %s", listed.Code, listed.Body.String())
	}
	pat := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/workspace-connection?workspace_id=workspace-2",
		`{"source":"pat","token":"mock-pat:carol"}`)
	if pat.Code != http.StatusOK {
		t.Fatalf("configure mock PAT: %d %s", pat.Code, pat.Body.String())
	}
	patStatus, err := svc.GetWorkspaceAuthStatus(context.Background(), "workspace-2", DefaultUserID)
	if err != nil || patStatus.Automation == nil || patStatus.Automation.Actor == nil ||
		patStatus.Automation.Actor.Login != "carol" {
		t.Fatalf("configured PAT status = %+v, err = %v", patStatus, err)
	}
	configured := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/workspace-connection?workspace_id=workspace-2",
		`{"source":"gh_cli","host":"github.com","login":"alice"}`)
	if configured.Code != http.StatusOK {
		t.Fatalf("configure injected CLI account: %d %s", configured.Code, configured.Body.String())
	}
	cliStatus, err := svc.GetWorkspaceAuthStatus(context.Background(), "workspace-2", DefaultUserID)
	if err != nil || cliStatus.Automation == nil || cliStatus.Automation.Actor == nil ||
		cliStatus.Automation.Actor.Login != "alice" {
		t.Fatalf("configured CLI status = %+v, err = %v", cliStatus, err)
	}
	transition := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/mock/workspace-connections/workspace-1/status", `{"status":"suspended"}`)
	if transition.Code != http.StatusOK {
		t.Fatalf("transition connection: %d %s", transition.Code, transition.Body.String())
	}
	status, err = svc.GetWorkspaceAuthStatus(context.Background(), "workspace-1", DefaultUserID)
	if err != nil || status.Authenticated || status.Automation.Status != ConnectionStatusSuspended {
		t.Fatalf("suspended status = %+v, err = %v", status, err)
	}

	reset := serveMockJSON(t, router, http.MethodDelete, "/api/v1/github/mock/reset", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", reset.Code, reset.Body.String())
	}
	connection, err := store.GetWorkspaceConnection(context.Background(), "workspace-1")
	if err != nil || connection != nil {
		t.Fatalf("connection after reset = %+v, err = %v", connection, err)
	}
	status, err = svc.GetWorkspaceAuthStatus(context.Background(), "workspace-1", DefaultUserID)
	if err != nil || status.GitHubAppAvailable || status.Personal != nil {
		t.Fatalf("status after reset = %+v, err = %v", status, err)
	}
}

func TestMockControllerResetRemovesBoundDeploymentApp(t *testing.T) {
	router, service, store := setupWorkspaceAuthMockController(t)
	repository := NewDeploymentAppRepository(store, service.connectionSecrets)
	registration := testDeploymentAppRegistration(1)
	if err := repository.SaveManagedRegistration(context.Background(), registration,
		DeploymentAppCredentials{
			PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook",
		}); err != nil {
		t.Fatal(err)
	}
	service.SetDeploymentAppRegistrationService(NewDeploymentAppRegistrationService(
		DeploymentAppRegistrationConfig{
			Repository: repository,
			Store:      store,
			Runtime:    service,
		},
	))
	service.deploymentAppRuntime = &githubAppRuntime{
		source: DeploymentAppSourceManaged, appID: registration.AppID, generation: 1,
	}
	flow := &DeploymentAppRegistrationFlow{
		StateHash: "pending-reset-flow", OperatorUserID: DefaultUserID,
		OwnerType: DeploymentAppOwnerOrganization, OwnerLogin: "acme",
		PublicBaseURL: "https://kandev.example", ManifestRevision: DeploymentAppManifestRevision,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateDeploymentAppRegistrationFlow(context.Background(), flow); err != nil {
		t.Fatal(err)
	}
	seed := serveMockJSON(t, router, http.MethodPut,
		"/api/v1/github/mock/workspace-connections/workspace-1",
		`{"source":"github_app_installation","status":"active","installation_id":42,"installation_account_login":"acme"}`)
	if seed.Code != http.StatusOK {
		t.Fatalf("seed App binding: %d %s", seed.Code, seed.Body.String())
	}

	reset := serveMockJSON(t, router, http.MethodDelete, "/api/v1/github/mock/reset", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", reset.Code, reset.Body.String())
	}
	connection, err := store.GetWorkspaceConnection(context.Background(), "workspace-1")
	if err != nil || connection != nil {
		t.Fatalf("connection after reset = %+v, error = %v", connection, err)
	}
	stored, _, err := repository.LoadManagedRegistration(context.Background())
	if err != nil || stored != nil || service.DeploymentAppRuntimeSnapshot().Ready {
		t.Fatalf("deployment App after reset = %+v, runtime = %+v, error = %v",
			stored, service.DeploymentAppRuntimeSnapshot(), err)
	}
	storedFlow, err := store.GetDeploymentAppRegistrationFlow(
		context.Background(), flow.StateHash,
	)
	if err != nil || storedFlow != nil {
		t.Fatalf("deployment App flow after reset = %+v, error = %v", storedFlow, err)
	}
}

func TestMockControllerDeletesWorkspaceAndPersonalConnections(t *testing.T) {
	router, _, store := setupWorkspaceAuthMockController(t)
	for _, seed := range []struct {
		path string
		body string
	}{
		{
			path: "/api/v1/github/mock/workspace-connections/workspace-1",
			body: `{"source":"github_app_installation","status":"active","installation_id":42,"installation_account_login":"acme"}`,
		},
		{
			path: "/api/v1/github/mock/personal-connections/workspace-1",
			body: `{"login":"bob","status":"active","github_user_id":7,"access_expires_at":"2030-01-01T00:00:00Z"}`,
		},
	} {
		response := serveMockJSON(t, router, http.MethodPut, seed.path, seed.body)
		if response.Code != http.StatusOK {
			t.Fatalf("seed %s: %d %s", seed.path, response.Code, response.Body.String())
		}
	}
	personal := serveMockJSON(t, router, http.MethodDelete,
		"/api/v1/github/mock/personal-connections/workspace-1", "")
	if personal.Code != http.StatusOK {
		t.Fatalf("delete personal: %d %s", personal.Code, personal.Body.String())
	}
	if got, err := store.GetUserConnection(context.Background(), "workspace-1", DefaultUserID); err != nil || got != nil {
		t.Fatalf("personal after delete = %+v, err = %v", got, err)
	}
	workspace := serveMockJSON(t, router, http.MethodDelete,
		"/api/v1/github/mock/workspace-connections/workspace-1", "")
	if workspace.Code != http.StatusOK {
		t.Fatalf("delete workspace: %d %s", workspace.Code, workspace.Body.String())
	}
	if got, err := store.GetWorkspaceConnection(context.Background(), "workspace-1"); err != nil || got != nil {
		t.Fatalf("workspace after delete = %+v, err = %v", got, err)
	}
}

func setupMockControllerTestForAddIssues() (*gin.Engine, *MockClient) {
	gin.SetMode(gin.TestMode)
	mock := NewMockClient()
	ctrl := NewMockController(mock, nil, nil, nil, newControllerTestLogger())
	router := gin.New()
	ctrl.RegisterRoutes(router)
	return router, mock
}

func TestMockControllerAddIssues(t *testing.T) {
	router, mock := setupMockControllerTestForAddIssues()
	body := bytes.NewBufferString(`{"issues":[{"number":1456,"title":"Fix picker","repo_owner":"owner","repo_name":"repo"}]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/issues", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Added int `json:"added"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Added != 1 {
		t.Fatalf("expected added=1, got %d", got.Added)
	}
	issue, err := mock.GetIssue(context.Background(), "owner", "repo", 1456)
	if err != nil {
		t.Fatalf("get seeded issue: %v", err)
	}
	if issue.Title != "Fix picker" {
		t.Fatalf("expected seeded issue title, got %q", issue.Title)
	}
}

func TestMockControllerAddIssuesInvalidPayload(t *testing.T) {
	router, _ := setupMockControllerTestForAddIssues()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/issues", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMockControllerAddRepoFiles(t *testing.T) {
	router, mock := setupMockControllerTestForAddIssues()
	body := bytes.NewBufferString(`{
		"owner":"o",
		"repo":"r",
		"ref":"main",
		"files":[{"path":"workflows/deploy.yaml","content":"name: deploy"}]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/repo-files", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Added int `json:"added"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Added != 1 {
		t.Fatalf("expected added=1, got %d", got.Added)
	}
	content, err := mock.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err != nil {
		t.Fatalf("get seeded repo file: %v", err)
	}
	if string(content) != "name: deploy" {
		t.Fatalf("expected seeded content, got %q", content)
	}
}

func TestMockControllerAddRepoFilesInvalidPayload(t *testing.T) {
	router, _ := setupMockControllerTestForAddIssues()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/repo-files", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMockControllerAddRepoFilesRequiresOwnerAndRepo(t *testing.T) {
	router, _ := setupMockControllerTestForAddIssues()
	body := bytes.NewBufferString(`{"files":[{"path":"a.yaml","content":"x"}]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/repo-files", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEnsureMockPRForRequestCopiesMergeableState(t *testing.T) {
	mock := NewMockClient()
	controller := &MockController{mock: mock}
	req := &associateTaskPRRequest{
		Owner:          "testorg",
		Repo:           "testrepo",
		PRNumber:       102,
		PRURL:          "https://github.com/testorg/testrepo/pull/102",
		PRTitle:        "Ready to ship",
		HeadBranch:     "feat/ready",
		BaseBranch:     "main",
		AuthorLogin:    "test-user",
		State:          "open",
		MergeableState: "clean",
	}

	controller.ensureMockPRForRequest(context.Background(), req, time.Now().UTC())

	pr, err := mock.GetPR(context.Background(), req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr == nil {
		t.Fatal("expected synthetic PR")
	}
	if pr.MergeableState != "clean" {
		t.Fatalf("MergeableState = %q, want clean", pr.MergeableState)
	}
}
