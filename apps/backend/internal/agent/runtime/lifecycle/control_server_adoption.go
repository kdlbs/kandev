package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// RequiredSurvivalCapabilities is the capability set this backend requires
// of a control server before it will adopt it (design 01, "Capability
// compatibility": "The backend requires a set and adopts only when its
// required set is a subset"). Must stay a subset of
// api.SurvivalCapabilities -- kept as an independent literal here rather
// than imported, since agentctl/server/api is not a package the backend
// package layer depends on.
var RequiredSurvivalCapabilities = []string{"agent-survival.v1"}

// AdoptionRecordStore is the narrow persistence interface the adoption
// orchestration reads and writes the single installation-scoped
// control-server record through, satisfied by *sqlite.Repository.
type AdoptionRecordStore interface {
	GetControlServerRecord(ctx context.Context) (*models.ControlServerRecord, error)
	UpsertControlServerRecord(ctx context.Context, record *models.ControlServerRecord) error
}

// staleExecutionRepairStore is the optional persistence capability
// AC-EXECUTORS-CONTROL-OWNERSHIP-004.3 and AC-EXECUTORS-SURVIVAL-005.5's
// immediate-repair step need: list this installation's live standalone
// recovery-inventory records and repair each in place. Satisfied by
// *sqlite.Repository alongside AdoptionRecordStore; type-asserted rather
// than folded into AdoptionRecordStore itself so RecordFreshControlServer's
// store parameter and every existing AdoptionRecordStore test double stay
// unaffected, matching this package's turnOutcomeApplier/recoveryStopper
// optional-capability convention.
type staleExecutionRepairStore interface {
	ListExecutorsRunningLiveStandalone(ctx context.Context) ([]*models.ExecutorRunning, error)
	RepairExecutorRunningDead(ctx context.Context, sessionID string) error
}

// repairLiveStandaloneRecordsAfterOwnServerStopped implements
// AC-EXECUTORS-CONTROL-OWNERSHIP-004.3's "repair their recovery-inventory
// records itself... performed immediately" for a control server this
// backend has just proved is its own and has itself successfully stopped:
// unlike the deferred unowned-shutdown repair of AC-EXECUTORS-CONTROL-
// OWNERSHIP-003.5 (no backend attached at kill time), a backend is attached
// here and can write the durable store right away. Since exactly one
// control server is ever recorded per installation, stopping it makes every
// currently-live standalone recovery-inventory record definitively dead --
// there is no live enumeration to correlate against, unlike an ordinary
// recovery pass. Repair preserves resume token and worktree identity
// (RepairExecutorRunningDead), never deletes. Best-effort: a listing or
// per-record repair failure is logged and does not block the caller, since
// the stopped server is gone regardless and a later reconciliation pass
// will eventually repair any row this call missed.
func repairLiveStandaloneRecordsAfterOwnServerStopped(ctx context.Context, store staleExecutionRepairStore, log *logger.Logger) {
	records, err := store.ListExecutorsRunningLiveStandalone(ctx)
	if err != nil {
		log.Warn("failed to list live standalone recovery-inventory records for immediate repair after stopping this installation's own control server", zap.Error(err))
		return
	}
	for _, record := range records {
		if record == nil || record.SessionID == "" {
			continue
		}
		if err := store.RepairExecutorRunningDead(ctx, record.SessionID); err != nil {
			log.Warn("failed to repair recovery-inventory record after stopping this installation's own control server",
				zap.String("session_id", record.SessionID), zap.Error(err))
		}
	}
}

// shutdownControlServerWithRetry issues the ownership-shutdown operation
// within the same bounded per-attempt timeout and retry count as
// AC-EXECUTORS-SURVIVAL-002.13 (AC-EXECUTORS-CONTROL-OWNERSHIP-004.7), using
// the operator-configured override when the caller supplies one (a zero
// timeout or negative retry count falls back to the AC's own two-second/
// two-retry default, mirroring StandaloneExecutor.stopWithRetry). A
// dial-level connection-refused error counts as success without consuming a
// retry: AC-004.7's explicit "already stopping or already absent" carve-out,
// reached when the target has already exited on its own (e.g. via
// runUnownedReaper) before this call was ever issued.
func shutdownControlServerWithRetry(ctx context.Context, client AdoptionControlClient, timeout time.Duration, retries int) error {
	if timeout <= 0 {
		timeout = defaultRecoveryReadTimeout
	}
	if retries < 0 {
		retries = defaultRecoveryReadRetries
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		lastErr = client.ShutdownControlServer(attemptCtx)
		cancel()
		if lastErr == nil || isAlreadyAbsentShutdownError(lastErr) {
			return nil
		}
	}
	return lastErr
}

