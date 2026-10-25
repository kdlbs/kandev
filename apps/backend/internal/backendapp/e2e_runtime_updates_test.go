package backendapp

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestNewE2ERuntimeUpdateHooksRequiresMockRuntimeAndVersion(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	t.Setenv("KANDEV_E2E_RUNTIME_UPDATE_LATEST_VERSION", "99.0.0")
	if hooks := newE2ERuntimeUpdateHooks(); hooks != nil {
		t.Fatal("hooks initialized without the full mock runtime")
	}

	t.Setenv("KANDEV_MOCK_AGENT", "true")
	t.Setenv("KANDEV_E2E_RUNTIME_UPDATE_LATEST_VERSION", " ")
	if hooks := newE2ERuntimeUpdateHooks(); hooks != nil {
		t.Fatal("hooks initialized without a test version")
	}
}

func TestE2ERuntimeUpdateHooksReleaseAndCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Setenv("KANDEV_E2E_MOCK", "true")
		t.Setenv("KANDEV_MOCK_AGENT", "true")
		t.Setenv("KANDEV_E2E_RUNTIME_UPDATE_LATEST_VERSION", "99.0.0")
		hooks := newE2ERuntimeUpdateHooks()
		if hooks == nil {
			t.Fatal("expected E2E runtime update hooks")
		}

		hostReady := make(chan struct{})
		startupReady := hooks.startupReadiness(context.Background(), hostReady)
		close(hostReady)
		synctest.Wait()
		select {
		case <-startupReady:
			t.Fatal("startup pass became ready before E2E release")
		default:
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := hooks.resolveLatestVersion(ctx, "@example/runtime"); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled resolver error = %v, want context canceled", err)
		}

		hooks.release()
		hooks.release()
		synctest.Wait()
		select {
		case <-startupReady:
		default:
			t.Fatal("startup pass did not become ready after E2E release")
		}
		version, err := hooks.resolveLatestVersion(context.Background(), "@example/runtime")
		if err != nil {
			t.Fatalf("resolve released version: %v", err)
		}
		if version != "99.0.0" {
			t.Fatalf("resolved version = %q, want 99.0.0", version)
		}
	})
}
