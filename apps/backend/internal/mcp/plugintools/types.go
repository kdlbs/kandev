package plugintools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
)

const (
	SurfaceKanban     = "kanban-task"
	SurfaceOffice     = "office-task"
	SurfaceManaged    = "managed-conversation"
	SurfaceAutomation = "automation"
)

const maxExposedNameLength = 64

const maxAutomationToolNames = 16

// Selection is one saved, exact plugin tool approval for a private automation.
// InstallationID and PolicyVersion bind the selection to the installed plugin
// policy that was displayed when the native editor saved it.
type Selection struct {
	PluginID       string   `json:"plugin_id"`
	InstallationID string   `json:"installation_id"`
	PolicyVersion  string   `json:"policy_version"`
	ToolNames      []string `json:"tool_names"`
}

// OptionTool describes one tool an automation editor may select.
type OptionTool struct {
	LocalName       string          `json:"local_name"`
	Description     string          `json:"description"`
	InputSchema     json.RawMessage `json:"input_schema"`
	OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
	ReadOnlyHint    bool            `json:"read_only_hint"`
	DestructiveHint bool            `json:"destructive_hint"`
	IdempotentHint  bool            `json:"idempotent_hint"`
	OpenWorldHint   bool            `json:"open_world_hint"`
}

// OptionGroup is one installed plugin's current automation-capable tools.
type OptionGroup struct {
	PluginID       string       `json:"plugin_id"`
	DisplayName    string       `json:"display_name"`
	Version        string       `json:"version"`
	InstallationID string       `json:"installation_id"`
	PolicyVersion  string       `json:"policy_version"`
	Tools          []OptionTool `json:"tools"`
}

// AutomationAuthorization is the backend-owned authority attached to one
// automation plugin invocation. MCP arguments never supply these values.
type AutomationAuthorization struct {
	AutomationID   string   `json:"automation_id"`
	WorkspaceID    string   `json:"workspace_id"`
	PluginID       string   `json:"plugin_id"`
	InstallationID string   `json:"installation_id"`
	PolicyVersion  string   `json:"policy_version"`
	ToolNames      []string `json:"tool_names"`
}

// NormalizeSelection sorts and deduplicates plugin entries and tool names so
// two equivalent approvals compare and persist identically.
func NormalizeSelection(selections []Selection) []Selection {
	if len(selections) == 0 {
		return []Selection{}
	}
	out := append([]Selection(nil), selections...)
	for i := range out {
		out[i].ToolNames = uniqueSorted(out[i].ToolNames)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PluginID < out[j].PluginID })
	return uniqueSelections(out)
}