// isAlreadyAbsentShutdownError reports whether err is the dial-level
// connection-refused shape produced when the target control server has
// already exited before this call reached it.
func isAlreadyAbsentShutdownError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "connection refused")
}

// withAdoptionRetry executes fn up to retries+1 times, each attempt bounded
// by its own per-attempt timeout derived from ctx rather than ctx's own
// (possibly absent) deadline -- the same bounded-per-attempt shape as
// shutdownControlServerWithRetry and StandaloneExecutor.listInstancesWithRetry
// (AC-EXECUTORS-SURVIVAL-002.13). A zero timeout or negative retry count
// falls back to the AC's own two-second/two-retry default.
func withAdoptionRetry[T any](ctx context.Context, timeout time.Duration, retries int, fn func(context.Context) (T, error)) (T, error) {
	if timeout <= 0 {
		timeout = defaultRecoveryReadTimeout
	}
	if retries < 0 {
		retries = defaultRecoveryReadRetries
	}

	var lastErr error
	var zero T
	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		result, err := fn(attemptCtx)
		cancel()
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return zero, lastErr
}

// AdoptionControlClient is the subset of agentctl.ControlClient's ownership
// surface the adoption orchestration drives, narrowed so the decision below
// is testable without a real HTTP round trip.
type AdoptionControlClient interface {
	SetAuthToken(token string)
	GetIdentity(ctx context.Context) (*agentctl.IdentityInfo, error)
	ProveOwnership(ctx context.Context, challenge string) ([]string, error)
	GetServerDetails(ctx context.Context) (*agentctl.ServerDetails, error)
	RotateCredential(ctx context.Context) (*agentctl.CredentialRotationResult, error)
	ConfirmCredentialRotation(ctx context.Context, rotationID int64) error
	ShutdownControlServer(ctx context.Context) error
}

// AdoptionControlClientFactory builds a client targeting the recorded
// control endpoint. Injected so tests never need a real listener.
type AdoptionControlClientFactory func(endpoint string) (AdoptionControlClient, error)

// AdoptionReason is a fixed, stable adoption-outcome label for observability
// (design 02: "adoption attempted, adopted, and refused with a reason drawn
// from a fixed set"). AC-EXECUTORS-CONTROL-OWNERSHIP-001.8's "no recorded
// endpoint, or nothing answers, or already shutting down" bucket is
// deliberately NOT a refusal reason (nothing about it was refused), so it
// gets its own value outside the AC-001.6 refusal-reason precedence list.
//
// AC-001.6's own refusal-reason list also names "no answer" as one of its six
// enumerated reasons, ahead of identity mismatch in its precedence order --
// which reads as a refusal, in tension with AC-001.8 treating the identical
// case ("an endpoint that nothing answers") as no refusal at all, and with
// AC-EXECUTORS-CONTROL-OWNERSHIP-003.9 explicitly mapping its own
// shutting-down outcome onto AC-001.8's bucket rather than AC-001.6's. Two
// ACs describing this case as "not a refusal" against one phrase in a third
// naming it as one: AC-001.8 (reinforced by 003.9) is treated as the
// authoritative behavior, and AC-001.6's "no answer" is read as the same
// no_server bucket by another name, not a distinct recorded refusal reason.
type AdoptionReason string

