package provideraccess

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func testMintClaim(grant Grant, lease *Lease) MintClaim {
	return MintClaim{LeaseID: lease.ID, GrantID: grant.ID,
		Expected: testExposureReceipt(grant, lease).Expected}
}

func TestMintIntentIsOneShotAcrossRestart(t *testing.T) {
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
	claim := testMintClaim(grant, lease)
	intent, err := store.ClaimMintIntent(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	if intent.PossibleProviderExpiry.Before(time.Now().Add(time.Hour)) {
		t.Fatalf("conservative expiry = %s", intent.PossibleProviderExpiry)
	}
	reopened, err := NewStore(store.db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetMintIntent(ctx, lease.ID)
	if err != nil || got == nil || got.PossibleProviderExpiry.Unix() != intent.PossibleProviderExpiry.Unix() {
		t.Fatalf("persisted intent = %+v, err = %v", got, err)
	}
	if _, err := reopened.ClaimMintIntent(ctx, claim); !errors.Is(err, ErrLeaseAlreadyRedeemed) {
		t.Fatalf("replayed mint = %v", err)
	}
}

func TestMintIntentRejectsRevokedAndStaleIdentity(t *testing.T) {
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
	claim := testMintClaim(grant, lease)
	claim.Expected.SessionID = "foreign-session"
	if _, err := store.ClaimMintIntent(ctx, claim); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("stale session claim = %v", err)
	}
	claim = testMintClaim(grant, lease)
	if err := store.RevokeGrant(ctx, grant.WorkspaceID, grant.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimMintIntent(ctx, claim); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("revoked grant claim = %v", err)
	}
	if got, err := store.GetMintIntent(ctx, lease.ID); err != nil || got != nil {
		t.Fatalf("denied mint intent = %+v, err = %v", got, err)
	}
}

func TestConcurrentMintClaimsHaveOneWinner(t *testing.T) {
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
	claim := testMintClaim(grant, lease)
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.ClaimMintIntent(ctx, claim)
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrLeaseAlreadyRedeemed) {
			t.Errorf("unexpected claim result: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("mint claims succeeded = %d, want 1", winners)
	}
}
