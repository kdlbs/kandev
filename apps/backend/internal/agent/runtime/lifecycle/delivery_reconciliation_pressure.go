package lifecycle

import agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"

const (
	deliveryFailedHealthState      = "delivery_failed"
	deliveryInterruptedHealthState = "interrupted"
)

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
	if failure := status.DeliveryHealth.Failure; failure != nil {
		// A sticky writer failure cannot be resolved by a live process or an
		// open socket. Keep the exact failure fence visible to recovery.
		if failure.Reason == "event_too_large" {
			return "delivery_event_too_large"
		}
		return "delivery_writer_failed"
	}
	switch status.DeliveryHealth.State {
	case "guarded", "cancelling", "cancellation_failed", "storage_error", deliveryFailedHealthState, deliveryInterruptedHealthState:
		return "delivery_storage_pressure"
	default:
		return ""
	}
}
