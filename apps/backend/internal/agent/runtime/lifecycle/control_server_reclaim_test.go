package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReclaimUnneededControlServerNoRecordDoesNothing(t *testing.T) {
	store := &fakeAdoptionRecordStore{}
	factory := func(string) (AdoptionControlClient, error) {
		t.Fatal("client factory must not be called with no record")
		return nil, nil
	}

	ReclaimUnneededControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))
}

func TestReclaimUnneededControlServerNothingAnswersDoesNothing(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "own-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{heldCredential: "own-token", identityErr: errors.New("connection refused")}
	store, factory := adoptionFixture(t, record, client)

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a server that answered no identity")
	}
}

func TestReclaimUnneededControlServerIdentityMismatchLeavesServerUntouched(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "own-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{
		identity:       validIdentity(),
		heldCredential: "own-token",
		boundHome:      "/home/someone-else",
	}
	store, factory := adoptionFixture(t, record, client)

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a server belonging to another installation")
	}
	if len(client.presentedTokens) != 0 {
		t.Fatalf("credential was sent to a server that could not prove possession: %v", client.presentedTokens)
	}
}

func TestReclaimUnneededControlServerServerIdentityMismatchLeavesServerUntouched(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "own-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	record.ServerIdentity = "recorded-identity"
	identity := validIdentity()
	identity.ServerIdentity = "a-different-live-identity"
	client := &fakeAdoptionControlClient{heldCredential: "own-token", identity: identity}
	store, factory := adoptionFixture(t, record, client)

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a ServerIdentity-mismatched server")
	}
}

func TestReclaimUnneededControlServerCredentialUnavailableLeavesServerUntouched(t *testing.T) {
	record := validRecord()
	record.CredentialSecretID = "does-not-exist"
	client := &fakeAdoptionControlClient{identity: validIdentity()}
	store, factory := adoptionFixture(t, record, client)

	ReclaimUnneededControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called despite the credential being unavailable")
	}
}

func TestReclaimUnneededControlServerStopsOwnServerAndRepairsRecords(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "own-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{heldCredential: "own-token", identity: validIdentity()}
	store, factory := adoptionFixture(t, record, client)
	store.liveStandaloneRecords = []*models.ExecutorRunning{{SessionID: "session-1"}}

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if !client.shutdownCalled {
		t.Fatal("ShutdownControlServer was not called for a proven-own detached server")
	}
	if client.rotateResult != nil || len(client.presentedTokens) == 0 || client.presentedTokens[0] != "own-token" {
		t.Fatalf("presented tokens = %v, want the stored own-token used directly, never rotated", client.presentedTokens)
	}
	if len(store.repairedSessionIDs) != 1 || store.repairedSessionIDs[0] != "session-1" {
		t.Fatalf("repaired sessions = %v, want session-1 repaired immediately after the successful stop", store.repairedSessionIDs)
	}
}

func TestReclaimUnneededControlServerRetriesStopAndSkipsRepairOnFailure(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "own-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &countingShutdownControlClient{
		fakeAdoptionControlClient: fakeAdoptionControlClient{
			heldCredential: "own-token",
			identity:       validIdentity(),
			shutdownErr:    errors.New("500 internal server error"),
		},
	}
	store, _ := adoptionFixture(t, record, &client.fakeAdoptionControlClient)
	factory := func(string) (AdoptionControlClient, error) { return client, nil }
	store.liveStandaloneRecords = []*models.ExecutorRunning{{SessionID: "session-1"}}

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if client.shutdownAttempts < 2 {
		t.Fatalf("shutdown attempts = %d, want more than one (bounded retry)", client.shutdownAttempts)
	}
	if len(store.repairedSessionIDs) != 0 {
		t.Fatalf("repaired sessions = %v, want none when every stop retry fails", store.repairedSessionIDs)
	}
}

func TestReclaimUnneededControlServerConnectionRefusedCountsAsSuccess(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "own-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &countingShutdownControlClient{
		fakeAdoptionControlClient: fakeAdoptionControlClient{
			heldCredential: "own-token",
			identity:       validIdentity(),
			shutdownErr:    errors.New("dial tcp 127.0.0.1:9999: connect: connection refused"),
		},
	}
	store, _ := adoptionFixture(t, record, &client.fakeAdoptionControlClient)
	factory := func(string) (AdoptionControlClient, error) { return client, nil }
	store.liveStandaloneRecords = []*models.ExecutorRunning{{SessionID: "session-1"}}

	ReclaimUnneededControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, 0, -1, newAdoptionTestLogger(t))

	if client.shutdownAttempts != 1 {
		t.Fatalf("shutdown attempts = %d, want exactly 1: a connection-refused error must not be retried", client.shutdownAttempts)
	}
	if len(store.repairedSessionIDs) != 1 || store.repairedSessionIDs[0] != "session-1" {
		t.Fatalf("repaired sessions = %v, want session-1 repaired: an already-absent server counts as a successful stop", store.repairedSessionIDs)
	}
}
