package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type installationAccessListLog struct {
	mu    sync.Mutex
	pages []int
}

func (l *installationAccessListLog) record(page int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pages = append(l.pages, page)
}

func (l *installationAccessListLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pages)
}

// newInstallationAccessOAuthServer serves the OAuth exchange, user lookup, and
// the documented installation list endpoint. The supplied handler only owns the
// list response so a test can vary pagination, absence, and provider errors.
func newInstallationAccessOAuthServer(
	t *testing.T,
	list http.HandlerFunc,
) (*httptest.Server, *installationAccessListLog) {
	t.Helper()
	log := &installationAccessListLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token", "refresh_token": "refresh-token",
				"expires_in": 28800, "refresh_token_expires_in": 15552000,
			})
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 11, "login": "octocat"})
		case userInstallationsPath:
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil {
				http.Error(w, "invalid page", http.StatusBadRequest)
				return
			}
			log.record(page)
			list(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, log
}

func newInstallationAccessOAuthClient(server *httptest.Server, now time.Time) *GitHubOAuthClient {
	client := NewGitHubOAuthClient("client-id", "client-secret")
	client.webBaseURL = server.URL
	client.apiBaseURL = server.URL
	client.httpClient = server.Client()
	client.now = func() time.Time { return now }
	return client
}

func newInstallationAccessFlowManager() *OAuthFlowManager {
	flows := NewOAuthFlowManager(&oauthFlowMemoryStore{})
	flows.random = strings.NewReader(strings.Repeat("i", oauthRandomBytes*4))
	return flows
}

func TestAppInstallationCompleteWithPaginatedUserAccess(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	const workspaceID = "workspace-1"
	priorConnection := func() *WorkspaceConnection {
		return &WorkspaceConnection{
			WorkspaceID: workspaceID, Source: ConnectionSourcePAT, Login: "old",
			Status: ConnectionStatusActive, CredentialGeneration: 3,
		}
	}

	t.Run("later page access commits the connection", func(t *testing.T) {
		server, log := newInstallationAccessOAuthServer(t, installationPageHandler(
			map[int][]int64{1: {7, 8}, 2: {9, 42}}, 4,
		))
		store := &appInstallationMemoryStore{connection: priorConnection()}
		service := newTestAppInstallationService(server, store, now)

		result, err := completeTestAppInstallation(t, service)
		if err != nil {
			t.Fatalf("Complete() error = %v", err)
		}
		if result.Installation.ID != 42 || log.count() != 2 {
			t.Fatalf("Complete() installation = %d, list requests = %d; want 42 on page 2", result.Installation.ID, log.count())
		}
		connection := store.connection
		if connection == nil || connection.Source != ConnectionSourceGitHubAppInstallation ||
			connection.InstallationID == nil || *connection.InstallationID != 42 ||
			connection.CredentialGeneration != 4 {
			t.Fatalf("persisted connection = %+v", connection)
		}
	})

	t.Run("confirmed absence retries before committing", func(t *testing.T) {
		var attempts int
		server, log := newInstallationAccessOAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			if attempts < 3 {
				writeInstallationListPayload(w, 1, 7)
				return
			}
			writeInstallationListPayload(w, 1, 42)
		})
		store := &appInstallationMemoryStore{}
		service := newTestAppInstallationService(server, store, now)
		service.installationAccessRetryDelays = []time.Duration{0, 0, 0}

		if _, err := completeTestAppInstallation(t, service); err != nil {
			t.Fatalf("Complete() error = %v", err)
		}
		if log.count() != 3 {
			t.Fatalf("installation list requests = %d, want 3", log.count())
		}
		if store.connection == nil || store.connection.InstallationID == nil ||
			*store.connection.InstallationID != 42 {
			t.Fatalf("persisted connection = %+v", store.connection)
		}
	})

	t.Run("provider error preserves the previous connection", func(t *testing.T) {
		server, log := newInstallationAccessOAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		store := &appInstallationMemoryStore{connection: priorConnection()}
		service := newTestAppInstallationService(server, store, now)
		service.installationAccessRetryDelays = []time.Duration{0, 0}

		_, err := completeTestAppInstallation(t, service)
		if !errors.Is(err, ErrInstallationAssociationUnverified) {
			t.Fatalf("Complete() error = %v, want %v", err, ErrInstallationAssociationUnverified)
		}
		if log.count() != 1 {
			t.Fatalf("installation list requests = %d, want 1 for a provider error", log.count())
		}
		if store.connection == nil || store.connection.Source != ConnectionSourcePAT ||
			store.connection.CredentialGeneration != 3 {
			t.Fatalf("previous connection changed: %+v", store.connection)
		}
	})

	t.Run("confirmed absence preserves the previous connection", func(t *testing.T) {
		server, _ := newInstallationAccessOAuthServer(t, installationPageHandler(
			map[int][]int64{1: {7}}, 1,
		))
		store := &appInstallationMemoryStore{connection: priorConnection()}
		service := newTestAppInstallationService(server, store, now)
		service.installationAccessRetryDelays = nil

		if _, err := completeTestAppInstallation(t, service); !errors.Is(err, ErrInstallationAssociationUnverified) {
			t.Fatalf("Complete() error = %v, want %v", err, ErrInstallationAssociationUnverified)
		}
		if store.connection == nil || store.connection.Source != ConnectionSourcePAT ||
			store.connection.CredentialGeneration != 3 {
			t.Fatalf("previous connection changed: %+v", store.connection)
		}
	})
}

