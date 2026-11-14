package dto

type BulkMovePendingApprover struct {
	AgentProfileID string `json:"agent_profile_id"`
	Name           string `json:"name"`
}

type BulkMoveTaskRefusal struct {
	TaskID           string                    `json:"task_id"`
	TaskTitle        string                    `json:"task_title"`
	Code             string                    `json:"code"`
	Message          string                    `json:"message"`
	PendingApprovers []BulkMovePendingApprover `json:"pending_approvers,omitempty"`
}

type BulkMoveErrorResponse struct {
	Code         string                `json:"code"`
	MovedTaskIDs []string              `json:"moved_task_ids"`
	Refusals     []BulkMoveTaskRefusal `json:"refusals"`
}