const (
	// AdoptionReasonNoServer covers "no recorded control endpoint", "the
	// recorded endpoint answers nothing", and "the recorded endpoint reports
	// it has already begun an unowned shutdown" alike (AC-001.8): none of
	// these is a refusal, so none carries a refusal reason, and all three
	// result in the same action, spawn fresh and report nothing recovered.
	AdoptionReasonNoServer AdoptionReason = "no_server"
	// AdoptionReasonIdentityMismatch covers a server that advertises no
	// identity or no capability set, one whose identity differs from the
	// recorded one, and one that cannot prove it holds the recorded
	// credential (AC-001.4, AC-001.5) -- refused, left running untouched,
	// never contacted again by this adoption attempt, and never sent the
	// credential.
	AdoptionReasonIdentityMismatch AdoptionReason = "identity_mismatch"
	// AdoptionReasonCredentialUnavailable is the stored credential could not
	// be retrieved (AC-001.9): refused before any authentication was
	// attempted, left running untouched, its own unowned shutdown reclaims it.
	AdoptionReasonCredentialUnavailable AdoptionReason = "credential_unavailable"
	// AdoptionReasonAuthenticationFailed is an attempted rotation the server
	// rejected -- including a rotate refused because the server has already
	// begun an unowned shutdown after this attempt's identity read succeeded,
	// which is intentionally not distinguished from a wrong credential here:
	// both refuse without issuing a stop and take the identical spawn-fresh
	// path, and only the logged reason label would differ.
	AdoptionReasonAuthenticationFailed AdoptionReason = "authentication_failed"
	// AdoptionReasonCapabilityIncompatible is an authenticated, identified,
	// own server missing a required capability (AC-004.3/004.4): refused
	// AND stopped, because only an own authenticated server may ever be
	// stopped.
	AdoptionReasonCapabilityIncompatible AdoptionReason = "capability_incompatible"
	// AdoptionReasonCredentialRotationFailed is a rotation the server
	// accepted whose durable storage (secret write or record write) did not
	// complete (AC-002.5): adoption is incomplete, nothing further is issued
	// to that server, and it is left exactly as it was left by the
	// two-phase window -- still holding both the superseded and the new
	// credential, unconfirmed.
	AdoptionReasonCredentialRotationFailed AdoptionReason = "credential_rotation_failed"
)

// AdoptionOutcome is the result of attempting to adopt a previously-detached
// control server. Endpoint and Credential are populated only when Adopted.
// ContactedAt is the instant this attempt first reached the recorded control
// endpoint (the GetIdentity call) -- the AC-EXECUTORS-SURVIVAL-003.7 recovery
// deadline's clock start. It is left zero when no live server was ever
// reached (no record, or the client/transport itself could not be built),
// since nothing recovery-relevant is running in that case.
type AdoptionOutcome struct {
	Adopted  bool
	Reason   AdoptionReason
	Endpoint string
	// Credential is the single credential that authenticates both
	// control-plane and per-instance operations and streams (design 01
	// "Single driver", AC-EXECUTORS-CONTROL-OWNERSHIP-002.6): use it for
	// every client built against this control server or any instance it
	// supervises.
	Credential  string
	ContactedAt time.Time
	// UnownedPeriod is the adopted server's own resolved unowned period, read
	// back from its /identity response (AC-EXECUTORS-CONTROL-OWNERSHIP-003.2/
	// .7). Only this value is safe to compute an ownership-renewal cadence
	// from: this backend's local config can disagree with what the adopted
	// process actually enforces across a restart that changed
	// agentctl.unownedPeriod. Zero when the server answered without the
	// field at all (a legacy pre-upgrade server) -- callers apply their own
	// floor in that case, never a locally-resolved value.
	UnownedPeriod time.Duration
}

