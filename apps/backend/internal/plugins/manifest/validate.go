package manifest

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// idPattern matches the required plugin id shape: lowercase alphanumerics,
// dots, underscores, and hyphens, starting with a lowercase alphanumeric.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// supportedAPIVersion is the only api_version this kandev build accepts.
const supportedAPIVersion = 1

// validCategories are the allowed values for Manifest.Categories entries.
var validCategories = map[string]bool{
	"connector":  true,
	"automation": true,
	"tools":      true,
	"analytics":  true,
}

// validUISurfaces are the allowed values for UIPage.Surface.
var validUISurfaces = map[string]bool{
	"settings":   true,
	"task-panel": true,
	"main-nav":   true,
}

// keybindingIDPattern matches the required UIKeybinding.ID shape: lowercase
// alphanumerics and hyphens, starting with a lowercase letter or digit. IDs
// are plugin-local (unique within the plugin's own keybindings list, not
// globally), so this is deliberately simpler than idPattern.
var keybindingIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// keybindingModifiers are the accepted modifier tokens in a keybinding combo
// (case-insensitive; matched after lowercasing). "mod" is the
// platform-agnostic primary modifier (Cmd on macOS, Ctrl elsewhere) that the
// frontend dispatcher resolves at runtime; the rest are explicit aliases.
var keybindingModifiers = map[string]bool{
	"mod":     true,
	"ctrl":    true,
	"control": true,
	"cmd":     true,
	"meta":    true,
	"super":   true,
	"alt":     true,
	"option":  true,
	"shift":   true,
}

// keybindingKeys are the accepted non-modifier key tokens in a keybinding
// combo (case-insensitive; matched after lowercasing). This is a
// conservative, documented vocabulary — not an exhaustive KeyboardEvent.key
// mirror — covering: single alphanumeric characters, function keys F1-F12,
// and the common named keys plugins are expected to bind.
var keybindingKeys = buildKeybindingKeySet()

func buildKeybindingKeySet() map[string]bool {
	keys := map[string]bool{
		"enter": true, "escape": true, "esc": true, "tab": true, "space": true,
		"backspace": true, "delete": true, "insert": true,
		"arrowup": true, "arrowdown": true, "arrowleft": true, "arrowright": true,
		"up": true, "down": true, "left": true, "right": true,
		"home": true, "end": true, "pageup": true, "pagedown": true,
		"comma": true, "period": true, "slash": true, "backslash": true,
		"semicolon": true, "quote": true, "minus": true, "equal": true,
		"bracketleft": true, "bracketright": true, "backquote": true,
	}
	for c := 'a'; c <= 'z'; c++ {
		keys[string(c)] = true
	}
	for c := '0'; c <= '9'; c++ {
		keys[string(c)] = true
	}
	for i := 1; i <= 12; i++ {
		keys[fmt.Sprintf("f%d", i)] = true
	}
	return keys
}

// Validate checks the manifest against the plugin registration rules
// described in docs/specs/plugins/spec.md. It returns nil if the manifest
// is well-formed, or a joined error describing every violation found.
func (m *Manifest) Validate() error {
	var errs []error
	errs = append(errs, m.validateIdentity()...)
	errs = append(errs, m.validateVersion()...)
	errs = append(errs, m.validateRuntimeType()...)
	if m.IsManaged() {
		errs = append(errs, m.validateManagedRuntime()...)
	} else {
		errs = append(errs, m.validateEndpoints()...)
	}
	errs = append(errs, m.validateCategories()...)
	errs = append(errs, m.validateRepoURL()...)
	errs = append(errs, m.validateUIPages()...)
	errs = append(errs, m.validateUIBundle()...)
	errs = append(errs, m.validateUIKeybindings()...)
	errs = append(errs, m.validateWebhooks()...)
	return errors.Join(errs...)
}

// dotConfigSuffix mirrors store.dotConfigSuffix: an id ending in ".config"
// would make its "<id>.yml" record filename collide with FSStore's
// "<id>.config.yml" operator-config naming convention (store.isRecordFile),
// silently hiding the record from FSStore.List(). Rejected here too so
// registration fails fast instead of relying solely on the store-level
// guard.
const dotConfigSuffix = ".config"

// validateIdentity checks id pattern and api_version.
func (m *Manifest) validateIdentity() []error {
	var errs []error
	if !idPattern.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("invalid plugin id %q: must match %s", m.ID, idPattern.String()))
	} else if strings.HasSuffix(m.ID, dotConfigSuffix) {
		errs = append(errs, fmt.Errorf("invalid plugin id %q: must not end in %q", m.ID, dotConfigSuffix))
	}
	if m.APIVersion != supportedAPIVersion {
		errs = append(errs, fmt.Errorf("unsupported api_version %d: only %d is supported", m.APIVersion, supportedAPIVersion))
	}
	return errs
}

