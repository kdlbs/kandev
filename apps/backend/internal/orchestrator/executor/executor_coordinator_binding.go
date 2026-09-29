package executor

import (
	"errors"
	"fmt"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// bindCoordinatorToolPolicy carries the conversation task's stored tool
// binding into the launch profile. A task with no binding keeps the phase-1
// tool set; a binding that is present but unreadable, or that names another
// task, fails the launch rather than widening or narrowing the surface.
func bindCoordinatorToolPolicy(profile *mcpprofile.Context, task *models.Task) error {
	value, present := task.Metadata[mcpprofile.CoordinatorToolPolicyMetadataKey]
	if !present {
		return nil
	}
	binding, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(value)
	if err != nil {
		return fmt.Errorf("resolve coordinator tool binding: %w", err)
	}
	if binding.ConversationTaskID != task.ID {
		return errors.New("resolve coordinator tool binding: binding names another task")
	}
	profile.CoordinatorToolPolicy = binding
	return nil
}
