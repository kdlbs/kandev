package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type PackageID string

const (
	PackageCore          PackageID = "core"
	PackageConfiguration PackageID = "configuration"
	PackageRichOutput    PackageID = "rich-output"
	PackageWalkthroughs  PackageID = "walkthroughs"
	PackageDiagnostics   PackageID = "diagnostics"
)

// BuiltinManifest is the backend-owned package selection bound to an execution.
// A nil manifest denotes a retained legacy, monolithic MCP catalog.
type BuiltinManifest struct {
	AutomationID            string               `json:"automation_id,omitempty"`
	AutomationRevision      int64                `json:"automation_revision,omitempty"`
	AutomationDefinitionIDs []string             `json:"automation_definition_ids,omitempty"`
	Version                 int                  `json:"version"`
	CatalogVersion          int                  `json:"catalog_version"`
	WorkspaceID             string               `json:"workspace_id"`
	TaskID                  string               `json:"task_id"`
	Surface                 Surface              `json:"surface"`
	WorkspaceRevision       int64                `json:"workspace_revision"`
	TaskRevision            int64                `json:"task_revision"`
	Packages                []PackageID          `json:"packages"`
	Origins                 map[PackageID]string `json:"origins"`
}

// Clone copies package, definition, and origin collections so launch snapshots remain independent.
func (m *BuiltinManifest) Clone() *BuiltinManifest {
	if m == nil {
		return nil
	}
	copy := *m
	copy.Packages = slices.Clone(m.Packages)
	copy.AutomationDefinitionIDs = slices.Clone(m.AutomationDefinitionIDs)
	copy.Origins = maps.Clone(m.Origins)
	return &copy
}

// Validate rejects incomplete or widened package authority.
func (m *BuiltinManifest) Validate() error {
	if m == nil {
		return nil
	}
	if err := m.validateIdentity(); err != nil {
		return err
	}
	seen := make(map[PackageID]bool, len(m.Packages))
	for _, id := range m.Packages {
		if seen[id] || !PackageEligible(id, m.Surface) {
			return fmt.Errorf("invalid or duplicate built-in package %q", id)
		}
		seen[id] = true
		if !m.validOrigin(id) {
			return errors.New("built-in package origin is invalid")
		}
	}
	if !seen[PackageCore] || len(m.Origins) != len(seen) {
		return errors.New("built-in package manifest is incomplete")
	}
	if m.Surface == SurfaceConfiguration && !seen[PackageConfiguration] {
		return errors.New("configuration package is required")
	}
	return nil
}

// validateIdentity separates version 2 automation grants from version 1 task grants and validates
// their owners.
func (m *BuiltinManifest) validateIdentity() error {
	if m.Surface == SurfaceAutomation {
		if m.Version != 2 || m.AutomationID == "" || strings.TrimSpace(m.AutomationID) != m.AutomationID || m.AutomationRevision < 1 {
			return errors.New("invalid automation package binding")
		}
		ids := make(map[string]bool)
		for _, id := range m.AutomationDefinitionIDs {
			if id == "" || strings.TrimSpace(id) != id || ids[id] {
				return errors.New("invalid automation MCP definitions")
			}
			ids[id] = true
		}
	} else if m.Version != 1 || m.AutomationID != "" || m.AutomationRevision != 0 || len(m.AutomationDefinitionIDs) != 0 {
		return errors.New("invalid built-in package binding version")
	}
	if m.CatalogVersion != 1 || m.WorkspaceRevision < 0 || m.TaskRevision < 0 {
		return errors.New("unsupported built-in package manifest version or revision")
	}
	for _, id := range []string{m.WorkspaceID, m.TaskID} {
		if id == "" || strings.TrimSpace(id) != id || strings.ContainsRune(id, '\x00') {
			return errors.New("built-in package owner is missing or invalid")
		}
	}
	if !PackageEligible(PackageCore, m.Surface) {
		return errors.New("surface does not accept built-in package selection")
	}
	return nil
}

// validOrigin requires package origins that match the manifest surface and mandatory Core authority.
func (m *BuiltinManifest) validOrigin(id PackageID) bool {
	origin := m.Origins[id]
	if id == PackageCore || m.Surface == SurfaceConfiguration {
		return origin == "required"
	}
	if m.Surface == SurfaceAutomation {
		return origin == "automation"
	}
	return origin == "task" || (origin == "workspace" && id != PackageConfiguration)
}

// PackageEligible defines the complete package allowlist for each execution surface.
func PackageEligible(id PackageID, surface Surface) bool {
	switch surface {
	case SurfaceKanbanTask, SurfaceAutomation:
		return id == PackageCore || id == PackageConfiguration || id == PackageRichOutput || id == PackageWalkthroughs || id == PackageDiagnostics
	case SurfaceOfficeTask:
		return id == PackageCore || id == PackageRichOutput
	case SurfaceConfiguration:
		return id == PackageCore || id == PackageConfiguration
	default:
		return false
	}
}

func (m *BuiltinManifest) UnmarshalJSON(data []byte) error {
	type wire BuiltinManifest
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	candidate := BuiltinManifest(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
}
