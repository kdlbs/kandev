package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ownershipproof"
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

// fakeAdoptionRecordStore is an in-memory AdoptionRecordStore double that
// also implements the optional staleExecutionRepairStore capability, so
// tests can observe the immediate-repair step of AC-EXECUTORS-CONTROL-
// OWNERSHIP-004.3/AC-EXECUTORS-SURVIVAL-005.5.
type fakeAdoptionRecordStore struct {
	record  *models.ControlServerRecord
	getErr  error
	upserts []*models.ControlServerRecord
	putErr  error

	liveStandaloneRecords []*models.ExecutorRunning
	listLiveErr           error
	repairedSessionIDs    []string
	repairErr             error
}

func (f *fakeAdoptionRecordStore) ListExecutorsRunningLiveStandalone(context.Context) ([]*models.ExecutorRunning, error) {
	if f.listLiveErr != nil {
		return nil, f.listLiveErr
	}
	return f.liveStandaloneRecords, nil
}

func (f *fakeAdoptionRecordStore) RepairExecutorRunningDead(_ context.Context, sessionID string) error {
	if f.repairErr != nil {
		return f.repairErr
	}
	f.repairedSessionIDs = append(f.repairedSessionIDs, sessionID)
	return nil
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
//
// heldCredential and boundHome are what this double's server actually
// possesses: it answers an ownership challenge by deriving from them, so a
// double whose held credential differs from the one the backend reveals, or
// whose bound home differs from the backend's, cannot produce an acceptable
// proof -- which is exactly the position a foreign control server is in.
// boundHome defaults to testHomeDir, so a double only has to name it when
// the point of the test is that the two installations differ.
type fakeAdoptionControlClient struct {
	identity        *agentctl.IdentityInfo
	identityErr     error
	heldCredential  string
	boundHome       string
	proveErr        error
	challenges      []string
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

func (f *fakeAdoptionControlClient) ProveOwnership(_ context.Context, challenge string) ([]string, error) {
	f.challenges = append(f.challenges, challenge)
	if f.proveErr != nil {
		return nil, f.proveErr
	}
	home := f.boundHome
	if home == "" {
		home = testHomeDir
	}
	return []string{ownershipproof.Derive(f.heldCredential, challenge, home)}, nil
}

func (f *fakeAdoptionControlClient) GetServerDetails(context.Context) (*agentctl.ServerDetails, error) {
	return &agentctl.ServerDetails{
		HomeDir:           testHomeDir,
		DiagnosticLogPath: testDiagnosticLogPath,
	}, nil
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

const (
	testHomeDir           = "/home/kandev"
	testDiagnosticLogPath = "/home/kandev/logs/agentctl-diagnostic.log"
)

func validRecord() *models.ControlServerRecord {
	return &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:9999",
		ServerIdentity:     "matching-identity",
		CredentialSecretID: "secret-1",
		Capabilities:       []string{"agent-survival.v1"},
		DiagnosticLogPath:  testDiagnosticLogPath,
	}
}

func validIdentity() *agentctl.IdentityInfo {
	return &agentctl.IdentityInfo{
		ServerIdentity:  "matching-identity",
		Capabilities:    []string{"agent-survival.v1"},
		UnownedPeriodMS: 300000,
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
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

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
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonNoServer {
		t.Fatalf("outcome = %+v, want unadopted no_server", outcome)
	}
}

// blockingIdentityControlClient's GetIdentity blocks until its call's own
// context is done, for proving a stalled control server cannot hold
// AttemptAdoptControlServer open past its configured per-attempt timeout.
type blockingIdentityControlClient struct {
	fakeAdoptionControlClient
	identityCalls int
}

func (c *blockingIdentityControlClient) GetIdentity(ctx context.Context) (*agentctl.IdentityInfo, error) {
	c.identityCalls++
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestAttemptAdoptControlServerGetIdentityBoundedByConfiguredTimeout pins
// Review round 5 finding 2: a stalled control server's GetIdentity call must
// not hold AttemptAdoptControlServer open past the configured
// recoveryReadTimeout -- it must not fall back to the HTTP client's own much
// larger internal timeout (or block forever on an undeadlined context).
func TestAttemptAdoptControlServerGetIdentityBoundedByConfiguredTimeout(t *testing.T) {
	client := &blockingIdentityControlClient{}
	store, _ := adoptionFixture(t, validRecord(), &client.fakeAdoptionControlClient)
	factory := func(string) (AdoptionControlClient, error) { return client, nil }

	start := time.Now()
	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, 20*time.Millisecond, 0, newAdoptionTestLogger(t))
	elapsed := time.Since(start)

	if outcome.Adopted || outcome.Reason != AdoptionReasonNoServer {
		t.Fatalf("outcome = %+v, want unadopted no_server (a timed-out identity read never answered)", outcome)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("AttemptAdoptControlServer took %v, want bounded by the configured 20ms per-attempt timeout, not an unbounded block", elapsed)
	}
	if client.identityCalls == 0 {
		t.Fatal("expected GetIdentity to have been attempted at least once")
	}
}

// TestAttemptAdoptControlServerHomeMismatchRefusesWithoutStop pins AC-001.4:
// a server belonging to a different installation is refused, and --
// critically -- ShutdownControlServer is never called, because a process
// this installation cannot prove is its own is never stopped. The mismatch
// is not something the server reports about itself; it falls out of the
// proof, which binds the home directory, so a server bound to another home
// cannot answer this backend's challenge.
func TestAttemptAdoptControlServerHomeMismatchRefusesWithoutStop(t *testing.T) {
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

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("outcome = %+v, want unadopted identity_mismatch", outcome)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called for a home-mismatched server")
	}
	if len(client.presentedTokens) != 0 {
		t.Fatalf("credential was sent to a server bound to another home: %v", client.presentedTokens)
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
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

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
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

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
		heldCredential: "stale-token",
		identity:       validIdentity(),
		rotateErr:      errors.New("401 invalid auth token"),
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

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
		heldCredential: "current-token",
		identity:       identity,
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 7, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)
	store.liveStandaloneRecords = []*models.ExecutorRunning{
		{SessionID: "session-1"}, {SessionID: "session-2"},
	}

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCapabilityIncompatible {
		t.Fatalf("outcome = %+v, want unadopted capability_incompatible", outcome)
	}
	if !client.shutdownCalled {
		t.Fatal("ShutdownControlServer was not called for an incompatible own server")
	}
	if len(store.upserts) != 0 {
		t.Fatal("control server record was rewritten by the refused adoption itself; that is the caller's job after spawning fresh")
	}
	// AC-EXECUTORS-CONTROL-OWNERSHIP-004.3 (Review round 1, finding 4): a
	// successful stop of a confirmed-own server immediately repairs every
	// live standalone recovery-inventory record, since exactly one control
	// server exists per installation and this one just proved it was that
	// server.
	if len(store.repairedSessionIDs) != 2 {
		t.Fatalf("repaired sessions = %v, want session-1 and session-2 repaired immediately after the successful stop", store.repairedSessionIDs)
	}
}

// TestAttemptAdoptControlServerIncompatibleCapabilityRetriesStopBeforeGivingUp
// pins AC-EXECUTORS-CONTROL-OWNERSHIP-004.7: the stop this branch issues is
// retried within a bounded budget, not a single unretried call, and no
// record is repaired when every retry fails. The injected failure is a
// genuinely retryable server error, not a connection-refused/dial failure --
// AC-004.7's "already absent counts as success" carve-out means that shape
// resolves on the first attempt instead of exhausting retries (see
// TestAttemptAdoptControlServerIncompatibleCapabilityConnectionRefusedCountsAsSuccess).
func TestAttemptAdoptControlServerIncompatibleCapabilityRetriesStopBeforeGivingUp(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	identity := validIdentity()
	identity.Capabilities = []string{"some-other-capability"}
	client := &countingShutdownControlClient{
		fakeAdoptionControlClient: fakeAdoptionControlClient{
			heldCredential: "current-token",
			identity:       identity,
			rotateResult:   &agentctl.CredentialRotationResult{RotationID: 7, Credential: "rotated-token"},
			shutdownErr:    errors.New("500 internal server error"),
		},
	}
	store, _ := adoptionFixture(t, record, &client.fakeAdoptionControlClient)
	factory := func(string) (AdoptionControlClient, error) { return client, nil }
	store.liveStandaloneRecords = []*models.ExecutorRunning{{SessionID: "session-1"}}

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCapabilityIncompatible {
		t.Fatalf("outcome = %+v, want unadopted capability_incompatible", outcome)
	}
	if client.shutdownAttempts < 2 {
		t.Fatalf("shutdown attempts = %d, want more than one (bounded retry, not a single unretried call)", client.shutdownAttempts)
	}
	if len(store.repairedSessionIDs) != 0 {
		t.Fatalf("repaired sessions = %v, want none: AC-004.7 forbids repair when the stop's retries are exhausted", store.repairedSessionIDs)
	}
}

// countingShutdownControlClient wraps fakeAdoptionControlClient to count
// ShutdownControlServer attempts, for asserting a retry budget was actually
// exercised rather than a single call.
type countingShutdownControlClient struct {
	fakeAdoptionControlClient
	shutdownAttempts int
}

func (c *countingShutdownControlClient) ShutdownControlServer(ctx context.Context) error {
	c.shutdownAttempts++
	return c.fakeAdoptionControlClient.ShutdownControlServer(ctx)
}

// TestAttemptAdoptControlServerIncompatibleCapabilityConnectionRefusedCountsAsSuccess
// pins AC-EXECUTORS-CONTROL-OWNERSHIP-004.7's "a control server reporting
// that it is already stopping or already absent shall count as success"
// carve-out: a dial-level connection-refused error (the shape produced when
// the target already exited on its own, e.g. via its own unowned reaper)
// must not be retried to exhaustion and reported as a genuine failure -- the
// immediate-repair step must still run.
func TestAttemptAdoptControlServerIncompatibleCapabilityConnectionRefusedCountsAsSuccess(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	identity := validIdentity()
	identity.Capabilities = []string{"some-other-capability"}
	client := &countingShutdownControlClient{
		fakeAdoptionControlClient: fakeAdoptionControlClient{
			heldCredential: "current-token",
			identity:       identity,
			rotateResult:   &agentctl.CredentialRotationResult{RotationID: 7, Credential: "rotated-token"},
			shutdownErr:    errors.New("dial tcp 127.0.0.1:9999: connect: connection refused"),
		},
	}
	store, _ := adoptionFixture(t, record, &client.fakeAdoptionControlClient)
	factory := func(string) (AdoptionControlClient, error) { return client, nil }
	store.liveStandaloneRecords = []*models.ExecutorRunning{{SessionID: "session-1"}}

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonCapabilityIncompatible {
		t.Fatalf("outcome = %+v, want unadopted capability_incompatible", outcome)
	}
	if client.shutdownAttempts != 1 {
		t.Fatalf("shutdown attempts = %d, want exactly 1: a connection-refused error must not be retried", client.shutdownAttempts)
	}
	if len(store.repairedSessionIDs) != 1 || store.repairedSessionIDs[0] != "session-1" {
		t.Fatalf("repaired sessions = %v, want session-1 repaired: an already-absent server counts as a successful stop", store.repairedSessionIDs)
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
		heldCredential: "current-token",
		identity:       identity,
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 3, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if !outcome.Adopted {
		t.Fatalf("outcome = %+v, want Adopted", outcome)
	}
	if outcome.Endpoint != record.Endpoint {
		t.Fatalf("Endpoint = %q, want %q", outcome.Endpoint, record.Endpoint)
	}
	if outcome.Credential != "rotated-token" {
		t.Fatalf("Credential = %q, want rotated-token", outcome.Credential)
	}
	// Review round 3, finding 5: the adopted server's own reported unowned
	// period must travel back on the outcome, not be left for the caller to
	// recompute from its own (potentially stale) local config.
	if outcome.UnownedPeriod != 300*time.Second {
		t.Fatalf("UnownedPeriod = %v, want 300s (identity.UnownedPeriodMS read back verbatim)", outcome.UnownedPeriod)
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
	// The remaining two fields decide whether a LATER launch can adopt this
	// server at all: capabilities gate compatibility, and CreatedAt must
	// survive rather than being reset to now by every adoption, which would
	// make the record look freshly created forever.
	if len(written.Capabilities) != len(identity.Capabilities) {
		t.Fatalf("Capabilities = %v, want the live server's %v", written.Capabilities, identity.Capabilities)
	}
	for i, capability := range identity.Capabilities {
		if written.Capabilities[i] != capability {
			t.Fatalf("Capabilities = %v, want the live server's %v", written.Capabilities, identity.Capabilities)
		}
	}
	if !written.CreatedAt.Equal(record.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want the recorded %v carried through", written.CreatedAt, record.CreatedAt)
	}
	if written.DiagnosticLogPath != testDiagnosticLogPath {
		t.Fatalf("DiagnosticLogPath = %q, want the live server's %q", written.DiagnosticLogPath, testDiagnosticLogPath)
	}
}

// TestAttemptAdoptControlServerServerIdentityMismatchRefusesWithoutStop pins
// AC-CONTROL-OWNERSHIP-001.2: a live server whose home directory matches but
// whose per-launch ServerIdentity does not match the previously recorded
// value is a different process sharing the same home directory, not the
// server this record describes, and must be refused exactly like a
// home-directory mismatch -- never stopped, since this backend cannot prove
// it is its own.
func TestAttemptAdoptControlServerServerIdentityMismatchRefusesWithoutStop(t *testing.T) {
	record := validRecord()
	record.ServerIdentity = "recorded-identity"
	identity := validIdentity()
	identity.ServerIdentity = "a-different-live-identity"
	client := &fakeAdoptionControlClient{identity: identity}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, newInMemorySecretStore(), factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("outcome = %+v, want unadopted identity_mismatch", outcome)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called for a ServerIdentity-mismatched server")
	}
}

// TestAttemptAdoptControlServerEmptyRecordedServerIdentityRefuses pins the
// fail-closed side of the identity proof end to end: a record that carries no
// server identity cannot prove which process it describes, and the home
// directory it shares with every control server on the installation is not a
// substitute. The server is left unadopted rather than claimed on an
// unprovable match.
func TestAttemptAdoptControlServerEmptyRecordedServerIdentityRefuses(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.ServerIdentity = ""
	record.CredentialSecretID = secretID
	identity := validIdentity()
	identity.ServerIdentity = "whatever-this-server-reports"
	client := &fakeAdoptionControlClient{
		heldCredential: "current-token",
		identity:       identity,
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 1, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if outcome.Adopted || outcome.Reason != AdoptionReasonIdentityMismatch {
		t.Fatalf("outcome = %+v, want unadopted identity_mismatch for an unset recorded ServerIdentity", outcome)
	}
	if client.shutdownCalled {
		t.Fatal("ShutdownControlServer was called for a server whose identity could not be proven")
	}
}

// TestAttemptAdoptControlServerUnownedPeriodFallsBackToFloorWhenUnreported
// pins the legacy/pre-upgrade shape: a server that answers /identity without
// an unowned_period_ms field (zero value) reports a zero UnownedPeriod on the
// outcome, rather than a value this package fabricates -- the caller
// (backendapp.resolveAdoptedRenewalPeriod) owns the floor fallback.
func TestAttemptAdoptControlServerUnownedPeriodFallsBackToFloorWhenUnreported(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "current-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	identity := validIdentity()
	identity.UnownedPeriodMS = 0
	client := &fakeAdoptionControlClient{
		heldCredential: "current-token",
		identity:       identity,
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 1, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

	if !outcome.Adopted {
		t.Fatalf("outcome = %+v, want Adopted", outcome)
	}
	if outcome.UnownedPeriod != 0 {
		t.Fatalf("UnownedPeriod = %v, want 0 (unreported), so the caller applies its own floor", outcome.UnownedPeriod)
	}
}

// TestAttemptAdoptControlServerCredentialKeepsRotatingAcrossAdoptions pins a
// chain of two or more consecutive restarts -- this feature's own core
// scenario: each adoption's rotated credential is the one the NEXT adoption
// must present and receive a further rotation for, under the SAME secret
// slot throughout, since design 01 "Single driver" uses exactly one
// credential for both control-plane and per-instance operations and there
// is nothing else to keep synchronized across the chain.
func TestAttemptAdoptControlServerCredentialKeepsRotatingAcrossAdoptions(t *testing.T) {
	secretStore := newInMemorySecretStore()
	secretID, err := storeControlServerCredential(context.Background(), secretStore, "", "bootstrap-token")
	if err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	record := validRecord()
	record.CredentialSecretID = secretID
	client := &fakeAdoptionControlClient{
		heldCredential: "bootstrap-token",
		identity:       validIdentity(),
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 1, Credential: "rotated-token-1"},
	}
	store, factory := adoptionFixture(t, record, client)

	first := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))
	if !first.Adopted {
		t.Fatalf("first adoption = %+v, want Adopted", first)
	}
	if first.Credential != "rotated-token-1" {
		t.Fatalf("first Credential = %q, want rotated-token-1", first.Credential)
	}
	if len(store.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1 after the first adoption", len(store.upserts))
	}
	if store.upserts[0].CredentialSecretID != secretID {
		t.Fatalf("CredentialSecretID = %q, want unchanged %q across the chain", store.upserts[0].CredentialSecretID, secretID)
	}

	// Simulate the next restart reading back the record the first call wrote.
	// The server now holds the credential the first adoption rotated it to,
	// so that is what it can prove possession of.
	store.record = store.upserts[0]
	client.heldCredential = "rotated-token-1"
	client.rotateResult = &agentctl.CredentialRotationResult{RotationID: 2, Credential: "rotated-token-2"}

	second := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))
	if !second.Adopted {
		t.Fatalf("second adoption = %+v, want Adopted", second)
	}
	if second.Credential != "rotated-token-2" {
		t.Fatalf("second Credential = %q, want rotated-token-2", second.Credential)
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
		heldCredential: "current-token",
		identity:       validIdentity(),
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 9, Credential: "rotated-token"},
		confirmErr:     errors.New("connection reset"),
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

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
		heldCredential: "current-token",
		identity:       validIdentity(),
		rotateResult:   &agentctl.CredentialRotationResult{RotationID: 5, Credential: "rotated-token"},
	}
	store, factory := adoptionFixture(t, record, client)

	outcome := AttemptAdoptControlServer(context.Background(), store, secretStore, factory,
		testHomeDir, RequiredSurvivalCapabilities, 0, -1, newAdoptionTestLogger(t))

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

