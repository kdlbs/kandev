package backendapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/secrets"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type issueMutationTransport func(*http.Request) (*http.Response, error)

func (f issueMutationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func issueMutationRequest(number int) github.LinkTaskIssueRequest {
	return github.LinkTaskIssueRequest{Owner: "acme", Repo: "api", Number: number}
}
func issueMutationRemote(r *http.Request) (*http.Response, error) {
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "token issue-fixture-token" {
		return nil, fmt.Errorf("unexpected provider request %s", r.URL)
	}
	body := `{"id":1,"name":"api","full_name":"acme/api","owner":{"login":"acme"}}`
	status := 200
	if strings.Contains(r.URL.Path, "/issues/") {
		body = `{"number":42,"title":"Fetched issue","state":"open","html_url":"https://github.com/acme/api/issues/42"}`
	}
	if strings.HasSuffix(r.URL.Path, "/issues/404") {
		status = 404
		body = `{"message":"Not Found"}`
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func issueMutationAuthenticatedGitHub(t *testing.T, h *issueMutationHarness, transport issueMutationTransport) (*github.Service, *github.Store) {
	t.Helper()
	ctx := context.Background()
	database := sqlx.NewDb(h.repos[0].DB(), "sqlite3")
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	secretStore, cleanup, err := secrets.Provide(database, database, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	require.NoError(t, secretStore.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: github.WorkspacePATSecretKey("issue-ws"), Name: "GitHub fixture", Scope: secrets.ScopeWorkspace, WorkspaceID: "issue-ws"}, Value: "issue-fixture-token"}))
	store, err := github.NewStore(database, database)
	require.NoError(t, err)
	require.NoError(t, store.UpsertWorkspaceConnection(ctx, &github.WorkspaceConnection{WorkspaceID: "issue-ws", Source: github.ConnectionSourcePAT, GitHubHost: "github.com", Login: "fixture", Status: github.ConnectionStatusActive, CredentialGeneration: 1}))
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	gh := github.NewService(nil, github.AuthMethodNone, &githubSecretAdapter{store: secretStore}, store, nil, newTestLogger())
	t.Cleanup(gh.Stop)
	gh.SetTaskIssueStore(githubTaskIssueStoreAdapter{svc: h.services[0]})
	return gh, store
}
func issueMutationRouter(t *testing.T, h *issueMutationHarness, gh *github.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	github.NewController(gh, newTestLogger()).RegisterHTTPRoutes(router)
	taskhandlers.RegisterTaskRoutes(router, ws.NewDispatcher(), h.services[1], nil, h.repos[1], nil, newTestLogger())
	return router
}
func issueMutationHTTP(ctx context.Context, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
