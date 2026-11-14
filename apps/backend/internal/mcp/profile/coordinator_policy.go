package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// CoordinatorToolPolicyMetadataKey is the conversation task metadata key that
// carries the coordinator's bound tool list.
const CoordinatorToolPolicyMetadataKey = "kandev.coordinator_tool_policy"

const (
	// CoordinatorToolPolicyVersionV1 binds only the core coordinator tools.
	CoordinatorToolPolicyVersionV1 = 1
	// CoordinatorToolPolicyVersionV2 adds exact instance-read plugin refs.
	CoordinatorToolPolicyVersionV2 = 2

	// MaxCoordinatorPluginTools bounds the aggregate binding across every
	// eligible plugin. Manifest validation caps each plugin at 16, so this
	// must stay above one plugin's share; overflow is trimmed deterministically
	// when the conversation binding is stamped.
	MaxCoordinatorPluginTools = 256
)

// coordinatorToolUniverse is every core tool name a coordinator binding may list.
var coordinatorToolUniverse = []string{
	"list_tasks_kandev", "get_task_conversation_kandev",
	"list_workflows_kandev", "list_workflow_steps_kandev",
	"list_repositories_kandev", "get_coordinator_item_kandev",
	"list_coordinator_activity_kandev",
	"propose_task_kandev", "propose_message_kandev",
	"propose_move_kandev", "propose_resume_kandev",
	"propose_improvement_kandev", "propose_answer_kandev", "propose_merge_kandev",
	"list_coordinator_turns_kandev",
}

// CoordinatorPluginTool is one exact instance-read plugin tool reference in a
// version-2 binding. InstallationID and PolicyVersion pin the reference to the
// installation that was eligible when the conversation opened.
type CoordinatorPluginTool struct {
	PluginID       string `json:"plugin_id"`
	LocalName      string `json:"local_name"`
	InstallationID string `json:"installation_id"`
	PolicyVersion  string `json:"policy_version"`
}

// CoordinatorToolPolicy is the tool list bound to one coordinator conversation
// when it opens. Agent-controlled MCP arguments never supply it.
type CoordinatorToolPolicy struct {
	Version            int                     `json:"version"`
	CoordinatorID      string                  `json:"coordinator_id"`
	WorkspaceID        string                  `json:"workspace_id"`
	ConversationTaskID string                  `json:"conversation_task_id"`
	PolicyRevision     int                     `json:"policy_revision"`
	ToolNames          []string                `json:"tool_names"`
	PluginTools        []CoordinatorPluginTool `json:"plugin_tools,omitempty"`
	// Notes is true when the coordinator notes feature was on as the
	// conversation opened; proposal tools advertise note_ids only then.
	Notes bool `json:"notes,omitempty"`
}

// Validate refuses a binding that is incomplete or names a tool outside the
// coordinator tool universe. A version-1 binding is readable and grants no
// plugin tools.
func (p CoordinatorToolPolicy) Validate() error {
	if p.Version != CoordinatorToolPolicyVersionV1 && p.Version != CoordinatorToolPolicyVersionV2 {
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
	if p.Version == CoordinatorToolPolicyVersionV1 {
		return nil
	}
	if len(p.PluginTools) > MaxCoordinatorPluginTools {
		return errors.New("coordinator tool policy declares too many plugin tools")
	}
	pluginSeen := make(map[string]struct{}, len(p.PluginTools))
	for _, tool := range p.PluginTools {
		if strings.TrimSpace(tool.PluginID) != tool.PluginID || tool.PluginID == "" ||
			strings.TrimSpace(tool.LocalName) != tool.LocalName || !managedToolNamePattern.MatchString(tool.LocalName) ||
			strings.TrimSpace(tool.InstallationID) != tool.InstallationID || tool.InstallationID == "" ||
			strings.TrimSpace(tool.PolicyVersion) != tool.PolicyVersion || tool.PolicyVersion == "" {
			return errors.New("coordinator plugin tool reference is incomplete")
		}
		key := tool.PluginID + "\x00" + tool.LocalName
		if _, dup := pluginSeen[key]; dup {
			return fmt.Errorf("coordinator tool policy duplicates plugin tool %q", tool.LocalName)
		}
		pluginSeen[key] = struct{}{}
	}
	return nil
}

// Allows reports whether the bound list names the core tool.
func (p CoordinatorToolPolicy) Allows(toolName string) bool {
	return slices.Contains(p.ToolNames, toolName)
}

// AllowsPlugin reports whether this version-2 binding carries the exact
// reference, including the installation and policy version it was pinned to.
func (p CoordinatorToolPolicy) AllowsPlugin(pluginID, localName, installationID, policyVersion string) bool {
	if p.Version != CoordinatorToolPolicyVersionV2 {
		return false
	}
	for _, tool := range p.PluginTools {
		if tool.PluginID == pluginID && tool.LocalName == localName &&
			tool.InstallationID == installationID && tool.PolicyVersion == policyVersion {
			return true
		}
	}
	return false
}

// PluginToolNames returns the bound plugin local names, for discovery filters.
func (p CoordinatorToolPolicy) PluginToolNames() []string {
	names := make([]string, 0, len(p.PluginTools))
	for _, tool := range p.PluginTools {
		names = append(names, tool.LocalName)
	}
	return names
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
	if policy.Version == CoordinatorToolPolicyVersionV1 {
		policy.PluginTools = nil
	}
	policy.PluginTools = slices.Clone(policy.PluginTools)
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
