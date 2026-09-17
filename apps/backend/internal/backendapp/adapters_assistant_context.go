package backendapp

import (
	"fmt"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/share"
)

const assistantBaseDescription = "orchestration_base_description"

func attachAssistantTaskContext(task *models.Task, metadata map[string]interface{}, profile string, ref shared.DelegationReference) (*string, error) {
	p := ref.Packet
	if p == nil {
		return nil, nil
	}
	if p.ProfileID != profile || p.WorkspaceID != task.WorkspaceID {
		return nil, fmt.Errorf("context must match the selected account and workspace")
	}
	if p.TaskID != "" && p.TaskID != task.ID {
		return nil, fmt.Errorf("context belongs to another task")
	}
	if binding, _ := metadata["orchestration_binding_id"].(string); binding != "" && binding != p.BindingID {
		return nil, fmt.Errorf("context belongs to another assistant")
	}
	base, hasBase := metadata[assistantBaseDescription].(string)
	if !hasBase {
		if old, _ := metadata["orchestration_context_ref"].(string); old != "" && old != ref.ContextRef {
			return nil, fmt.Errorf("legacy context requires a new task handoff")
		}
		base = task.Description
	}
	base = share.NewRedactor().String(base)
	metadata[assistantBaseDescription] = base
	metadata["orchestration_binding_id"] = p.BindingID
	metadata["orchestration_objective_id"] = ref.ObjectiveID
	metadata["orchestration_context_ref"] = ref.ContextRef
	metadata["orchestration_acceptance_revision"] = ref.AcceptanceRevision
	metadata["orchestration_source_comment_id"] = ref.SourceCommentID
	metadata["orchestration_operation_id"] = ref.DispatchOperationID
	description := assistantDelegationPrompt(base, ref)
	return &description, nil
}
