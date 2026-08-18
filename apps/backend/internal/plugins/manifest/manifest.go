// Package manifest defines the plugin manifest data model and validation
// rules described in docs/specs/plugins/spec.md ("Data model / Plugin
// registration"). A manifest is the YAML document an operator supplies to
// POST /api/plugins/register.
package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// MaxActionBodyBytes is the largest request body an authenticated action may
// declare. Individual actions must still declare their own smaller bound.
const MaxActionBodyBytes = 1 << 20

const (
	// LegacyAPIVersion preserves the original webhook behavior: an omitted
	// access field means public.
	LegacyAPIVersion = 1
	// CurrentAPIVersion makes omitted webhook access authenticated. Authors can
	// still opt individual integration callbacks into public access.
	CurrentAPIVersion = 2
)

// Manifest is the plugin registration manifest.
type Manifest struct {
	ID          string   `yaml:"id" json:"id"`
	APIVersion  int      `yaml:"api_version" json:"api_version"`
	Version     string   `yaml:"version" json:"version"`
	DisplayName string   `yaml:"display_name" json:"display_name"`
	Description string   `yaml:"description" json:"description"`
	Author      string   `yaml:"author" json:"author"`
	Categories  []string `yaml:"categories" json:"categories"`

	// Icon is an optional package-relative path (e.g. "icon.svg") to an
	// image the plugin ships for display in the marketplace and plugin
	// lists. The registry index-build resolves it to an absolute icon_url;
	// for an installed plugin it is served from the extracted package.
	Icon string `yaml:"icon,omitempty" json:"icon,omitempty"`

	// RepoURL is an optional URL to the plugin's source repository (e.g.
	// "https://github.com/owner/plugin"). kandev renders it as a "Repo" link
	// in the installed-plugin list and detail. Unlike the marketplace
	// catalog's repo_url — which the registry index-build derives from
	// plugins.yaml — this is author-declared in the manifest, so sideloaded
	// and directly-installed plugins carry the link too. When set it must be
	// an http(s) URL (see Validate); the frontend href guard is enforced here
	// as well so a bad scheme fails registration rather than reaching the UI.
	RepoURL string `yaml:"repo_url,omitempty" json:"repo_url,omitempty"`

	BaseURL string `yaml:"base_url" json:"base_url"`

	Endpoints    Endpoints    `yaml:"endpoints" json:"endpoints"`
	Capabilities Capabilities `yaml:"capabilities" json:"capabilities"`

	Webhooks []Webhook `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
	Actions  []Action  `yaml:"actions,omitempty" json:"actions,omitempty"`
	// RepositoryProviders declares provider IDs this plugin owns while active.
	// Ownership is checked across active plugins by the runtime registry.
	RepositoryProviders []string `yaml:"repository_providers,omitempty" json:"repository_providers,omitempty"`
	// ReferenceSources declares composer sources this plugin owns while active.
	ReferenceSources []ReferenceSource `yaml:"reference_sources,omitempty" json:"reference_sources,omitempty"`

	// AuthProviders declares external login options (OIDC/SAML) this plugin
	// contributes to the pre-auth login screen. Only meaningful alongside
	// capabilities.auth. Each provider's Initiate names one of the plugin's
	// webhook keys — the login button navigates to that webhook, which
	// 302-redirects the browser to the IdP.
	AuthProviders []AuthProvider `yaml:"auth_providers,omitempty" json:"auth_providers,omitempty"`

	ConfigSchema map[string]any `yaml:"config_schema,omitempty" json:"config_schema,omitempty"`
	AgentTools   []AgentTool    `yaml:"agent_tools,omitempty" json:"agent_tools,omitempty"`

	UI UISection `yaml:"ui,omitempty" json:"ui,omitempty"`

	Runtime          Runtime `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	MinKandevVersion string  `yaml:"min_kandev_version,omitempty" json:"min_kandev_version,omitempty"`
}

const (
	AgentToolSurfaceKanban = "kanban-task"
	AgentToolSurfaceOffice = "office-task"
)

