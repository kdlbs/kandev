// Package profile defines the backend-owned MCP tool profile contract.
// Profiles describe a small base surface plus additive capability groups. An
// agent can receive a profile, but it cannot request arbitrary tool names.
package profile

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/common/mcpmode"
)

const ManagedToolPolicyMetadataKey = "kandev.managed_tool_policy"

type Surface string

const (
	SurfaceKanbanTask          Surface = "kanban-task"
	SurfaceOfficeTask          Surface = "office-task"
	SurfaceConfiguration       Surface = "configuration"
	SurfaceExternal            Surface = "external"
	SurfaceAutomation          Surface = "automation"
	SurfaceManagedConversation Surface = "managed-conversation"
)

type Capability string

const (
	CapabilityUserQuestion   Capability = "user-question"
	CapabilityParentQuestion Capability = "parent-question"
	CapabilityTaskTitle      Capability = "task-title"
	CapabilityGitHubPR       Capability = "github-pr"
	CapabilityGitLabMR       Capability = "gitlab-mr"
	// CapabilityWorkspaceTaskTreeRead permits an attested Office Coordinator
	// to inspect compact relation trees for unrelated tasks in its workspace.
	// It never grants document or verbose-description access.
	CapabilityWorkspaceTaskTreeRead Capability = "workspace-task-tree-read"
	CapabilityCanvas                Capability = "canvas-authoring"
)

// Context is the complete, backend-resolved MCP profile for one agent
// instance. Surface selects the base tool set. Capabilities add small
// context-specific groups. Providers add provider-specific automation tools.
type Context struct {
	Surface           Surface            `json:"surface"`
	Capabilities      []Capability       `json:"capabilities,omitempty"`
	Providers         []string           `json:"providers,omitempty"`
	ManagedToolPolicy *ManagedToolPolicy `json:"managed_tool_policy,omitempty"`
}

// ManagedToolPolicy is the backend-resolved authority for one retained plugin
// conversation. Agent-controlled MCP arguments never supply this value.
type ManagedToolPolicy struct {
	PluginID             string   `json:"plugin_id"`
	InstallationID       string   `json:"installation_id"`
	WorkspaceID          string   `json:"workspace_id"`
	InstanceKey          string   `json:"instance_key"`
	ConversationRevision uint64   `json:"conversation_revision"`
	ApprovalRevision     uint64   `json:"approval_revision"`
	ManifestDigest       string   `json:"manifest_digest"`
	AgentToolNames       []string `json:"agent_tool_names,omitempty"`
}

var managedToolNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,31}$`)

// Validate rejects incomplete or ambiguous managed tool grants before they
// cross the runtime boundary.
func (p ManagedToolPolicy) Validate() error {
	if p.PluginID == "" || p.InstallationID == "" || p.WorkspaceID == "" || p.InstanceKey == "" ||
		strings.TrimSpace(p.PluginID) != p.PluginID || strings.TrimSpace(p.InstallationID) != p.InstallationID ||
		strings.TrimSpace(p.WorkspaceID) != p.WorkspaceID || strings.TrimSpace(p.InstanceKey) != p.InstanceKey ||
		strings.ContainsAny(p.PluginID+p.InstallationID+p.WorkspaceID+p.InstanceKey, "\x00") ||
		p.ConversationRevision == 0 || p.ApprovalRevision == 0 || len(p.ManifestDigest) != 64 ||
		p.ManifestDigest != strings.ToLower(p.ManifestDigest) {
		return errors.New("managed tool policy identity is incomplete")
	}
	if _, err := hex.DecodeString(p.ManifestDigest); err != nil {
		return errors.New("managed tool policy manifest digest is invalid")
	}
	return ValidateManagedToolNames(p.AgentToolNames)
}

// ValidateManagedToolNames checks the bounded local names accepted by the
// managed-conversation tool allowlist.
func ValidateManagedToolNames(names []string) error {
	seen := make(map[string]struct{}, len(names))
	if len(names) > 16 {
		return errors.New("managed tool policy declares too many tools")
	}
	for _, name := range names {
		if !managedToolNamePattern.MatchString(name) {
			return fmt.Errorf("managed tool policy contains invalid tool name %q", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("managed tool policy duplicates tool name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// Allows reports whether a tool belongs to the exact plugin-scoped selection.
func (p ManagedToolPolicy) Allows(pluginID, localName string) bool {
	if pluginID != p.PluginID || localName == "" {
		return false
	}
	return slices.Contains(p.AgentToolNames, localName)
}

// MarshalManagedToolPolicy returns the canonical JSON value stored with a
// retained launch and revalidated on recovery.
func MarshalManagedToolPolicy(policy ManagedToolPolicy) (string, error) {
	if err := policy.Validate(); err != nil {
		return "", err
	}
	canonical := policy
	canonical.AgentToolNames = slices.Clone(policy.AgentToolNames)
	slices.Sort(canonical.AgentToolNames)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ParseManagedToolPolicyMetadata parses either its persisted JSON form or a
// map produced by a JSON persistence round trip. It rejects malformed stored
// authority instead of silently treating the launch as unrestricted.
func ParseManagedToolPolicyMetadata(value any) (*ManagedToolPolicy, error) {
	var encoded []byte
	switch typed := value.(type) {
	case string:
		encoded = []byte(typed)
	case map[string]any:
		var err error
		encoded, err = json.Marshal(typed)
		if err != nil {
			return nil, errors.New("managed tool policy metadata is invalid")
		}
	default:
		return nil, errors.New("managed tool policy metadata is missing")
	}
	var policy ManagedToolPolicy
	if err := json.Unmarshal(encoded, &policy); err != nil {
		return nil, errors.New("managed tool policy metadata is invalid")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	policy.AgentToolNames = slices.Clone(policy.AgentToolNames)
	slices.Sort(policy.AgentToolNames)
	return &policy, nil
}

// WithManagedToolPolicy marks this profile as restricted and clones the
// selected tool names so callers cannot mutate the source policy later.
func (c Context) WithManagedToolPolicy(policy ManagedToolPolicy) Context {
	copy := policy
	copy.AgentToolNames = slices.Clone(policy.AgentToolNames)
	slices.Sort(copy.AgentToolNames)
	c.Surface = SurfaceManagedConversation
	c.ManagedToolPolicy = &copy
	return c
}

// Normalize preserves the managed policy while applying the ordinary profile
// canonicalization rules.
func Normalize(c Context) Context {
	normalized := New(c.Surface, c.Capabilities, c.Providers)
	if c.ManagedToolPolicy != nil {
		policy := *c.ManagedToolPolicy
		policy.AgentToolNames = slices.Clone(c.ManagedToolPolicy.AgentToolNames)
		slices.Sort(policy.AgentToolNames)
		normalized.Surface = SurfaceManagedConversation
		normalized.ManagedToolPolicy = &policy
	}
	return normalized
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
	case SurfaceKanbanTask, SurfaceOfficeTask, SurfaceConfiguration, SurfaceExternal, SurfaceAutomation, SurfaceManagedConversation:
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