// TestRecordFreshControlServerReusesPriorSecretID pins the "own server
// started after a refused or failed adoption" failure-table row: the record
// is rewritten to name the new server, and the existing credential secret
// row is reused in place (updated to the new server's bootstrap token)
// rather than leaving the old row orphaned.
func TestRecordFreshControlServerReusesPriorSecretID(t *testing.T) {
	secretStore := newInMemorySecretStore()
	priorSecretID, err := storeControlServerCredential(context.Background(), secretStore, "", "old-servers-token")
	if err != nil {
		t.Fatalf("seed prior credential: %v", err)
	}
	store := &fakeAdoptionRecordStore{record: &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:8888",
		CredentialSecretID: priorSecretID,
	}}
	client := &fakeAdoptionControlClient{heldCredential: "old-servers-token", identity: validIdentity()}

	err = RecordFreshControlServer(context.Background(), store, secretStore, client, "127.0.0.1:9001", "new-servers-token")
	if err != nil {
		t.Fatalf("RecordFreshControlServer: %v", err)
	}

	if len(store.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(store.upserts))
	}
	written := store.upserts[0]
	if written.Endpoint != "127.0.0.1:9001" {
		t.Fatalf("Endpoint = %q, want the new server's endpoint", written.Endpoint)
	}
	if written.CredentialSecretID != priorSecretID {
		t.Fatalf("CredentialSecretID = %q, want reused %q", written.CredentialSecretID, priorSecretID)
	}

	got, err := secretStore.Reveal(context.Background(), priorSecretID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if got != "new-servers-token" {
		t.Fatalf("stored credential = %q, want new-servers-token", got)
	}
}

