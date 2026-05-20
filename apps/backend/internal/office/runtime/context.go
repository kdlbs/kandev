// Package runtime defines the narrow execution contract used by Office agent runs.
package runtime

// Capabilities describes what an agent run may do through the runtime action surface.
type Capabilities struct {
	CanPostComments     bool     `json:"post_comment"`
	CanUpdateTaskStatus bool     `json:"update_task_status"`
	CanCreateSubtasks   bool     `json:"create_subtask"`
	CanCreateAgents     bool     `json:"create_agent"`
	CanRequestApproval  bool     `json:"request_approval"`
	CanReadMemory       bool     `json:"read_memory"`
	CanWriteMemory      bool     `json:"write_memory"`
	CanListSkills       bool     `json:"list_skills"`
	CanSpawnAgentRun    bool     `json:"spawn_agent_run"`
	CanModifyAgents     bool     `json:"modify_agents"`
	CanDeleteSkills     bool     `json:"delete_skills"`
	AllowedTaskIDs      []string `json:"allowed_task_ids"`
}

// RunContext is the identity and capability envelope for one agent execution.
type RunContext struct {
	WorkspaceID  string       `json:"workspace_id"`
	AgentID      string       `json:"agent_id"`
	TaskID       string       `json:"task_id"`
	RunID        string       `json:"run_id"`
	SessionID    string       `json:"session_id"`
	Reason       string       `json:"reason"`
	Capabilities Capabilities `json:"capabilities"`
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
