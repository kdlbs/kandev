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

// Validate checks the manifest against the plugin registration rules
// described in docs/specs/plugins/spec.md. It returns nil if the manifest
// is well-formed, or a joined error describing every violation found.
func (m *Manifest) Validate() error {
	var errs []error
	errs = append(errs, m.validateIdentity()...)
	errs = append(errs, m.validateRuntimeType()...)
	if m.IsManaged() {
		errs = append(errs, m.validateManagedRuntime()...)
	} else {
		errs = append(errs, m.validateEndpoints()...)
	}
	errs = append(errs, m.validateCategories()...)
	errs = append(errs, m.validateUIPages()...)
	errs = append(errs, m.validateUIBundle()...)
	errs = append(errs, m.validateTools()...)
	errs = append(errs, m.validateWebhooks()...)
	return errors.Join(errs...)
}

// validateIdentity checks id pattern and api_version.
func (m *Manifest) validateIdentity() []error {
	var errs []error
	if !idPattern.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("invalid plugin id %q: must match %s", m.ID, idPattern.String()))
	}
	if m.APIVersion != supportedAPIVersion {
		errs = append(errs, fmt.Errorf("unsupported api_version %d: only %d is supported", m.APIVersion, supportedAPIVersion))
	}
	return errs
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
	if m.Endpoints.Tools == "" {
		errs = append(errs, errors.New("endpoints.tools is required"))
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

// validateTools checks for duplicate tool names.
func (m *Manifest) validateTools() []error {
	seen := make(map[string]bool, len(m.Tools))
	var errs []error
	for _, tool := range m.Tools {
		if seen[tool.Name] {
			errs = append(errs, fmt.Errorf("duplicate tool name %q", tool.Name))
			continue
		}
		seen[tool.Name] = true
	}
	return errs
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
