package provideraccess

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func testExposureReceipt(grant Grant, lease *Lease) ExposureReceipt {
	return ExposureReceipt{
		LeaseID: lease.ID, GrantID: grant.ID, Provider: "github",
		ProviderPrincipalID: "installation:42", RepositoryID: grant.RepositoryID,
		PermissionProfile: "github_actions_rerun",
		ProviderExpiresAt: time.Now().UTC().Add(45 * time.Minute),
		Expected: FinalLeaseIdentity{
			GrantGeneration: grant.Generation, Scope: grant.Scope(),
			ManagedTaskID: lease.ManagedTaskID, SessionID: lease.SessionID,
			TargetDigest: lease.TargetDigest, ApprovalRevision: lease.ApprovalRevision,
			ConnectionGeneration: lease.ConnectionGeneration,
		},
	}
}

func TestExportedBearerRemainsResidualAfterLeaseExpiryAndFailedRevoke(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-access.db")
	open := func() (*sqlx.DB, *Store) {
		t.Helper()
		conn, err := sqlx.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewStore(conn)
		if err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		return conn, store
	}
	ctx := context.Background()
	conn, store := open()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	claim := testLeaseClaim(grant)
	lease, err := store.IssueLease(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	providerExpiry := time.Now().UTC().Add(45 * time.Minute)
	receipt := testExposureReceipt(grant, lease)
	receipt.ProviderExpiresAt = providerExpiry
	if err := store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error {
		t.Fatal("successful admission revoked its token")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRevocationResult(ctx, lease.ID, claim.ExpiresAt, false); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	conn, store = open() // Process memory, including exact-token revocation material, is gone.
	defer func() { _ = conn.Close() }()
	state, err := store.ExposureStateAt(ctx, lease.ID, claim.ExpiresAt.Add(time.Second))
	if err != nil || state != ExposureResidual {
		t.Fatalf("after failed revoke and restart: state = %s, err = %v, want residual", state, err)
	}
	state, err = store.ExposureStateAt(ctx, lease.ID, providerExpiry.Add(time.Second))
	if err != nil || state != ExposureProviderExpired {
		t.Fatalf("after provider expiry: state = %s, err = %v, want expired", state, err)
	}
}

func TestConfirmedProviderRevocationEndsExposure(t *testing.T) {
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
	if err := store.RecordExposureOrRevoke(ctx, testExposureReceipt(grant, lease), func(context.Context) error {
		t.Fatal("successful admission revoked its token")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRevocationResult(ctx, lease.ID, time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	state, err := store.ExposureStateAt(ctx, lease.ID, time.Now().UTC())
	if err != nil || state != ExposureRevokedAtProvider {
		t.Fatalf("confirmed revoke: state = %s, err = %v", state, err)
	}
}

func TestExposureAdmissionFailureRevokesUnexportedToken(t *testing.T) {
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
	receipt := testExposureReceipt(grant, lease)
	revocations := 0
	err = store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error {
		revocations++
		return nil
	})
	if !errors.Is(err, ErrGrantUnavailable) || revocations != 1 {
		t.Fatalf("losing revocation race: err = %v, revocations = %d", err, revocations)
	}
	state, err := store.ExposureStateAt(ctx, lease.ID, time.Now().UTC())
	if err != nil || state != ExposureUnknown {
		t.Fatalf("exposure after denial: state = %s, err = %v", state, err)
	}
	err = store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error {
		return errors.New("provider response with secret bytes")
	})
	if !errors.Is(err, ErrGrantUnavailable) || !errors.Is(err, ErrRevocationUnconfirmed) ||
		strings.Contains(err.Error(), "secret bytes") {
		t.Fatalf("failed provider revocation error = %v", err)
	}
}

func TestDuplicateExposureAdmissionRevokesSecondToken(t *testing.T) {
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
	receipt := testExposureReceipt(grant, lease)
	if err := store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error {
		t.Fatal("first token was revoked")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	revocations := 0
	if err := store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error {
		revocations++
		return nil
	}); err == nil || revocations != 1 {
		t.Fatalf("duplicate admission: err = %v, second-token revocations = %d", err, revocations)
	}
}

func TestFinalExposureRejectsStaleExpectedLeaseIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExposureReceipt)
	}{
		{"grant generation", func(r *ExposureReceipt) { r.Expected.GrantGeneration++ }},
		{"workspace", func(r *ExposureReceipt) { r.Expected.Scope.WorkspaceID = "other-workspace" }},
		{"plugin installation", func(r *ExposureReceipt) { r.Expected.Scope.PluginInstallationID = "other-installation" }},
		{"managed task", func(r *ExposureReceipt) { r.Expected.ManagedTaskID = "other-task" }},
		{"session", func(r *ExposureReceipt) { r.Expected.SessionID = "other-session" }},
		{"target", func(r *ExposureReceipt) { r.Expected.TargetDigest = "other-pr-head-run" }},
		{"approval", func(r *ExposureReceipt) { r.Expected.ApprovalRevision++ }},
		{"connection", func(r *ExposureReceipt) { r.Expected.ConnectionGeneration = "other-connection" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			receipt := testExposureReceipt(grant, lease)
			test.mutate(&receipt)
			revocations := 0
			err = store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error {
				revocations++
				return nil
			})
			if !errors.Is(err, ErrGrantUnavailable) || revocations != 1 {
				t.Fatalf("stale identity admission = %v, revocations = %d", err, revocations)
			}
			state, err := store.ExposureStateAt(ctx, lease.ID, time.Now().UTC())
			if err != nil || state != ExposureUnknown {
				t.Fatalf("stale identity exposure = %s, err = %v", state, err)
			}
		})
	}
}
