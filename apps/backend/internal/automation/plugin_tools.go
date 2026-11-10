package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
)

const maxAutomationPluginGroups = 16

const emptyPluginToolsJSON = "[]"

// ErrPluginToolOptionsUnavailable distinguishes a plugin catalog failure from
// a workspace authorization failure in the native options action.
var ErrPluginToolOptionsUnavailable = errors.New("automation: plugin tool options unavailable")

func encodePluginTools(selections []plugintools.Selection) (string, error) {
	normalized := plugintools.NormalizeSelection(selections)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode plugin tools: %w", err)
	}
	return string(encoded), nil
}

// decodePluginTools treats malformed stored authority as no access. It never
// fails the surrounding row read, because a corrupt selection must not make an
// otherwise valid automation unreadable.
func decodePluginTools(raw string) []plugintools.Selection {
	if raw == "" || raw == emptyPluginToolsJSON {
		return []plugintools.Selection{}
	}
	var selections []plugintools.Selection
	if err := json.Unmarshal([]byte(raw), &selections); err != nil {
		return []plugintools.Selection{}
	}
	if err := validatePluginToolSelectionShape(selections); err != nil {
		return []plugintools.Selection{}
	}
	for _, selection := range selections {
		if err := selection.Validate(); err != nil {
			return []plugintools.Selection{}
		}
	}
	return plugintools.NormalizeSelection(selections)
}

// PluginToolOptionsProvider is the narrow plugin-system dependency used for
// the native automation editor's options and save validation. The plugin
// service owns installation identity, policy versions, declarations, and
// configuration readiness.
type PluginToolOptionsProvider interface {
	PluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, error)
}

// SetPluginToolOptionsProvider wires the plugin catalog dependency. Without
// it, automations may still be saved but they cannot select plugin tools.
func (s *Service) SetPluginToolOptionsProvider(provider PluginToolOptionsProvider) {
	s.pluginToolOptions = provider
}

type nativePluginToolIntentKey struct{}

// WithNativePluginToolIntent marks a configuration save that came from the
// native automation editor. Only native handlers may set it; MCP callers and
// plugin Host schedule commands cannot create or renew a selection. An MCP
// execution or principal context is never native, even when a shared
// dispatcher forwards a raw automation action from an agent stream.
func WithNativePluginToolIntent(ctx context.Context) context.Context {
	if isMCPAutomationContext(ctx) {
		return ctx
	}
	return context.WithValue(ctx, nativePluginToolIntentKey{}, true)
}

func hasNativePluginToolIntent(ctx context.Context) bool {
	native, _ := ctx.Value(nativePluginToolIntentKey{}).(bool)
	return native && !isMCPAutomationContext(ctx)
}

// isMCPAutomationContext reports whether this request came from an agent MCP
// stream or carries a server-derived MCP principal. Native UI requests have
// neither.
func isMCPAutomationContext(ctx context.Context) bool {
	if _, ok := streams.MCPExecutionContextFromContext(ctx); ok {
		return true
	}
	_, ok := mcpscope.PrincipalFromContext(ctx)
	return ok
}

// SetPluginToolCatalogNotifier wires the existing plugin catalog notification
// path. Automation selection changes do not alter plugin declarations; the
// notification makes any automation MCP session refetch its filtered catalog.
func (s *Service) SetPluginToolCatalogNotifier(notify func()) {
	s.pluginToolCatalogChanged = notify
}

func (s *Service) notifyPluginToolCatalogChanged() {
	if s.pluginToolCatalogChanged != nil {
		s.pluginToolCatalogChanged()
	}
}

// PluginToolOptions returns the current selectable plugin tools after checking
// the caller's workspace access.
func (s *Service) PluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAutomationNotFound, err)
	}
	groups, err := s.currentPluginToolOptions(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPluginToolOptionsUnavailable, err)
	}
	return groups, nil
}

func (s *Service) currentPluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, error) {
	if s.pluginToolOptions == nil {
		return []plugintools.OptionGroup{}, nil
	}
	groups, err := s.pluginToolOptions.PluginToolOptions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []plugintools.OptionGroup{}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].PluginID < groups[j].PluginID })
	return groups, nil
}

