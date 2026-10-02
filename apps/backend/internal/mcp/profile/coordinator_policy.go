package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// CoordinatorToolPolicyMetadataKey is the conversation task metadata key that
// carries the coordinator's bound tool list.
const CoordinatorToolPolicyMetadataKey = "kandev.coordinator_tool_policy"

const coordinatorToolPolicyVersion = 1

// coordinatorToolUniverse is every tool name a coordinator binding may list.
var coordinatorToolUniverse = []string{
	"list_tasks_kandev", "get_task_conversation_kandev",
	"list_workflows_kandev", "list_workflow_steps_kandev",
	"list_repositories_kandev", "get_coordinator_item_kandev",
	"list_coordinator_activity_kandev",
	"propose_task_kandev", "propose_message_kandev",
	"propose_move_kandev", "propose_resume_kandev",
	"propose_improvement_kandev",
	"list_coordinator_turns_kandev",
}

// CoordinatorToolPolicy is the tool list bound to one coordinator conversation
// when it opens. Agent-controlled MCP arguments never supply it.
type CoordinatorToolPolicy struct {
	Version            int      `json:"version"`
	CoordinatorID      string   `json:"coordinator_id"`
	WorkspaceID        string   `json:"workspace_id"`
	ConversationTaskID string   `json:"conversation_task_id"`
	PolicyRevision     int      `json:"policy_revision"`
	ToolNames          []string `json:"tool_names"`
}

// Validate refuses a binding that is incomplete or names a tool outside the
// coordinator tool universe.
func (p CoordinatorToolPolicy) Validate() error {
	if p.Version != coordinatorToolPolicyVersion {
		return errors.New("coordinator tool policy version is unsupported")
	}
	if p.CoordinatorID == "" || p.WorkspaceID == "" || p.ConversationTaskID == "" {
		return errors.New("coordinator tool policy identity is incomplete")
	}
	if p.PolicyRevision < 0 {
		return errors.New("coordinator tool policy revision is invalid")
	}
	seen := make(map[string]struct{}, len(p.ToolNames))
	for _, name := range p.ToolNames {
		if !slices.Contains(coordinatorToolUniverse, name) {
			return fmt.Errorf("coordinator tool policy names unknown tool %q", name)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("coordinator tool policy duplicates tool %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// Allows reports whether the bound list names the tool.
func (p CoordinatorToolPolicy) Allows(toolName string) bool {
	return slices.Contains(p.ToolNames, toolName)
}

// MarshalCoordinatorToolPolicy returns the canonical JSON stored in task
// metadata. It accepts a binding without a conversation task id, which is the
// form stamped at task creation before the id exists.
func MarshalCoordinatorToolPolicy(policy CoordinatorToolPolicy) (string, error) {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ParseCoordinatorToolPolicyMetadata parses the persisted JSON form, or a map
// produced by a JSON persistence round trip, and validates it.
func ParseCoordinatorToolPolicyMetadata(value any) (*CoordinatorToolPolicy, error) {
	var encoded []byte
	switch typed := value.(type) {
	case string:
		encoded = []byte(typed)
	case map[string]any:
		var err error
		if encoded, err = json.Marshal(typed); err != nil {
			return nil, errors.New("coordinator tool policy metadata is invalid")
		}
	default:
		return nil, errors.New("coordinator tool policy metadata is missing")
	}
	var policy CoordinatorToolPolicy
	if err := json.Unmarshal(encoded, &policy); err != nil {
		return nil, errors.New("coordinator tool policy metadata is invalid")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	policy.ToolNames = slices.Clone(policy.ToolNames)
	return &policy, nil
}

// coordinatorPhaseOneTools is the tool list of a conversation with no binding.
var coordinatorPhaseOneTools = []string{
	"list_tasks_kandev", "get_task_conversation_kandev",
	"list_workflows_kandev", "list_workflow_steps_kandev",
	"list_repositories_kandev", "get_coordinator_item_kandev",
	"propose_task_kandev",
}

// BoundCoordinatorToolNames returns the tool names a coordinator session may
// use: the binding's list, or the phase-1 seven when the session has none.
func BoundCoordinatorToolNames(ctx Context) []string {
	if ctx.CoordinatorToolPolicy != nil {
		return slices.Clone(ctx.CoordinatorToolPolicy.ToolNames)
	}
	return slices.Clone(coordinatorPhaseOneTools)
}
