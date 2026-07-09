package sentry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// drainProbe waits for one async auth-health probe fired by a copy/create write.
func drainProbe(t *testing.T, f *svcFixture) {
	t.Helper()
	select {
	case <-f.probed:
	case <-time.After(2 * time.Second):
		t.Fatalf("async probe hook did not fire within 2s")
	}
}

func TestCopyConfig_CopiesInstancesAndSecrets(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	a := f.seedInstance(t, src, "SaaS", "sec-a")
	f.seedInstance(t, src, "Self", "sec-b")

	copied, err := f.svc.CopyConfigToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	drainProbe(t, f)
	drainProbe(t, f)

	if len(copied) != 2 {
		t.Fatalf("expected 2 copied instances, got %d", len(copied))
	}
	names := map[string]bool{}
	for _, c := range copied {
		if c.WorkspaceID != dst {
			t.Errorf("copied instance in wrong workspace: %+v", c)
		}
		if c.ID == a.ID {
			t.Errorf("copied instance reused source ID %q", c.ID)
		}
		names[c.Name] = true
		v, err := f.secrets.Reveal(ctx, secretKeyForInstance(c.ID))
		if err != nil || v == "" {
			t.Errorf("secret not copied for %q: %v / %q", c.Name, err, v)
		}
	}
	if !names["SaaS"] || !names["Self"] {
		t.Errorf("expected names preserved, got %v", names)
	}
}

// TestCopyConfig_DedupNamesWithInUseTarget pins acceptance (i): copying into a
// target that already holds an instance (referenced by a watch) appends a
// name-deduped copy and leaves the target's existing instance + watch intact.
func TestCopyConfig_DedupNamesWithInUseTarget(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	f.seedInstance(t, src, "SaaS", "sec-src")
	targetExisting := f.seedInstance(t, dst, "SaaS", "sec-dst")
	// Target instance is in use by a watch.
	w := newTestIssueWatch(dst)
	w.SentryInstanceID = targetExisting.ID
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed target watch: %v", err)
	}

	copied, err := f.svc.CopyConfigToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	drainProbe(t, f)
	if len(copied) != 1 || copied[0].Name != "SaaS (2)" {
		t.Fatalf("expected one deduped copy 'SaaS (2)', got %+v", copied)
	}
	// Target now has both instances; the original + its watch survive.
	all, _ := f.svc.ListInstances(ctx, dst)
	if len(all) != 2 {
		t.Fatalf("expected 2 instances in target, got %d", len(all))
	}
	if n, _ := f.store.CountWatchesForInstance(ctx, targetExisting.ID); n != 1 {
		t.Errorf("expected target's existing watch untouched, got count %d", n)
	}
}

func TestCopyConfig_SameWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.CopyConfigToWorkspace(context.Background(), "ws-1", "ws-1"); !errors.Is(err, ErrSameWorkspace) {
		t.Fatalf("expected ErrSameWorkspace, got %v", err)
	}
}

func TestCopyConfig_NothingToCopy(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.CopyConfigToWorkspace(context.Background(), "ws-empty", "ws-dst"); !errors.Is(err, ErrNothingToCopy) {
		t.Fatalf("expected ErrNothingToCopy, got %v", err)
	}
}