// ApprovedPluginTools returns the currently approved definitions for one
// automation. It re-reads the stored row and current plugin policy on every
// call; a malformed, foreign, or stale selection contributes no tools.
func (s *Service) ApprovedPluginTools(ctx context.Context, automationID, workspaceID string) ([]plugintools.Definition, error) {
	automation, err := s.pluginToolAutomation(ctx, automationID, workspaceID)
	if err != nil || automation == nil {
		return nil, err
	}
	groups, err := s.currentPluginToolOptions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return approvedPluginToolDefinitions(automation.PluginTools, groups), nil
}

// AutomationToolAuthorization returns the exact approval for one plugin, or
// nil when the stored selection no longer matches the installed policy.
func (s *Service) AutomationToolAuthorization(ctx context.Context, automationID, workspaceID, pluginID string) (*plugintools.AutomationAuthorization, error) {
	automation, err := s.pluginToolAutomation(ctx, automationID, workspaceID)
	if err != nil || automation == nil {
		return nil, err
	}
	groups, err := s.currentPluginToolOptions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	selection, ok := matchingPluginSelection(automation.PluginTools, groups, pluginID)
	if !ok {
		return nil, nil
	}
	return &plugintools.AutomationAuthorization{
		AutomationID:   automation.ID,
		WorkspaceID:    automation.WorkspaceID,
		PluginID:       selection.PluginID,
		InstallationID: selection.InstallationID,
		PolicyVersion:  selection.PolicyVersion,
		ToolNames:      append([]string(nil), selection.ToolNames...),
	}, nil
}

func (s *Service) pluginToolAutomation(ctx context.Context, automationID, workspaceID string) (*Automation, error) {
	if automationID == "" || workspaceID == "" {
		return nil, nil
	}
	automation, err := s.store.GetAutomation(ctx, automationID)
	if err != nil {
		return nil, err
	}
	if automation == nil || automation.WorkspaceID != workspaceID ||
		automation.TaskMode != TaskModeAutomationRun {
		return nil, nil
	}
	return automation, nil
}

func approvedPluginToolDefinitions(selections []plugintools.Selection, groups []plugintools.OptionGroup) []plugintools.Definition {
	byID := make(map[string]plugintools.OptionGroup, len(groups))
	for _, group := range groups {
		byID[group.PluginID] = group
	}
	definitions := make([]plugintools.Definition, 0)
	for _, selection := range selections {
		group, ok := byID[selection.PluginID]
		if !ok || group.InstallationID != selection.InstallationID || group.PolicyVersion != selection.PolicyVersion {
			continue
		}
		tools := make(map[string]plugintools.OptionTool, len(group.Tools))
		for _, tool := range group.Tools {
			tools[tool.LocalName] = tool
		}
		groupTools := make([]plugintools.OptionTool, 0, len(selection.ToolNames))
		invalid := false
		for _, name := range selection.ToolNames {
			tool, ok := tools[name]
			if !ok {
				invalid = true
				break
			}
			groupTools = append(groupTools, tool)
		}
		if invalid {
			continue
		}
		for _, tool := range groupTools {
			definitions = append(definitions, plugintools.Definition{
				PluginID:          group.PluginID,
				PluginDisplayName: group.DisplayName,
				LocalName:         tool.LocalName,
				ExposedName:       plugintools.ExposedName(group.PluginID, tool.LocalName),
				Description:       tool.Description,
				Surfaces:          []string{plugintools.SurfaceAutomation},
				InvocationScope:   tool.InvocationScope,
				InstallationID:    group.InstallationID,
				PolicyVersion:     group.PolicyVersion,
				InputSchema:       tool.InputSchema,
				OutputSchema:      tool.OutputSchema,
				ReadOnlyHint:      tool.ReadOnlyHint,
				DestructiveHint:   tool.DestructiveHint,
				IdempotentHint:    tool.IdempotentHint,
				OpenWorldHint:     tool.OpenWorldHint,
			})
		}
	}
	return definitions
}

