package coordinator

import (
	"context"
	"sort"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// CoordinatorPluginBindingSource supplies the exact instance-read plugin
// references eligible for a new coordinator conversation binding.
type CoordinatorPluginBindingSource interface {
	CoordinatorPluginBindings() ([]mcpprofile.CoordinatorPluginTool, error)
}

// SetPluginBindingSource installs the plugin catalog used when a phase-2
// conversation binding is created. A nil source grants no plugin tools.
func (s *Service) SetPluginBindingSource(source CoordinatorPluginBindingSource) {
	s.pluginBindings = source
}

// stampToolPolicy writes the conversation's tool binding into metadata. With
// phase 2 off nothing is written, so the conversation keeps the phase-1 seven.
func (s *Service) stampToolPolicy(metadata map[string]interface{}, c *Coordinator, conversationTaskID string) error {
	if !s.phase2 {
		return nil
	}
	pluginTools := []mcpprofile.CoordinatorPluginTool{}
	if s.pluginBindings != nil {
		bound, err := s.pluginBindings.CoordinatorPluginBindings()
		if err != nil {
			// A catalog read failure must not invent a grant: the conversation
			// still opens with the core tools and no plugin reads.
			s.logger.Warn("coordinator plugin tool catalog unreadable; opening without plugin reads",
				zap.String("coordinator_id", c.ID), zap.Error(err))
		} else {
			pluginTools = bound
		}
	}
	sort.Slice(pluginTools, func(i, j int) bool {
		if pluginTools[i].PluginID == pluginTools[j].PluginID {
			return pluginTools[i].LocalName < pluginTools[j].LocalName
		}
		return pluginTools[i].PluginID < pluginTools[j].PluginID
	})
	if len(pluginTools) > mcpprofile.MaxCoordinatorPluginTools {
		s.logger.Warn("coordinator plugin tool catalog exceeds the binding capacity; trimming deterministically",
			zap.String("coordinator_id", c.ID),
			zap.Int("eligible", len(pluginTools)),
			zap.Int("bound", mcpprofile.MaxCoordinatorPluginTools))
		pluginTools = pluginTools[:mcpprofile.MaxCoordinatorPluginTools]
	}
	binding := mcpprofile.CoordinatorToolPolicy{
		Version:            mcpprofile.CoordinatorToolPolicyVersionV2,
		CoordinatorID:      c.ID,
		WorkspaceID:        c.WorkspaceID,
		ConversationTaskID: conversationTaskID,
		PolicyRevision:     c.PolicyRevision,
		ToolNames:          ToolNamesPhase31(s.policyFor(c), true, s.phase3, s.phase31),
		PluginTools:        pluginTools,
	}
	// The empty-id form is stamped before the task exists, so it cannot pass
	// full validation. Once the id is available the binding must validate
	// before it is published to launch metadata.
	if conversationTaskID != "" {
		if err := binding.Validate(); err != nil {
			return err
		}
	}
	encoded, err := mcpprofile.MarshalCoordinatorToolPolicy(binding)
	if err != nil {
		return err
	}
	metadata[mcpprofile.CoordinatorToolPolicyMetadataKey] = encoded
	return nil
}

// bindConversationTask fills the created task's own id into its binding: the
// id does not exist until the task is created, and a binding with no
// conversation task id is invalid at execution time.
func (s *Service) bindConversationTask(ctx context.Context, task *taskmodels.Task, c *Coordinator, metadata map[string]interface{}) error {
	if !s.phase2 {
		return nil
	}
	if err := s.stampToolPolicy(metadata, c, task.ID); err != nil {
		return err
	}
	bindCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bindTimeout)
	defer cancel()
	_, err := s.conversationTasks.UpdateTask(bindCtx, task.ID, &taskservice.UpdateTaskRequest{
		Metadata:              metadata,
		AllowReservedMetadata: true,
	})
	return err
}