// AgentTool is an MCP tool a plugin contributes to matching task sessions.
// The exposed MCP name is derived by the host and is not author-controlled.
type AgentTool struct {
	Name         string               `yaml:"name" json:"name"`
	Description  string               `yaml:"description" json:"description"`
	Surfaces     []string             `yaml:"surfaces" json:"surfaces"`
	InputSchema  map[string]any       `yaml:"input_schema" json:"input_schema"`
	OutputSchema map[string]any       `yaml:"output_schema,omitempty" json:"output_schema,omitempty"`
	Annotations  AgentToolAnnotations `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// AgentToolAnnotations are MCP hints. Nil values preserve omission so the
// runtime descriptor can apply conservative defaults explicitly.
type AgentToolAnnotations struct {
	ReadOnlyHint    *bool `yaml:"read_only_hint,omitempty" json:"read_only_hint,omitempty"`
	DestructiveHint *bool `yaml:"destructive_hint,omitempty" json:"destructive_hint,omitempty"`
	IdempotentHint  *bool `yaml:"idempotent_hint,omitempty" json:"idempotent_hint,omitempty"`
	OpenWorldHint   *bool `yaml:"open_world_hint,omitempty" json:"open_world_hint,omitempty"`
}

// Runtime declares that a plugin ships a kandev-managed binary rather than
// registering an already-running remote service. Type is currently only
// ever "binary"; kandev spawns the matching Executables entry as a
// subprocess. Executables maps "<goos>-<goarch>" (e.g. "linux-amd64",
// "darwin-arm64", "windows-amd64") to a package-relative path under
// server/ (Windows values already include the ".exe" suffix).
type Runtime struct {
	Type        string            `yaml:"type,omitempty" json:"type,omitempty"`
	Executables map[string]string `yaml:"executables,omitempty" json:"executables,omitempty"`
}

// Endpoints declares the HTTP paths kandev calls on the plugin.
type Endpoints struct {
	Health   string `yaml:"health" json:"health"`
	Events   string `yaml:"events" json:"events"`
	Webhooks string `yaml:"webhooks" json:"webhooks"`
}

// Capabilities declares what the plugin is allowed to do.
type Capabilities struct {
	Events   []string `yaml:"events,omitempty" json:"events,omitempty"`
	APIRead  []string `yaml:"api_read,omitempty" json:"api_read,omitempty"`
	APIWrite []string `yaml:"api_write,omitempty" json:"api_write,omitempty"`
	State    bool     `yaml:"state,omitempty" json:"state,omitempty"`
	Secrets  bool     `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	// AgentInvoke gates Host.InvokeUtilityAgent (ADR 0048): a one-shot,
	// non-interactive completion run by the operator-configured utility agent.
	AgentInvoke bool `yaml:"agent_invoke,omitempty" json:"agent_invoke,omitempty"`
	// Auth gates a plugin's ability to establish an authenticated kandev
	// browser session for an external identity it has validated against an
	// OIDC/SAML IdP. The plugin asserts the identity via the reserved login
	// directive header on its webhook response; the host mints and sets the
	// session cookie itself, so the plugin never receives the raw session
	// token. This is the highest-privilege capability — a plugin that holds it
	// can log a visitor in as any (or a new) user — so it should only be
	// granted to plugins an operator explicitly trusts.
	Auth bool `yaml:"auth,omitempty" json:"auth,omitempty"`
	// UserState gates the authenticated, browser-reachable per-user storage
	// routes (PUT/GET/DELETE /api/plugins/:id/user-state/...). Unlike State
	// (plugin_state, written by the plugin's own gRPC-connected backend),
	// this lets a UI-only plugin bundle persist data scoped to the calling
	// browser session's user with no Go backend of its own (Approach D1,
	// docs/decisions/2026-08-01-per-user-plugin-storage.md).
	UserState bool `yaml:"user_state,omitempty" json:"user_state,omitempty"`
}

// AuthProvider is a login option a plugin contributes to the pre-auth login
// screen. ID is a stable provider key; DisplayName is the button label;
// Initiate is the plugin webhook key that starts the login redirect.
type AuthProvider struct {
	ID          string `yaml:"id" json:"id"`
	DisplayName string `yaml:"display_name" json:"display_name"`
	Initiate    string `yaml:"initiate" json:"initiate"`
}

// Webhook is a proxied external webhook endpoint the plugin declares. The
// manifest api_version selects the default when Access is omitted: v1 remains
// public for compatibility, while v2 defaults to authenticated.
type Webhook struct {
	Key          string `yaml:"key" json:"key"`
	Description  string `yaml:"description,omitempty" json:"description,omitempty"`
	Method       string `yaml:"method,omitempty" json:"method,omitempty"`
	Access       string `yaml:"access,omitempty" json:"access,omitempty"`
	MaxBodyBytes int64  `yaml:"max_body_bytes,omitempty" json:"max_body_bytes,omitempty"`
}

const (
	WebhookAccessPublic              = "public"
	WebhookAccessAuthenticated       = "authenticated"
	DefaultWebhookMaxBodyBytes int64 = 4 << 20
	MaximumWebhookMaxBodyBytes int64 = 16 << 20
)

func (w Webhook) EffectiveAccess(apiVersion int) string {
	if w.Access == "" {
		if apiVersion == LegacyAPIVersion {
			return WebhookAccessPublic
		}
		return WebhookAccessAuthenticated
	}
	return w.Access
}