func uniqueSelections(selections []Selection) []Selection {
	out := make([]Selection, 0, len(selections))
	for _, selection := range selections {
		if len(out) > 0 && out[len(out)-1].PluginID == selection.PluginID {
			continue
		}
		out = append(out, selection)
	}
	return out
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// ProjectNonAutomation removes automation surface tags from a snapshot so an
// older retained agentctl that does not know the automation surface can still
// accept the catalog. Tools declared only for automation are omitted.
func ProjectNonAutomation(snapshot Snapshot) Snapshot {
	projected := Snapshot{Generation: snapshot.Generation, Revision: snapshot.Revision}
	projected.Tools = make([]Definition, 0, len(snapshot.Tools))
	for _, definition := range snapshot.Tools {
		surfaces := slices.DeleteFunc(slices.Clone(definition.Surfaces), func(surface string) bool {
			return surface == SurfaceAutomation
		})
		if len(surfaces) == 0 {
			continue
		}
		definition.Surfaces = surfaces
		projected.Tools = append(projected.Tools, definition)
	}
	return projected
}

// Validate checks the stored shape without consulting an installation. The
// automation service additionally checks every entry against current plugin
// options before persistence.
func (s Selection) Validate() error {
	if strings.TrimSpace(s.PluginID) != s.PluginID || s.PluginID == "" ||
		strings.TrimSpace(s.InstallationID) != s.InstallationID || s.InstallationID == "" ||
		strings.TrimSpace(s.PolicyVersion) != s.PolicyVersion || s.PolicyVersion == "" {
		return fmt.Errorf("plugin tool selection identity is incomplete")
	}
	if strings.ContainsAny(s.PluginID+s.InstallationID+s.PolicyVersion, "\x00") {
		return fmt.Errorf("plugin tool selection identity is invalid")
	}
	if len(s.ToolNames) == 0 || len(s.ToolNames) > maxAutomationToolNames {
		return fmt.Errorf("plugin tool selection for %q must name between 1 and %d tools", s.PluginID, maxAutomationToolNames)
	}
	seen := make(map[string]struct{}, len(s.ToolNames))
	for _, name := range s.ToolNames {
		if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\x00") {
			return fmt.Errorf("plugin tool selection for %q contains an invalid tool name", s.PluginID)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("plugin tool selection for %q duplicates tool name %q", s.PluginID, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// ValidateAutomationAuthorization rejects incomplete invocation authority.
func (a AutomationAuthorization) Validate() error {
	if a.AutomationID == "" || a.WorkspaceID == "" || a.PluginID == "" || a.InstallationID == "" || a.PolicyVersion == "" {
		return fmt.Errorf("automation tool authorization is incomplete")
	}
	if len(a.ToolNames) == 0 || len(a.ToolNames) > maxAutomationToolNames {
		return fmt.Errorf("automation tool authorization names too many tools")
	}
	return nil
}

// Allows reports whether one local tool is inside this exact approval.
func (a AutomationAuthorization) Allows(pluginID, localName string) bool {
	if pluginID != a.PluginID || localName == "" {
		return false
	}
	return slices.Contains(a.ToolNames, localName)
}

// Definition is the backend-owned descriptor transported to agentctl. Schemas
// are raw JSON so agentctl can register them without reinterpreting YAML.
type Definition struct {
	PluginID          string          `json:"plugin_id"`
	PluginDisplayName string          `json:"plugin_display_name,omitempty"`
	LocalName         string          `json:"local_name"`
	ExposedName       string          `json:"exposed_name"`
	Description       string          `json:"description"`
	Surfaces          []string        `json:"surfaces"`
	InputSchema       json.RawMessage `json:"input_schema"`
	OutputSchema      json.RawMessage `json:"output_schema,omitempty"`
	ReadOnlyHint      bool            `json:"read_only_hint"`
	DestructiveHint   bool            `json:"destructive_hint"`
	IdempotentHint    bool            `json:"idempotent_hint"`
	OpenWorldHint     bool            `json:"open_world_hint"`
}

type Snapshot struct {
	Generation string       `json:"generation"`
	Revision   uint64       `json:"revision"`
	Tools      []Definition `json:"tools"`
}

func ExposedName(pluginID, localName string) string {
	slug := pluginIDSlug(pluginID)
	readable := "kandev_" + slug + "_" + localName
	if len(readable) <= maxExposedNameLength {
		return readable
	}

	// Keep long names mostly readable, using a short stable suffix only when
	// the provider-safe length budget requires disambiguation.
	digest := sha256.Sum256([]byte(pluginID))
	suffix := "_" + hex.EncodeToString(digest[:4])
	available := maxExposedNameLength - len("kandev__") - len(localName) - len(suffix)
	if available < 1 {
		available = 1
	}
	if len(slug) > available {
		slug = slug[:available]
	}
	return "kandev_" + slug + "_" + localName + suffix
}

func pluginIDSlug(pluginID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(pluginID) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune('_')
		} else {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func Normalize(snapshot Snapshot) Snapshot {
	snapshot.Tools = append([]Definition(nil), snapshot.Tools...)
	for i := range snapshot.Tools {
		snapshot.Tools[i].Surfaces = append([]string(nil), snapshot.Tools[i].Surfaces...)
		sort.Strings(snapshot.Tools[i].Surfaces)
	}
	sort.Slice(snapshot.Tools, func(i, j int) bool {
		return snapshot.Tools[i].ExposedName < snapshot.Tools[j].ExposedName
	})
	return snapshot
}

func Equal(left, right Snapshot) bool {
	l, r := Normalize(left), Normalize(right)
	bl, _ := json.Marshal(l)
	br, _ := json.Marshal(r)
	return string(bl) == string(br)
}
