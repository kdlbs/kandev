package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSecretManager is an in-memory SecretManager + SecretProvider used to
// drive ConfigureToken / ClearToken without touching the real secret store.
type fakeSecretManager struct {
	items       []*SecretListItem
	values      map[string]string
	createErr   error
	updateErr   error
	deleteErr   error
	createCalls int
	updateCalls int
	deleteCalls int
}

func newFakeSecretManager() *fakeSecretManager {
	return &fakeSecretManager{values: map[string]string{}}
}

func (f *fakeSecretManager) List(context.Context) ([]*SecretListItem, error) {
	return f.items, nil
}

func (f *fakeSecretManager) Reveal(_ context.Context, id string) (string, error) {
	return f.values[id], nil
}

func (f *fakeSecretManager) Create(_ context.Context, name, value string) (string, error) {
	f.createCalls++
	if f.createErr != nil {
		return "", f.createErr
	}
	id := "id-" + name
	f.items = append(f.items, &SecretListItem{ID: id, Name: name, HasValue: true})
	f.values[id] = value
	return id, nil
}

func (f *fakeSecretManager) Update(_ context.Context, id, value string) error {
	f.updateCalls++
	if f.updateErr != nil {
		return f.updateErr
	}
	f.values[id] = value
	return nil
}

func (f *fakeSecretManager) Delete(_ context.Context, id string) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.values, id)
	out := f.items[:0]
	for _, it := range f.items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	f.items = out
	return nil
}

// userHandler returns an http.Handler that responds to /api/v4/user as if it
// were a real GitLab. accept controls whether a request is treated as
// authorized.
func userHandler(accept func(token string) bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		if !accept(r.Header.Get("PRIVATE-TOKEN")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"username":"alice"}`))
	})
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"16.0.0"}`))
	})
	return mux
}

func newServiceFixture(t *testing.T, handler http.Handler) (*Service, *httptest.Server, *fakeSecretManager) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	log := newTestLogger(t)
	mgr := newFakeSecretManager()
	svc := NewService(srv.URL, NewNoopClient(srv.URL), AuthMethodNone, mgr, log)
	svc.SetSecretManager(mgr)
	return svc, srv, mgr
}

func TestService_ConfigureToken_ValidTokenCreatesSecretAndSwapsClient(t *testing.T) {
	svc, _, mgr := newServiceFixture(t, userHandler(func(token string) bool {
		return token == "good-token"
	}))

	if err := svc.ConfigureToken(context.Background(), "good-token"); err != nil {
		t.Fatalf("ConfigureToken err = %v", err)
	}
	if mgr.createCalls != 1 {
		t.Errorf("create calls = %d, want 1", mgr.createCalls)
	}
	if mgr.updateCalls != 0 {
		t.Errorf("update calls = %d, want 0 on first configure", mgr.updateCalls)
	}
	if _, ok := svc.Client().(*PATClient); !ok {
		t.Errorf("client = %T, want *PATClient after configure", svc.Client())
	}
	status, _ := svc.GetStatus(context.Background())
	if status.AuthMethod != AuthMethodPAT {
		t.Errorf("auth_method = %q, want pat", status.AuthMethod)
	}
}

func TestService_ConfigureToken_InvalidTokenReturnsErrorAndKeepsNoop(t *testing.T) {
	svc, _, mgr := newServiceFixture(t, userHandler(func(string) bool { return false }))

	err := svc.ConfigureToken(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("err = %v, want 'invalid token' prefix", err)
	}
	if mgr.createCalls != 0 {
		t.Errorf("create calls = %d, want 0 — secret must not be written on invalid token", mgr.createCalls)
	}
	if _, ok := svc.Client().(*NoopClient); !ok {
		t.Errorf("client = %T, want *NoopClient (unchanged after failure)", svc.Client())
	}
}

func TestService_ConfigureToken_PreExistingTokenCallsUpdateNotCreate(t *testing.T) {
	svc, _, mgr := newServiceFixture(t, userHandler(func(token string) bool {
		return token == "new-token"
	}))
	mgr.items = []*SecretListItem{{ID: "id-GITLAB_TOKEN", Name: "GITLAB_TOKEN", HasValue: true}}
	mgr.values["id-GITLAB_TOKEN"] = "old-token"

	if err := svc.ConfigureToken(context.Background(), "new-token"); err != nil {
		t.Fatalf("ConfigureToken err = %v", err)
	}
	if mgr.updateCalls != 1 {
		t.Errorf("update calls = %d, want 1", mgr.updateCalls)
	}
	if mgr.createCalls != 0 {
		t.Errorf("create calls = %d, want 0 when secret exists", mgr.createCalls)
	}
	if mgr.values["id-GITLAB_TOKEN"] != "new-token" {
		t.Errorf("stored token = %q, want new-token", mgr.values["id-GITLAB_TOKEN"])
	}
}

