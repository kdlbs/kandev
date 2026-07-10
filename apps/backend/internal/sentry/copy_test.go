package sentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
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

func TestCopyConfig_SecretSetFailureRollsBackAllCreatedInstances(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	f.seedInstance(t, src, "First", "sec-first")
	f.seedInstance(t, src, "Second", "sec-second")
	existing := f.seedInstance(t, dst, "Existing", "sec-existing")
	watch := newTestIssueWatch(dst)
	watch.SentryInstanceID = existing.ID
	if err := f.store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("seed target watch: %v", err)
	}

	injected := errors.New("secret store unavailable")
	missingDelete := errors.New("secret not found during rollback")
	f.secrets.setErr = injected
	f.secrets.setErrAfter = 1 // Let the first copied secret succeed; fail the second.
	f.secrets.deleteMissingErr = missingDelete
	_, err := f.svc.CopyConfigToWorkspace(ctx, src, dst)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected secret error, got %v", err)
	}
	if errors.Is(err, missingDelete) {
		t.Fatalf("rollback attempted to delete an unstored copied secret: %v", err)
	}

	target, err := f.svc.ListInstances(ctx, dst)
	if err != nil {
		t.Fatalf("list target after failed copy: %v", err)
	}
	if len(target) != 1 || target[0].ID != existing.ID {
		t.Fatalf("failed copy changed target instances: %+v", target)
	}
	if secret, err := f.secrets.Reveal(ctx, secretKeyForInstance(existing.ID)); err != nil || secret != "sec-existing" {
		t.Errorf("failed copy changed target secret: secret=%q err=%v", secret, err)
	}
	if watches, err := f.store.CountWatchesForInstance(ctx, existing.ID); err != nil || watches != 1 {
		t.Errorf("failed copy changed target watch count: count=%d err=%v", watches, err)
	}
	if got := f.secrets.count(); got != 3 {
		t.Errorf("failed copy left copied secrets: count=%d, want 3", got)
	}
}

func TestCopyConfig_PartialSecretSetFailureRollsBackAllCopiedSecrets(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	f.seedInstance(t, src, "First", "sec-first")
	f.seedInstance(t, src, "Second", "sec-second")
	existing := f.seedInstance(t, dst, "Existing", "sec-existing")

	injected := errors.New("secret store partially wrote then failed")
	f.secrets.setErr = injected
	f.secrets.setErrAfter = 1 // Let the first copied secret succeed; partially write the second.
	f.secrets.setErrAfterWrite = true
	if _, err := f.svc.CopyConfigToWorkspace(ctx, src, dst); !errors.Is(err, injected) {
		t.Fatalf("expected injected secret error, got %v", err)
	}

	target, err := f.svc.ListInstances(ctx, dst)
	if err != nil {
		t.Fatalf("list target after partial secret failure: %v", err)
	}
	if len(target) != 1 || target[0].ID != existing.ID {
		t.Fatalf("partial secret failure changed target instances: %+v", target)
	}
	if got := f.secrets.count(); got != 3 {
		t.Errorf("partial secret failure left copied secrets: count=%d, want 3", got)
	}
}

func TestRollbackCopiedInstances_DeletesSecretAfterRowDeleteFailure(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created := f.seedInstance(t, "ws-dst", "Copied", "sec-copied")
	watch := newTestIssueWatch("ws-dst")
	watch.SentryInstanceID = created.ID
	if err := f.store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("seed watch: %v", err)
	}

	core, observed := observer.New(zap.WarnLevel)
	testLogger, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	f.svc.log = testLogger

	cause := errors.New("copy failed")
	err = f.svc.rollbackCopiedInstances(ctx, []*SentryConfig{created}, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("expected original copy error, got %v", err)
	}
	var inUse ErrInstanceInUse
	if !errors.As(err, &inUse) {
		t.Fatalf("expected joined row cleanup error, got %v", err)
	}
	if exists, err := f.secrets.Exists(ctx, secretKeyForInstance(created.ID)); err != nil || exists {
		t.Errorf("secret remains after row cleanup failure: exists=%t err=%v", exists, err)
	}
	entries := observed.All()
	if len(entries) != 1 || entries[0].Message != "sentry: copy rollback cleanup failed" {
		t.Fatalf("expected one rollback cleanup warning, got %+v", entries)
	}
}

func TestCopyConfig_SourceSecretReadFailureRollsBackAllCreatedInstances(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	f.seedInstance(t, src, "First", "sec-first")
	second := f.seedInstance(t, src, "Second", "sec-second")
	existing := f.seedInstance(t, dst, "Existing", "sec-existing")

	injected := errors.New("source secret unavailable")
	f.secrets.revealErr = injected
	f.secrets.revealErrForID = secretKeyForInstance(second.ID)
	if _, err := f.svc.CopyConfigToWorkspace(ctx, src, dst); !errors.Is(err, injected) {
		t.Fatalf("expected injected source secret error, got %v", err)
	}

	target, err := f.svc.ListInstances(ctx, dst)
	if err != nil {
		t.Fatalf("list target after failed copy: %v", err)
	}
	if len(target) != 1 || target[0].ID != existing.ID {
		t.Fatalf("failed copy changed target instances: %+v", target)
	}
	if got := f.secrets.count(); got != 3 {
		t.Errorf("failed copy left copied secrets: count=%d, want 3", got)
	}
}

func TestCopyConfig_SourceSecretReadFailurePrecedesTargetMutations(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, dst = "ws-src", "ws-dst"
	f.seedInstance(t, src, "First", "sec-first")
	second := f.seedInstance(t, src, "Second", "sec-second")
	existing := f.seedInstance(t, dst, "Existing", "sec-existing")

	sourceReadErr := errors.New("source secret unavailable")
	targetSetErr := errors.New("target secret should not be stored")
	f.secrets.revealErr = sourceReadErr
	f.secrets.revealErrForID = secretKeyForInstance(second.ID)
	f.secrets.setErr = targetSetErr
	_, err := f.svc.CopyConfigToWorkspace(ctx, src, dst)
	if !errors.Is(err, sourceReadErr) {
		t.Fatalf("expected source read error before target mutation, got %v", err)
	}
	if errors.Is(err, targetSetErr) {
		t.Fatalf("copy attempted target secret storage before reading all sources: %v", err)
	}

	target, err := f.svc.ListInstances(ctx, dst)
	if err != nil {
		t.Fatalf("list target after failed source preflight: %v", err)
	}
	if len(target) != 1 || target[0].ID != existing.ID {
		t.Fatalf("source preflight changed target instances: %+v", target)
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
