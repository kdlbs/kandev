// Package secretadapter wraps a global secrets.SecretStore so per-integration
// services (jira, linear, future) can use a small, upsert-style API without
// each one re-implementing the Create-or-Update fallback or the
// "secret not found:" string-prefix check.
//
// A single Adapter satisfies any integration's local SecretStore interface
// shaped as { Reveal, Set, Delete, Exists } — Go's structural typing means
// callers don't need to import this package to consume the adapter.
package secretadapter

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/secrets"
)

// Adapter exposes upsert-style access on top of secrets.SecretStore.
type Adapter struct {
	store secrets.SecretStore
}

// New returns an adapter backed by the given store.
func New(store secrets.SecretStore) *Adapter {
	return &Adapter{store: store}
}

// Reveal returns the decrypted value for id.
func (a *Adapter) Reveal(ctx context.Context, id string) (string, error) {
	return a.store.Reveal(ctx, id)
}

// Set upserts the secret value: tries Update first, falls back to Create
// when the secret does not yet exist.
func (a *Adapter) Set(ctx context.Context, id, name, value string) error {
	// Detect existence via Exists (which inspects the "secret not found:"
	// prefix) instead of treating any Get error as "not found": a transient
	// DB error on an existing row would otherwise turn into a constraint-
	// violation Create that masks the real cause.
	exists, err := a.Exists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return a.store.Update(ctx, id, &secrets.UpdateSecretRequest{Value: &value})
	}
	return a.store.Create(ctx, &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: id, Name: name},
		Value:  value,
	})
}

// Delete removes the secret with the given id.
func (a *Adapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

// Exists reports whether a secret with id exists. Returns (false, nil) when
// the row is absent, and (false, err) on any other error so callers can
// distinguish "not configured" from a backend outage.
func (a *Adapter) Exists(ctx context.Context, id string) (bool, error) {
	_, err := a.store.Get(ctx, id)
	if err != nil {
		// secrets layer reports "not found" via fmt.Errorf with the
		// "secret not found:" prefix; treat that as the absence case.
		if strings.HasPrefix(err.Error(), "secret not found:") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
