package jira

import (
	"context"
	"errors"
	"testing"
	"time"
)

// drainProbe waits for the async auth-health probe fired by a config write so
// the test does not race the background DB write.
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
		SiteURL:           "https://acme.atlassian.net",
		Email:             "u@example.com",
		AuthMethod:        AuthMethodAPIToken,
		InstanceType:      InstanceTypeCloud,
		DefaultProjectKey: "ENG",
		Secret:            "tok-src",
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	drainProbe(t, f)

	got, err := f.svc.CopyConfigToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	drainProbe(t, f)

	if got.SiteURL != "https://acme.atlassian.net" || got.Email != "u@example.com" ||
		got.InstanceType != InstanceTypeCloud || got.DefaultProjectKey != "ENG" {
		t.Errorf("copied config mismatch: %+v", got)
	}
	v, err := f.secrets.Reveal(ctx, SecretKeyForWorkspace(dst))
	if err != nil {
		t.Fatalf("reveal copied secret: %v", err)
	}
	if v != "tok-src" {
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
