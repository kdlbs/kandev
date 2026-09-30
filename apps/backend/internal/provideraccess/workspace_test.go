package provideraccess

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFenceWorkspaceBlocksNewGrantsAndPreservesResidualExposure(t *testing.T) {
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
	if _, err := store.ClaimMintIntent(ctx, testMintClaim(grant, lease)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordExposureOrRevoke(ctx, testExposureReceipt(grant, lease),
		func(context.Context) error { t.Fatal("successful admission revoked token"); return nil }); err != nil {
		t.Fatal(err)
	}
	result, err := store.FenceWorkspace(ctx, grant.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ExposedLeaseIDs) != 1 || result.ExposedLeaseIDs[0] != lease.ID {
		t.Fatalf("tokens requiring revoke = %+v", result)
	}
	if state, err := store.ExposureStateAt(ctx, lease.ID, time.Now().UTC()); err != nil || state != ExposureResidual {
		t.Fatalf("fenced exposure = %s, err = %v", state, err)
	}
	newGrant := testGrant("grant-2")
	if err := store.ReplaceGrant(ctx, &newGrant); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("new grant after workspace fence = %v", err)
	}
	if _, err := store.ClaimMintIntent(ctx, testMintClaim(grant, lease)); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("mint after workspace fence = %v", err)
	}
	if _, err := store.FenceWorkspace(ctx, grant.WorkspaceID); err != nil {
		t.Fatalf("idempotent workspace fence: %v", err)
	}
}

func TestFenceWorkspacePreservesUnknownMintAndRemovesUnusedLease(t *testing.T) {
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
	if _, err := store.ClaimMintIntent(ctx, testMintClaim(grant, lease)); err != nil {
		t.Fatal(err)
	}
	result, err := store.FenceWorkspace(ctx, grant.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UnknownMintLeaseIDs) != 1 || result.UnknownMintLeaseIDs[0] != lease.ID {
		t.Fatalf("unknown mint receipt = %+v", result)
	}
	if got, err := store.GetMintIntent(ctx, lease.ID); err != nil || got == nil {
		t.Fatalf("retained unknown mint = %+v, err = %v", got, err)
	}
}