func (w Webhook) EffectiveMaxBodyBytes() int64 {
	if w.MaxBodyBytes == 0 {
		return DefaultWebhookMaxBodyBytes
	}
	return w.MaxBodyBytes
}

// Action is an authenticated browser-to-plugin operation. Unlike Webhook,
// actions always pass through Kandev's normal request authentication and
// resource authorization before reaching the plugin.
type Action struct {
	Key           string `yaml:"key" json:"key"`
	ResourceScope string `yaml:"scope" json:"scope"`
	MaxBodyBytes  int    `yaml:"max_body_bytes" json:"max_body_bytes"`
}

const (
	ActionScopeWorkspace  = "workspace"
	ActionScopeTask       = "task"
	ActionScopeRepository = "repository"
)

// UnmarshalYAML accepts the frozen manifest field name (scope) and the
// pre-release resource_scope spelling so stored local plugin records remain
// readable through the contract correction. Marshal paths emit scope only.
func (a *Action) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Key                 string `yaml:"key"`
		Scope               string `yaml:"scope"`
		LegacyResourceScope string `yaml:"resource_scope"`
		MaxBodyBytes        int    `yaml:"max_body_bytes"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.Scope != "" && raw.LegacyResourceScope != "" && raw.Scope != raw.LegacyResourceScope {
		return fmt.Errorf("action scope and resource_scope disagree")
	}
	a.Key = raw.Key
	a.ResourceScope = raw.Scope
	if a.ResourceScope == "" {
		a.ResourceScope = raw.LegacyResourceScope
	}
	a.MaxBodyBytes = raw.MaxBodyBytes
	return nil
}

// ReferenceSource describes one plugin-owned entity-reference source. The
// host owns canonical reference construction and validates every use through
// the live plugin; these fields describe presentation and routing only.
type ReferenceSource struct {
	Source      string `yaml:"source" json:"source"`
	Provider    string `yaml:"provider" json:"provider"`
	Kind        string `yaml:"kind" json:"kind"`
	DisplayName string `yaml:"display_name" json:"display_name"`
	KindLabel   string `yaml:"kind_label" json:"kind_label"`
	Order       int    `yaml:"order,omitempty" json:"order,omitempty"`
}

// UISection declares UI pages the plugin contributes, and/or a native UI
// bundle. Bundle is a root-relative path on the plugin process serving an ES
// module (e.g. "/ui/bundle.js"), fetched by kandev via
// /api/plugins/{id}/bundle and proxied to the plugin. Styles are optional
// root-relative CSS paths served alongside the bundle. Pages remain
// optional/secondary when a bundle is declared: native routes come from the
// bundle at runtime.
type UISection struct {
	Pages       []UIPage       `yaml:"pages,omitempty" json:"pages,omitempty"`
	Bundle      string         `yaml:"bundle,omitempty" json:"bundle,omitempty"`
	Styles      []string       `yaml:"styles,omitempty" json:"styles,omitempty"`
	Keybindings []UIKeybinding `yaml:"keybindings,omitempty" json:"keybindings,omitempty"`
}

// UIPage is a single UI page contributed by the plugin.
type UIPage struct {
	Key     string `yaml:"key" json:"key"`
	Title   string `yaml:"title" json:"title"`
	Path    string `yaml:"path" json:"path"`
	Surface string `yaml:"surface" json:"surface"`
}

// UIKeybinding is a single keyboard shortcut a plugin's UI bundle wants
// kandev to register and dispatch on its behalf. ID is plugin-local (unique
// within the plugin, not globally); Default is a combo like "mod+shift+k"
// (see parseKeybindingCombo in validate.go for the accepted grammar).
// Keybindings are only meaningful alongside ui.bundle, since the plugin's JS
// bundle is what handles the dispatched event.
type UIKeybinding struct {
	ID          string `yaml:"id" json:"id"`
	Default     string `yaml:"default" json:"default"`
	Description string `yaml:"description" json:"description"`
	// AllowInEditor opts this one binding out of the dispatcher's
	// skip-while-typing rule. By default a plugin keybinding does not fire
	// when the keydown target is an input, textarea or contenteditable, so a
	// plugin cannot shadow a character the user is trying to type. A binding
	// whose whole purpose is to act on the focused composer (dictation,
	// for instance) has to run there, and declares it here.
	AllowInEditor bool `yaml:"allow_in_editor,omitempty" json:"allow_in_editor,omitempty"`
}

// Parse decodes a plugin manifest from YAML bytes. It does not validate the
// result; callers should call (*Manifest).Validate() afterward.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
