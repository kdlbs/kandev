package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// seedProofFixture builds an adoption fixture whose durable record names a
// credential the secret store really holds, so every refusal these tests
// observe is the ownership gate refusing and not an earlier gate short-
// circuiting on a missing record, identity, or secret.
func seedProofFixture(t *testing.T, client *fakeAdoptionControlClient) (*fakeAdoptionRecordStore, AdoptionControlClientFactory, *inMemorySecretStore) {
	t.Helper()
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "backend-credential")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	store, factory := adoptionFixture(t, record, client)
	return store, factory, secretStore
}

// TestAdoptionNeverSendsTheCredentialToAServerThatCannotProvePossession is
// the reason this gate exists. Everything a control server needs in order to
// be adopted up to this point -- the endpoint, the advertised identity, the
// capability set -- is either public or attacker-chosen, so a process that
// binds the recorded port and echoes the record's values satisfies all of
// it. Only possession of the credential distinguishes the real server, and
// the credential itself must never be the thing that demonstrates it: if a
// backend sends it first and reads the response, it has already handed a
// long-lived secret to whatever answered.
func TestAdoptionNeverSendsTheCredentialToAServerThatCannotProvePossession(t *testing.T) {
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "some-other-credential",
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 1, Credential: "rotated"},
	}
	store, factory, secretStore := seedProofFixture(t, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted {
		t.Fatalf("outcome = %+v, want unadopted", outcome)
	}
	if outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("Reason = %q, want %q", outcome.Reason, AdoptionReasonIdentityMismatch)
	}
	if len(client.presentedTokens) != 0 {
		t.Fatalf("credential was transmitted to an unproven server: %v", client.presentedTokens)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a server this backend cannot prove is its own")
	}
	if len(store.upserts) != 0 {
		t.Fatalf("the record was rewritten for an unproven server: %+v", store.upserts)
	}
}

// TestReclaimNeverSendsTheCredentialToAServerThatCannotProvePossession
// covers the same gate on the path that runs with the capability OFF. That
// path reaches a foreign server through the identical sequence and then
// stops it, so leaving it ungated would keep the vulnerability reachable in
// every shipped profile.
func TestReclaimNeverSendsTheCredentialToAServerThatCannotProvePossession(t *testing.T) {
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "some-other-credential",
	}
	store, factory, secretStore := seedProofFixture(t, client)

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if len(client.presentedTokens) != 0 {
		t.Fatalf("credential was transmitted to an unproven server: %v", client.presentedTokens)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a server this backend cannot prove is its own")
	}
}

// TestAdoptionRefusesAServerThatWillNotAnswerTheChallenge pins the fail-
// closed direction: a server that errors on the proof request, or answers
// with nothing, has not proved possession, so it is refused exactly like one
// that answers wrongly.
func TestAdoptionRefusesAServerThatWillNotAnswerTheChallenge(t *testing.T) {
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "backend-credential",
		proveErr:       errors.New("404 page not found"),
	}
	store, factory, secretStore := seedProofFixture(t, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted {
		t.Fatalf("outcome = %+v, want unadopted", outcome)
	}
	if len(client.presentedTokens) != 0 {
		t.Fatalf("credential was transmitted despite no proof: %v", client.presentedTokens)
	}
}

// TestEachAdoptionAttemptIssuesAFreshChallenge pins that the challenge is
// per-attempt. A fixed challenge would let an attacker who once observed a
// valid answer replay it forever without ever holding the credential, which
// is the entire property the proof buys.
func TestEachAdoptionAttemptIssuesAFreshChallenge(t *testing.T) {
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "backend-credential",
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 1, Credential: "backend-credential"},
	}
	store, factory, secretStore := seedProofFixture(t, client)

	for range 3 {
		AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
			testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))
	}

	if len(client.challenges) != 3 {
		t.Fatalf("challenges issued = %d, want 3 (one per attempt)", len(client.challenges))
	}
	seen := make(map[string]bool, len(client.challenges))
	for _, challenge := range client.challenges {
		if challenge == "" {
			t.Fatal("an empty challenge was issued, which proves nothing")
		}
		if seen[challenge] {
			t.Fatalf("challenge %q was reused across attempts", challenge)
		}
		seen[challenge] = true
	}
}

// TestAdoptionAcceptsAServerThatProvesPossession is the open side: the same
// gate that refuses everything above must let the real server through, or it
// would be indistinguishable from disabling adoption outright.
func TestAdoptionAcceptsAServerThatProvesPossession(t *testing.T) {
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "backend-credential",
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 4, Credential: "rotated-credential"},
	}
	store, factory, secretStore := seedProofFixture(t, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if !outcome.Adopted {
		t.Fatalf("outcome = %+v, want adopted", outcome)
	}
	if len(client.presentedTokens) == 0 || client.presentedTokens[0] != "backend-credential" {
		t.Fatalf("presented tokens = %v, want the recorded credential first", client.presentedTokens)
	}
}