// TestRecordFreshControlServerCreatesSecretWhenNoPriorRecord pins the
// first-ever-launch case: no prior record exists at all, so a brand new
// secret is created rather than a reuse being attempted.
func TestRecordFreshControlServerCreatesSecretWhenNoPriorRecord(t *testing.T) {
	secretStore := newInMemorySecretStore()
	store := &fakeAdoptionRecordStore{}
	client := &fakeAdoptionControlClient{identity: validIdentity()}

	err := RecordFreshControlServer(context.Background(), store, secretStore, client, "127.0.0.1:9001", "fresh-token")
	if err != nil {
		t.Fatalf("RecordFreshControlServer: %v", err)
	}

	if len(store.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(store.upserts))
	}
	written := store.upserts[0]
	if written.CredentialSecretID == "" {
		t.Fatalf("written = %+v, want a populated credential secret ID", written)
	}
	got, err := secretStore.Reveal(context.Background(), written.CredentialSecretID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if got != "fresh-token" {
		t.Fatalf("stored credential = %q, want fresh-token", got)
	}
}

// TestRecordFreshControlServerPropagatesIdentityFailure pins that a failure
// to read the freshly spawned server's own identity aborts before any
// durable write, rather than writing a record with blank fields.
func TestRecordFreshControlServerPropagatesIdentityFailure(t *testing.T) {
	secretStore := newInMemorySecretStore()
	store := &fakeAdoptionRecordStore{}
	client := &fakeAdoptionControlClient{identityErr: errors.New("connection refused")}

	err := RecordFreshControlServer(context.Background(), store, secretStore, client, "127.0.0.1:9001", "fresh-token")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if len(store.upserts) != 0 {
		t.Fatal("record was written despite the identity read failing")
	}
}

