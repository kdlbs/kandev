package linear

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/common/logger"
)

// pollerFixture wires a Service against a real in-memory store and fake
// client/secret store. The auth-loop semantics live in
// internal/integrations/healthpoll; the tests here cover the linear-specific
// integration of the loop with Service.RecordAuthHealth (org_slug capture,
// error preservation) and the Start/Stop smoke wiring.
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
	f.svc = NewService(f.store, f.secrets, func(_ *LinearConfig, _ string) Client {
		return f.client
	}, logger.Default())
	f.poller = NewPoller(f.svc, logger.Default())
	return f
}

// saveConfig persists a workspace config directly via the store + secret
// fakes — bypassing Service.SetConfig avoids racing against its async probe.
func (f *pollerFixture) saveConfig(t *testing.T, workspaceID, secret string) {
	t.Helper()
	ctx := context.Background()
	if err := f.store.UpsertConfig(ctx, &LinearConfig{
		WorkspaceID: workspaceID,
		AuthMethod:  AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("save config %s: %v", workspaceID, err)
	}
	if err := f.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID),
		"linear", secret); err != nil {
		t.Fatalf("save secret %s: %v", workspaceID, err)
	}
}

func TestService_RecordAuthHealth_RecordsSuccess(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, OrgSlug: "acme"}, nil
	}

	f.svc.RecordAuthHealth(context.Background(), "ws-1")

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg == nil {
		t.Fatal("config disappeared")
	}
	if !cfg.LastOk {
		t.Error("expected LastOk=true after successful probe")
	}
	if cfg.OrgSlug != "acme" {
		t.Errorf("expected org_slug captured, got %q", cfg.OrgSlug)
	}
}

func TestService_RecordAuthHealth_RecordsFailure(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}

	f.svc.RecordAuthHealth(context.Background(), "ws-1")

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "401 unauthorized" {
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
}

func TestPoller_Start_ProbesEachConfiguredWorkspace(t *testing.T) {
	// Smoke test: confirms the prober adapter actually wires
	// Service.Store().ListConfiguredWorkspaces → Service.RecordAuthHealth
	// when the loop is started, end-to-end. Wrapped in synctest so the
	// goroutine the poller spawns runs deterministically against fake time
	// instead of relying on a wall-clock deadline.
	synctest.Test(t, func(t *testing.T) {
		f := newPollerFixture(t)
		f.saveConfig(t, "ws-a", "tok-a")
		f.saveConfig(t, "ws-b", "tok-b")
		probed := map[string]bool{}
		f.client.testAuthFn = func() (*TestConnectionResult, error) {
			return &TestConnectionResult{OK: true}, nil
		}
		f.svc.SetProbeHook(func(workspaceID string) {
			probed[workspaceID] = true
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.poller.Start(ctx)
		defer f.poller.Stop()

		// Wait for the immediate-on-Start probe pass to finish.
		synctest.Wait()

		if !probed["ws-a"] || !probed["ws-b"] {
			t.Errorf("expected both ws-a and ws-b probed, got %v", probed)
		}
	})
}
