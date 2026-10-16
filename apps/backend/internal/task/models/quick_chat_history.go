package models

import "github.com/kandev/kandev/internal/agentruntime"

const SessionMetaKeyQuickChatAgentHistory = "quick_chat_agent_history"

// QuickChatAgentHistory identifies one provider conversation without retaining
// credentials or the execution's full environment.
type QuickChatAgentHistory struct {
	AgentName   string               `json:"agent_name"`
	ResumeToken string               `json:"resume_token"`
	WorkDir     string               `json:"work_dir"`
	Runtime     agentruntime.Runtime `json:"runtime"`
}

// IsRestorableQuickChatTask excludes tasks with a separate managed lifecycle.
func IsRestorableQuickChatTask(task *Task) bool {
	if task == nil || !task.IsEphemeral || task.WorkflowID != "" ||
		task.Origin == TaskOriginAutomationRun || task.Origin == TaskOriginCoordinator {
		return false
	}
	_, managed := task.Metadata["kandev.plugin_id"].(string)
	return !managed
}