// validateVersion checks that version is a non-empty, path-safe single path
// segment: it is used as a filesystem directory name both at install time
// (pkgtar.extractPackage joins destRoot/<id>/<version>) and to resolve an
// already-installed plugin's data on disk, so an empty, "."/".."-only, or
// separator-containing value would either produce a confusing deep failure
// (securejoin does reject traversal, but only after a misleading top-level
// error) or silently collide with another version's directory.
func (m *Manifest) validateVersion() []error {
	v := m.Version
	if v == "" {
		return []error{errors.New("version must not be empty")}
	}
	if strings.TrimSpace(v) != v || strings.ContainsAny(v, " \t\n\r") {
		return []error{fmt.Errorf("version %q must not contain whitespace", v)}
	}
	if strings.ContainsAny(v, "/\\") {
		return []error{fmt.Errorf("version %q must be a single path segment (no \"/\" or \"\\\")", v)}
	}
	if v == "." || v == ".." {
		return []error{fmt.Errorf("version %q must not be %q or %q", v, ".", "..")}
	}
	return nil
}

// validateRuntimeType checks that runtime.type, when set, is the only
// currently supported value ("binary"). An empty runtime.type is valid: it
// means the manifest is legacy-remote (base_url/endpoints).
func (m *Manifest) validateRuntimeType() []error {
	if m.Runtime.Type != "" && m.Runtime.Type != runtimeTypeBinary {
		return []error{fmt.Errorf("runtime.type %q is invalid: only %q is supported", m.Runtime.Type, runtimeTypeBinary)}
	}
	return nil
}

// validateManagedRuntime checks the rules that apply to a runtime-managed
// manifest (runtime.type: binary): at least one executables entry, every
// entry a clean package-relative path, and base_url/endpoints absent since
// a managed plugin is spawned by kandev rather than registered remotely.
func (m *Manifest) validateManagedRuntime() []error {
	var errs []error
	if len(m.Runtime.Executables) == 0 {
		errs = append(errs, errors.New("runtime.executables must declare at least one entry when runtime.type is \"binary\""))
	}
	for platformKey, execPath := range m.Runtime.Executables {
		if err := validateRelativePackagePath(execPath); err != nil {
			errs = append(errs, fmt.Errorf("runtime.executables[%q]: %w", platformKey, err))
		}
	}
	if m.BaseURL != "" {
		errs = append(errs, errors.New("base_url must be empty for a runtime-managed plugin (runtime.type: binary)"))
	}
	if m.Endpoints != (Endpoints{}) {
		errs = append(errs, errors.New("endpoints must be empty for a runtime-managed plugin (runtime.type: binary)"))
	}
	return errs
}

// validateRelativePackagePath checks that p is a non-empty, clean,
// package-relative path: no leading "/" and no ".." segment.
func validateRelativePackagePath(p string) error {
	if p == "" {
		return errors.New("path must not be empty")
	}
	if path.IsAbs(p) {
		return fmt.Errorf("path %q must be relative", p)
	}
	cleaned := path.Clean(p)
	if cleaned != p || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path %q must be a clean relative path with no \"..\" segments", p)
	}
	return nil
}

// validateEndpoints checks base_url and the required endpoint paths.
func (m *Manifest) validateEndpoints() []error {
	var errs []error
	if m.BaseURL == "" {
		errs = append(errs, errors.New("base_url is required"))
	}
	if m.Endpoints.Health == "" {
		errs = append(errs, errors.New("endpoints.health is required"))
	}
	if m.Endpoints.Events == "" {
		errs = append(errs, errors.New("endpoints.events is required"))
	}
	if m.Endpoints.Webhooks == "" {
		errs = append(errs, errors.New("endpoints.webhooks is required"))
	}
	return errs
}

// validateCategories checks each category against the known enum.
func (m *Manifest) validateCategories() []error {
	var errs []error
	for _, c := range m.Categories {
		if !validCategories[c] {
			errs = append(errs, fmt.Errorf("unknown category %q", c))
		}
	}
	return errs
}

// validateRepoURL checks that repo_url, when set, is an http(s) URL. It is
// surfaced as a clickable "Repo" link in the plugin UI, so a non-http(s)
// scheme (e.g. "javascript:") is rejected at registration rather than relying
// solely on the frontend href guard. An empty repo_url is valid (optional).
func (m *Manifest) validateRepoURL() []error {
	// Normalise in place so the stored/serialised value matches what was
	// validated (a manifest with surrounding whitespace would otherwise pass
	// the check but keep the spaces in the href).
	m.RepoURL = strings.TrimSpace(m.RepoURL)
	if m.RepoURL == "" {
		return nil
	}
	u := strings.ToLower(m.RepoURL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return []error{fmt.Errorf("repo_url %q must be an http(s) URL", m.RepoURL)}
	}
	return nil
}