func newTestAppInstallationService(
	server *httptest.Server,
	store *appInstallationMemoryStore,
	now time.Time,
) *AppInstallationService {
	return NewAppInstallationService(
		AppInstallationConfig{
			RegistrationID: "registration-test", Slug: "kandev-app",
			CallbackURL: "https://kandev.example/callback",
		},
		newInstallationAccessFlowManager(), store,
		&fakeAppInstallationVerifier{installation: AppInstallation{
			ID: 42, AccountLogin: "acme", AccountType: "Organization",
		}},
		newInstallationAccessOAuthClient(server, now),
	)
}

func completeTestAppInstallation(
	t *testing.T,
	service *AppInstallationService,
) (AppInstallationResult, error) {
	t.Helper()
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		return AppInstallationResult{}, err
	}
	return service.Complete(context.Background(), AppInstallationCallback{
		State: started.State, Code: "oauth-code", InstallationID: 42,
	})
}

func TestPersonalAuthCompleteWithPaginatedUserAccess(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	priorRepository := func() *personalAuthMemoryRepository {
		return &personalAuthMemoryRepository{
			workspace: activeAppWorkspace("workspace-1", 42),
			connection: &UserConnection{
				WorkspaceID: "workspace-1", UserID: "user-1", AppRegistrationID: "registration-test",
				GitHubUserID: 11, Login: "octocat", Status: ConnectionStatusActive,
				CredentialGeneration: 2,
			},
			tokens: GitHubOAuthTokens{AccessToken: "old-access", RefreshToken: "old-refresh"},
		}
	}

	t.Run("later page access saves the personal connection", func(t *testing.T) {
		server, log := newInstallationAccessOAuthServer(t, installationPageHandler(
			map[int][]int64{1: {7}, 2: {11, 42}}, 3,
		))
		repo := priorRepository()
		service := newTestPersonalAuthService(server, repo, now)

		result, err := completeTestPersonalAuth(t, service)
		if err != nil {
			t.Fatalf("Complete() error = %v", err)
		}
		if result.Connection.CredentialGeneration != 3 || log.count() != 2 {
			t.Fatalf("connection generation = %d, list requests = %d; want 3 on page 2",
				result.Connection.CredentialGeneration, log.count())
		}
		if repo.tokens.AccessToken != "access-token" || repo.tokens.RefreshToken != "refresh-token" {
			t.Fatalf("stored tokens = %+v", repo.tokens)
		}
		if repo.connection == nil || repo.connection.Status != ConnectionStatusActive ||
			repo.connection.GitHubUserID != 11 {
			t.Fatalf("stored connection = %+v", repo.connection)
		}
	})

	t.Run("confirmed absence rejects without replacing the connection", func(t *testing.T) {
		server, _ := newInstallationAccessOAuthServer(t, installationPageHandler(
			map[int][]int64{1: {7, 8}}, 2,
		))
		repo := priorRepository()
		service := newTestPersonalAuthService(server, repo, now)

		if _, err := completeTestPersonalAuth(t, service); !errors.Is(err, ErrInstallationAssociationUnverified) {
			t.Fatalf("Complete() error = %v, want %v", err, ErrInstallationAssociationUnverified)
		}
		assertPriorPersonalConnectionPreserved(t, repo, "old-access", "old-refresh")
	})

	t.Run("provider error rejects without replacing the connection", func(t *testing.T) {
		server, _ := newInstallationAccessOAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		repo := priorRepository()
		service := newTestPersonalAuthService(server, repo, now)

		if _, err := completeTestPersonalAuth(t, service); !errors.Is(err, ErrInstallationAssociationUnverified) {
			t.Fatalf("Complete() error = %v, want %v", err, ErrInstallationAssociationUnverified)
		}
		assertPriorPersonalConnectionPreserved(t, repo, "old-access", "old-refresh")
	})
}

func newTestPersonalAuthService(
	server *httptest.Server,
	repo *personalAuthMemoryRepository,
	now time.Time,
) *PersonalAuthService {
	service := NewPersonalAuthService(
		PersonalAuthConfig{
			RegistrationID: "registration-test", ClientID: "client-id",
			CallbackURL: "https://kandev.example/callback",
		},
		newInstallationAccessFlowManager(), repo, newInstallationAccessOAuthClient(server, now),
	)
	service.now = func() time.Time { return now }
	return service
}

