package jira

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

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

// saveConfig persists the singleton config directly via the store + secret
// fakes. We deliberately avoid Service.SetConfig here because it fires an
// async auth probe in a goroutine — fine for production but it would race
// against the deterministic RecordAuthHealth calls these tests make.
func (f *pollerFixture) saveConfig(t *testing.T, secret string) {
	t.Helper()
	ctx := context.Background()
	if err := f.store.UpsertConfig(ctx, &JiraConfig{
		SiteURL:    "https://acme.atlassian.net",
		Email:      "user@example.com",
		AuthMethod: AuthMethodAPIToken,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := f.secrets.Set(ctx, SecretKey, "jira", secret); err != nil {
		t.Fatalf("save secret: %v", err)
	}
}

func TestService_RecordAuthHealth_RecordsSuccess(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}

	f.svc.RecordAuthHealth(context.Background())

	cfg, _ := f.store.GetConfig(context.Background())
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
	f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		// Service convention: TestAuth returns the failure inside the result
		// rather than as an error, so the UI can render the message inline.
		return &TestConnectionResult{OK: false, Error: "step-up auth required"}, nil
	}

	f.svc.RecordAuthHealth(context.Background())

	cfg, _ := f.store.GetConfig(context.Background())
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "step-up auth required" {
		t.Errorf("expected error preserved, got %q", cfg.LastError)
	}
}

func TestService_RecordAuthHealth_ClientError(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return nil, errors.New("network timeout")
	}

	f.svc.RecordAuthHealth(context.Background())

	cfg, _ := f.store.GetConfig(context.Background())
	if cfg.LastOk {
		t.Error("expected LastOk=false on client error")
	}
	if cfg.LastError != "network timeout" {
		t.Errorf("expected error from client preserved, got %q", cfg.LastError)
	}
}

func TestPoller_Start_ProbesWhenConfigured(t *testing.T) {
	// Smoke test: confirms the prober adapter actually wires HasConfig →
	// Service.RecordAuthHealth when the loop is started, end-to-end. Wrapped
	// in synctest so the goroutine the poller spawns runs deterministically
	// against fake time instead of relying on a wall-clock deadline.
	synctest.Test(t, func(t *testing.T) {
		f := newPollerFixture(t)
		f.saveConfig(t, "tok")
		var probed bool
		f.client.testAuthFn = func() (*TestConnectionResult, error) {
			return &TestConnectionResult{OK: true}, nil
		}
		f.svc.SetProbeHook(func() {
			probed = true
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.poller.Start(ctx)
		defer f.poller.Stop()

		// Wait for the immediate-on-Start probe pass to finish.
		synctest.Wait()

		if !probed {
			t.Errorf("expected probe to fire when configured")
		}
	})
}
