package github

const ciRunEvidenceVerdictPending = "pending"

func setCIRunIdempotencyStatus(receipt *CIRunReceipt, status string) {
	if receipt != nil {
		receipt.IdempotencyStatus = status
	}
}

func receiptFromCIRunRequest(request *CIRunRequest) *CIRunReceipt {
	return &CIRunReceipt{
		RequestID: request.ID, TaskID: request.TargetTaskID, RunID: request.ProviderRunID,
		WorkflowID: request.ProviderWorkflowID, WorkflowName: request.ProviderWorkflowName,
		WorkflowPath: request.ProviderWorkflowPath, HeadRepository: request.ProviderHeadRepo,
		HeadRef: request.ProviderHeadRef, HeadSHA: request.ProviderHeadSHA,
		Attempt: request.ProviderAttempt, Operation: request.Operation, EvidenceKind: request.EvidenceKind,
		Status: request.Status, FailureClass: request.FailureClass,
		ProviderRetryAfter: request.ProviderRetryAfter, Repository: request.CanonicalRepository,
		PRNumber: request.PRNumber, ExpectedHeadSHA: request.ExpectedHeadSHA,
		SourceRunID: request.SourceRunID, SourceAttempt: request.ExpectedSourceAttempt,
		ProviderEvent: request.ProviderEvent, ObservedPRHeadSHA: request.ObservedPRHeadSHA,
		ProviderPrincipal: decodeCIRunProviderPrincipal(request.ProviderPrincipalJSON),
		ProviderRequestID: request.ProviderRequestID, ProviderURL: request.ProviderURL,
		EvidenceVerdict: evidenceVerdict(request), CreatedAt: request.CreatedAt, UpdatedAt: request.UpdatedAt,
	}
}

func evidenceVerdict(request *CIRunRequest) string {
	if request.Status == CIRunRequestSucceeded {
		return string(request.EvidenceKind)
	}
	if request.Status == CIRunRequestPending || request.Status == CIRunRequestReconciling {
		return ciRunEvidenceVerdictPending
	}
	return "denied"
}