func TestService_ConfigureToken_RejectsEmpty(t *testing.T) {
	svc, _, mgr := newServiceFixture(t, userHandler(func(string) bool { return true }))
	err := svc.ConfigureToken(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if mgr.createCalls != 0 {
		t.Errorf("create calls = %d, want 0", mgr.createCalls)
	}
}

func TestService_ConfigureToken_ErrorsWhenSecretManagerMissing(t *testing.T) {
	log := newTestLogger(t)
	svc := NewService("", NewNoopClient(""), AuthMethodNone, nil, log)
	err := svc.ConfigureToken(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error when secret manager not configured")
	}
}

func TestService_ClearToken_NoSecretIsNoOp(t *testing.T) {
	svc, _, mgr := newServiceFixture(t, userHandler(func(string) bool { return true }))
	t.Setenv("GITLAB_TOKEN", "") // make sure factory falls through to noop
	if err := svc.ClearToken(context.Background()); err != nil {
		t.Fatalf("ClearToken err = %v on empty store, want nil", err)
	}
	if mgr.deleteCalls != 0 {
		t.Errorf("delete calls = %d, want 0", mgr.deleteCalls)
	}
}

func TestService_ClearToken_DeletesSecretAndRebuildsClient(t *testing.T) {
	svc, _, mgr := newServiceFixture(t, userHandler(func(string) bool { return true }))
	t.Setenv("GITLAB_TOKEN", "")
	mgr.items = []*SecretListItem{{ID: "id-GITLAB_TOKEN", Name: "GITLAB_TOKEN", HasValue: true}}
	mgr.values["id-GITLAB_TOKEN"] = "old"

	if err := svc.ClearToken(context.Background()); err != nil {
		t.Fatalf("ClearToken err = %v", err)
	}
	if mgr.deleteCalls != 1 {
		t.Errorf("delete calls = %d, want 1", mgr.deleteCalls)
	}
	// After clearing the only PAT, the factory should fall back to noop
	// (glab CLI may or may not be on the test host, so accept both).
	if _, isPAT := svc.Client().(*PATClient); isPAT {
		t.Errorf("client = *PATClient after clear, want non-PAT fallback")
	}
}

func TestService_ConfigureHost_NormalisesAndPersists(t *testing.T) {
	svc, srv, _ := newServiceFixture(t, userHandler(func(string) bool { return true }))

	if err := svc.ConfigureHost(context.Background(), srv.URL+"/"); err != nil {
		t.Fatalf("ConfigureHost err = %v", err)
	}
	if svc.Host() != srv.URL {
		t.Errorf("host = %q, want %q (trailing slash trimmed)", svc.Host(), srv.URL)
	}
}

func TestService_ConfigureHost_RejectsMissingScheme(t *testing.T) {
	svc, _, _ := newServiceFixture(t, userHandler(func(string) bool { return true }))

	err := svc.ConfigureHost(context.Background(), "gitlab.example.com")
	if err == nil {
		t.Fatal("expected error for host without scheme")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("err = %v, want mention of scheme", err)
	}
}

func TestService_ConfigureHost_UnreachableHostReturnsError(t *testing.T) {
	svc, _, _ := newServiceFixture(t, userHandler(func(string) bool { return true }))

	// 192.0.2.0/24 is reserved by RFC 5737 for documentation; connecting
	// to it deterministically fails (no route / no listener).
	err := svc.ConfigureHost(context.Background(), "http://192.0.2.1:1/")
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("err = %v, want 'unreachable' in message", err)
	}
}

func TestService_GetStatus_ReportsAuthenticatedAfterConfigure(t *testing.T) {
	svc, _, _ := newServiceFixture(t, userHandler(func(token string) bool {
		return token == "ok"
	}))
	if err := svc.ConfigureToken(context.Background(), "ok"); err != nil {
		t.Fatalf("ConfigureToken err = %v", err)
	}
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus err = %v", err)
	}
	if !status.Authenticated {
		t.Error("status.Authenticated = false, want true")
	}
	if status.Username != "alice" {
		t.Errorf("status.Username = %q, want alice", status.Username)
	}
	if !status.TokenConfigured {
		t.Error("status.TokenConfigured = false, want true")
	}
	if status.ConnectionError != "" {
		t.Errorf("status.ConnectionError = %q, want empty on healthy probe", status.ConnectionError)
	}
}

// Regression: a 5xx (or any non-401/403 error) from the IsAuthenticated probe
// must surface as ConnectionError so the UI can render "unreachable" instead
// of "not connected" — the prior code dropped the error and made transient
// outages look like a bad token.
func TestService_GetStatus_PopulatesConnectionErrorOn5xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	svc, _, _ := newServiceFixture(t, mux)
	// Force the service to actually call the upstream by swapping in a
	// PATClient pointed at our fake-failing server.
	svc.mu.Lock()
	svc.client = NewPATClient(svc.host, "tok")
	svc.authMethod = AuthMethodPAT
	svc.mu.Unlock()

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus err = %v", err)
	}
	if status.Authenticated {
		t.Error("Authenticated = true on 5xx, want false")
	}
	if status.ConnectionError == "" {
		t.Error("ConnectionError empty on 5xx, want populated (a transport failure must distinguish itself from 'not connected')")
	}
}

// Counterpart: a 401 is a clean "bad token" signal — IsAuthenticated returns
// (false, nil) — and must leave ConnectionError empty so the UI doesn't
// render an "unreachable" warning over a legitimately-rejected token.
func TestService_GetStatus_ClearsConnectionErrorOn401(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	svc, _, _ := newServiceFixture(t, mux)
	svc.mu.Lock()
	svc.client = NewPATClient(svc.host, "bad")
	svc.authMethod = AuthMethodPAT
	svc.mu.Unlock()

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus err = %v", err)
	}
	if status.Authenticated {
		t.Error("Authenticated = true on 401, want false")
	}
	if status.ConnectionError != "" {
		t.Errorf("ConnectionError = %q on 401, want empty (401 is a clean not-authenticated signal, not a transport failure)", status.ConnectionError)
	}
}

// Sanity: the package-level errors used by the tests are real values, not
// just docstrings — guards against accidental rename.
var _ error = ErrNoClient
var _ = errors.New
