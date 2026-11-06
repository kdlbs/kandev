package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const automationMCPNullJSON = "null"

var ErrMCPSelectionConflict = errors.New("automation MCP selection changed; reload choices")

// MCPSelection stores references only. Runtime credentials stay in the workspace catalog.
type MCPSelection struct {
	BuiltinPackages []p.PackageID `json:"builtin_packages"`
	DefinitionIDs   []string      `json:"definition_ids"`
}

// Clone copies selection slices so drafts cannot mutate stored grants.
func (s *MCPSelection) Clone() *MCPSelection {
	if s == nil {
		return nil
	}
	return &MCPSelection{BuiltinPackages: append([]p.PackageID{}, s.BuiltinPackages...), DefinitionIDs: append([]string{}, s.DefinitionIDs...)}
}

// validate requires Core, explicit definition IDs, and unique packages supported on task surfaces.
func (s *MCPSelection) validate() error {
	if s == nil {
		return nil
	}
	if s.BuiltinPackages == nil || s.DefinitionIDs == nil || !slices.Contains(s.BuiltinPackages, p.PackageCore) {
		return errors.New("automation MCP selection requires Core and definition_ids")
	}
	seen := make(map[p.PackageID]bool)
	for _, id := range s.BuiltinPackages {
		if seen[id] || !p.PackageEligible(id, p.SurfaceKanbanTask) {
			return fmt.Errorf("invalid built-in package %q", id)
		}
		seen[id] = true
	}
	ids := make(map[string]bool)
	for _, id := range s.DefinitionIDs {
		if id == "" || ids[id] {
			return errors.New("invalid or duplicate MCP definition")
		}
		ids[id] = true
	}
	return nil
}

// decodeMCPSelection preserves legacy absence and validates each stored explicit selection.
func decodeMCPSelection(a *Automation) error {
	if a.MCPSelectionJSON == "" || a.MCPSelectionJSON == automationMCPNullJSON {
		a.MCPSelection = nil
		return nil
	}
	if err := json.Unmarshal([]byte(a.MCPSelectionJSON), &a.MCPSelection); err != nil {
		return fmt.Errorf("invalid automation MCP selection: %w", err)
	}
	return a.MCPSelection.validate()
}

type MCPDefinitionReader interface {
	ListMCPServerDefinitions(context.Context, string) ([]*mcpconfig.MCPServerDefinition, error)
	GetMCPServerDefinition(context.Context, string, string) (*mcpconfig.MCPServerDefinition, error)
}
type MCPBuiltinPolicyReader interface {
	GetBuiltinPolicySnapshot(context.Context, string, string) (*mcpconfig.BuiltinPolicySnapshot, error)
}

func (s *Service) SetMCPSelectionReaders(definitions MCPDefinitionReader, policy MCPBuiltinPolicyReader) {
	s.mcpDefinitions = definitions
	s.mcpBuiltinPolicy = policy
}

// validateMCPSelection admits only native editor grants to available workspace definitions.
// Normal-task templates also obey workspace package policy.
func (s *Service) validateMCPSelection(ctx context.Context, workspaceID string, mode TaskMode, selection *MCPSelection) error {
	if selection == nil {
		return nil
	}
	if !hasNativePluginToolIntent(ctx) {
		return errors.New("MCP selection may only be changed by the native automation editor")
	}
	if mode == TaskModeManagedConversation {
		return errors.New("managed conversations own their MCP selection")
	}
	if err := selection.validate(); err != nil {
		return err
	}
	for _, id := range selection.DefinitionIDs {
		if s.mcpDefinitions == nil {
			return errors.New("MCP catalog unavailable")
		}
		d, err := s.mcpDefinitions.GetMCPServerDefinition(ctx, workspaceID, id)
		if err != nil {
			return err
		}
		if d == nil || d.WorkspaceID != workspaceID || !d.Enabled {
			return errors.New("MCP definition unavailable in this workspace")
		}
	}
	if mode == TaskModeNormalTask {
		return s.validateMCPTaskPackages(ctx, workspaceID, selection)
	}
	return nil
}

