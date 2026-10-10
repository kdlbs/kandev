package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// Delivery recovery types preserve the lifecycle contract at the runtime boundary.
type AgentDeliveryRecoveryIdentity = lifecycle.AgentDeliveryRecoveryIdentity
type DeliveryReconciliationOutcome = lifecycle.DeliveryReconciliationOutcome

const (
	DeliveryReconciliationRunningAttached      = lifecycle.DeliveryReconciliationRunningAttached
	DeliveryReconciliationTerminalSettled      = lifecycle.DeliveryReconciliationTerminalSettled
	DeliveryReconciliationUncertain            = lifecycle.DeliveryReconciliationUncertain
	DeliveryReconciliationBlocked              = lifecycle.DeliveryReconciliationBlocked
	DeliveryReconciliationOwnerMismatch        = lifecycle.DeliveryReconciliationOwnerMismatch
	DeliveryReconciliationTransportUnavailable = lifecycle.DeliveryReconciliationTransportUnavailable
)

var (
	ErrDeliveryRecoveryBlocked = lifecycle.ErrDeliveryRecoveryBlocked
	ErrDeliveryOwnerMismatch   = lifecycle.ErrDeliveryOwnerMismatch
	ErrUncertainPromptDelivery = lifecycle.ErrUncertainPromptDelivery
)
