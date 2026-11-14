package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// Delivery recovery types preserve the lifecycle contract at the runtime boundary.
type AgentDeliveryRecoveryIdentity = lifecycle.AgentDeliveryRecoveryIdentity
type DeliveryReconciliationOutcome = lifecycle.DeliveryReconciliationOutcome

const (
	DeliveryReconciliationRunningAttached         = lifecycle.DeliveryReconciliationRunningAttached
	DeliveryReconciliationTerminalSettled         = lifecycle.DeliveryReconciliationTerminalSettled
	DeliveryReconciliationUncertain               = lifecycle.DeliveryReconciliationUncertain
	DeliveryReconciliationBlocked                 = lifecycle.DeliveryReconciliationBlocked
	DeliveryReconciliationOwnerMismatch           = lifecycle.DeliveryReconciliationOwnerMismatch
	DeliveryReconciliationTransportUnavailable    = lifecycle.DeliveryReconciliationTransportUnavailable
	DeliveryRecoveryRefusalLiveWork               = lifecycle.DeliveryRecoveryRefusalLiveWork
	DeliveryRecoveryRefusalPeerRecordMissing      = lifecycle.DeliveryRecoveryRefusalPeerRecordMissing
	DeliveryRecoveryRefusalPeerRecordMismatch     = lifecycle.DeliveryRecoveryRefusalPeerRecordMismatch
	DeliveryRecoveryRefusalGenerationSuperseded   = lifecycle.DeliveryRecoveryRefusalGenerationSuperseded
	DeliveryRecoveryRefusalNativeIdentityMismatch = lifecycle.DeliveryRecoveryRefusalNativeIdentityMismatch
	DeliveryRecoveryRefusalJournalUnresolved      = lifecycle.DeliveryRecoveryRefusalJournalUnresolved
	DeliveryRecoveryRefusalProjectionPending      = lifecycle.DeliveryRecoveryRefusalProjectionPending
	DeliveryRecoveryRefusalExecutionUnavailable   = lifecycle.DeliveryRecoveryRefusalExecutionUnavailable
	DeliveryRecoveryRefusalEvidenceUnavailable    = lifecycle.DeliveryRecoveryRefusalEvidenceUnavailable
	DeliveryRecoveryRefusalOfficeOwned            = lifecycle.DeliveryRecoveryRefusalOfficeOwned
	DeliveryRecoveryRefusalSQLResolutionRefused   = lifecycle.DeliveryRecoveryRefusalSQLResolutionRefused
	DeliveryRecoveryRefusalUnclassified           = lifecycle.DeliveryRecoveryRefusalUnclassified
)

func IsDeliveryRecoveryRefusalReason(reason string) bool {
	return lifecycle.IsDeliveryRecoveryRefusalReason(reason)
}

type DeliveryRecoveryRefusalReasoner interface {
	DeliveryRecoveryRefusalReason() string
}

var (
	ErrDeliveryRecoveryBlocked            = lifecycle.ErrDeliveryRecoveryBlocked
	ErrDeliveryOwnerMismatch              = lifecycle.ErrDeliveryOwnerMismatch
	ErrUncertainPromptDelivery            = lifecycle.ErrUncertainPromptDelivery
	ErrDurableAdoptionBlocked             = lifecycle.ErrDurableAdoptionBlocked
	ErrDurableAdoptionEvidenceUnavailable = lifecycle.ErrDurableAdoptionEvidenceUnavailable
	ErrDurableAdoptionIdentityMismatch    = lifecycle.ErrDurableAdoptionIdentityMismatch
)