// validateMCPTaskPackages rejects template choices that workspace policy would remove at task
// creation.
func (s *Service) validateMCPTaskPackages(ctx context.Context, workspaceID string, selection *MCPSelection) error {
	if s.mcpBuiltinPolicy == nil {
		return errors.New("built-in package policy unavailable")
	}
	snapshot, err := s.mcpBuiltinPolicy.GetBuiltinPolicySnapshot(ctx, workspaceID, "")
	if err != nil {
		return err
	}
	if snapshot == nil || snapshot.Policy == nil {
		return errors.New("built-in package policy unavailable")
	}
	preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: workspaceID, TaskID: "automation-template", Packages: map[p.PackageID]string{}}
	for _, d := range p.BuiltinPackageCatalog() {
		if d.ID == p.PackageCore {
			continue
		}
		value := mcpconfig.BuiltinPreferenceOff
		if slices.Contains(selection.BuiltinPackages, d.ID) {
			value = mcpconfig.BuiltinPreferenceOn
		}
		preferences.Packages[d.ID] = value
	}
	manifest, err := mcpconfig.ResolveBuiltinPackages(workspaceID, "automation-template", p.SurfaceKanbanTask, snapshot.Policy, preferences)
	if err != nil {
		return err
	}
	for _, id := range selection.BuiltinPackages {
		if !slices.Contains(manifest.Packages, id) {
			return errors.New("built-in package is blocked by workspace policy")
		}
	}
	return nil
}

// GetAutomationMCPBinding derives authority from durable run references, not task metadata.
func (s *Service) GetAutomationMCPBinding(ctx context.Context, workspaceID, taskID string) (*p.BuiltinManifest, error) {
	var ids []string
	err := s.store.ro.SelectContext(ctx, &ids, s.store.ro.Rebind(`SELECT DISTINCT a.id FROM automations a WHERE a.workspace_id = ? AND (a.continuation_task_id = ? OR EXISTS (SELECT 1 FROM automation_runs r WHERE r.automation_id = a.id AND r.task_id = ?))`), workspaceID, taskID, taskID)
	if err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, errors.New("automation MCP ownership is unavailable or ambiguous")
	}
	a, err := s.store.GetAutomation(ctx, ids[0])
	if err != nil {
		return nil, err
	}
	if a == nil || a.WorkspaceID != workspaceID || a.TaskMode != TaskModeAutomationRun {
		return nil, errors.New("automation MCP ownership mismatch")
	}
	if a.MCPSelection == nil {
		return nil, nil
	}
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspaceID, TaskID: taskID, Surface: p.SurfaceAutomation, AutomationID: a.ID, AutomationRevision: a.MCPSelectionRevision, AutomationDefinitionIDs: append([]string(nil), a.MCPSelection.DefinitionIDs...), Packages: append([]p.PackageID(nil), a.MCPSelection.BuiltinPackages...), Origins: make(map[p.PackageID]string)}
	for _, id := range manifest.Packages {
		manifest.Origins[id] = "automation"
	}
	manifest.Origins[p.PackageCore] = "required"
	return manifest, manifest.Validate()
}

// AutomationMCPTaskTemplate captures the current template before creating a normal task.
func (s *Service) AutomationMCPTaskTemplate(ctx context.Context, automationID string) (*mcpconfig.BuiltinCreationPreferences, []string, []mcpconfig.MCPResolutionDecision, error) {
	a, err := s.GetAutomationForDispatch(ctx, automationID)
	if err != nil {
		return nil, nil, nil, err
	}
	if a == nil || a.TaskMode != TaskModeNormalTask {
		return nil, nil, nil, errors.New("normal automation template unavailable")
	}
	if a.MCPSelection == nil {
		return nil, nil, nil, nil
	}
	if s.mcpBuiltinPolicy == nil {
		return nil, nil, nil, errors.New("built-in policy unavailable")
	}
	snapshot, err := s.mcpBuiltinPolicy.GetBuiltinPolicySnapshot(ctx, a.WorkspaceID, "")
	if err != nil {
		return nil, nil, nil, err
	}
	if snapshot == nil || snapshot.Policy == nil {
		return nil, nil, nil, errors.New("built-in policy unavailable")
	}
	choices := make(mcpconfig.BuiltinPackageChoices)
	for _, d := range p.BuiltinPackageCatalog() {
		if d.ID == p.PackageCore {
			continue
		}
		choices[d.ID] = mcpconfig.BuiltinPreferenceOff
		if slices.Contains(a.MCPSelection.BuiltinPackages, d.ID) {
			choices[d.ID] = mcpconfig.BuiltinPreferenceOn
		}
	}
	revision := snapshot.Policy.Revision
	ids, decisions, err := s.automationMCPTemplateDefinitions(ctx, a)
	if err != nil {
		return nil, nil, nil, err
	}
	return &mcpconfig.BuiltinCreationPreferences{Version: 1, ExpectedPolicyRevision: &revision, Packages: choices}, ids, decisions, nil
}