func completeTestPersonalAuth(
	t *testing.T,
	service *PersonalAuthService,
) (PersonalAuthResult, error) {
	t.Helper()
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		return PersonalAuthResult{}, err
	}
	return service.Complete(context.Background(), PersonalAuthCallback{
		State: started.State, Code: "oauth-code",
	})
}

func assertPriorPersonalConnectionPreserved(
	t *testing.T,
	repo *personalAuthMemoryRepository,
	accessToken, refreshToken string,
) {
	t.Helper()
	if repo.invalidated || repo.connection == nil ||
		repo.connection.Status != ConnectionStatusActive ||
		repo.connection.CredentialGeneration != 2 {
		t.Fatalf("personal connection changed: %+v (invalidated=%v)", repo.connection, repo.invalidated)
	}
	if repo.tokens.AccessToken != accessToken || repo.tokens.RefreshToken != refreshToken {
		t.Fatalf("stored tokens changed: %+v", repo.tokens)
	}
}

func TestPersonalAuthRefreshWithPaginatedUserAccess(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

	t.Run("later page access rotates tokens atomically", func(t *testing.T) {
		server, log := newInstallationAccessOAuthServer(t, installationPageHandler(
			map[int][]int64{1: {7}, 2: {42}}, 2,
		))
		repo := expiringPersonalAuthRepository(now)
		service := newTestPersonalAuthService(server, repo, now)

		token, connection, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
		if err != nil {
			t.Fatalf("ResolvePersonalToken() error = %v", err)
		}
		if token != "access-token" || connection == nil || connection.CredentialGeneration != 3 || log.count() != 2 {
			t.Fatalf("ResolvePersonalToken() = %q, %+v; list requests = %d",
				token, connection, log.count())
		}
		if repo.tokens.AccessToken != "access-token" || repo.tokens.RefreshToken != "refresh-token" ||
			repo.version != 3 {
			t.Fatalf("stored tokens = %+v, generation = %d", repo.tokens, repo.version)
		}
	})

	t.Run("confirmed absence invalidates the connection", func(t *testing.T) {
		server, _ := newInstallationAccessOAuthServer(t, installationPageHandler(
			map[int][]int64{1: {7}}, 1,
		))
		repo := expiringPersonalAuthRepository(now)
		service := newTestPersonalAuthService(server, repo, now)

		_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
		if !errors.Is(err, ErrInstallationAssociationUnverified) {
			t.Fatalf("ResolvePersonalToken() error = %v, want %v", err, ErrInstallationAssociationUnverified)
		}
		assertPersonalConnectionInvalidated(t, repo)
	})

	t.Run("unauthorized response invalidates the connection", func(t *testing.T) {
		server, _ := newInstallationAccessOAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		repo := expiringPersonalAuthRepository(now)
		service := newTestPersonalAuthService(server, repo, now)

		_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
		if err == nil {
			t.Fatal("ResolvePersonalToken() error = nil, want provider error")
		}
		assertPersonalConnectionInvalidated(t, repo)
	})

	nonDefinitiveFailures := []struct {
		name   string
		status int
	}{
		{name: "forbidden", status: http.StatusForbidden},
		{name: "not found", status: http.StatusNotFound},
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "server error", status: http.StatusInternalServerError},
	}
	for _, failure := range nonDefinitiveFailures {
		t.Run(failure.name+" preserves stored credentials", func(t *testing.T) {
			server, _ := newInstallationAccessOAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(failure.status)
			})
			repo := expiringPersonalAuthRepository(now)
			service := newTestPersonalAuthService(server, repo, now)

			_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
			if err == nil {
				t.Fatal("ResolvePersonalToken() error = nil, want provider error")
			}
			assertPriorPersonalConnectionPreserved(t, repo, "old", "refresh")
		})
	}

	t.Run("invalid json preserves stored credentials", func(t *testing.T) {
		server, _ := newInstallationAccessOAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count":`))
		})
		repo := expiringPersonalAuthRepository(now)
		service := newTestPersonalAuthService(server, repo, now)

		_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
		if err == nil {
			t.Fatal("ResolvePersonalToken() error = nil, want decoding error")
		}
		assertPriorPersonalConnectionPreserved(t, repo, "old", "refresh")
	})
}

func assertPersonalConnectionInvalidated(t *testing.T, repo *personalAuthMemoryRepository) {
	t.Helper()
	if !repo.invalidated || repo.connection == nil ||
		repo.connection.Status != ConnectionStatusInvalid {
		t.Fatalf("personal connection not invalidated: %+v (invalidated=%v)",
			repo.connection, repo.invalidated)
	}
}
