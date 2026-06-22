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

// saveConfig persists an instance directly via store + secret fakes, bypassing
// Service.CreateInstance (and its async probe), and returns the instance id.
func (f *pollerFixture) saveConfig(t *testing.T, secret string) string {
	t.Helper()
	ctx := context.Background()
	cfg := &SentryConfig{Name: "Prod", AuthMethod: AuthMethodAuthToken, URL: "https://sentry.io"}
	if err := f.store.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := f.secrets.Set(ctx, secretKeyFor(cfg.ID), "sentry", secret); err != nil {
		t.Fatalf("save secret: %v", err)
	}
	return cfg.ID
}

func TestService_RecordAuthHealth_Success(t *testing.T) {
	f := newPollerFixture(t)
	id := f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true}, nil
	}

	f.svc.RecordAuthHealth(context.Background())

	cfg, _ := f.store.GetConfig(context.Background(), id)
	if cfg == nil || !cfg.LastOk {
		t.Errorf("expected LastOk=true, got %+v", cfg)
	}
}

func TestService_RecordAuthHealth_Failure(t *testing.T) {
	f := newPollerFixture(t)
	id := f.saveConfig(t, "tok")
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}

	f.svc.RecordAuthHealth(context.Background())

	cfg, _ := f.store.GetConfig(context.Background(), id)
	if cfg.LastOk {
		t.Error("expected LastOk=false")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected error preserved, got %q", cfg.LastError)
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
