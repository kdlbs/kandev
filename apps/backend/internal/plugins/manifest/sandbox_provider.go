package manifest

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const SandboxProviderProtocolMajor = 1

var sandboxProviderKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// SandboxProvider declares a policy compiler. It does not own child processes.
type SandboxProvider struct {
	Key           string `yaml:"key" json:"key"`
	DisplayName   string `yaml:"display_name" json:"display_name"`
	ProtocolMajor int    `yaml:"protocol_major" json:"protocol_major"`
}

func (p *SandboxProvider) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("sandbox provider must be a mapping")
	}
	for index := 0; index < len(node.Content); index += 2 {
		switch node.Content[index].Value {
		case "key", "display_name", "protocol_major":
		default:
			return fmt.Errorf("sandbox provider contains an unknown field")
		}
	}
	type plain SandboxProvider
	return node.Decode((*plain)(p))
}

func (m *Manifest) validateSandboxProviders() []error {
	if len(m.SandboxProviders) == 0 && !m.Capabilities.SandboxProvider {
		return nil
	}
	if !m.Capabilities.SandboxProvider || len(m.SandboxProviders) == 0 {
		return []error{fmt.Errorf("sandbox_providers require capabilities.sandbox_provider and at least one provider")}
	}
	if !m.IsManaged() || m.APIVersion != CurrentAPIVersion {
		return []error{fmt.Errorf("sandbox_providers require a managed API version 2 plugin")}
	}
	if len(m.SandboxProviders) > 16 {
		return []error{fmt.Errorf("sandbox_providers must declare at most 16 providers")}
	}
	var errs []error
	seen := make(map[string]bool, len(m.SandboxProviders))
	for _, p := range m.SandboxProviders {
		if !sandboxProviderKeyPattern.MatchString(p.Key) || seen[p.Key] {
			errs = append(errs, fmt.Errorf("sandbox provider key must be a unique lowercase identifier"))
		}
		seen[p.Key] = true
		if p.ProtocolMajor != SandboxProviderProtocolMajor {
			errs = append(errs, fmt.Errorf("sandbox provider protocol major is unsupported"))
		}
		if !validSandboxProviderDisplayName(p.DisplayName) {
			errs = append(errs, fmt.Errorf("sandbox provider display name must be plain text with 1-100 characters"))
		}
	}
	return errs
}

func validSandboxProviderDisplayName(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 100 &&
		!strings.ContainsAny(value, "<>") && strings.IndexFunc(value, unicode.IsControl) == -1
}
