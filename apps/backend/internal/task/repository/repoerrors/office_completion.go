package repoerrors

// OfficeApprovalsPendingError carries stable IDs out of a rolled-back admission.
type OfficeApprovalsPendingError struct{ AgentProfileIDs []string }

func (e *OfficeApprovalsPendingError) Error() string { return "task approvals pending" }
