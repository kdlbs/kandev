package profile

import (
	"slices"
	"strings"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type BuiltinPackageDescriptor struct {
	ID               PackageID               `json:"id"`
	RuntimeName      string                  `json:"runtime_name"`
	LabelKey         string                  `json:"label_key"`
	DescriptionKey   string                  `json:"description_key"`
	Surfaces         []Surface               `json:"surfaces"`
	RequiredSurfaces []Surface               `json:"required_surfaces"`
	DefaultPolicy    string                  `json:"default_policy"`
	Tools            []BuiltinToolDescriptor `json:"tools"`
}

type BuiltinToolDescriptor struct {
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// BuiltinPackageForRuntimeName accepts only a complete, canonical package name.
func BuiltinPackageForRuntimeName(name string) PackageID {
	if name == "kandev" {
		return PackageCore
	}
	id := PackageID(strings.TrimPrefix(name, "kandev-"))
	if id != PackageCore && PackageEligible(id, SurfaceKanbanTask) && name == "kandev-"+string(id) {
		return id
	}
	return ""
}

// ReservedBuiltinRuntimeName is a configuration validation rule, not transport
// provenance. Legacy dotted identities stay reserved alongside exact packages.
func ReservedBuiltinRuntimeName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return BuiltinPackageForRuntimeName(name) != "" || strings.HasPrefix(name, "kandev.")
}

// BuiltinPackageCatalog returns fresh descriptors. Tool eligibility within a
// surface remains constrained by the existing capability and provider registry.
func BuiltinPackageCatalog() []BuiltinPackageDescriptor {
	result := []BuiltinPackageDescriptor{
		{ID: PackageCore, RuntimeName: "kandev", DefaultPolicy: "required", Surfaces: []Surface{SurfaceKanbanTask, SurfaceOfficeTask, SurfaceConfiguration}},
		{ID: PackageConfiguration, RuntimeName: "kandev-configuration", DefaultPolicy: "available", Surfaces: []Surface{SurfaceKanbanTask, SurfaceConfiguration}, RequiredSurfaces: []Surface{SurfaceConfiguration}},
		{ID: PackageRichOutput, RuntimeName: "kandev-rich-output", DefaultPolicy: "default", Surfaces: []Surface{SurfaceKanbanTask, SurfaceOfficeTask}},
		{ID: PackageWalkthroughs, RuntimeName: "kandev-walkthroughs", DefaultPolicy: "default", Surfaces: []Surface{SurfaceKanbanTask}},
		{ID: PackageDiagnostics, RuntimeName: "kandev-diagnostics", DefaultPolicy: "default", Surfaces: []Surface{SurfaceKanbanTask}},
	}
	result[0].RequiredSurfaces = slices.Clone(result[0].Surfaces)
	for i := range result {
		d := &result[i]
		d.LabelKey = "settings:builtinMcp.packages." + string(d.ID) + ".label"
		d.DescriptionKey = "settings:builtinMcp.packages." + string(d.ID) + ".description"
		for name, id := range builtinToolPackages {
			if id == d.ID {
				d.Tools = append(d.Tools, BuiltinToolDescriptor{Name: name, Actions: BuiltinToolActions(name)})
			}
		}
		slices.SortFunc(d.Tools, func(a, b BuiltinToolDescriptor) int { return strings.Compare(a.Name, b.Name) })
	}
	return result
}

// BuiltinToolActions enumerates the backend operations a registered native tool
// needs. This does not replace the operation's resource or parameter checks.
func BuiltinToolActions(name string) []string {
	if BuiltinToolPackage(name) == "" {
		return nil
	}
	if name == "show_rich_output_kandev" {
		return []string{ws.ActionWorkspaceFileContentGet}
	}
	actions := []string{"mcp." + strings.TrimSuffix(name, "_kandev")}
	switch name {
	case "ask_user_question_kandev", "ask_parent_question_kandev":
		actions = append(actions, ws.ActionMCPClarificationTimeout)
	case "transfer_task_kandev":
		actions = append(actions, ws.ActionMCPAuditTaskTransferAttempt)
	}
	return actions
}
