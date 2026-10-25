package lifecycle

import "testing"

func TestAgentFailurePayloadCarriesOnlyMatchingDurableSubmissionIdentity(t *testing.T) {
	const submissionID = "prompt:initial:session:1:execution"

	execution := &AgentExecution{
		FailureCode:    durableDeliveryUncertainFailureCode,
		FailureDetails: submissionID,
	}
	execution.setDeliverySubmissionID(submissionID)
	payload := newAgentEventPayload(execution)
	if payload.DeliverySubmissionID != submissionID {
		t.Fatalf("delivery submission identity = %q, want %q", payload.DeliverySubmissionID, submissionID)
	}

	execution.setDeliverySubmissionID("successor-submission")
	payload = newAgentEventPayload(execution)
	if payload.DeliverySubmissionID != "" {
		t.Fatalf("mismatched delivery submission identity = %q, want empty", payload.DeliverySubmissionID)
	}

	execution.FailureCode = "EXECUTOR_START_FAILED"
	execution.FailureDetails = submissionID
	execution.setDeliverySubmissionID(submissionID)
	payload = newAgentEventPayload(execution)
	if payload.DeliverySubmissionID != "" {
		t.Fatalf("non-delivery failure submission identity = %q, want empty", payload.DeliverySubmissionID)
	}
}