// validateUIPages checks each declared UI page's surface against the known
// enum (settings | task-panel | main-nav).
func (m *Manifest) validateUIPages() []error {
	var errs []error
	for _, p := range m.UI.Pages {
		if !validUISurfaces[p.Surface] {
			errs = append(errs, fmt.Errorf("unknown ui surface %q for page %q", p.Surface, p.Key))
		}
	}
	return errs
}

// validateUIBundle checks that ui.bundle, if set, and every ui.styles entry
// are root-relative paths (start with "/"). ui.pages remain optional/valid
// on their own; a bundle-only manifest (no pages) is valid.
func (m *Manifest) validateUIBundle() []error {
	var errs []error
	if m.UI.Bundle != "" && !strings.HasPrefix(m.UI.Bundle, "/") {
		errs = append(errs, fmt.Errorf("ui.bundle %q must be a root-relative path (start with \"/\")", m.UI.Bundle))
	}
	for _, style := range m.UI.Styles {
		if !strings.HasPrefix(style, "/") {
			errs = append(errs, fmt.Errorf("ui.styles entry %q must be a root-relative path (start with \"/\")", style))
		}
	}
	return errs
}

// validateUIKeybindings checks each declared ui.keybindings entry: id is a
// non-empty slug unique within the plugin, description is non-empty, and
// default parses as a valid combo (see parseKeybindingCombo). Keybindings
// are optional, but when present they require ui.bundle — kandev dispatches
// the combo as an event the plugin's JS bundle handles, so a keybinding
// without a bundle can never do anything.
func (m *Manifest) validateUIKeybindings() []error {
	var errs []error
	if len(m.UI.Keybindings) > 0 && m.UI.Bundle == "" {
		errs = append(errs, errors.New("ui.keybindings requires ui.bundle to be set"))
	}
	seen := make(map[string]bool, len(m.UI.Keybindings))
	for _, kb := range m.UI.Keybindings {
		errs = append(errs, validateKeybindingID(kb.ID, seen)...)
		if strings.TrimSpace(kb.Description) == "" {
			errs = append(errs, fmt.Errorf("ui.keybindings[%q]: description must not be empty", kb.ID))
		}
		if err := parseKeybindingCombo(kb.Default); err != nil {
			errs = append(errs, fmt.Errorf("ui.keybindings[%q]: %w", kb.ID, err))
		}
	}
	return errs
}

// validateKeybindingID checks a single keybinding id against the slug
// pattern and records it in seen, reporting a duplicate-id error on repeat.
func validateKeybindingID(id string, seen map[string]bool) []error {
	var errs []error
	if !keybindingIDPattern.MatchString(id) {
		errs = append(errs, fmt.Errorf("invalid ui.keybindings id %q: must match %s", id, keybindingIDPattern.String()))
		return errs
	}
	if seen[id] {
		errs = append(errs, fmt.Errorf("duplicate ui.keybindings id %q", id))
		return errs
	}
	seen[id] = true
	return errs
}

// parseKeybindingCombo validates a keybinding combo string such as
// "mod+shift+k": tokens are split on "+", trimmed of surrounding whitespace,
// and lowercased. Every token but exactly one must be a recognized modifier
// (keybindingModifiers); the remaining single token must be a recognized key
// (keybindingKeys). It returns an error describing the first problem found;
// it does not return a normalized struct because validation is currently the
// only caller.
func parseKeybindingCombo(combo string) error {
	if strings.TrimSpace(combo) == "" {
		return errors.New("default combo must not be empty")
	}
	rawTokens := strings.Split(combo, "+")
	var keyToken string
	keyCount := 0
	for _, raw := range rawTokens {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			return fmt.Errorf("default combo %q has an empty token", combo)
		}
		if keybindingModifiers[token] {
			continue
		}
		if !keybindingKeys[token] {
			return fmt.Errorf("default combo %q: unknown token %q (not a recognized modifier or key)", combo, token)
		}
		keyToken = token
		keyCount++
	}
	if keyCount == 0 {
		return fmt.Errorf("default combo %q must include exactly one non-modifier key", combo)
	}
	if keyCount > 1 {
		return fmt.Errorf("default combo %q must include exactly one non-modifier key, found more than one (last: %q)", combo, keyToken)
	}
	return nil
}

// validateWebhooks checks for duplicate webhook keys.
func (m *Manifest) validateWebhooks() []error {
	seen := make(map[string]bool, len(m.Webhooks))
	var errs []error
	for _, wh := range m.Webhooks {
		if seen[wh.Key] {
			errs = append(errs, fmt.Errorf("duplicate webhook key %q", wh.Key))
			continue
		}
		seen[wh.Key] = true
	}
	return errs
}
