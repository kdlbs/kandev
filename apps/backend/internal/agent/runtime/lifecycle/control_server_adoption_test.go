package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// failingUpdateSecretStore fails Update for one specific secret ID, so a
// test can force the durable-storage failure branch of adoption's rotation
// finalize step without the fallback-to-create path in
// storeControlServerCredential silently absorbing it.
type failingUpdateSecretStore struct {
	*inMemorySecretStore
	failUpdateFor string
}

func (s *failingUpdateSecretStore) Update(ctx context.Context, id string, req *secrets.UpdateSecretRequest) error {
	if id == s.failUpdateFor {
		return errors.New("injected secret update failure")
	}
	return s.inMemorySecretStore.Update(ctx, id, req)
}

func newAdoptionTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

// fakeAdoptionRecordStore is an in-memory AdoptionRecordStore double.
type fakeAdoptionRecordStore struct {
	record  *models.ControlServerRecord
	getErr  error
	upserts []*models.ControlServerRecord
	putErr  error
}

func (f *fakeAdoptionRecordStore) GetControlServerRecord(context.Context) (*models.ControlServerRecord, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.record == nil {
		return nil, models.ErrControlServerRecordNotFound
	}
	return f.record, nil
}

func (f *fakeAdoptionRecordStore) UpsertControlServerRecord(_ context.Context, record *models.ControlServerRecord) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.upserts = append(f.upserts, record)
	return nil
}

// fakeAdoptionControlClient is an in-memory AdoptionControlClient double.
type fakeAdoptionControlClient struct {
	identity        *agentctl.IdentityInfo
	identityErr     error
	rotateResult    *agentctl.CredentialRotationResult
	rotateErr       error
	confirmErr      error
	shutdownErr     error
	shutdownCalled  bool
	confirmedID     int64
	confirmCalled   bool
	presentedTokens []string
}

func (f *fakeAdoptionControlClient) SetAuthToken(token string) {
	f.presentedTokens = append(f.presentedTokens, token)
}

func (f *fakeAdoptionControlClient) GetIdentity(context.Context) (*agentctl.IdentityInfo, error) {
	return f.identity, f.identityErr
}

func (f *fakeAdoptionControlClient) RotateCredential(context.Context) (*agentctl.CredentialRotationResult, error) {
	return f.rotateResult, f.rotateErr
}

func (f *fakeAdoptionControlClient) ConfirmCredentialRotation(_ context.Context, rotationID int64) error {
	f.confirmCalled = true
	f.confirmedID = rotationID
	return f.confirmErr
}

func (f *fakeAdoptionControlClient) ShutdownControlServer(context.Context) error {
	f.shutdownCalled = true
	return f.shutdownErr
}

const testHomeDir = "/home/kandev"

func validRecord() *models.ControlServerRecord {
	return &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:9999",
		ServerIdentity:     "old-identity",
		CredentialSecretID: "secret-1",
		Capabilities:       []string{"agent-survival.v1"},
		DiagnosticLogPath:  "/home/kandev/logs/agentctl-diagnostic.log",
	}
}

func validIdentity() *agentctl.IdentityInfo {
	return &agentctl.IdentityInfo{
		HomeDir:           testHomeDir,
		ServerIdentity:    "new-identity",
		Capabilities:      []string{"agent-survival.v1"},
		DiagnosticLogPath: "/home/kandev/logs/agentctl-diagnostic.log",
	}
}

func adoptionFixture(t *testing.T, record *models.ControlServerRecord, client *fakeAdoptionControlClient) (*fakeAdoptionRecordStore, func(string) (AdoptionControlClient, error)) {
	t.Helper()
	store := &fakeAdoptionRecordStore{record: record}
	factory := func(string) (AdoptionControlClient, error) { return client, nil }
	return store, factory
}

// TestAttemptAdoptControlServerNoRecordSpawnsFresh pins
// AC-EXECUTORS-CONTROL-OWNERSHIP-001.8's "no recorded control endpoint"
// branch: nothing to adopt, no refusal reason, caller spawns fresh.
func TestAttemptAdoptControlServerNoRecordSpawnsFresh(t *testing.T) {
	store := &fakeAdoptionRecordStore{}
	factory := func(string) (AdoptionControlClient, error) {
		t.Fatal("client factory must not be called with no record")
		return nil, nil
	}

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted {
		t.Fatal("Adopted = true, want false")
	}
	if outcome.Reason != AdoptionReasonNoServer {
		t.Fatalf("Reason = %q, want %q", outcome.Reason, AdoptionReasonNoServer)
	}
}

