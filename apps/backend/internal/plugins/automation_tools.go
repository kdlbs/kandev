package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

// PluginToolOptions returns the current automation-capable tools of every
// active, configured plugin. It is the only source of truth for both the
// editor's checkboxes and the service-side validation of a saved selection.
func (s *Service) PluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, error) {
	s.agentToolPolicyMu.RLock()
	defer s.agentToolPolicyMu.RUnlock()
	return s.pluginToolOptionsUnlocked(ctx, workspaceID)
}

// LockPluginToolOptions returns the current automation options while holding
// the policy read lease. The automation service keeps that lease through its
// own selection write so a concurrent plugin configuration save cannot rotate
// the policy between validation and persistence.
func (s *Service) LockPluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, func(), error) {
	s.agentToolPolicyMu.RLock()
	groups, err := s.pluginToolOptionsUnlocked(ctx, workspaceID)
	if err != nil {
		s.agentToolPolicyMu.RUnlock()
		return nil, nil, err
	}
	return groups, s.agentToolPolicyMu.RUnlock, nil
}

func (s *Service) pluginToolOptionsUnlocked(context.Context, string) ([]plugintools.OptionGroup, error) {
	records := s.configReadyAgentToolRecords()
	groups := make([]plugintools.OptionGroup, 0, len(records))
	for _, record := range records {
		tools, err := automationOptionTools(record)
		if err != nil {
			return nil, err
		}
		if len(tools) == 0 {
			continue
		}
		groups = append(groups, plugintools.OptionGroup{
			PluginID:       record.ID,
			DisplayName:    record.DisplayName,
			Version:        record.Version,
			InstallationID: record.InstallationID,
			PolicyVersion:  record.AgentToolPolicyVersion,
			Tools:          tools,
		})
	}
	return groups, nil
}

func automationOptionTools(record *store.Record) ([]plugintools.OptionTool, error) {
	tools := make([]plugintools.OptionTool, 0, len(record.AgentTools))
	for _, tool := range record.AgentTools {
		if !containsString(tool.Surfaces, manifest.AgentToolSurfaceAutomation) {
			continue
		}
		input, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("plugin %q tool %q input schema: %w", record.ID, tool.Name, err)
		}
		var output json.RawMessage
		if len(tool.OutputSchema) > 0 {
			output, err = json.Marshal(tool.OutputSchema)
			if err != nil {
				return nil, fmt.Errorf("plugin %q tool %q output schema: %w", record.ID, tool.Name, err)
			}
		}
		tools = append(tools, plugintools.OptionTool{
			LocalName:       tool.Name,
			Description:     tool.Description,
			InvocationScope: tool.NormalizedInvocationScope(),
			InputSchema:     input,
			OutputSchema:    output,
			ReadOnlyHint:    boolValue(tool.Annotations.ReadOnlyHint, false),
			DestructiveHint: boolValue(tool.Annotations.DestructiveHint, true),
			IdempotentHint:  boolValue(tool.Annotations.IdempotentHint, false),
			OpenWorldHint:   boolValue(tool.Annotations.OpenWorldHint, true),
		})
	}
	return tools, nil
}

func (s *Service) invokeAutomationAgentTool(
	ctx context.Context,
	pluginID, localName string,
	arguments map[string]any,
	invocation AgentToolInvocationContext,
) (result *pluginsdk.AgentToolResult, err error) {
	started := time.Now()
	authorization := invocation.AutomationToolAuthorization
	defer func() {
		if s.log == nil {
			return
		}
		outcome := agentToolOutcomeSuccess
		if err != nil {
			outcome = agentToolOutcomeError
		} else if result != nil && result.IsError {
			outcome = agentToolOutcomeToolError
		}
		automationID, workspaceID := "", ""
		if authorization != nil {
			automationID, workspaceID = authorization.AutomationID, authorization.WorkspaceID
		}
		s.log.Info("automation plugin agent tool invocation",
			zap.String("invocation_id", invocation.InvocationID),
			zap.String("plugin_id", pluginID),
			zap.String("local_name", localName),
			zap.String("automation_id", automationID),
			zap.String("workspace_id", workspaceID),
			zap.String("task_id", invocation.TaskID),
			zap.String("session_id", invocation.SessionID),
			zap.Duration("duration", time.Since(started)),
			zap.String("outcome", outcome))
	}()
	if err := validateAutomationToolApproval(invocation, pluginID, localName); err != nil {
		return nil, err
	}

	return s.invokeDeclaredAgentTool(ctx, pluginID, localName, arguments, invocation, manifest.AgentToolSurfaceAutomation, func(record *store.Record) error {
		if err := validateAutomationToolRecord(record, authorization, pluginID); err != nil {
			return err
		}
		if !s.requiredConfigSet(record) {
			return fmt.Errorf("plugins: plugin %q configuration is not ready", pluginID)
		}
		return nil
	})
}

func validateAutomationToolApproval(invocation AgentToolInvocationContext, pluginID, localName string) error {
	authorization := invocation.AutomationToolAuthorization
	if invocation.Surface != manifest.AgentToolSurfaceAutomation || authorization == nil ||
		authorization.Validate() != nil || !authorization.Allows(pluginID, localName) ||
		authorization.WorkspaceID != invocation.WorkspaceID ||
		invocation.ExecutionID == "" || invocation.TaskID == "" || invocation.SessionID == "" ||
		invocation.InvocationID == "" {
		return fmt.Errorf("plugins: automation agent tool approval denied the invocation")
	}
	return nil
}

func validateAutomationToolRecord(record *store.Record, authorization *plugintools.AutomationAuthorization, pluginID string) error {
	if record.Status != StatusActive {
		return fmt.Errorf("plugins: plugin %q is not active", pluginID)
	}
	if record.InstallationID != authorization.InstallationID ||
		record.AgentToolPolicyVersion != authorization.PolicyVersion {
		return fmt.Errorf("plugins: automation agent tool approval is stale")
	}
	return nil
}
