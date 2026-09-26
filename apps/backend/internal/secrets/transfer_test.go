package secrets

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
)

// testTransferContext returns a context carrying the given user identity, or a bare background context when userID is empty.
func testTransferContext(userID string) context.Context {
	if userID == "" {
		return context.Background()
	}
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID})
}

// mustCreate seeds a secret via the store, failing the test on error.
func mustCreate(t *testing.T, store SecretStore, secret *SecretWithValue) {
	t.Helper()
	if err := store.Create(context.Background(), secret); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
}

// strPtr returns a pointer to s for presence-aware transfer arguments.
func strPtr(s string) *string { return &s }

// TestTransferStore_CopyGlobalToWorkspace verifies a global-to-workspace copy keeps the source, sets timestamps, and carries the value server-side.
func TestTransferStore_CopyGlobalToWorkspace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v1"})

	got, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", strPtr("copied"), nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.Scope != ScopeWorkspace || got.WorkspaceID != "workspace-a" || got.Name != "copied" {
		t.Fatalf("copy metadata = %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("copy timestamps are zero: %+v", got)
	}
	// Source intact.
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source missing after copy: %v", err)
	}
	// Value carried server-side, verifiable only through reveal.
	value, err := store.RevealForWorkspace(ctx, got.ID, "workspace-a")
	if err != nil {
		t.Fatalf("reveal copy: %v", err)
	}
	if value != "v1" {
		t.Fatalf("copy value = %q, want v1", value)
	}
}

// TestTransferStore_CopyWorkspaceToGlobal verifies a workspace-to-global copy lands in the global scope with the value intact and the source kept.
func TestTransferStore_CopyWorkspaceToGlobal(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v2"})

	got, err := store.CopyScoped(ctx, "w1", "workspace-a", ScopeGlobal, "", strPtr("global-copy"), nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.Scope != ScopeGlobal || got.WorkspaceID != "" || got.Name != "global-copy" {
		t.Fatalf("copy metadata = %+v", got)
	}
	value, err := store.RevealGlobal(ctx, got.ID)
	if err != nil {
		t.Fatalf("reveal global copy: %v", err)
	}
	if value != "v2" {
		t.Fatalf("copy value = %q, want v2", value)
	}
	if _, err := store.Get(ctx, "w1"); err != nil {
		t.Fatalf("source missing after copy: %v", err)
	}
}

// TestTransferStore_CopyWorkspaceToWorkspace verifies a workspace-to-workspace copy lands in the target workspace with the value intact.
func TestTransferStore_CopyWorkspaceToWorkspace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v3"})

	got, err := store.CopyScoped(ctx, "w1", "workspace-a", ScopeWorkspace, "workspace-b", strPtr("moved"), nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.WorkspaceID != "workspace-b" {
		t.Fatalf("copy workspace = %q, want workspace-b", got.WorkspaceID)
	}
	value, err := store.RevealForWorkspace(ctx, got.ID, "workspace-b")
	if err != nil {
		t.Fatalf("reveal copy: %v", err)
	}
	if value != "v3" {
		t.Fatalf("copy value = %q, want v3", value)
	}
}

// TestTransferStore_MoveWorkspaceToGlobalRemovesSource verifies moving a workspace secret to global removes the source and preserves the value.
func TestTransferStore_MoveWorkspaceToGlobalRemovesSource(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v4"})

	got, err := store.MoveScoped(ctx, "w1", "workspace-a", ScopeGlobal, "", strPtr("global-copy"), nil)
	if err != nil {
		t.Fatalf("MoveScoped: %v", err)
	}
	if got.Scope != ScopeGlobal {
		t.Fatalf("moved metadata scope = %q", got.Scope)
	}
	if _, err := store.Get(ctx, "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
	value, err := store.RevealGlobal(ctx, got.ID)
	if err != nil {
		t.Fatalf("reveal moved copy: %v", err)
	}
	if value != "v4" {
		t.Fatalf("moved value = %q, want v4", value)
	}
}

// TestTransferStore_MoveGlobalToWorkspaceRemovesSource verifies moving a global secret into a workspace removes the source.
func TestTransferStore_MoveGlobalToWorkspaceRemovesSource(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v5"})

	got, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", strPtr("copied"), nil)
	if err != nil {
		t.Fatalf("MoveScoped: %v", err)
	}
	if got.WorkspaceID != "workspace-a" {
		t.Fatalf("moved workspace = %q", got.WorkspaceID)
	}
	if _, err := store.Get(ctx, "g1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
}

// TestTransferStore_ConflictGlobal verifies a Global target name collision returns ErrSecretNameConflict.
func TestTransferStore_ConflictGlobal(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g2", Name: "taken"}, Value: "other"})

	_, err := store.CopyScoped(ctx, "g1", "", ScopeGlobal, "", strPtr("taken"), nil)
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("error = %v, want ErrSecretNameConflict", err)
	}
}

// TestTransferStore_ConflictWorkspace verifies a workspace target name collision returns ErrSecretNameConflict.
func TestTransferStore_ConflictWorkspace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "taken", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "other"})

	_, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", strPtr("taken"), nil)
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("error = %v, want ErrSecretNameConflict", err)
	}
}