// TestAttemptAdoptControlServerUnreachableSpawnsFresh pins the "recorded
// endpoint answers nothing" branch of AC-001.8, folded into the same
// no-refusal reason as no-record.
func TestAttemptAdoptControlServerUnreachableSpawnsFresh(t *testing.T) {
	client := &fakeAdoptionControlClient{identityErr: errors.New("connection refused")}
	store, factory := adoptionFixture(t, validRecord(), client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonNoServer {
		t.Fatalf("outcome = %+v, want unadopted no_server", outcome)
	}
}

// TestAttemptAdoptControlServerHomeMismatchRefusesWithoutStop pins AC-001.4:
// a home-directory mismatch is refused, and -- critically -- ShutdownControlServer
// is never called, because a process this installation cannot prove is its
// own is never stopped.
func TestAttemptAdoptControlServerHomeMismatchRefusesWithoutStop(t *testing.T) {
	identity := validIdentity()
	identity.HomeDir = "/home/someone-else"
	client := &fakeAdoptionControlClient{identity: identity}
	store, factory := adoptionFixture(t, validRecord(), client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("outcome = %+v, want unadopted identity_mismatch", outcome)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called for a home-mismatched server")
	}
}

// TestAttemptAdoptControlServerNoCapabilitiesAdvertisedIsIdentityMismatch
// pins AC-001.5: a server that advertises no capability set at all is
// treated as AC-001.4, not as a capability-incompatible server -- so it is
// never stopped either.
func TestAttemptAdoptControlServerNoCapabilitiesAdvertisedIsIdentityMismatch(t *testing.T) {
	identity := validIdentity()
	identity.Capabilities = nil
	client := &fakeAdoptionControlClient{identity: identity}
	store, factory := adoptionFixture(t, validRecord(), client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("Reason = %q, want %q", outcome.Reason, AdoptionReasonIdentityMismatch)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called for a server advertising no capabilities")
	}
}

// TestAttemptAdoptControlServerCredentialUnavailableRefusesWithoutStop pins
// AC-001.9: the stored credential cannot be retrieved (here, a stale
// reference the fake secret store has no row for), so no authentication is
// even attempted, the server is left running, and RotateCredential is never
// called.
func TestAttemptAdoptControlServerCredentialUnavailableRefusesWithoutStop(t *testing.T) {
	client := &fakeAdoptionControlClient{identity: validIdentity()}
	record := validRecord()
	record.CredentialSecretID = "does-not-exist"
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCredentialUnavailable {
		t.Fatalf("outcome = %+v, want unadopted credential_unavailable", outcome)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a credential-unavailable refusal")
	}
	if len(client.presentedTokens) != 0 {
		t.Fatal("RotateCredential's SetAuthToken was reached despite no credential being available")
	}
}

// TestAttemptAdoptControlServerAuthenticationFailureRefusesWithoutStop pins
// AC-001.4's authentication-failure branch: rotate is attempted and refused
// by the server, so this backend treats it as foreign and never stops it.
func TestAttemptAdoptControlServerAuthenticationFailureRefusesWithoutStop(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "stale-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{
		identity:  validIdentity(),
		rotateErr: errors.New("401 invalid auth token"),
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonAuthenticationFailed {
		t.Fatalf("outcome = %+v, want unadopted authentication_failed", outcome)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on an authentication-failure refusal")
	}
}

// TestAttemptAdoptControlServerIncompatibleCapabilityStopsSurvivor pins
// AC-004.3/004.4: an own, authenticated server missing a required
// capability is refused AND stopped -- the one refusal reason that DOES
// issue a stop, because only an own authenticated server may ever be
// stopped.
func TestAttemptAdoptControlServerIncompatibleCapabilityStopsSurvivor(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	identity := validIdentity()
	identity.Capabilities = []string{"some-other-capability"}
	client := &fakeAdoptionControlClient{
		identity:     identity,
		rotateResult: &agentctl.CredentialRotationResult{RotationID: 7, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCapabilityIncompatible {
		t.Fatalf("outcome = %+v, want unadopted capability_incompatible", outcome)
	}
	if !client.shutdownCalled {
		t.Fatal("ShutdownControlServer was not called for an incompatible own server")
	}
	if len(store.upserts) != 0 {
		t.Fatal("control server record was rewritten by the refused adoption itself; that is the caller's job after spawning fresh")
	}
}

// TestAttemptAdoptControlServerSucceedsRotatesAndPersists pins the happy
// path: home matches, rotation succeeds, capability subset is satisfied,
// the new credential is durably stored under the SAME secret ID, the record
// is rewritten with the freshly observed identity fields, and confirm is
// sent last.
func TestAttemptAdoptControlServerSucceedsRotatesAndPersists(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	identity := validIdentity()
	client := &fakeAdoptionControlClient{
		identity:     identity,
		rotateResult: &agentctl.CredentialRotationResult{RotationID: 3, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if !outcome.Adopted {
		t.Fatalf("outcome = %+v, want Adopted", outcome)
	}
	if outcome.Endpoint != record.Endpoint {
		t.Fatalf("Endpoint = %q, want %q", outcome.Endpoint, record.Endpoint)
	}
	if outcome.Credential != "rotated-token" {
		t.Fatalf("Credential = %q, want rotated-token", outcome.Credential)
	}
	if !client.confirmCalled || client.confirmedID != 3 {
		t.Fatalf("confirm not called with rotation id 3: called=%v id=%d", client.confirmCalled, client.confirmedID)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called on a successful adoption")
	}

	got, err := secretStore.Reveal(context.Background(), secretID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if got != "rotated-token" {
		t.Fatalf("stored credential = %q, want rotated-token", got)
	}

	if len(store.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(store.upserts))
	}
	written := store.upserts[0]
	if written.ServerIdentity != identity.ServerIdentity {
		t.Fatalf("ServerIdentity = %q, want %q", written.ServerIdentity, identity.ServerIdentity)
	}
	if written.CredentialSecretID != secretID {
		t.Fatalf("CredentialSecretID = %q, want unchanged %q (rotation reuses the same secret)", written.CredentialSecretID, secretID)
	}
	if written.Endpoint != record.Endpoint {
		t.Fatalf("Endpoint = %q, want unchanged %q", written.Endpoint, record.Endpoint)
	}
}

// TestAttemptAdoptControlServerConfirmFailureStillReportsAdopted pins the
// narrow edge this Build turn documents explicitly: both durable writes
// (secret + record) already completed by the time confirm is attempted, so
// a confirm-call failure does not undo local durable state and adoption is
// still reported successful. The credential and record already name the
// rotated value regardless of whether the server ever hears about it.
func TestAttemptAdoptControlServerConfirmFailureStillReportsAdopted(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{
		identity:     validIdentity(),
		rotateResult: &agentctl.CredentialRotationResult{RotationID: 9, Credential: "rotated-token"},
		confirmErr:   errors.New("connection reset"),
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if !outcome.Adopted {
		t.Fatalf("outcome = %+v, want Adopted despite confirm failure", outcome)
	}
}

// TestAttemptAdoptControlServerRotationStorageFailureIsIncomplete pins
// AC-002.5: when the durable secret write fails, adoption is incomplete,
// confirm must never be sent (it would tell the server to drop a credential
// this backend never durably recorded), and the record must not be rewritten.
func TestAttemptAdoptControlServerRotationStorageFailureIsIncomplete(t *testing.T) {
	secretStore := &failingUpdateSecretStore{inMemorySecretStore: newInMemorySecretStore()}
	secretID, err := storeControlServerCredential(context.Background(), secretStore.inMemorySecretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	secretStore.failUpdateFor = secretID
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{
		identity:     validIdentity(),
		rotateResult: &agentctl.CredentialRotationResult{RotationID: 5, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCredentialRotationFailed {
		t.Fatalf("outcome = %+v, want unadopted credential_rotation_failed", outcome)
	}
	if client.confirmCalled {
		t.Fatal("ConfirmCredentialRotation was called despite the durable secret write failing")
	}
	if len(store.upserts) != 0 {
		t.Fatal("control server record was rewritten despite the durable secret write failing")
	}
}
