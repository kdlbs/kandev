package gitlab

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestGitLabRequestSurfaceCannotImportWebSocketTransport(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate GitLab package")
	}
	packageDir := filepath.Dir(sourceFile)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read GitLab package: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(packageDir, entry.Name())
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}
		for _, importSpec := range file.Imports {
			importPath, unquoteErr := strconv.Unquote(importSpec.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("read import in %s: %v", entry.Name(), unquoteErr)
			}
			if importPath == "github.com/kandev/kandev/pkg/websocket" {
				t.Errorf("%s imports the WebSocket request transport; GitLab requests must remain HTTP-only", entry.Name())
			}
		}
	}
}

func TestWebSocketCatalogContainsOnlyAuthorizedGitLabActions(t *testing.T) {
	authorized := map[string]bool{
		"ActionGitLabCheckSessionMR":          true,
		"ActionGitLabTaskMRUpdated":           true,
		"ActionGitLabMRFeedbackNotify":        true,
		"ActionGitLabNewReviewMRNotify":       true,
		"ActionGitLabTaskMRAutomationUpdated": true,
		"ActionGitLabNewIssueNotify":          true,
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate GitLab package")
	}
	actionsPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "pkg", "websocket", "actions.go")
	file, err := parser.ParseFile(token.NewFileSet(), actionsPath, nil, 0)
	if err != nil {
		t.Fatalf("parse WebSocket action catalog: %v", err)
	}

	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, isValueSpec := node.(*ast.ValueSpec)
		if !isValueSpec {
			return true
		}
		for _, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "ActionGitLab") {
				continue
			}
			found[name.Name] = true
			if !authorized[name.Name] {
				t.Errorf("legacy GitLab WebSocket action constant %s is present", name.Name)
			}
		}
		return true
	})
	for action := range authorized {
		if !found[action] {
			t.Errorf("authorized GitLab WebSocket action constant %s is missing", action)
		}
	}
}

