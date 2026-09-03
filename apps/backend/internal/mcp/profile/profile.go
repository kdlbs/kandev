// Package profile defines the backend-owned MCP tool profile contract.
// Profiles describe a small base surface plus additive capability groups. An
// agent can receive a profile, but it cannot request arbitrary tool names.
package profile

import (
	"slices"

	"github.com/kandev/kandev/internal/common/mcpmode"
)

type Surface string

const (
	SurfaceKanbanTask    Surface = "kanban-task"
	SurfaceOfficeTask    Surface = "office-task"
	SurfaceConfiguration Surface = "configuration"
	SurfaceExternal      Surface = "external"
	SurfaceAutomation    Surface = "automation"
)

type Capability string

const (
	CapabilityUserQuestion   Capability = "user-question"
	CapabilityParentQuestion Capability = "parent-question"
	CapabilityTaskTitle      Capability = "task-title"
	CapabilityGitHubPR       Capability = "github-pr"
	CapabilityGitLabMR       Capability = "gitlab-mr"
)

// Context is the complete, backend-resolved MCP profile for one agent
// instance. Surface selects the base tool set. Capabilities add small
// context-specific groups. Providers add provider-specific automation tools.
type Context struct {
	Surface      Surface      `json:"surface"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	Providers    []string     `json:"providers,omitempty"`
}

func New(surface Surface, capabilities []Capability, providers []string) Context {
	ctx := Context{Surface: normalizeSurface(surface)}
	for _, capability := range capabilities {
		ctx = ctx.WithCapability(capability)
	}
	ctx.Providers = normalizeStrings(providers)
	return ctx
}

// NewAutomation returns the fixed profile used by automation task sessions.
// Automation tasks coordinate workspace work and never receive task-local
// question capabilities.
func NewAutomation() Context {
	return New(SurfaceAutomation, nil, nil)
}

func (c Context) HasCapability(capability Capability) bool {
	return slices.Contains(c.Capabilities, capability)
}

func (c Context) WithCapability(capability Capability) Context {
	if capability == "" || c.HasCapability(capability) {
		return c
	}
	c.Capabilities = append(c.Capabilities, capability)
	return c
}

func (c Context) WithoutCapability(capability Capability) Context {
	// Clone before filtering so changing a derived profile never overwrites the
	// backing array owned by the source profile.
	capabilities := slices.Clone(c.Capabilities)
	capabilities = slices.DeleteFunc(capabilities, func(value Capability) bool { return value == capability })
	c.Capabilities = capabilities
	return c
}

// Legacy maps the current mode/boolean API to the typed profile contract.
// Keep this adapter while older runtime callers migrate to Context.
func Legacy(mode string, disableAskQuestion bool, providers []string) Context {
	surface := SurfaceKanbanTask
	capabilities := []Capability{}
	switch mode {
	case mcpmode.Office:
		surface = SurfaceOfficeTask
	case mcpmode.Config:
		surface = SurfaceConfiguration
	case mcpmode.External:
		surface = SurfaceExternal
	case mcpmode.Automation:
		surface = SurfaceAutomation
	case mcpmode.TaskTitlePending:
		capabilities = append(capabilities, CapabilityTaskTitle)
	}
	if !disableAskQuestion && surface != SurfaceExternal && surface != SurfaceAutomation {
		capabilities = append(capabilities, CapabilityUserQuestion)
	}
	return New(surface, capabilities, providers)
}

func normalizeSurface(surface Surface) Surface {
	switch surface {
	case SurfaceKanbanTask, SurfaceOfficeTask, SurfaceConfiguration, SurfaceExternal, SurfaceAutomation:
		return surface
	default:
		return SurfaceKanbanTask
	}
}

func normalizeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
