// Package approvals provides types, service, and HTTP handlers for the office
// approvals feature.
package approvals

import "github.com/kandev/kandev/internal/office/models"

// ApprovalType constants for approval request types.
const (
	ApprovalTypeHireAgent      = models.ApprovalTypeHireAgent
	ApprovalTypeBudgetIncrease = models.ApprovalTypeBudgetIncrease
	ApprovalTypeBoardApproval  = models.ApprovalTypeBoardApproval
	ApprovalTypeTaskReview     = models.ApprovalTypeTaskReview
	ApprovalTypeSkillCreation  = models.ApprovalTypeSkillCreation
)

// Approval status constants.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// Approval is a type alias for models.Approval.
type Approval = models.Approval
