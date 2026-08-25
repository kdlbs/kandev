package backendapp

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// pluginsAutomationSourceAdapter maps Automation service records to the
// deliberately compact plugin Host projection. Keeping this adapter in
// backendapp preserves the plugins package's provider-neutral boundary.
type pluginsAutomationSourceAdapter struct {
	svc *automation.Service
}

func (a pluginsAutomationSourceAdapter) ListPluginAutomations(ctx context.Context, workspaceID string) ([]pluginsdk.Automation, error) {
	items, err := a.svc.ListAutomations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]pluginsdk.Automation, len(items))
	for i, item := range items {
		out[i] = pluginAutomationDescriptor(item)
	}
	return out, nil
}

func (a pluginsAutomationSourceAdapter) GetPluginAutomation(ctx context.Context, workspaceID, automationID string) (*pluginsdk.Automation, error) {
	item, err := a.svc.GetAutomation(ctx, automationID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.WorkspaceID != workspaceID {
		return nil, status.Error(codes.NotFound, "automation not found")
	}
	out := pluginAutomationDescriptor(item)
	return &out, nil
}

func pluginAutomationTriggers(items []automation.AutomationTrigger) []pluginsdk.AutomationTrigger {
	if len(items) == 0 {
		return nil
	}
	out := make([]pluginsdk.AutomationTrigger, len(items))
	for i, t := range items {
		configJSON := ""
		if len(t.Config) > 0 {
			configJSON = string(t.Config)
		} else if t.ConfigJSON != "" {
			// Ensure the ConfigJSON is valid JSON even if stored as text
			configJSON = t.ConfigJSON
		}
		out[i] = pluginsdk.AutomationTrigger{
			ID:         t.ID,
			Type:       string(t.Type),
			ConfigJSON: configJSON,
			Enabled:    t.Enabled,
		}
	}
	return out
}

func pluginAutomationDescriptor(item *automation.Automation) pluginsdk.Automation {
	updatedAt := ""
	if !item.UpdatedAt.IsZero() {
		updatedAt = item.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return pluginsdk.Automation{
		ID:                item.ID,
		WorkspaceID:       item.WorkspaceID,
		Name:              item.Name,
		Description:       item.Description,
		AgentProfileID:    item.AgentProfileID,
		ExecutorProfileID: item.ExecutorProfileID,
		Prompt:            item.Prompt,
		Enabled:           item.Enabled,
		MaxConcurrentRuns: int32(item.MaxConcurrentRuns),
		UpdatedAt:         updatedAt,
		WorkflowID:        item.WorkflowID,
		WorkflowStepID:    item.WorkflowStepID,
		TaskMode:          string(item.TaskMode),
		RepositoryMode:    string(item.RepositoryMode),
		TaskTitleTemplate: item.TaskTitleTemplate,
		Triggers:          pluginAutomationTriggers(item.Triggers),
	}
}
