package jira

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// pollerFixture wires a Service against a real in-memory store and fake
// client/secret store. Loop semantics are tested in
// internal/integrations/healthpoll; the cases here exercise the jira-specific
// integration of the loop with Service.RecordAuthHealth (error preservation,
// LastCheckedAt) and the Start/Stop wiring.
type pollerFixture struct {
	store   *Store
	secrets *fakeSecretStore
	client  *fakeClient
	svc     *Service
	poller  *Poller
}

func newPollerFixture(t *testing.T) *pollerFixture {
	t.Helper()
	f := &pollerFixture{
		store:   newTestStore(t),
		secrets: newFakeSecretStore(),
		client:  &fakeClient{},
	}
	f.svc = NewService(f.store, f.secrets, func(_ *JiraConfig, _ string) Client {
		return f.client
	}, logger.Default())
	f.poller = NewPoller(f.svc, logger.Default())
	return f
}

// saveConfig persists a workspace config directly via the store + secret
// fakes. We deliberately avoid Service.SetConfig here because it fires an
// async auth probe in a goroutine — fine for production but it would race
// against the deterministic RecordAuthHealth calls these tests make.
func (f *pollerFixture) saveConfig(t *testing.T, workspaceID, secret string) {
	t.Helper()
	ctx := context.Background()
	if err := f.store.UpsertConfig(ctx, &JiraConfig{
		WorkspaceID: workspaceID,
		SiteURL:     "https://" + workspaceID + ".atlassian.net",
		Email:       workspaceID + "@example.com",
		AuthMethod:  AuthMethodAPIToken,
	}); err != nil {
		t.Fatalf("save config %s: %v", workspaceID, err)
	}
	if err := f.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID),
		"jira", secret); err != nil {
		t.Fatalf("save secret %s: %v", workspaceID, err)
	}
}

func TestService_RecordAuthHealth_RecordsSuccess(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}

	f.svc.RecordAuthHealth(context.Background(), "ws-1")

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg == nil {
		t.Fatal("config disappeared")
	}
	if !cfg.LastOk {
		t.Error("expected LastOk=true after successful probe")
	}
	if cfg.LastError != "" {
		t.Errorf("expected empty error, got %q", cfg.LastError)
	}
	if cfg.LastCheckedAt == nil {
		t.Error("expected LastCheckedAt to be set")
	}
}

func TestService_RecordAuthHealth_RecordsFailure(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		// Service convention: TestAuth returns the failure inside the result
		// rather than as an error, so the UI can render the message inline.
		return &TestConnectionResult{OK: false, Error: "step-up auth required"}, nil
	}

	f.svc.RecordAuthHealth(context.Background(), "ws-1")

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "step-up auth required" {
		t.Errorf("expected error preserved, got %q", cfg.LastError)
	}
}

func TestService_RecordAuthHealth_ClientError(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return nil, errors.New("network timeout")
	}

	f.svc.RecordAuthHealth(context.Background(), "ws-1")

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false on client error")
	}
	if cfg.LastError != "network timeout" {
		t.Errorf("expected error from client preserved, got %q", cfg.LastError)
	}
}

func TestPoller_Start_ProbesEachConfiguredWorkspace(t *testing.T) {
	// Smoke test: confirms the prober adapter actually wires
	// Service.Store().ListConfiguredWorkspaces → Service.RecordAuthHealth
	// when the loop is started, end-to-end.
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-a", "tok-a")
	f.saveConfig(t, "ws-b", "tok-b")
	probed := make(chan string, 2)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true}, nil
	}
	f.svc.SetProbeHook(func(workspaceID string) {
		probed <- workspaceID
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.poller.Start(ctx)
	defer f.poller.Stop()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-probed:
			seen[id] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("only saw %v probed within 2s", seen)
		}
	}
	if !seen["ws-a"] || !seen["ws-b"] {
		t.Errorf("expected both ws-a and ws-b probed, got %v", seen)
	}
}
