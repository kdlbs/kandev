package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
)

// TestStoreControlServerCredentialCreatesWhenNoExistingSecretID pins the
// bootstrap path: the control-server record starts with no
// CredentialSecretID, so storing the token received from the bootstrap
// handshake must create a new secret and return its ID for the caller to
// persist onto the record.
func TestStoreControlServerCredentialCreatesWhenNoExistingSecretID(t *testing.T) {
	store := newInMemorySecretStore()

	secretID, err := storeControlServerCredential(context.Background(), store, "", "the-bearer-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential: %v", err)
	}
	if secretID == "" {
		t.Fatal("secretID is empty, want a generated ID")
	}

	got, err := store.Reveal(context.Background(), secretID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if got != "the-bearer-token" {
		t.Fatalf("revealed value = %q, want %q", got, "the-bearer-token")
	}
}

// TestStoreControlServerCredentialUpdatesInPlaceWhenSecretIDGiven pins
// rotation: a credential rotation replaces the token value under the SAME
// secret ID, so ControlServerRecord.CredentialSecretID never has to change
// on an ordinary rotation.
func TestStoreControlServerCredentialUpdatesInPlaceWhenSecretIDGiven(t *testing.T) {
	store := newInMemorySecretStore()
	existingID, err := storeControlServerCredential(context.Background(), store, "", "original-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential(initial): %v", err)
	}

	gotID, err := storeControlServerCredential(context.Background(), store, existingID, "rotated-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential(rotation): %v", err)
	}
	if gotID != existingID {
		t.Fatalf("gotID = %q, want unchanged %q", gotID, existingID)
	}

	got, err := store.Reveal(context.Background(), existingID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if got != "rotated-token" {
		t.Fatalf("revealed value = %q, want %q", got, "rotated-token")
	}
}

// TestStoreControlServerCredentialFallsBackToCreateWhenSecretIDMissing pins
// the defensive case: the record names a secret ID the store no longer has
// (e.g. deleted out of band). Rotation must still durably store the new
// credential rather than fail, because the alternative -- refusing rotation
// -- would leave the backend unable to ever adopt this server again.
func TestStoreControlServerCredentialFallsBackToCreateWhenSecretIDMissing(t *testing.T) {
	store := newInMemorySecretStore()

	secretID, err := storeControlServerCredential(context.Background(), store, "does-not-exist", "fresh-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential: %v", err)
	}
	if secretID == "" || secretID == "does-not-exist" {
		t.Fatalf("secretID = %q, want a freshly generated ID", secretID)
	}

	got, err := store.Reveal(context.Background(), secretID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if got != "fresh-token" {
		t.Fatalf("revealed value = %q, want %q", got, "fresh-token")
	}
}

// TestStoreControlServerCredentialDoesNotOverwriteUnreadableExistingSecret
// pins AC-EXECUTORS-CONTROL-OWNERSHIP-001.9's "shall not delete the stored
// credential" for a transient read error: when the existing secret is
// present but a Reveal on it currently fails (not absent -- a real row a
// later attempt might still recover), storing a new credential must not
// blindly overwrite that row via Update, since Update never re-decrypts the
// prior value and would silently destroy it. It must fall back to a fresh
// secret instead, and the old row's value must remain exactly as it was.
func TestStoreControlServerCredentialDoesNotOverwriteUnreadableExistingSecret(t *testing.T) {
	store := newInMemorySecretStore()
	existingID, err := storeControlServerCredential(context.Background(), store, "", "still-needed-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential(initial): %v", err)
	}

	store.revealErr = errors.New("transient read failure")

	gotID, err := storeControlServerCredential(context.Background(), store, existingID, "unrelated-fresh-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential: %v", err)
	}
	if gotID == existingID {
		t.Fatalf("gotID = %q, want a freshly allocated ID distinct from the unreadable existing one", gotID)
	}

	store.revealErr = nil
	got, err := store.Reveal(context.Background(), existingID)
	if err != nil {
		t.Fatalf("Reveal(existingID) after fallback: %v", err)
	}
	if got != "still-needed-token" {
		t.Fatalf("existing secret value = %q, want untouched %q", got, "still-needed-token")
	}

	gotFresh, err := store.Reveal(context.Background(), gotID)
	if err != nil {
		t.Fatalf("Reveal(gotID): %v", err)
	}
	if gotFresh != "unrelated-fresh-token" {
		t.Fatalf("fresh secret value = %q, want %q", gotFresh, "unrelated-fresh-token")
	}
}

// TestStoreControlServerCredentialRejectsEmptyToken pins that an empty
// bearer token is never durably stored -- storing a blank credential would
// silently make adoption impossible to distinguish from "not yet bootstrapped".
func TestStoreControlServerCredentialRejectsEmptyToken(t *testing.T) {
	store := newInMemorySecretStore()

	if _, err := storeControlServerCredential(context.Background(), store, "", ""); err == nil {
		t.Fatal("want error for empty token, got nil")
	}
}

// TestRevealControlServerCredentialDelegatesToStore pins the read side: the
// helper returns the decrypted token for a known reference.
func TestRevealControlServerCredentialDelegatesToStore(t *testing.T) {
	store := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), store, "", "the-bearer-token")
	if err != nil {
		t.Fatalf("storeControlServerCredential: %v", err)
	}

	got, err := revealControlServerCredential(context.Background(), store, secretID)
	if err != nil {
		t.Fatalf("revealControlServerCredential: %v", err)
	}
	if got != "the-bearer-token" {
		t.Fatalf("got = %q, want %q", got, "the-bearer-token")
	}
}

// TestRevealControlServerCredentialRejectsEmptyReference pins the
// credential-unavailable case distinct from authentication failure (design
// 02's failure table): a control-server record with no credential reference
// at all must fail before ever contacting the secret store.
func TestRevealControlServerCredentialRejectsEmptyReference(t *testing.T) {
	store := newInMemorySecretStore()

	if _, err := revealControlServerCredential(context.Background(), store, ""); err == nil {
		t.Fatal("want error for empty secret reference, got nil")
	}
}

// TestRevealControlServerCredentialPropagatesNotFound pins that a stale
// reference (secret deleted out of band) surfaces as secrets.ErrNotFound so
// the caller can map it to the distinct credential-unavailable adoption
// refusal reason rather than a generic failure.
func TestRevealControlServerCredentialPropagatesNotFound(t *testing.T) {
	store := newInMemorySecretStore()

	_, err := revealControlServerCredential(context.Background(), store, "does-not-exist")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("err = %v, want wrapped %v", err, secrets.ErrNotFound)
	}
}
