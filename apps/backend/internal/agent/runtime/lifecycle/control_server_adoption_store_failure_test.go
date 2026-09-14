package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestAdoptionTreatsAnUnreadableRecordAsNoServer pins the durable-read
// failure branch. A store that cannot answer is not evidence that a control
// server is absent, but it is also not a refusal: there is nothing to refuse
// yet. Adoption takes the same spawn-fresh path as a genuinely empty record,
// and in particular never contacts an endpoint it could not read.
func TestAdoptionTreatsAnUnreadableRecordAsNoServer(t *testing.T) {
	store := &fakeAdoptionRecordStore{getErr: errors.New("database is locked")}
	factory := func(string) (AdoptionControlClient, error) {
		t.Fatal("a client was built for a record that could not be read")
		return nil, nil
	}

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted {
		t.Fatalf("outcome = %+v, want unadopted", outcome)
	}
	if outcome.Reason != AdoptionReasonNoServer {
		t.Fatalf("Reason = %q, want %q", outcome.Reason, AdoptionReasonNoServer)
	}
}

// TestAdoptionIsIncompleteWhenTheRecordWriteFails is the sibling of the
// secret-write failure. The rotation succeeded on the server, so the server
// now holds a credential this backend could not durably record. Confirming
// it would tell the server to drop the superseded credential, which is the
// only one still recoverable from durable state, stranding the server
// permanently. Adoption is therefore incomplete and confirm is never sent.
func TestAdoptionIsIncompleteWhenTheRecordWriteFails(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "current-token",
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 9, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)
	store.putErr = errors.New("database is locked")

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCredentialRotationFailed {
		t.Fatalf("outcome = %+v, want unadopted credential_rotation_failed", outcome)
	}
	if client.confirmCalled {
		t.Fatal("ConfirmCredentialRotation was sent despite the record write failing")
	}
	if len(store.upserts) != 0 {
		t.Fatalf("upserts = %+v, want none recorded when the write failed", store.upserts)
	}
}
