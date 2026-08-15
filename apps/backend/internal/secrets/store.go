package secrets

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/jmoiron/sqlx"
)

// ErrNotFound is the sentinel returned (wrapped) by store implementations when
// a secret id is absent. Consumers that must tell "absent entry" apart from a
// genuine backend fault should match with errors.Is(err, secrets.ErrNotFound)
// rather than string-matching the error text.
var ErrNotFound = errors.New("secret not found")

// ErrSecretNameConflict is returned when a copy/move target name already
// exists in the destination scope. Handlers map it to 409 Conflict.
var ErrSecretNameConflict = errors.New("a secret with this name already exists in the target scope")

// ErrWorkspaceAccessDenied is returned for missing or unauthorized
// destination workspaces (and unconfigured workspace verification). Handlers
// map it to 404 Not Found so a nonexistent workspace is indistinguishable
// from an inaccessible one.
var ErrWorkspaceAccessDenied = errors.New("workspace access denied or workspace not found")

// ErrSecretValidation marks validation failures on copy/move requests.
// Handlers map it to 400 Bad Request.
var ErrSecretValidation = errors.New("secret validation failed")

// GlobalSecretTransferLockKey is the advisory-lock key used for transfers
// whose target scope is Global. It is a distinct stable constant, never
// derived from a workspace ID, so concurrent same-name Global transfers
// serialize and exactly one wins.
const GlobalSecretTransferLockKey int64 = 0x4B53544600000001 // "KSTF"

const transferLockPrefix = "kandev-secret-transfer:"

// WorkspaceLockKey returns the deterministic advisory-lock key for a
// workspace. The FNV-1a 64-bit hash is stable across restarts and
// collision-safe for UUID inputs (a theoretical collision only over-serializes
// transfers, never corrupts data). PostgreSQL uses these keys to coordinate
// secret transfers with workspace deletion; SQLite's single writer needs no
// advisory locks.
func WorkspaceLockKey(workspaceID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(transferLockPrefix + workspaceID))
	return int64(h.Sum64())
}

// SecretStore abstracts secret storage. Implementations handle
// encryption/decryption internally.
type SecretStore interface {
	// Create stores a new secret (encrypts the value).
	Create(ctx context.Context, secret *SecretWithValue) error

	// Get retrieves secret metadata (without value).
	Get(ctx context.Context, id string) (*Secret, error)

	// Reveal retrieves the decrypted value of a secret.
	Reveal(ctx context.Context, id string) (string, error)

	// Update updates a secret's name and/or value.
	Update(ctx context.Context, id string, req *UpdateSecretRequest) error

	// Delete permanently removes a secret.
	Delete(ctx context.Context, id string) error

	// List returns all secrets without values.
	List(ctx context.Context) ([]*SecretListItem, error)

	// Close releases resources.
	Close() error
}

// ValidateGlobalReference verifies that a stored reference is usable by a
// shared agent or executor profile without decrypting its value.
func ValidateGlobalReference(ctx context.Context, store SecretStore, id string) error {
	if store == nil || id == "" {
		return fmt.Errorf("global secret reference is required")
	}
	secret, err := store.Get(ctx, id)
	if err != nil {
		return err
	}
	if normalizeStoredScope(secret.Scope) != ScopeGlobal {
		return fmt.Errorf("secret reference must use the global scope")
	}
	return nil
}

// ScopedSecretStore is the optional scope-aware extension used by user-facing
// settings, repository binding validation, and task-environment resolution.
// SecretStore deliberately remains small so existing integration adapters and
// test doubles do not need to know about workspace ownership.
type ScopedSecretStore interface {
	SecretStore
	ListScoped(ctx context.Context, opts SecretListOptions) ([]*SecretListItem, error)
	GetForWorkspace(ctx context.Context, id, workspaceID string) (*Secret, error)
	RevealGlobal(ctx context.Context, id string) (string, error)
	RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error)
	DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error
}

// SecretTransferStore is implemented by stores that can atomically copy or
// move a secret into another scope/workspace. It is a narrow optional
// interface, separate from ScopedSecretStore, so scope-aware read and
// lifecycle stores (and their test doubles) are not forced to implement
// transfer behavior.
type SecretTransferStore interface {
	// CopyScoped atomically copies a secret into another scope/workspace and
	// returns the new row's metadata. requestedName nil keeps the source
	// name, resolved from the source row read inside the transaction (so a
	// concurrent rename can never split the copied name from its value).
	// verifyDestination runs inside the transaction while the destination
	// lock is held, before the conflict check and insert; an error rolls back
	// and surfaces. sourceWorkspaceID is the resolved source's workspace
	// ("" for Global) and is locked alongside the destination for Move so
	// concurrent same-source moves serialize without deadlock.
	CopyScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error) (*Secret, error)
	// MoveScoped atomically copies a secret into another scope/workspace and
	// deletes the source in the same transaction; the name contract matches
	// CopyScoped.
	MoveScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error) (*Secret, error)
}

// WorkspaceSecretTransactionalDeleter is implemented by stores that can
// remove workspace-owned secrets on a caller-owned SQL transaction.
type WorkspaceSecretTransactionalDeleter interface {
	DeleteWorkspaceSecretsTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error
}