// --- ReclaimUnneededControlServer (Review round 1, finding 1: AC-EXECUTORS-SURVIVAL-005.5) ---

// TestReclaimUnneededControlServerNoRecordDoesNothing pins that a disabled
// launch with no recorded control endpoint contacts nothing.
// TestReclaimUnneededControlServerNothingAnswersDoesNothing pins that an
// endpoint that answers nothing is left alone -- nothing to reclaim. The
// credential store is seeded exactly as
// TestReclaimUnneededControlServerStopsOwnServerAndRepairsRecords is, so a
// real credential is available: if the GetIdentity-error check under test
// were ever removed, the function would proceed to a genuine shutdown
// attempt and this assertion would actually catch it, rather than passing
// only because credential-unavailable happened to short-circuit first.
// TestReclaimUnneededControlServerIdentityMismatchLeavesServerUntouched pins
// that a server belonging to another installation is never stopped -- the
// same ownership gate adoption applies, on the path that runs with the
// capability off. The credential store is seeded exactly as
// TestReclaimUnneededControlServerStopsOwnServerAndRepairsRecords is, so a
// real credential is available: if the ownership check under test were ever
// removed, the function would proceed to a genuine shutdown attempt and this
// assertion would actually catch it, rather than passing only because
// credential-unavailable happened to short-circuit first.
// TestReclaimUnneededControlServerServerIdentityMismatchLeavesServerUntouched
// pins the same ServerIdentity gate AttemptAdoptControlServer applies
// (Review round 3, finding 2) for the reclaim path too: a home-directory
// match alone is not enough to prove this is the exact recorded server.
// TestReclaimUnneededControlServerCredentialUnavailableLeavesServerUntouched
// pins AC-EXECUTORS-SURVIVAL-005.5's refusal-before-any-stop-attempt when
// the stored credential cannot be read.
// TestReclaimUnneededControlServerStopsOwnServerAndRepairsRecords pins the
// success path: a proven-own server is stopped via the ownership-shutdown
// operation without ever rotating the credential (adoption is forbidden
// while the capability is disabled, AC-EXECUTORS-SURVIVAL-005.1), and every
// live standalone recovery-inventory record is repaired immediately.
// TestReclaimUnneededControlServerRetriesStopAndSkipsRepairOnFailure pins
// the shared AC-EXECUTORS-CONTROL-OWNERSHIP-004.7 retry contract applied to
// this stop too: bounded retries, and no repair when they're exhausted. The
// injected failure is a genuinely retryable server error, not a
// connection-refused/dial failure -- see
// TestReclaimUnneededControlServerConnectionRefusedCountsAsSuccess for that
// carve-out.
// TestReclaimUnneededControlServerConnectionRefusedCountsAsSuccess pins
// AC-EXECUTORS-CONTROL-OWNERSHIP-004.7's "already absent counts as success"
// carve-out for this stop too: a dial-level connection-refused error must
// not be retried to exhaustion, and the immediate-repair step still runs.
