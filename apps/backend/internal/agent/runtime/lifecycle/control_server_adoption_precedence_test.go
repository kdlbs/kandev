package lifecycle

import (
	"context"
	"errors"
	"testing"
)

// TestAttemptAdoptControlServerIdentityMismatchTakesPrecedenceOverCredentialUnavailable
// pins AC-001.6's refusal-reason precedence order: identity mismatch is
// evaluated before credential unavailability, so when both conditions hold --
// here, a mismatched ServerIdentity AND a CredentialSecretID the secret store
// has no row for -- the recorded reason is identity_mismatch, not
// credential_unavailable, because the credential-reveal gate is never
// reached.
func TestAttemptAdoptControlServerIdentityMismatchTakesPrecedenceOverCredentialUnavailable(t *testing.T) {
	record := validRecord()
	record.ServerIdentity = "recorded-identity"
	record.CredentialSecretID = "does-not-exist"
	identity := validIdentity()
	identity.ServerIdentity = "a-different-live-identity"
	client := &fakeAdoptionControlClient{identity: identity}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("outcome = %+v, want unadopted identity_mismatch even though credential_unavailable also applies", outcome)
	}
	if len(client.presentedTokens) != 0 {
		t.Fatal("RotateCredential's SetAuthToken was reached despite an identity mismatch")
	}
}

// TestAttemptAdoptControlServerCredentialUnavailableTakesPrecedenceOverAuthenticationFailed
// pins AC-001.6's refusal-reason precedence order: credential unavailability
// is evaluated before authentication failure, so when both conditions hold --
// here, a CredentialSecretID the secret store has no row for AND a client
// that would refuse any rotation attempt -- the recorded reason is
// credential_unavailable, not authentication_failed, because RotateCredential
// is never called.
func TestAttemptAdoptControlServerCredentialUnavailableTakesPrecedenceOverAuthenticationFailed(t *testing.T) {
	record := validRecord()
	record.CredentialSecretID = "does-not-exist"
	client := &fakeAdoptionControlClient{
		identity:  validIdentity(),
		rotateErr: errors.New("401 invalid auth token"),
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCredentialUnavailable {
		t.Fatalf("outcome = %+v, want unadopted credential_unavailable even though authentication_failed also applies", outcome)
	}
	if len(client.presentedTokens) != 0 {
		t.Fatal("RotateCredential's SetAuthToken was reached despite no credential being available")
	}
}
