package github

func receiptFromCIRunRequest(request *CIRunRequest) *CIRunReceipt {
	return &CIRunReceipt{RequestID: request.ID, TaskID: request.TargetTaskID, RunID: request.ProviderRunID, WorkflowID: request.ProviderWorkflowID, WorkflowName: request.ProviderWorkflowName, WorkflowPath: request.ProviderWorkflowPath, HeadRepository: request.ProviderHeadRepo, HeadRef: request.ProviderHeadRef, HeadSHA: request.ProviderHeadSHA, Attempt: request.ProviderAttempt, Operation: request.Operation, EvidenceKind: request.EvidenceKind, Status: request.Status, FailureClass: request.FailureClass, PRNumber: request.PRNumber, ExpectedHeadSHA: request.ExpectedHeadSHA, SourceRunID: request.SourceRunID, SourceAttempt: request.ExpectedSourceAttempt, EvidenceVerdict: evidenceVerdict(request), CreatedAt: request.CreatedAt, UpdatedAt: request.UpdatedAt}
}

func evidenceVerdict(request *CIRunRequest) string {
	if request.Status == CIRunRequestSucceeded {
		return string(request.EvidenceKind)
	}
	if request.Status == CIRunRequestReconciling {
		return "pending"
	}
	return "denied"
}
