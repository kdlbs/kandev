package approvals

// DecideApprovalRequest is the request body for deciding an approval.
type DecideApprovalRequest struct {
	Status       string `json:"status"`
	DecisionNote string `json:"decision_note"`
	DecidedBy    string `json:"decided_by"`
}

// ApprovalListResponse wraps a list of approvals.
type ApprovalListResponse struct {
	Approvals []*Approval `json:"approvals"`
}