// AttemptAdoptControlServer implements startup steps 4 through 6 of design
// 02's "Startup" control flow: read the recorded control endpoint and
// identity, evaluate the gates in the required order -- home match, then
// authentication, then capability subset (AC-EXECUTORS-CONTROL-OWNERSHIP-001.3)
// -- and on a match rotate the ownership credential and persist the new
// record. It never spawns a process: the caller decides what to do with a
// refusal, which is always "spawn a fresh server on a free port and record
// it" regardless of which reason produced the refusal.
func AttemptAdoptControlServer(
	ctx context.Context,
	store AdoptionRecordStore,
	secretStore secrets.SecretStore,
	newClient AdoptionControlClientFactory,
	homeDir string,
	requiredCapabilities []string,
	recoveryReadTimeout time.Duration,
	recoveryReadRetries int,
	log *logger.Logger,
) AdoptionOutcome {
	record, err := store.GetControlServerRecord(ctx)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonNoServer}
	}

	client, err := newClient(record.Endpoint)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonNoServer}
	}

	// The AC-EXECUTORS-SURVIVAL-003.7 recovery deadline's clock starts here,
	// at the first real contact with the recorded control endpoint --
	// regardless of which gate below ultimately refuses or accepts adoption.
	contactedAt := time.Now()
	identity, err := withAdoptionRetry(ctx, recoveryReadTimeout, recoveryReadRetries, client.GetIdentity)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonNoServer}
	}

	if !identityMatchesRecordedServer(identity, record) || len(identity.Capabilities) == 0 {
		return AdoptionOutcome{Reason: AdoptionReasonIdentityMismatch, ContactedAt: contactedAt}
	}

	credential, err := revealControlServerCredential(ctx, secretStore, record.CredentialSecretID)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonCredentialUnavailable, ContactedAt: contactedAt}
	}

	if !controlServerHoldsCredential(ctx, client, credential, homeDir, recoveryReadTimeout, recoveryReadRetries) {
		return AdoptionOutcome{Reason: AdoptionReasonIdentityMismatch, ContactedAt: contactedAt}
	}

	client.SetAuthToken(credential)
	rotated, err := withAdoptionRetry(ctx, recoveryReadTimeout, recoveryReadRetries, client.RotateCredential)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonAuthenticationFailed, ContactedAt: contactedAt}
	}

	if !capabilitiesSatisfy(requiredCapabilities, identity.Capabilities) {
		client.SetAuthToken(rotated.Credential)
		return stopIncompatibleControlServer(ctx, client, store, record, recoveryReadTimeout, recoveryReadRetries, contactedAt, log)
	}

	client.SetAuthToken(rotated.Credential)
	return finalizeControlServerAdoption(ctx, finalizeAdoptionArgs{
		store:               store,
		secretStore:         secretStore,
		client:              client,
		record:              record,
		identity:            identity,
		rotated:             rotated,
		contactedAt:         contactedAt,
		recoveryReadTimeout: recoveryReadTimeout,
		recoveryReadRetries: recoveryReadRetries,
		log:                 log,
	})
}

// finalizeAdoptionArgs carries the state the two durable writes and the
// confirmation call need, so the adoption gates above stay readable as a
// sequence of refusals.
type finalizeAdoptionArgs struct {
	store               AdoptionRecordStore
	secretStore         secrets.SecretStore
	client              AdoptionControlClient
	record              *models.ControlServerRecord
	identity            *agentctl.IdentityInfo
	rotated             *agentctl.CredentialRotationResult
	contactedAt         time.Time
	recoveryReadTimeout time.Duration
	recoveryReadRetries int
	log                 *logger.Logger
}

// finalizeControlServerAdoption performs the two durable writes adoption
// depends on -- the rotated credential and the updated record -- and only
// then tells the server the rotation is confirmed. Either write failing
// leaves adoption incomplete, because confirming a credential this backend
// did not durably record would tell the server to drop the one credential
// still recoverable from durable state.
func finalizeControlServerAdoption(ctx context.Context, a finalizeAdoptionArgs) AdoptionOutcome {
	secretID, err := storeControlServerCredential(ctx, a.secretStore, a.record.CredentialSecretID, a.rotated.Credential)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonCredentialRotationFailed, ContactedAt: a.contactedAt}
	}

	updated := &models.ControlServerRecord{
		Endpoint:           a.record.Endpoint,
		ServerIdentity:     a.identity.ServerIdentity,
		CredentialSecretID: secretID,
		Capabilities:       a.identity.Capabilities,
		DiagnosticLogPath:  recordedDiagnosticLogPath(ctx, a.client, a.recoveryReadTimeout, a.recoveryReadRetries),
		CreatedAt:          a.record.CreatedAt,
	}
	if err := a.store.UpsertControlServerRecord(ctx, updated); err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonCredentialRotationFailed, ContactedAt: a.contactedAt}
	}

	// Sent only after both durable writes above completed
	// (AC-EXECUTORS-CONTROL-OWNERSHIP-002.7). A failure here is not treated
	// as fatal to adoption: the durable state already names the rotated
	// credential as current, so a future retry or the server's own
	// idempotent replay (AC-002.10) recovers it, and the superseded
	// credential merely stays adoption-only-acceptable a little longer than
	// necessary in the meantime.
	if _, err := withAdoptionRetry(ctx, a.recoveryReadTimeout, a.recoveryReadRetries, func(c context.Context) (struct{}, error) {
		return struct{}{}, a.client.ConfirmCredentialRotation(c, a.rotated.RotationID)
	}); err != nil {
		a.log.Warn("control server credential rotation stored durably but confirmation call failed",
			zap.String("endpoint", a.record.Endpoint), zap.Error(err))
	}

	return AdoptionOutcome{
		Adopted:       true,
		Endpoint:      a.record.Endpoint,
		Credential:    a.rotated.Credential,
		ContactedAt:   a.contactedAt,
		UnownedPeriod: time.Duration(a.identity.UnownedPeriodMS) * time.Millisecond,
	}
}

