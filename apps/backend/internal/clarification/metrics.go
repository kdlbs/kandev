package clarification

import "expvar"

const (
	clarificationResponsePhaseIdentity   = "identity"
	clarificationResponsePhaseValidation = "validation"
	clarificationResponsePhaseClaim      = "claim"
	clarificationResponsePhaseDelivery   = "delivery"
)

var clarificationResponseTimeoutTotal = expvar.NewMap("clarification_response_timeout_total")

func recordClarificationResponseTimeout(phase string) {
	clarificationResponseTimeoutTotal.Add(phase, 1)
}

func responsePhaseOutcome(err error) string {
	if err == nil {
		return "success"
	}
	if IsPreClaimTimeoutError(err) {
		return "timeout"
	}
	return "failure"
}
