package lifecycle

import (
	"context"
	"fmt"
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

// AdoptionControlClient is the subset of agentctl.ControlClient's ownership
// surface the adoption orchestration drives, narrowed so the decision below
// is testable without a real HTTP round trip.
type AdoptionControlClient interface {
	SetAuthToken(token string)
	GetIdentity(ctx context.Context) (*agentctl.IdentityInfo, error)
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
type AdoptionReason string

const (
	// AdoptionReasonNoServer covers "no recorded control endpoint", "the
	// recorded endpoint answers nothing", and "the recorded endpoint reports
	// it has already begun an unowned shutdown" alike (AC-001.8): none of
	// these is a refusal, so none carries a refusal reason, and all three
	// result in the same action, spawn fresh and report nothing recovered.
	AdoptionReasonNoServer AdoptionReason = "no_server"
	// AdoptionReasonIdentityMismatch covers a home-directory mismatch and a
	// server that advertises no identity or no capability set at all
	// (AC-001.4, AC-001.5) -- refused, left running untouched, never
	// contacted again by this adoption attempt.
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
	Adopted     bool
	Reason      AdoptionReason
	Endpoint    string
	Credential  string
	ContactedAt time.Time
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
	identity, err := client.GetIdentity(ctx)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonNoServer}
	}

	if identity.HomeDir == "" || identity.HomeDir != homeDir || len(identity.Capabilities) == 0 {
		return AdoptionOutcome{Reason: AdoptionReasonIdentityMismatch, ContactedAt: contactedAt}
	}

	credential, err := revealControlServerCredential(ctx, secretStore, record.CredentialSecretID)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonCredentialUnavailable, ContactedAt: contactedAt}
	}

	client.SetAuthToken(credential)
	rotated, err := client.RotateCredential(ctx)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonAuthenticationFailed, ContactedAt: contactedAt}
	}

	if !capabilitiesSatisfy(requiredCapabilities, identity.Capabilities) {
		client.SetAuthToken(rotated.Credential)
		if stopErr := client.ShutdownControlServer(ctx); stopErr != nil {
			log.Warn("failed to stop incompatible control server; leaving it to its own unowned shutdown",
				zap.String("endpoint", record.Endpoint), zap.Error(stopErr))
		}
		return AdoptionOutcome{Reason: AdoptionReasonCapabilityIncompatible, ContactedAt: contactedAt}
	}

	client.SetAuthToken(rotated.Credential)
	secretID, err := storeControlServerCredential(ctx, secretStore, record.CredentialSecretID, rotated.Credential)
	if err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonCredentialRotationFailed, ContactedAt: contactedAt}
	}

	updated := &models.ControlServerRecord{
		Endpoint:           record.Endpoint,
		ServerIdentity:     identity.ServerIdentity,
		CredentialSecretID: secretID,
		Capabilities:       identity.Capabilities,
		DiagnosticLogPath:  identity.DiagnosticLogPath,
		CreatedAt:          record.CreatedAt,
	}
	if err := store.UpsertControlServerRecord(ctx, updated); err != nil {
		return AdoptionOutcome{Reason: AdoptionReasonCredentialRotationFailed, ContactedAt: contactedAt}
	}

	// Sent only after both durable writes above completed
	// (AC-EXECUTORS-CONTROL-OWNERSHIP-002.7). A failure here is not treated
	// as fatal to adoption: the durable state already names the rotated
	// credential as current, so a future retry or the server's own
	// idempotent replay (AC-002.10) recovers it, and the superseded
	// credential merely stays adoption-only-acceptable a little longer than
	// necessary in the meantime.
	if err := client.ConfirmCredentialRotation(ctx, rotated.RotationID); err != nil {
		log.Warn("control server credential rotation stored durably but confirmation call failed",
			zap.String("endpoint", record.Endpoint), zap.Error(err))
	}

	return AdoptionOutcome{Adopted: true, Endpoint: record.Endpoint, Credential: rotated.Credential, ContactedAt: contactedAt}
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
		DiagnosticLogPath:  identity.DiagnosticLogPath,
	}
	return store.UpsertControlServerRecord(ctx, record)
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
