// Package manifest defines the plugin manifest data model and validation
// rules described in docs/specs/plugins/spec.md ("Data model / Plugin
// registration"). A manifest is the YAML document an operator supplies to
// POST /api/plugins/register.
package manifest

import "gopkg.in/yaml.v3"

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

	ConfigSchema map[string]any `yaml:"config_schema,omitempty" json:"config_schema,omitempty"`

	UI UISection `yaml:"ui,omitempty" json:"ui,omitempty"`

	Runtime          Runtime `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	MinKandevVersion string  `yaml:"min_kandev_version,omitempty" json:"min_kandev_version,omitempty"`
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
}

// Webhook is a proxied external webhook endpoint the plugin declares.
type Webhook struct {
	Key         string `yaml:"key" json:"key"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Method      string `yaml:"method,omitempty" json:"method,omitempty"`
}

// UISection declares UI pages the plugin contributes, and/or a native UI
// bundle. Bundle is a root-relative path on the plugin process serving an ES
// module (e.g. "/ui/bundle.js"), fetched by kandev via
// /api/plugins/{id}/bundle and proxied to the plugin. Styles are optional
// root-relative CSS paths served alongside the bundle. Pages remain
// optional/secondary when a bundle is declared: native routes come from the
// bundle at runtime.
type UISection struct {
	Pages  []UIPage `yaml:"pages,omitempty" json:"pages,omitempty"`
	Bundle string   `yaml:"bundle,omitempty" json:"bundle,omitempty"`
	Styles []string `yaml:"styles,omitempty" json:"styles,omitempty"`
}

// UIPage is a single UI page contributed by the plugin.
type UIPage struct {
	Key     string `yaml:"key" json:"key"`
	Title   string `yaml:"title" json:"title"`
	Path    string `yaml:"path" json:"path"`
	Surface string `yaml:"surface" json:"surface"`
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
