package statussummary

import (
	"encoding/json"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// QuotaWaitSummaryFromRouteState maps one session's durable routing state to the
// bounded task-list projection, or nil when the session is not parked on a
// provider limit.
//
// A limit wait is a route that recorded an exhausted quota and was told to try
// again at a known instant, so the projection needs both: the classified quota
// code and the deadline that instant comes from. A route waiting for any other
// reason keeps its routing-owned presentation. The policy body, the classifier
// rule and the provider payload behind the decision stay in the routing tables.
//
// A route parked because every candidate is suspended is deliberately not a
// limit wait: that state records a deadline but no cause, so calling it a usage
// limit would be a guess. It only becomes a limit wait when routing starts
// attributing the suspension, and this projection follows that attribution
// rather than inventing one.
func QuotaWaitSummaryFromRouteState(state *dynamicruntime.RouteState) *QuotaWaitSummary {
	if state == nil || state.SessionID == "" {
		return nil
	}
	if !isDeadlineBackedWait(state.Status) {
		return nil
	}
	var policy dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policy); err != nil {
		return nil
	}
	if policy.FailureCode != routingerr.CodeQuotaLimited || policy.Deadline == nil {
		return nil
	}
	deadline := policy.Deadline.UTC()
	if deadline.IsZero() {
		return nil
	}
	return &QuotaWaitSummary{
		SessionID: state.SessionID,
		Reason:    QuotaWaitReasonQuotaLimited,
		Deadline:  deadline,
	}
}

// isDeadlineBackedWait names the durable statuses a fresh selection is retried
// from automatically. They are the only ones that carry a deadline the
// projection can report.
func isDeadlineBackedWait(status string) bool {
	return status == "retry_wait" || status == "waiting_for_reset"
}
