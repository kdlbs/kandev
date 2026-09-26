package cursorcloud

import (
	"expvar"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

var (
	dispatchTotal          = expvar.NewMap("cursor_cloud_dispatch_total")
	reconnectTotal         = expvar.NewMap("cursor_cloud_reconnect_total")
	submissionUnknownTotal = expvar.NewMap("cursor_cloud_submission_unknown_total")
	cancelTotal            = expvar.NewMap("cursor_cloud_cancel_total")
)

const (
	reconnectReasonDisconnect     = "disconnect"
	reconnectReasonBackendRestart = "backend_restart"
)

func cursorCloudMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	labels := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		labels = append(labels, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(labels, ";")
}

func recordDispatch(kind models.ManagedAgentOperationKind, outcome string) {
	if kind != models.ManagedAgentOperationCreate && kind != models.ManagedAgentOperationFollowup {
		return
	}
	if outcome != "accepted" && outcome != "rejected" && outcome != "unknown" {
		return
	}
	dispatchTotal.Add(cursorCloudMetricLabel("operation", string(kind), "outcome", outcome), 1)
}

func recordReconnect(reason, outcome string) {
	if reason != reconnectReasonDisconnect && reason != reconnectReasonBackendRestart {
		return
	}
	if outcome != "connected" && outcome != "retryable_error" && outcome != "auth_error" && outcome != "history_expired" {
		return
	}
	reconnectTotal.Add(cursorCloudMetricLabel("reason", reason, "outcome", outcome), 1)
}

func recordSubmissionUnknown(kind models.ManagedAgentOperationKind, reason string) {
	if kind != models.ManagedAgentOperationCreate && kind != models.ManagedAgentOperationFollowup {
		return
	}
	if reason != "timeout" && reason != "transport" && reason != "crash_recovery" && reason != "persistence" {
		return
	}
	submissionUnknownTotal.Add(cursorCloudMetricLabel("operation", string(kind), "reason", reason), 1)
}

func recordCancel(outcome string) {
	if outcome != "confirmed" && outcome != "pending" && outcome != "rejected" {
		return
	}
	cancelTotal.Add(cursorCloudMetricLabel("outcome", outcome), 1)
}