// RecordFreshControlServer durably records a freshly spawned (not adopted)
// control server as the new installation-scoped record. Per the failure
// table's "Own server started after a refused or failed adoption" row, the
// single control-server record is always rewritten to name the new server:
// the record cannot keep pointing at a server that is about to reap itself
// through its own unowned shutdown. credential is the server's initial
// ownership credential (the bootstrap auth token minted at launch). A
// prior record's CredentialSecretID, when one is readable, is reused so a
// spawn-after-refused-adoption does not leave an orphaned secret row behind.
func RecordFreshControlServer(
	ctx context.Context,
	store AdoptionRecordStore,
	secretStore secrets.SecretStore,
	client AdoptionControlClient,
	endpoint string,
	credential string,
) error {
	identity, err := client.GetIdentity(ctx)
	if err != nil {
		return fmt.Errorf("read freshly spawned control server identity: %w", err)
	}

	var priorSecretID string
	if prior, err := store.GetControlServerRecord(ctx); err == nil {
		priorSecretID = prior.CredentialSecretID
	}

	secretID, err := storeControlServerCredential(ctx, secretStore, priorSecretID, credential)
	if err != nil {
		return fmt.Errorf("store freshly spawned control server credential: %w", err)
	}

	record := &models.ControlServerRecord{
		Endpoint:           endpoint,
		ServerIdentity:     identity.ServerIdentity,
		CredentialSecretID: secretID,
		Capabilities:       identity.Capabilities,
		DiagnosticLogPath:  recordedDiagnosticLogPath(ctx, client, defaultRecoveryReadTimeout, defaultRecoveryReadRetries),
	}
	return store.UpsertControlServerRecord(ctx, record)
}

// ReclaimUnneededControlServer implements AC-EXECUTORS-SURVIVAL-005.5: when
// the agent-survival capability is disabled, a detached control server left
// running by an earlier survival-enabled launch is stopped rather than left
// running unowned -- but only when this backend can prove that server is
// its own by the same identity+credential test AttemptAdoptControlServer
// applies to adoption. A server this backend cannot prove is its own is
// left untouched, and so is one that answers nothing (nothing left to
// reclaim) or has no recorded endpoint at all.
//
// This never adopts: it does not rotate the credential, so a backend
// holding only a superseded credential can still perform it, which
// AC-EXECUTORS-SURVIVAL-005.1 forbids adoption itself from doing. The stop
// is issued as the ownership-shutdown operation of
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.9, which requires no prior adoption or
// rotation for exactly this reason. Every failure mode here is logged and
// non-fatal to startup: the caller always falls through to spawning a fresh
// server regardless of what this reclaim attempt did.
func ReclaimUnneededControlServer(
	ctx context.Context,
	store AdoptionRecordStore,
	secretStore secrets.SecretStore,
	newClient AdoptionControlClientFactory,
	homeDir string,
	recoveryReadTimeout time.Duration,
	recoveryReadRetries int,
	log *logger.Logger,
) {
	record, err := store.GetControlServerRecord(ctx)
	if err != nil {
		return
	}

	client, err := newClient(record.Endpoint)
	if err != nil {
		return
	}

	identity, err := client.GetIdentity(ctx)
	if err != nil {
		return
	}
	if !identityMatchesRecordedServer(identity, record) {
		return
	}

	credential, err := revealControlServerCredential(ctx, secretStore, record.CredentialSecretID)
	if err != nil {
		log.Warn("cannot reclaim detached control server: stored credential unavailable",
			zap.String("endpoint", record.Endpoint), zap.Error(err))
		return
	}

	if !controlServerHoldsCredential(ctx, client, credential, homeDir, recoveryReadTimeout, recoveryReadRetries) {
		log.Warn("cannot reclaim detached control server: it could not prove it holds this installation's credential",
			zap.String("endpoint", record.Endpoint))
		return
	}

	client.SetAuthToken(credential)
	if err := shutdownControlServerWithRetry(ctx, client, recoveryReadTimeout, recoveryReadRetries); err != nil {
		log.Warn("failed to stop detached control server left by an earlier survival-enabled launch after exhausting retries; leaving it to its own unowned shutdown",
			zap.String("endpoint", record.Endpoint), zap.Error(err))
		return
	}

	log.Info("stopped a detached control server left by an earlier survival-enabled launch now that the capability is disabled",
		zap.String("endpoint", record.Endpoint))
	if repairStore, ok := store.(staleExecutionRepairStore); ok {
		repairLiveStandaloneRecordsAfterOwnServerStopped(ctx, repairStore, log)
	}
}

