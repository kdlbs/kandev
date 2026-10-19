package sqlite

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/task/models"
)

const durableDeliveryUncertainErrorCode = "DURABLE_DELIVERY_UNCERTAIN"

func clearSilentRestoreUncertainError(metadata map[string]json.RawMessage, recovery models.AgentDeliveryRecovery) {
	raw, exists := metadata[models.SessionMetaKeyLastAgentError]
	if !exists {
		return
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return
	}
	lastError, exists := models.LoadLastAgentError(map[string]interface{}{
		models.SessionMetaKeyLastAgentError: decoded,
	})
	if exists && recovery.HistoryGap != nil && lastError.Code == models.AgentDeliveryHistoryGapFailureCode &&
		lastError.Details == models.AgentDeliveryHistoryGapFailureCode+": "+models.AgentDeliveryHistoryGapDetails(recovery.StreamID, recovery.HistoryGap.FirstSequence, recovery.HistoryGap.LastSequence) {
		delete(metadata, models.SessionMetaKeyLastAgentError)
		return
	}
	structuredSubmission, hasStructuredSubmission := decoded["delivery_submission_id"]
	submissionMatches := false
	if hasStructuredSubmission {
		if submissionID, ok := structuredSubmission.(string); ok {
			submissionMatches = submissionID == recovery.SubmissionID
		}
	} else {
		submissionMatches = lastError.Details == recovery.SubmissionID
	}
	if !exists || lastError.Code != durableDeliveryUncertainErrorCode || !submissionMatches ||
		(lastError.AgentExecutionID != recovery.AgentExecutionID && lastError.ExecutionID != recovery.AgentExecutionID) {
		return
	}
	delete(metadata, models.SessionMetaKeyLastAgentError)
}
