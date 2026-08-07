package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/httpmw"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// resolveCredentialLeaseBody mirrors the JSON shape httpResolveCredentialLease
// binds against (controller_auth.go); BrokerCredentialRequest itself carries
// no json tags, so it cannot be marshaled directly for this HTTP-level test.
type resolveCredentialLeaseBody struct {
	Lease        string `json:"lease"`
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	RepositoryID string `json:"repository_id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Host         string `json:"host"`
}

// newEnabledAuthService builds a real auth.Service in enabled mode (flag on,
// admin provisioned) so the broker HTTP test below exercises the production
// httpmw.Middleware, not a stub — this is what pins that the middleware, not
// just the handler, lets these routes through.
func newEnabledAuthService(t *testing.T) *auth.Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	store, err := authstore.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc, err := auth.NewService(context.Background(), auth.Deps{Cfg: cfg, Store: store, Users: users})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	if _, _, err := svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", ""); err != nil {
		t.Fatalf("setup admin: %v", err)
	}
	return svc
}

func newBrokerHTTPTestRouter(t *testing.T, broker *CredentialBroker) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	svc := NewService(&stubClient{}, "pat", nil, nil, nil, log)
	svc.SetCredentialBroker(broker)
	ctrl := NewController(svc, log)

	router := gin.New()
	router.Use(httpmw.Middleware(newEnabledAuthService(t)))
	useControllerTestWorkspace(router)
	ctrl.RegisterHTTPRoutes(router)
	return router
}

// TestCredentialBrokerRoutesPassRealAuthMiddleware pins that the credential
// broker's readiness and resolve routes reach the broker (and its own
// scope/lease checks) instead of being rejected by httpmw.Middleware in
// enabled mode. This is the regression guard for the missing-allowlist bug:
// without the isPublicPath entries, every assertion below fails with a
// generic 401 "authentication required" instead of the broker's own status
// and error code.
func TestCredentialBrokerRoutesPassRealAuthMiddleware(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatalf("issue lease: %v", err)
	}
	router := newBrokerHTTPTestRouter(t, broker)

	t.Run("GET readiness reaches the broker unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/github/credentials/resolve", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusNoContent)
		}
	})

	t.Run("POST with a valid lease redeems a credential", func(t *testing.T) {
		body, err := json.Marshal(resolveCredentialLeaseBody{
			Lease: lease.Token, TaskID: "task-1", SessionID: "session-1",
			RepositoryID: "repository-1", Owner: "kdlbs", Repo: "kandev", Host: "github.com",
		})
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/github/credentials/resolve", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusOK)
		}
		var credential struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &credential); err != nil {
			t.Fatalf("decode credential: %v", err)
		}
		if credential.Username == "" || credential.Password == "" {
			t.Fatalf("expected populated credential, got %+v", credential)
		}
	})

	t.Run("POST with an invalid lease is denied by the broker, not the middleware", func(t *testing.T) {
		body, err := json.Marshal(resolveCredentialLeaseBody{
			Lease: "not-a-real-lease", TaskID: "task-1", SessionID: "session-1",
			RepositoryID: "repository-1", Owner: "kdlbs", Repo: "kandev", Host: "github.com",
		})
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/github/credentials/resolve", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("github_credential_denied")) {
			t.Fatalf("body = %s, want broker code github_credential_denied", rec.Body.String())
		}
	})
}
