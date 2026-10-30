package manifest

import (
	"reflect"
	"strings"
	"testing"
)

const themeManifestYAML = `id: example-themes
api_version: 2
version: 1.0.0
display_name: Example themes
distribution:
  schema_version: 1
  kind: theme
themes:
  - key: charcoal
    label: Charcoal
    base: dark
    path: themes/charcoal.json
  - key: paper
    label: Paper
    base: light
    path: themes/paper.json
`

// @covers AC-PLUGINS-THEMES-001.1
func TestThemeManifestAcceptsDataOnly(t *testing.T) {
	m, err := Parse([]byte(themeManifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid theme pack rejected: %v", err)
	}
}

func TestThemeManifestRejectsMixedAndUnknownFields(t *testing.T) {
	for _, suffix := range []string{
		"runtime:\n  type: binary\n  executables: {linux-amd64: server/main}\n",
		"ui:\n  bundle: ui/main.js\n",
		"capabilities:\n  secrets: true\n",
		"future_contribution: dangerous\n",
	} {
		t.Run(suffix, func(t *testing.T) {
			m, err := Parse([]byte(themeManifestYAML + suffix))
			if err == nil {
				err = m.Validate()
			}
			if err == nil {
				t.Fatal("unsafe theme accepted")
			}
		})
	}
}

func TestThemeCloneIsolation(t *testing.T) {
	m, err := Parse([]byte(themeManifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	clone := m.Clone()
	clone.Themes[0].Label = "Changed"
	clone.Distribution.License = "Changed"
	if m.Themes[0].Label == "Changed" || m.Distribution.License == "Changed" {
		t.Fatal("clone aliases original")
	}
}

func TestThemeManifestVariants(t *testing.T) {
	for _, replace := range [][2]string{{"key: paper", "key: charcoal"}, {"base: light", "base: unknown"}, {"themes/paper.json", "../paper.json"}, {"themes/paper.json", `themes\paper.json`}, {"label: Paper", "label: ''"}} {
		m, err := Parse([]byte(strings.ReplaceAll(themeManifestYAML, replace[0], replace[1])))
		if err == nil {
			err = m.Validate()
		}
		if err == nil {
			t.Fatalf("accepted %v", replace)
		}
	}
}

func TestThemeProfileFieldDecisions(t *testing.T) {
	// Every new manifest field needs an explicit metadata/contribution decision.
	denied := map[string]bool{"Icon": true, "BaseURL": true, "Endpoints": true, "Capabilities": true, "AutomationConditions": true, "Webhooks": true, "Actions": true, "RepositoryProviders": true, "ExecutorProviders": true, "ReferenceSources": true, "AuthProviders": true, "ConfigSchema": true, "AgentTools": true, "UI": true, "Runtime": true}
	typ := reflect.TypeOf(Manifest{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if themeMetadataFields[name] == denied[name] {
			t.Errorf("field %s needs exactly one profile decision", name)
		}
	}
}
