// Package runtime defines the narrow execution contract used by Office agent runs.
package runtime

// Capabilities describes what an agent run may do through the runtime HTTP
// action surface. These permissions are serialized into the run JWT.
type Capabilities struct {
	CanPostComments     bool     `json:"post_comment"`
	CanUpdateTaskStatus bool     `json:"update_task_status"`
	CanCreateTasks      bool     `json:"create_task"`
	CanCreateSubtasks   bool     `json:"create_subtask"`
	CanCreateAgents     bool     `json:"create_agent"`
	CanListProjects     bool     `json:"list_projects"`
	CanCreateProjects   bool     `json:"create_project"`
	CanRequestApproval  bool     `json:"request_approval"`
	CanReadMemory       bool     `json:"read_memory"`
	CanWriteMemory      bool     `json:"write_memory"`
	CanListSkills       bool     `json:"list_skills"`
	CanSpawnAgentRun    bool     `json:"spawn_agent_run"`
	CanModifyAgents     bool     `json:"modify_agents"`
	CanDeleteSkills     bool     `json:"delete_skills"`
	AllowedTaskIDs      []string `json:"allowed_task_ids"`
}

// RunContext is the identity, runtime-permission, and advertised-action
// envelope for one agent execution. AvailableActions only describes tools the
// prompt can advertise. The backend performs live authorization when a tool is
// called.
type RunContext struct {
	WorkspaceID      string       `json:"workspace_id"`
	AgentID          string       `json:"agent_id"`
	TaskID           string       `json:"task_id"`
	RunID            string       `json:"run_id"`
	SessionID        string       `json:"session_id"`
	Reason           string       `json:"reason"`
	Capabilities     Capabilities `json:"capabilities"`
	AvailableActions []string     `json:"available_actions,omitempty"`
}

// CanMutateTask reports whether the run may mutate the given task.
func (c RunContext) CanMutateTask(taskID string) bool {
	if taskID == "" {
		return false
	}
	if taskID == c.TaskID {
		return true
	}
	for _, allowed := range c.Capabilities.AllowedTaskIDs {
		if allowed == WildcardTaskScope || allowed == taskID {
			return true
		}
	}
	return false
}
