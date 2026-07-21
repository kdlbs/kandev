package sentry

import (
	"context"
	"testing"
	"testing/synctest"

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
	f.svc = NewService(f.store, f.secrets, func(_ *SentryConfig, _ string) Client {
		return f.client
	}, logger.Default())
	f.poller = NewPoller(f.svc, logger.Default())
	return f
}

// saveConfig seeds a single instance in a default workspace, bypassing
// Service.CreateInstance (and its async probe). Returns the seeded instance.
func (f *pollerFixture) saveConfig(t *testing.T, secret string) *SentryConfig {
	t.Helper()
	return f.saveConfigForWorkspace(t, "ws-1", secret)
}

// saveConfigForWorkspace seeds one instance (+ secret) in a workspace, so an
// unbound watch in that workspace resolves to it at poll time.
func (f *pollerFixture) saveConfigForWorkspace(t *testing.T, workspaceID, secret string) *SentryConfig {
	t.Helper()
	ctx := context.Background()
	cfg := &SentryConfig{WorkspaceID: workspaceID, Name: "Primary", AuthMethod: AuthMethodAuthToken, URL: DefaultSentryURL}
	if err := f.store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if secret != "" {
		if err := f.secrets.Set(ctx, secretKeyForInstance(cfg.ID), "sentry", secret); err != nil {
			t.Fatalf("save secret: %v", err)
		}
	}
	return cfg
}

func TestService_RecordAuthHealth_Success(t *testing.T) {
	f := newPollerFixture(t)
	cfg := f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true}, nil
	}

	f.svc.RecordAuthHealth(context.Background())

	got, _ := f.store.GetInstance(context.Background(), cfg.ID)
	if got == nil || !got.LastOk {
		t.Errorf("expected LastOk=true, got %+v", got)
	}
}

func TestService_RecordAuthHealth_Failure(t *testing.T) {
	f := newPollerFixture(t)
	cfg := f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}

	f.svc.RecordAuthHealth(context.Background())

	got, _ := f.store.GetInstance(context.Background(), cfg.ID)
	if got.LastOk {
		t.Error("expected LastOk=false")
	}
	if got.LastError != "401 unauthorized" {
		t.Errorf("expected error preserved, got %q", got.LastError)
	}
}

// TestService_RecordAuthHealth_MultipleInstances verifies the bounded worker
// pool probes every instance, not just one.
func TestService_RecordAuthHealth_MultipleInstances(t *testing.T) {
	f := newPollerFixture(t)
	a := f.saveConfigForWorkspace(t, "ws-1", "tok-a")
	b := f.saveConfigForWorkspace(t, "ws-2", "tok-b")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true}, nil
	}
	f.svc.RecordAuthHealth(context.Background())
	for _, id := range []string{a.ID, b.ID} {
		got, _ := f.store.GetInstance(context.Background(), id)
		if got == nil || !got.LastOk {
			t.Errorf("instance %s not probed: %+v", id, got)
		}
	}
}

func TestPoller_Start_ProbesWhenConfigured(t *testing.T) {
	// Smoke test of the prober adapter under fake time so the immediate probe
	// on Start completes deterministically.
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

		synctest.Wait()

		if !probed {
			t.Error("expected probe to fire when configured")
		}
	})
}
