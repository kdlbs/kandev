package linear

import (
	"context"
	"errors"
	"testing"
	"time"
)

// drainProbe waits for the async auth-health probe fired by a config write.
func drainProbe(t *testing.T, f *svcFixture) {
	t.Helper()
	select {
	case <-f.probed:
	case <-time.After(2 * time.Second):
		t.Fatalf("async probe hook did not fire within 2s")
	}
}

func TestCopyConfigToWorkspace_CopiesConfigAndSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"

	if _, err := f.svc.SetConfigForWorkspace(ctx, src, &SetConfigRequest{
		AuthMethod:     AuthMethodAPIKey,
		DefaultTeamKey: "ENG",
		Secret:         "lin-src",
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	drainProbe(t, f)

	got, err := f.svc.CopyConfigToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	drainProbe(t, f)

	if got.DefaultTeamKey != "ENG" || got.AuthMethod != AuthMethodAPIKey {
		t.Errorf("copied config mismatch: %+v", got)
	}
	v, err := f.secrets.Reveal(ctx, SecretKeyForWorkspace(dst))
	if err != nil {
		t.Fatalf("reveal copied secret: %v", err)
	}
	if v != "lin-src" {
		t.Errorf("secret not copied: %q", v)
	}
}

func TestCopyConfigToWorkspace_SameWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.CopyConfigToWorkspace(context.Background(), "ws-1", "ws-1"); !errors.Is(err, ErrSameWorkspace) {
		t.Fatalf("expected ErrSameWorkspace, got %v", err)
	}
}

func TestCopyConfigToWorkspace_NothingToCopy(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.CopyConfigToWorkspace(context.Background(), "ws-empty", "ws-dst"); !errors.Is(err, ErrNothingToCopy) {
		t.Fatalf("expected ErrNothingToCopy, got %v", err)
	}
}

// TestCopyConfigToWorkspace_MissingSecret guards the empty-secret boundary: a
// source config row whose stored secret is empty must fail with ErrNothingToCopy
// rather than copy an empty secret (which SetConfigForWorkspace treats as
// "preserve existing", silently leaving the target on its old credential).
func TestCopyConfigToWorkspace_MissingSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	// Seed a config row directly and an empty stored secret so revealSecret
	// returns ("", nil) instead of a not-found error.
	if err := f.store.UpsertConfigForWorkspace(ctx, src, &LinearConfig{
		AuthMethod:     AuthMethodAPIKey,
		DefaultTeamKey: "ENG",
	}); err != nil {
		t.Fatalf("seed source config: %v", err)
	}
	// Store empty values for both the workspace-scoped and legacy keys so
	// revealSecret returns ("", nil) rather than a not-found error.
	if err := f.secrets.Set(ctx, SecretKeyForWorkspace(src), "Linear API key", ""); err != nil {
		t.Fatalf("seed empty secret: %v", err)
	}
	if err := f.secrets.Set(ctx, SecretKey, "Linear API key", ""); err != nil {
		t.Fatalf("seed empty legacy secret: %v", err)
	}
	if _, err := f.svc.CopyConfigToWorkspace(ctx, src, dst); !errors.Is(err, ErrNothingToCopy) {
		t.Fatalf("expected ErrNothingToCopy for empty secret, got %v", err)
	}
}
