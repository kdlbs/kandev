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
	Version           int                  `json:"version"`
	CatalogVersion    int                  `json:"catalog_version"`
	WorkspaceID       string               `json:"workspace_id"`
	TaskID            string               `json:"task_id"`
	Surface           Surface              `json:"surface"`
	WorkspaceRevision int64                `json:"workspace_revision"`
	TaskRevision      int64                `json:"task_revision"`
	Packages          []PackageID          `json:"packages"`
	Origins           map[PackageID]string `json:"origins"`
}

func (m *BuiltinManifest) Clone() *BuiltinManifest {
	if m == nil {
		return nil
	}
	copy := *m
	copy.Packages = slices.Clone(m.Packages)
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

func (m *BuiltinManifest) validateIdentity() error {
	if m.Version != 1 || m.CatalogVersion != 1 || m.WorkspaceRevision < 0 || m.TaskRevision < 0 {
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

func (m *BuiltinManifest) validOrigin(id PackageID) bool {
	origin := m.Origins[id]
	if id == PackageCore || m.Surface == SurfaceConfiguration {
		return origin == "required"
	}
	return origin == "task" || (origin == "workspace" && id != PackageConfiguration)
}

func PackageEligible(id PackageID, surface Surface) bool {
	switch surface {
	case SurfaceKanbanTask:
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
