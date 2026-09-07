package runtime

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

// Standalone control-server ownership, exposed through the agent-runtime seam.
//
// The adoption handshake, its durable record, and the ownership renewer all
// live in runtime/lifecycle, but the caller that drives them at startup is the
// backend application package, which must not import lifecycle directly (see
// apps/backend/CLAUDE.md, "only internal/agent/runtime/ ... may import
// runtime/lifecycle"). These aliases and wrappers are that seam, in the same
// shape as SSHTaskDirReclaimer above: higher-level packages depend on this
// contract rather than on the lifecycle implementation.

// InheritedRecordScope reports what this launch found at the recorded control
// endpoint, which decides how an inherited executors_running row may be judged.
type InheritedRecordScope = lifecycle.InheritedRecordScope

const (
	// InheritedRecordScopeNoServer means nothing answered, so no server holds
	// the inherited rows and the process-identifier probe applies.
	InheritedRecordScopeNoServer = lifecycle.InheritedRecordScopeNoServer
	// InheritedRecordScopeForeignServer means a server answered but was not
	// adopted, so its instances may still be alive on a server this backend
	// does not drive.
	InheritedRecordScopeForeignServer = lifecycle.InheritedRecordScopeForeignServer
	// InheritedRecordScopeAdopted means this backend adopted the recorded
	// server, so its enumeration is authoritative for inherited rows.
	InheritedRecordScopeAdopted = lifecycle.InheritedRecordScopeAdopted
)

// AdoptionRecordStore is the narrow persistence contract the adoption
// orchestration reads and writes the installation-scoped record through.
type AdoptionRecordStore = lifecycle.AdoptionRecordStore

// AdoptionControlClient is the ownership surface adoption drives on a live
// control server.
type AdoptionControlClient = lifecycle.AdoptionControlClient

// AdoptionControlClientFactory builds a client targeting a recorded endpoint.
type AdoptionControlClientFactory = lifecycle.AdoptionControlClientFactory

// AdoptionOutcome is the result of attempting to adopt a detached server.
type AdoptionOutcome = lifecycle.AdoptionOutcome

// OwnershipRenewer keeps an adopted server's ownership lease renewed for as
// long as this backend drives it.
type OwnershipRenewer = lifecycle.OwnershipRenewer

// OwnershipClaimer is the single ownership-claim operation the renewer issues.
type OwnershipClaimer = lifecycle.OwnershipClaimer

// RequiredSurvivalCapabilities is the capability set this backend requires of
// a control server before adopting it.
var RequiredSurvivalCapabilities = lifecycle.RequiredSurvivalCapabilities

// AttemptAdoptControlServer runs the adoption handshake against the recorded
// control endpoint and reports whether this backend may drive that server.
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
	return lifecycle.AttemptAdoptControlServer(ctx, store, secretStore, newClient, homeDir,
		requiredCapabilities, recoveryReadTimeout, recoveryReadRetries, log)
}

// ReclaimUnneededControlServer stops and clears a recorded server this launch
// will not adopt because the capability is disabled.
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
	lifecycle.ReclaimUnneededControlServer(ctx, store, secretStore, newClient, homeDir,
		recoveryReadTimeout, recoveryReadRetries, log)
}

// RecordFreshControlServer persists the record and credential for a control
// server this backend just spawned, so a later launch can adopt it.
func RecordFreshControlServer(
	ctx context.Context,
	store AdoptionRecordStore,
	secretStore secrets.SecretStore,
	client AdoptionControlClient,
	endpoint string,
	credential string,
) error {
	return lifecycle.RecordFreshControlServer(ctx, store, secretStore, client, endpoint, credential)
}

// NewOwnershipRenewer builds the renewer that holds an adopted server's
// ownership lease for this backend's lifetime.
func NewOwnershipRenewer(
	claimer OwnershipClaimer,
	interval time.Duration,
	log *logger.Logger,
) *OwnershipRenewer {
	return lifecycle.NewOwnershipRenewer(claimer, interval, log)
}
