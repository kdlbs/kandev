package mcpconfig

import (
	"errors"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

const (
	BuiltinPolicyBlocked     = "blocked"
	BuiltinPolicyAvailable   = "available"
	BuiltinPolicyDefault     = "default"
	BuiltinPreferenceOn      = "on"
	BuiltinPreferenceOff     = "off"
	BuiltinPreferenceInherit = "inherit"
)

var ErrBuiltinPolicyInvalid = errors.New("invalid built-in MCP policy")

type BuiltinWorkspacePolicy struct {
	WorkspaceID string                `json:"workspace_id"`
	Version     int                   `json:"version"`
	Revision    int64                 `json:"revision"`
	Packages    BuiltinPackageChoices `json:"packages"`
	UpdatedBy   string                `json:"updated_by,omitempty"`
	UpdatedAt   time.Time             `json:"updated_at,omitempty"`
}

type BuiltinTaskPreferences struct {
	WorkspaceID string                `json:"workspace_id"`
	TaskID      string                `json:"task_id"`
	Version     int                   `json:"version"`
	Revision    int64                 `json:"revision"`
	Packages    BuiltinPackageChoices `json:"packages"`
	UpdatedBy   string                `json:"updated_by,omitempty"`
	UpdatedAt   time.Time             `json:"updated_at,omitempty"`
}

func ResolveBuiltinPackages(workspaceID, taskID string, surface mcpprofile.Surface, policy *BuiltinWorkspacePolicy, preferences *BuiltinTaskPreferences) (*mcpprofile.BuiltinManifest, error) {
	switch surface {
	case mcpprofile.SurfaceExternal, mcpprofile.SurfaceAutomation, mcpprofile.SurfaceCoordinator, mcpprofile.SurfaceManagedConversation:
		return nil, nil
	}
	manifest := &mcpprofile.BuiltinManifest{
		Version: 1, CatalogVersion: 1, WorkspaceID: workspaceID, TaskID: taskID, Surface: surface,
		Packages: []mcpprofile.PackageID{mcpprofile.PackageCore}, Origins: map[mcpprofile.PackageID]string{mcpprofile.PackageCore: "required"},
	}
	if surface == mcpprofile.SurfaceConfiguration {
		manifest.Packages = append(manifest.Packages, mcpprofile.PackageConfiguration)
		manifest.Origins[mcpprofile.PackageConfiguration] = "required"
		return manifest, manifest.Validate()
	}
	if policy == nil {
		policy = DefaultBuiltinWorkspacePolicy(workspaceID)
	}
	if preferences == nil {
		preferences = &BuiltinTaskPreferences{WorkspaceID: workspaceID, TaskID: taskID, Version: 1}
	}
	if policy.WorkspaceID != workspaceID || preferences.WorkspaceID != workspaceID || preferences.TaskID != taskID {
		return nil, ErrBuiltinPolicyInvalid
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if err := preferences.Validate(surface); err != nil {
		return nil, err
	}
	manifest.WorkspaceRevision, manifest.TaskRevision = policy.Revision, preferences.Revision
	appendBuiltinSelections(manifest, policy, preferences)
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return manifest, nil
}

func appendBuiltinSelections(manifest *mcpprofile.BuiltinManifest, policy *BuiltinWorkspacePolicy, preferences *BuiltinTaskPreferences) {
	for _, descriptor := range mcpprofile.BuiltinPackageCatalog() {
		id := descriptor.ID
		if id == mcpprofile.PackageCore || !mcpprofile.PackageEligible(id, manifest.Surface) {
			continue
		}
		limit, preference := policy.Packages[id], preferences.Packages[id]
		if limit == BuiltinPolicyBlocked || preference == BuiltinPreferenceOff || (preference != BuiltinPreferenceOn && limit != BuiltinPolicyDefault) {
			continue
		}
		manifest.Packages = append(manifest.Packages, id)
		manifest.Origins[id] = "workspace"
		if preference == BuiltinPreferenceOn {
			manifest.Origins[id] = "task"
		}
	}
}

func DefaultBuiltinWorkspacePolicy(workspaceID string) *BuiltinWorkspacePolicy {
	policy := &BuiltinWorkspacePolicy{WorkspaceID: workspaceID, Version: 1, Packages: make(map[mcpprofile.PackageID]string)}
	for _, descriptor := range mcpprofile.BuiltinPackageCatalog() {
		if descriptor.ID != mcpprofile.PackageCore {
			policy.Packages[descriptor.ID] = descriptor.DefaultPolicy
		}
	}
	return policy
}

func (p *BuiltinWorkspacePolicy) Validate() error {
	if p.Version != 1 || p.Revision < 0 || p.WorkspaceID == "" || len(p.Packages) != 4 {
		return ErrBuiltinPolicyInvalid
	}
	for id, value := range p.Packages {
		if !mcpprofile.PackageEligible(id, mcpprofile.SurfaceKanbanTask) || id == mcpprofile.PackageCore {
			return ErrBuiltinPolicyInvalid
		}
		if value != BuiltinPolicyBlocked && value != BuiltinPolicyAvailable && value != BuiltinPolicyDefault {
			return ErrBuiltinPolicyInvalid
		}
		if id == mcpprofile.PackageConfiguration && value == BuiltinPolicyDefault {
			return ErrBuiltinPolicyInvalid
		}
	}
	return nil
}

func (p *BuiltinTaskPreferences) Validate(surface mcpprofile.Surface) error {
	if p.Version != 1 || p.Revision < 0 || p.WorkspaceID == "" || p.TaskID == "" {
		return ErrBuiltinPolicyInvalid
	}
	if surface != mcpprofile.SurfaceKanbanTask && surface != mcpprofile.SurfaceOfficeTask {
		return ErrBuiltinPolicyInvalid
	}
	for id, value := range p.Packages {
		if !mcpprofile.PackageEligible(id, surface) || id == mcpprofile.PackageCore {
			return ErrBuiltinPolicyInvalid
		}
		if value != BuiltinPreferenceInherit && value != BuiltinPreferenceOn && value != BuiltinPreferenceOff {
			return ErrBuiltinPolicyInvalid
		}
	}
	return nil
}
