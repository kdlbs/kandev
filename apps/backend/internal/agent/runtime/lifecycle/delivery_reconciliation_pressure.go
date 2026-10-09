package lifecycle

import agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"

type deliveryPressureRecoveryError struct{ reason string }

func (e *deliveryPressureRecoveryError) Error() string { return e.reason }

func deliveryPressureRecoveryReason(status *agentctl.StatusResponse) string {
	if status == nil {
		return ""
	}
	if status.DeliveryHealth.CancellationPending {
		return "delivery_cancellation_pending"
	}
	if status.DeliveryHealth.ProducerPaused {
		return "delivery_output_paused"
	}
	switch status.DeliveryHealth.State {
	case "guarded", "cancelling", "cancellation_failed", "storage_error":
		return "delivery_storage_pressure"
	default:
		return ""
	}
}