func TestRegisterHTTPRoutesDoesNotExposeUnscopedTaskMRSync(t *testing.T) {
	router := gin.New()
	NewController(
		NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t)),
		newTestLogger(t),
	).RegisterHTTPRoutes(router)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/gitlab/tasks/fabricated/mrs/sync",
		strings.NewReader(`{"project_path":"group/project","iid":7}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sync route status = %d, want 404; body=%s", response.Code, response.Body.String())
	}
}

func TestEnvironmentTokenRejectsUntrustedWorkspaceHostBeforeClientConstruction(t *testing.T) {
	t.Setenv(secretNameToken, "glpat-environment-secret")
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	service := newWorkspaceConfigService(t, store, &configTestSecrets{values: make(map[string]string)})
	var constructions atomic.Int32
	service.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		constructions.Add(1)
		return NewMockClient(cfg.Host), nil
	}

	result := service.TestConfigForWorkspace(t.Context(), "workspace-a", &SetConfigRequest{
		Host: "https://attacker.invalid", AuthMethod: AuthMethodEnvironment,
	})
	if result.OK || !strings.Contains(result.Error, "environment credential host") {
		t.Fatalf("test result = %#v, want trusted-origin rejection", result)
	}
	if got := constructions.Load(); got != 0 {
		t.Fatalf("client constructions = %d, want zero", got)
	}
}

func TestEnvironmentTokenAllowsImmutableStartupHost(t *testing.T) {
	t.Setenv(secretNameToken, "glpat-environment-secret")
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	service := NewService("https://gitlab.internal", NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	service.SetStore(store)
	service.SetWorkspaceSecretStore(&configTestSecrets{values: make(map[string]string)})
	var constructions atomic.Int32
	service.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, token string) (Client, error) {
		constructions.Add(1)
		if cfg.Host != "https://gitlab.internal" || token != "glpat-environment-secret" {
			t.Fatalf("constructed client for host=%q token=%q", cfg.Host, token)
		}
		return NewMockClient(cfg.Host), nil
	}

	result := service.TestConfigForWorkspace(t.Context(), "workspace-a", &SetConfigRequest{
		Host: "https://gitlab.internal/", AuthMethod: AuthMethodEnvironment,
	})
	if !result.OK || result.Error != "" {
		t.Fatalf("test result = %#v, want success", result)
	}
	if got := constructions.Load(); got != 1 {
		t.Fatalf("client constructions = %d, want one", got)
	}
}

func TestResolveExecutionCredentialsRejectsPersistedEnvironmentHostMismatch(t *testing.T) {
	t.Setenv(secretNameToken, "glpat-environment-secret")
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	if err := store.SaveConfigForWorkspace(t.Context(), "workspace-a", &GitLabConfig{
		Host: "https://attacker.invalid", AuthMethod: AuthMethodEnvironment,
	}); err != nil {
		t.Fatalf("seed mismatched config: %v", err)
	}
	service := NewService("https://gitlab.internal", NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	service.SetStore(store)

	host, token, err := service.ResolveGitLabExecutionCredentials(t.Context(), "workspace-a")
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if host != "" || token != "" {
		t.Fatalf("credentials = (%q, %q), want empty values", host, token)
	}
}

func TestAssociateExistingMRRejectsFabricatedProviderIdentityWithoutPersistence(t *testing.T) {
	const host = "https://gitlab.internal"
	tests := []struct {
		name string
		mr   *MR
	}{
		{
			name: "wrong returned iid",
			mr:   &MR{IID: 8, WebURL: host + "/group/project/-/merge_requests/8"},
		},
		{
			name: "wrong returned host",
			mr:   &MR{IID: 7, WebURL: "https://attacker.invalid/group/project/-/merge_requests/7"},
		},
		{
			name: "wrong returned project path",
			mr:   &MR{IID: 7, WebURL: host + "/group/other/-/merge_requests/7"},
		},
		{
			name: "malformed returned url",
			mr:   &MR{IID: 7, WebURL: "not-a-url"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, store, client := newTaskMRLinkService(t, host)
			seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
			setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/project")
			test.mr.Title = "untrusted provider response"
			test.mr.State = "opened"
			test.mr.CreatedAt = time.Now().UTC()
			service.workspaceClients["ws-1"] = &fixedMRStatusClient{
				Client: client,
				status: &MRStatus{MR: test.mr},
			}

			_, err := service.AssociateExistingMRByURL(
				t.Context(), "ws-1", "task-1", "repo-1",
				host+"/group/project/-/merge_requests/7",
			)
			if !errors.Is(err, ErrTaskMRNotFound) {
				t.Fatalf("error = %v, want ErrTaskMRNotFound", err)
			}
			rows, listErr := store.ListTaskMRsByTask(t.Context(), "task-1")
			if listErr != nil || len(rows) != 0 {
				t.Fatalf("stored rows = %d, err = %v; want zero", len(rows), listErr)
			}
		})
	}
}

type fixedMRStatusClient struct {
	Client
	status *MRStatus
}

func (c *fixedMRStatusClient) GetMRStatus(context.Context, string, int) (*MRStatus, error) {
	return c.status, nil
}

func TestAPIErrorNeverIncludesProviderResponseBody(t *testing.T) {
	const hostileBody = `{"message":"private@example.com glpat-secret"}`
	err := (&APIError{StatusCode: http.StatusBadGateway, Endpoint: "/projects", Body: hostileBody}).Error()
	if strings.Contains(err, hostileBody) || strings.Contains(err, "glpat-secret") {
		t.Fatalf("APIError leaked provider response body: %q", err)
	}
}

func TestHostileProviderBodyDoesNotReachHTTPStatusOrLogs(t *testing.T) {
	const secret = "glpat-secret-do-not-log"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"contact private@example.com with ` + secret + `"}`))
	}))
	t.Cleanup(server.Close)

	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	if err := store.SaveConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{
		Host: server.URL, AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	service := NewService(server.URL, NewPATClient(server.URL, secret), AuthMethodPAT, nil, log)
	service.SetStore(store)
	service.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace("workspace-test"): secret,
	}})
	router := gin.New()
	NewController(service, log).RegisterHTTPRoutes(router)

	response := hit(router, "/api/v1/gitlab/user/mrs")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("HTTP status = %d, want 502; body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "private@example.com") {
		t.Fatalf("HTTP response leaked provider body: %s", response.Body.String())
	}
	statusResponse := hit(router, "/api/v1/gitlab/status")
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d, want 200; body=%s", statusResponse.Code, statusResponse.Body.String())
	}
	if strings.Contains(statusResponse.Body.String(), secret) || strings.Contains(statusResponse.Body.String(), "private@example.com") {
		t.Fatalf("status response leaked provider body: %s", statusResponse.Body.String())
	}
	_, _ = service.GetStatsForWorkspace(t.Context(), "workspace-test")
	for _, entry := range observed.All() {
		serialized := entry.ContextMap()
		contextText := fmt.Sprint(serialized)
		if strings.Contains(entry.Message, secret) || strings.Contains(entry.Message, "private@example.com") ||
			strings.Contains(contextText, secret) || strings.Contains(contextText, "private@example.com") {
			t.Fatalf("log leaked provider body: message=%q context=%v", entry.Message, serialized)
		}
	}
}