func matchingPluginSelection(selections []plugintools.Selection, groups []plugintools.OptionGroup, pluginID string) (plugintools.Selection, bool) {
	for _, group := range groups {
		if group.PluginID != pluginID {
			continue
		}
		for _, selection := range selections {
			if selection.PluginID != pluginID || selection.InstallationID != group.InstallationID ||
				selection.PolicyVersion != group.PolicyVersion || len(selection.ToolNames) == 0 {
				continue
			}
			available := make(map[string]struct{}, len(group.Tools))
			for _, tool := range group.Tools {
				available[tool.LocalName] = struct{}{}
			}
			valid := true
			for _, name := range selection.ToolNames {
				if _, ok := available[name]; !ok {
					valid = false
					break
				}
			}
			if valid {
				return selection, true
			}
		}
	}
	return plugintools.Selection{}, false
}

type pluginToolOptionsLocker interface {
	LockPluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, func(), error)
}

func (s *Service) pluginToolsForSave(ctx context.Context, workspaceID string, requested []plugintools.Selection, native bool, taskMode TaskMode) ([]plugintools.Selection, func(), error) {
	noop := func() {}
	if len(requested) == 0 {
		return []plugintools.Selection{}, noop, nil
	}
	if !native {
		return nil, noop, errors.New("plugin tools may only be approved by the native automation editor")
	}
	groups, unlock, err := s.lockPluginToolOptions(ctx, workspaceID)
	if err != nil {
		return nil, noop, err
	}
	selections, err := validatePluginToolSelectionWithGroups(requested, taskMode, groups)
	if err != nil {
		unlock()
		return nil, noop, err
	}
	return selections, unlock, nil
}

func (s *Service) lockPluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, func(), error) {
	if locker, ok := s.pluginToolOptions.(pluginToolOptionsLocker); ok {
		return locker.LockPluginToolOptions(ctx, workspaceID)
	}
	groups, err := s.currentPluginToolOptions(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	return groups, func() {}, nil
}

func validatePluginToolSelectionWithGroups(selections []plugintools.Selection, taskMode TaskMode, groups []plugintools.OptionGroup) ([]plugintools.Selection, error) {
	if taskMode != TaskModeAutomationRun {
		if len(selections) > 0 {
			return nil, errors.New("plugin tools are only available to private automations")
		}
		return []plugintools.Selection{}, nil
	}
	if len(selections) == 0 {
		return []plugintools.Selection{}, nil
	}
	if err := validatePluginToolSelectionShape(selections); err != nil {
		return nil, err
	}
	if err := validatePluginToolSelectionAgainstOptions(selections, groups); err != nil {
		return nil, err
	}
	return plugintools.NormalizeSelection(selections), nil
}

func validatePluginToolSelectionShape(selections []plugintools.Selection) error {
	if len(selections) > maxAutomationPluginGroups {
		return fmt.Errorf("at most %d plugin tool groups may be selected", maxAutomationPluginGroups)
	}
	seenPlugins := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		if err := selection.Validate(); err != nil {
			return err
		}
		if _, ok := seenPlugins[selection.PluginID]; ok {
			return fmt.Errorf("plugin %q is selected more than once", selection.PluginID)
		}
		seenPlugins[selection.PluginID] = struct{}{}
	}
	return nil
}

func validatePluginToolSelectionAgainstOptions(selections []plugintools.Selection, groups []plugintools.OptionGroup) error {
	byID := make(map[string]plugintools.OptionGroup, len(groups))
	for _, group := range groups {
		byID[group.PluginID] = group
	}
	for _, selection := range selections {
		group, ok := byID[selection.PluginID]
		if !ok {
			return fmt.Errorf("plugin %q is not available for private automations", selection.PluginID)
		}
		if group.InstallationID != selection.InstallationID || group.PolicyVersion != selection.PolicyVersion {
			return fmt.Errorf("plugin %q configuration changed; review the tool selection", selection.PluginID)
		}
		available := make(map[string]struct{}, len(group.Tools))
		for _, tool := range group.Tools {
			available[tool.LocalName] = struct{}{}
		}
		for _, name := range selection.ToolNames {
			if _, ok := available[name]; !ok {
				return fmt.Errorf("plugin %q tool %q is not available for private automations", selection.PluginID, name)
			}
		}
	}
	return nil
}
