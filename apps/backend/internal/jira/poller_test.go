package jira

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// pollerFixture wires a Service with an in-memory store and fake client/secret
// store, then drives the Poller's probe step directly. We don't run the
// timer-driven loop because the unit value of testing the loop is mostly that
// it ticks on a context — which is just stdlib behavior.
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

func (f *pollerFixture) saveConfig(t *testing.T, workspaceID, secret string) {
	t.Helper()
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: workspaceID,
		SiteURL:     "https://" + workspaceID + ".atlassian.net",
		Email:       workspaceID + "@example.com",
		AuthMethod:  AuthMethodAPIToken,
		Secret:      secret,
	}); err != nil {
		t.Fatalf("save config %s: %v", workspaceID, err)
	}
}

func TestPoller_ProbeAll_RecordsSuccess(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}

	f.poller.probeAll(context.Background())

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

func TestPoller_ProbeAll_RecordsFailure(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		// Service convention: TestAuth returns the failure inside the result
		// rather than as an error, so the UI can render the message inline.
		return &TestConnectionResult{OK: false, Error: "step-up auth required"}, nil
	}

	f.poller.probeAll(context.Background())

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "step-up auth required" {
		t.Errorf("expected error preserved, got %q", cfg.LastError)
	}
}

func TestPoller_ProbeAll_ClientError(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return nil, errors.New("network timeout")
	}

	f.poller.probeAll(context.Background())

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false on client error")
	}
	if cfg.LastError != "network timeout" {
		t.Errorf("expected error from client preserved, got %q", cfg.LastError)
	}
}

func TestPoller_ProbeAll_NoWorkspaces(t *testing.T) {
	f := newPollerFixture(t)
	// Should be a clean no-op when nothing is configured.
	f.poller.probeAll(context.Background())
}

func TestPoller_ProbeAll_MultipleWorkspaces(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-a", "tok-a")
	f.saveConfig(t, "ws-b", "tok-b")
	calls := 0
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		calls++
		return &TestConnectionResult{OK: true}, nil
	}

	f.poller.probeAll(context.Background())

	if calls != 2 {
		t.Errorf("expected 2 probe calls, got %d", calls)
	}
	for _, id := range []string{"ws-a", "ws-b"} {
		cfg, _ := f.store.GetConfig(context.Background(), id)
		if !cfg.LastOk || cfg.LastCheckedAt == nil {
			t.Errorf("workspace %s missing health update: %+v", id, cfg)
		}
	}
}

func TestPoller_StartStop(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.poller.Start(ctx)
	defer f.poller.Stop()

	// The loop runs an initial probe immediately on Start. Wait for it to land
	// in the store. We poll the store rather than sleeping a fixed duration so
	// the test stays robust under load.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
		if cfg != nil && cfg.LastCheckedAt != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("poller did not record an initial probe within 2s")
}

func TestPoller_StartIsIdempotent(t *testing.T) {
	f := newPollerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.poller.Start(ctx)
	f.poller.Start(ctx) // second call must be a no-op
	f.poller.Stop()
}
