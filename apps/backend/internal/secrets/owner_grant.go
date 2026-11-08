package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// OwnerGrantedSecretStore resolves a global secret through an explicit owner
// binding instead of the caller's identity. It backs agent-profile secret
// bindings that the secret's owner shared with every profile starter.
//
// Both methods match only a user-visible Global secret whose stored owner is
// exactly ownerUserID. Unowned secrets, internal secrets, workspace secrets,
// and a blank owner never match, and the request context's identity is not
// consulted: callers must authorize the use before calling.
type OwnerGrantedSecretStore interface {
	GetGlobalOwnedBy(ctx context.Context, id, ownerUserID string) (*Secret, error)
	RevealGlobalOwnedBy(ctx context.Context, id, ownerUserID string) (string, error)
}

var (
	_ OwnerGrantedSecretStore = (*sqliteStore)(nil)
	_ OwnerGrantedSecretStore = (*UserVisibleStore)(nil)
)

func ownerGrantMiss(id, ownerUserID string) bool {
	return strings.TrimSpace(ownerUserID) == "" || strings.TrimSpace(id) == "" || IsInternalID(id)
}

// GetGlobalOwnedBy returns the Global secret id when ownerUserID owns it.
func (s *sqliteStore) GetGlobalOwnedBy(ctx context.Context, id, ownerUserID string) (*Secret, error) {
	if ownerGrantMiss(id, ownerUserID) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	var row secretRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT id, name, scope, workspace_id, created_at, updated_at
		FROM secrets WHERE id = ? AND user_id = ?`), id, ownerUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get owned secret: %w", err)
	}
	secret := row.toSecret()
	if secret.Scope != ScopeGlobal {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return secret, nil
}

// RevealGlobalOwnedBy returns the value of the Global secret id when
// ownerUserID owns it.
func (s *sqliteStore) RevealGlobalOwnedBy(ctx context.Context, id, ownerUserID string) (string, error) {
	if ownerGrantMiss(id, ownerUserID) {
		return "", fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	var scope SecretScope
	var ciphertext, nonce []byte
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT scope, encrypted_value, nonce FROM secrets WHERE id = ? AND user_id = ?`),
		id, ownerUserID).Scan(&scope, &ciphertext, &nonce)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return "", fmt.Errorf("reveal owned secret: %w", err)
	}
	if normalizeStoredScope(scope) != ScopeGlobal {
		return "", fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	plaintext, err := Decrypt(ciphertext, nonce, s.crypto.Key())
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

// GetGlobalOwnedBy forwards to the wrapped store when it supports owner
// bindings and treats internal ids as not found.
func (s *UserVisibleStore) GetGlobalOwnedBy(ctx context.Context, id, ownerUserID string) (*Secret, error) {
	granted, ok := s.store.(OwnerGrantedSecretStore)
	if !ok || ownerGrantMiss(id, ownerUserID) {
		return nil, internalSecretNotFound(id)
	}
	return granted.GetGlobalOwnedBy(ctx, id, ownerUserID)
}

// RevealGlobalOwnedBy forwards to the wrapped store when it supports owner
// bindings and treats internal ids as not found.
func (s *UserVisibleStore) RevealGlobalOwnedBy(ctx context.Context, id, ownerUserID string) (string, error) {
	granted, ok := s.store.(OwnerGrantedSecretStore)
	if !ok || ownerGrantMiss(id, ownerUserID) {
		return "", internalSecretNotFound(id)
	}
	return granted.RevealGlobalOwnedBy(ctx, id, ownerUserID)
}
