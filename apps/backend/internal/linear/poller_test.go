package linear

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

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

func TestPoller_ProbeAll_RecordsSuccess(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, OrgSlug: "acme"}, nil
	}

	f.poller.probeAll(context.Background())

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

func TestPoller_ProbeAll_RecordsFailure(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}

	f.poller.probeAll(context.Background())

	cfg, _ := f.store.GetConfig(context.Background(), "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "401 unauthorized" {
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
}

func TestPoller_ProbeAll_NoWorkspaces(t *testing.T) {
	f := newPollerFixture(t)
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
	probed := make(chan struct{}, 1)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		select {
		case probed <- struct{}{}:
		default:
		}
		return &TestConnectionResult{OK: true}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.poller.Start(ctx)
	defer f.poller.Stop()

	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not record an initial probe within 2s")
	}
}

func TestPoller_StartIsIdempotent(t *testing.T) {
	f := newPollerFixture(t)
	f.saveConfig(t, "ws-1", "tok")
	var calls int32
	probed := make(chan struct{}, 1)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		atomic.AddInt32(&calls, 1)
		select {
		case probed <- struct{}{}:
		default:
		}
		return &TestConnectionResult{OK: true}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.poller.Start(ctx)
	f.poller.Start(ctx)

	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not run an initial probe within 2s")
	}
	f.poller.Stop()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected exactly 1 probe call from the initial run, got %d", got)
	}
}
