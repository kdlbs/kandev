package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

func newSurvivalTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

// fakeControlServerStore is a minimal lifecycle.AdoptionRecordStore double
// scoped to this file's wiring tests.
type fakeControlServerStore struct {
	record  *models.ControlServerRecord
	upserts []*models.ControlServerRecord
}

func (s *fakeControlServerStore) GetControlServerRecord(context.Context) (*models.ControlServerRecord, error) {
	if s.record == nil {
		return nil, models.ErrControlServerRecordNotFound
	}
	return s.record, nil
}

func (s *fakeControlServerStore) UpsertControlServerRecord(_ context.Context, record *models.ControlServerRecord) error {
	s.upserts = append(s.upserts, record)
	s.record = record
	return nil
}

// idGeneratingSecretStore is a minimal secrets.SecretStore double that
// auto-generates an ID on Create, scoped to this file's wiring tests
// (mirrors the lifecycle package's own inMemorySecretStore test double).
type idGeneratingSecretStore struct {
	mu    sync.Mutex
	store map[string]string
	next  int
}

func newIDGeneratingSecretStore() *idGeneratingSecretStore {
	return &idGeneratingSecretStore{store: make(map[string]string)}
}

func (s *idGeneratingSecretStore) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if secret.ID == "" {
		s.next++
		secret.ID = fmt.Sprintf("survival-secret-%d", s.next)
	}
	s.store[secret.ID] = secret.Value
	return nil
}

func (s *idGeneratingSecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.store[id]; !ok {
		return nil, fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	return &secrets.Secret{ID: id}, nil
}

func (s *idGeneratingSecretStore) Reveal(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.store[id]
	if !ok {
		return "", fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	return value, nil
}

func (s *idGeneratingSecretStore) Update(_ context.Context, id string, req *secrets.UpdateSecretRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.store[id]; !ok {
		return fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	if req.Value != nil {
		s.store[id] = *req.Value
	}
	return nil
}

func (s *idGeneratingSecretStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, id)
	return nil
}

func (s *idGeneratingSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}

func (*idGeneratingSecretStore) Close() error { return nil }

func TestSplitEndpointParsesHostAndPort(t *testing.T) {
	host, port, err := splitEndpoint("127.0.0.1:39429")
	if err != nil {
		t.Fatalf("splitEndpoint: %v", err)
	}
	if host != "127.0.0.1" || port != 39429 {
		t.Fatalf("host=%q port=%d, want 127.0.0.1:39429", host, port)
	}
}

func TestSplitEndpointRejectsMalformedEndpoint(t *testing.T) {
	if _, _, err := splitEndpoint("not-a-valid-endpoint"); err == nil {
		t.Fatal("want error for malformed endpoint, got nil")
	}
}

// TestAdoptSurvivingAgentctlReturnsNilWhenNoRecord pins the fallback
// wiring: with no control-server record at all, adoptSurvivingAgentctl must
// return nil so provideAgentctlLauncher falls through to spawning fresh --
// exactly like the capability being disabled.
func TestAdoptSurvivingAgentctlReturnsNilWhenNoRecord(t *testing.T) {
	cfg := &config.Config{}
	store := &fakeControlServerStore{}

	result := adoptSurvivingAgentctl(context.Background(), cfg, newSurvivalTestLogger(t), store, nil)

	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
}

// TestAdoptSurvivingAgentctlAdoptsAndUpdatesConfig drives the real wiring
// end to end against an httptest server standing in for a surviving
// agentctl: identity, rotate, and confirm are all real HTTP round trips
// through the production agentctl.ControlClient, not a fake. Verifies the
// adopted endpoint/credential land on cfg.Agent.Standalone* and that the
// returned result's cleanup is a no-op (nothing to stop for an adopted
// server per AC-EXECUTORS-SURVIVAL-001.1).
func TestAdoptSurvivingAgentctlAdoptsAndUpdatesConfig(t *testing.T) {
	const homeDir = "/home/kandev-test"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/identity":
			_ = json.NewEncoder(w).Encode(agentctlclient.IdentityInfo{
				HomeDir:           homeDir,
				ServerIdentity:    "survivor-identity",
				Capabilities:      lifecycle.RequiredSurvivalCapabilities,
				DiagnosticLogPath: "/home/kandev-test/logs/agentctl-diagnostic.log",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/rotate":
			_ = json.NewEncoder(w).Encode(agentctlclient.CredentialRotationResult{
				RotationID: 1,
				Credential: "rotated-credential",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/confirm":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	secretStore := newIDGeneratingSecretStore()
	var seedSecret secrets.SecretWithValue
	seedSecret.Name = "seed"
	seedSecret.Value = "bootstrap-credential"
	if err := secretStore.Create(context.Background(), &seedSecret); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	endpoint := server.Listener.Addr().String()
	store := &fakeControlServerStore{record: &models.ControlServerRecord{
		Endpoint:           endpoint,
		CredentialSecretID: seedSecret.ID,
	}}
	cfg := &config.Config{}
	cfg.HomeDir = homeDir

	result := adoptSurvivingAgentctl(context.Background(), cfg, newSurvivalTestLogger(t), store, secretStore)

	if result == nil {
		t.Fatal("result = nil, want a non-nil adopted result")
	}
	if err := result.cleanup(); err != nil {
		t.Fatalf("cleanup() = %v, want nil (adopted server must not be stopped)", err)
	}
	if cfg.Agent.StandaloneAuthToken != "rotated-credential" {
		t.Fatalf("StandaloneAuthToken = %q, want rotated-credential", cfg.Agent.StandaloneAuthToken)
	}
	wantHost, wantPort, err := splitEndpoint(endpoint)
	if err != nil {
		t.Fatalf("splitEndpoint(endpoint): %v", err)
	}
	if cfg.Agent.StandaloneHost != wantHost || cfg.Agent.StandalonePort != wantPort {
		t.Fatalf("StandaloneHost/Port = %s:%d, want %s:%d", cfg.Agent.StandaloneHost, cfg.Agent.StandalonePort, wantHost, wantPort)
	}
	if cfg.Agent.StandalonePID != 0 {
		t.Fatalf("StandalonePID = %d, want 0 (this process never spawned the adopted server)", cfg.Agent.StandalonePID)
	}
	if len(store.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(store.upserts))
	}
}