// stopIncompatibleControlServer stops an authenticated, identified, own
// server that is missing a required capability and, on success, repairs its
// live standalone records immediately (AC-EXECUTORS-CONTROL-OWNERSHIP-004.3).
// Always returns the AdoptionReasonCapabilityIncompatible refusal regardless
// of whether the stop itself succeeded.
func stopIncompatibleControlServer(
	ctx context.Context,
	client AdoptionControlClient,
	store AdoptionRecordStore,
	record *models.ControlServerRecord,
	recoveryReadTimeout time.Duration,
	recoveryReadRetries int,
	contactedAt time.Time,
	log *logger.Logger,
) AdoptionOutcome {
	if stopErr := shutdownControlServerWithRetry(ctx, client, recoveryReadTimeout, recoveryReadRetries); stopErr != nil {
		// AC-EXECUTORS-CONTROL-OWNERSHIP-004.7: retries exhausted -- record no
		// repair, report no recovered instances (already true, Adopted stays
		// false below), and leave the server to its own unowned shutdown; a
		// later backend repairs through the deferred
		// AC-EXECUTORS-CONTROL-OWNERSHIP-003.5 path once it stops itself.
		log.Warn("failed to stop incompatible control server after exhausting retries; leaving it to its own unowned shutdown",
			zap.String("endpoint", record.Endpoint), zap.Error(stopErr))
	} else if repairStore, ok := store.(staleExecutionRepairStore); ok {
		// AC-EXECUTORS-CONTROL-OWNERSHIP-004.3: the stop succeeded and a
		// backend is attached, so repair every live standalone record
		// immediately rather than deferring to the next start.
		repairLiveStandaloneRecordsAfterOwnServerStopped(ctx, repairStore, log)
	}
	return AdoptionOutcome{Reason: AdoptionReasonCapabilityIncompatible, ContactedAt: contactedAt}
}

// identityMatchesRecordedServer reports whether a live server's per-launch
// identity nonce is the exact one the control-server record carries, which is
// what proves the live server IS the recorded process rather than merely
// another launch. Installation identity (the home directory) is a separate
// question, proved over the authenticated ownership-details exchange rather
// than here, because /identity deliberately discloses no filesystem path
// (AC-EXECUTORS-CONTROL-OWNERSHIP-001.11). An identity either side cannot
// produce leaves the match unprovable and is refused rather than assumed.
func identityMatchesRecordedServer(identity *agentctl.IdentityInfo, record *models.ControlServerRecord) bool {
	if record.ServerIdentity == "" || identity.ServerIdentity != record.ServerIdentity {
		return false
	}
	return true
}

func capabilitiesSatisfy(required, advertised []string) bool {
	have := make(map[string]struct{}, len(advertised))
	for _, c := range advertised {
		have[c] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; !ok {
			return false
		}
	}
	return true
}
