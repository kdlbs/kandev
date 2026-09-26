package provideraccess

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func testLeaseClaim(grant Grant) LeaseClaim {
	return LeaseClaim{
		ID: "lease-1", GrantID: grant.ID, GrantGeneration: grant.Generation,
		Scope: grant.Scope(), ManagedTaskID: "managed-task-1", SessionID: "session-1",
		TargetDigest: "pr-99-head-a-run-1", ApprovalRevision: 3,
		ConnectionGeneration: "connection-1", IdempotencyKey: "logical-request-1",
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
	}
}

func TestStoreIssueLeaseReplaysExactIdentityAndRejectsChangedScope(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	claim := testLeaseClaim(grant)
	first, err := store.IssueLease(ctx, claim)
	if err != nil || first == nil || first.ID != claim.ID {
		t.Fatalf("first lease = %+v, err = %v", first, err)
	}
	claim.ID = "ignored-on-replay"
	replay, err := store.IssueLease(ctx, claim)
	if err != nil || replay == nil || replay.ID != first.ID {
		t.Fatalf("replay = %+v, err = %v, want original lease", replay, err)
	}
	claim.TargetDigest = "different-pr-head"
	if _, err := store.IssueLease(ctx, claim); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("changed target error = %v, want ErrLeaseConflict", err)
	}
}

func TestStoreRevocationFencesOutstandingLease(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	lease, err := store.IssueLease(ctx, testLeaseClaim(grant))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeGrant(ctx, grant.WorkspaceID, grant.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetActiveLease(ctx, lease.ID)
	if err != nil || active != nil {
		t.Fatalf("active lease = %+v, err = %v, want none", active, err)
	}
}

func TestStoreReplacementFencesPreviousGenerationLease(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	first := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &first); err != nil {
		t.Fatal(err)
	}
	lease, err := store.IssueLease(ctx, testLeaseClaim(first))
	if err != nil {
		t.Fatal(err)
	}
	second := testGrant("grant-2")
	if err := store.ReplaceGrant(ctx, &second); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetActiveLease(ctx, lease.ID)
	if err != nil || active != nil {
		t.Fatalf("previous generation lease = %+v, err = %v", active, err)
	}
	if _, err := store.IssueLease(ctx, testLeaseClaim(first)); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("stale generation error = %v", err)
	}
}

func TestStoreLeaseAdmissionRejectsCrossScopeAndOverlongExpiry(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	grant.ExpiresAt = time.Now().UTC().Add(3 * time.Minute)
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	claim := testLeaseClaim(grant)
	claim.Scope.WorkspaceID = "other-workspace"
	if _, err := store.IssueLease(ctx, claim); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("cross-scope error = %v", err)
	}
	claim = testLeaseClaim(grant)
	claim.ExpiresAt = grant.ExpiresAt.Add(time.Minute)
	if _, err := store.IssueLease(ctx, claim); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("overlong lease error = %v", err)
	}
}

func TestStoreLeaseAdmissionRejectsLifetimeBeyondFiveMinutes(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	claim := testLeaseClaim(grant)
	claim.ExpiresAt = time.Now().UTC().Add(6 * time.Minute)
	if _, err := store.IssueLease(ctx, claim); !errors.Is(err, ErrLeaseTooLong) {
		t.Fatalf("six-minute lease error = %v, want ErrLeaseTooLong", err)
	}
}

func TestStoreConcurrentLeaseReplayProducesOneReceipt(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	claim := testLeaseClaim(grant)
	const count = 8
	var wg sync.WaitGroup
	results := make(chan *Lease, count)
	errs := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := store.IssueLease(ctx, claim)
			if err != nil {
				errs <- err
				return
			}
			results <- lease
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent lease: %v", err)
	}
	close(results)
	for lease := range results {
		if lease.ID != claim.ID {
			t.Fatalf("concurrent receipt ID = %s", lease.ID)
		}
	}
}