// TestTransferStore_ConflictLegacyEmptyScopeGlobal verifies a legacy empty-scope row conflicts with a Global target name.
func TestTransferStore_ConflictLegacyEmptyScopeGlobal(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	// Seed a legacy row whose stored scope is empty (pre-scope migration state).
	ciphertext, nonce, err := Encrypt([]byte("legacy"), store.crypto.Key())
	if err != nil {
		t.Fatalf("encrypt legacy: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`
		INSERT INTO secrets (id, name, user_id, scope, workspace_id, encrypted_value, nonce, created_at, updated_at)
		VALUES (?, ?, '', '', '', ?, ?, ?, ?)`),
		"legacy-1", "taken", ciphertext, nonce, now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	_, err = store.CopyScoped(ctx, "g1", "", ScopeGlobal, "", strPtr("taken"), nil)
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("error = %v, want ErrSecretNameConflict (legacy empty scope is Global)", err)
	}
}

// TestTransferStore_SourceMissing verifies copying an unknown source returns ErrNotFound.
func TestTransferStore_SourceMissing(t *testing.T) {
	store := newTestSQLiteStore(t)
	_, err := store.CopyScoped(context.Background(), "nope", "", ScopeWorkspace, "workspace-a", strPtr("x"), nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

// TestTransferStore_VerifyDestinationErrorRollsBack verifies a failing verifyDestination callback rolls back the copy, leaving no destination row and the source intact.
func TestTransferStore_VerifyDestinationErrorRollsBack(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	denied := errors.New("destination denied")
	_, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", strPtr("copied"), func(context.Context) error {
		return denied
	})
	if !errors.Is(err, denied) {
		t.Fatalf("error = %v, want the callback error", err)
	}
	items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("destination rows = %d, want 0 after callback rollback", len(items))
	}
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source lost after callback rollback: %v", err)
	}
}

// TestTransferStore_MoveFailpointAfterInsertRollsBack verifies Move is transactional: an injected delete failure rolls back the inserted copy.
func TestTransferStore_MoveFailpointAfterInsertRollsBack(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	store.failAfterInsert = func() error { return errors.New("injected delete failure") }
	_, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", strPtr("copied"), nil)
	if err == nil {
		t.Fatal("MoveScoped succeeded despite injected failpoint")
	}
	// Source intact, no destination row: Move is transactional, not copy-then-commit-delete.
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source lost after failpoint rollback: %v", err)
	}
	items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("destination rows = %d, want 0 after failpoint rollback", len(items))
	}
}

// TestTransferStore_RollbackSurvivesCanceledContext verifies the deferred
// ROLLBACK reaches SQLite even when the caller cancels the context mid-transfer:
// a canceled rollback would leave the BEGIN IMMEDIATE transaction and its write
// lock open on the pooled connection, breaking every later transfer.
func TestTransferStore_RollbackSurvivesCanceledContext(t *testing.T) {
	store := newTestSQLiteStore(t)
	// Pin the writer pool to one connection so the follow-up transfer MUST
	// reuse the connection that ran the failed transfer: a leaked BEGIN
	// IMMEDIATE transaction on it would break the next transfer.
	store.db.SetMaxOpenConns(1)
	ctx, cancel := context.WithCancel(context.Background())
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	// Cancel after the destination insert: the source delete then fails with
	// context.Canceled, and the deferred rollback must still reach SQLite.
	store.failAfterInsert = func() error {
		cancel()
		return nil
	}
	_, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", strPtr("copied"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("MoveScoped error = %v, want context.Canceled", err)
	}
	// The write lock must be released: a second transfer on the same pooled
	// connection can BEGIN IMMEDIATE again. If the rollback leaked, this
	// fails with "cannot start a transaction within a transaction".
	store.failAfterInsert = nil
	if _, err := store.MoveScoped(context.Background(), "g1", "", ScopeWorkspace, "workspace-a", strPtr("copied-again"), nil); err != nil {
		t.Fatalf("second transfer failed: %v (leaked transaction/write lock?)", err)
	}
}

// TestTransferStore_OmittedNameUsesCurrentSourceName verifies an omitted name
// resolves from the source row read inside the transaction: a rename that
// lands before the transfer is reflected in the copy (and Move removes the
// renamed row), never a stale pre-transaction read.
func TestTransferStore_OmittedNameUsesCurrentSourceName(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})
	// Simulate a rename committing between a stale pre-read and the transfer.
	if err := store.Update(ctx, "g1", &UpdateSecretRequest{Name: strPtr("renamed")}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", nil, nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.Name != "renamed" {
		t.Fatalf("copy name = %q, want the current source name %q", got.Name, "renamed")
	}
	// Move with an omitted name deletes the renamed source and carries its name.
	got2, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-b", nil, nil)
	if err != nil {
		t.Fatalf("MoveScoped: %v", err)
	}
	if got2.Name != "renamed" {
		t.Fatalf("moved name = %q, want %q", got2.Name, "renamed")
	}
	if _, err := store.Get(ctx, "g1"); err == nil {
		t.Fatal("source still present after move")
	}
}

// TestTransferStore_PerUserScoping verifies secrets are per-user: another user cannot copy or read them, while the owner can move and read the copy.
func TestTransferStore_PerUserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	alice := testTransferContext("user-a")
	bob := testTransferContext("user-b")

	// Alice owns a workspace secret; Bob must not see or move it.
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "alice-ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v"})
	// The seed above ran unscoped (background ctx), so claim it for Alice by
	// re-creating under her identity: delete the unowned row first.
	if err := store.Delete(context.Background(), "w1"); err != nil {
		t.Fatalf("cleanup seed: %v", err)
	}
	if err := store.Create(alice, &SecretWithValue{Secret: Secret{ID: "w1", Name: "alice-ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v"}); err != nil {
		t.Fatalf("seed as alice: %v", err)
	}

	if _, err := store.CopyScoped(bob, "w1", "workspace-a", ScopeGlobal, "", strPtr("x"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob copying alice's secret: err = %v, want ErrNotFound", err)
	}

	// Alice can move her own secret to Global.
	got, err := store.MoveScoped(alice, "w1", "workspace-a", ScopeGlobal, "", strPtr("alice-global"), nil)
	if err != nil {
		t.Fatalf("alice move: %v", err)
	}
	// The moved copy is owned by Alice and invisible to Bob.
	if _, err := store.Get(bob, got.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob reading alice's copy: err = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(alice, got.ID); err != nil {
		t.Fatalf("alice reading her copy: %v", err)
	}
}

// TestTransferStore_UserVisibleStoreRejectsInternalSource verifies the user-visible wrapper rejects copying internal backend-owned secrets as ErrNotFound.
func TestTransferStore_UserVisibleStoreRejectsInternalSource(t *testing.T) {
	raw := newTestSQLiteStore(t)
	// Seed an internal backend-owned row through the RAW store (the wrapper
	// refuses to create it).
	mustCreate(t, raw, &SecretWithValue{Secret: Secret{ID: "github:user:workspace:user:access", Name: "internal"}, Value: "token"})

	wrapper, ok := NewUserVisibleStore(raw).(SecretTransferStore)
	if !ok {
		t.Fatal("wrapper is not a SecretTransferStore")
	}
	_, err := wrapper.CopyScoped(context.Background(), "github:user:workspace:user:access", "", ScopeGlobal, "", strPtr("x"), nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound for internal source", err)
	}
}

// TestTransferStore_ConcurrentSameTargetSQLite verifies concurrent same-name copies to a Global target serialize on the SQLite write lock with exactly one winner.
func TestTransferStore_ConcurrentSameTargetSQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "secrets.db")

	conn1, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db 1: %v", err)
	}
	t.Cleanup(func() { _ = conn1.Close() })
	conn2, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db 2: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })

	crypto, err := NewMasterKeyProvider(dir)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store1, cleanup1, err := Provide(sqlx.NewDb(conn1, "sqlite3"), sqlx.NewDb(conn1, "sqlite3"), crypto)
	if err != nil {
		t.Fatalf("provide store 1: %v", err)
	}
	t.Cleanup(func() { _ = cleanup1() })
	store2, cleanup2, err := Provide(sqlx.NewDb(conn2, "sqlite3"), sqlx.NewDb(conn2, "sqlite3"), crypto)
	if err != nil {
		t.Fatalf("provide store 2: %v", err)
	}
	t.Cleanup(func() { _ = cleanup2() })

	ctx := context.Background()
	mustCreate(t, store1, &SecretWithValue{Secret: Secret{ID: "src", Name: "orig"}, Value: "v"})

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := store1.CopyScoped(ctx, "src", "", ScopeGlobal, "", strPtr("dup"), func(context.Context) error {
			close(locked)
			<-release
			return nil
		})
		done <- err
	}()

	// A holds the SQLite write lock (BEGIN IMMEDIATE + callback in-flight).
	<-locked

	bErr := make(chan error, 1)
	go func() {
		_, err := store2.CopyScoped(ctx, "src", "", ScopeGlobal, "", strPtr("dup"), nil)
		bErr <- err
	}()

	select {
	case err := <-bErr:
		t.Fatalf("B completed while A held the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
		// B is blocked at the SQLite transaction lock, not at a Go pool.
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("A: %v", err)
	}
	err = <-bErr
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("B error = %v, want ErrSecretNameConflict", err)
	}
}
