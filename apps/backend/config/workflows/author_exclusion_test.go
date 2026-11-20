package workflows

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestConvertEvents_AcceptsExcludeAuthorProvider(t *testing.T) {
	const doc = `
on_enter:
  - type: ensure_participant_seat
    config:
      role: reviewer
      exclude_author_provider: true
`
	var e stepEventsYAML
	if err := yaml.Unmarshal([]byte(doc), &e); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	events, err := convertEvents(e)
	if err != nil {
		t.Fatalf("convertEvents: %v", err)
	}
	if roles := models.AuthorProviderExclusionRoles(events); !roles["reviewer"] || len(roles) != 1 {
		t.Fatalf("exclusion roles = %v, want reviewer", roles)
	}
}

func TestBuiltinTemplatesDoNotExcludeAuthorProvider(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	for _, tmpl := range templates {
		for _, step := range tmpl.Steps {
			if roles := models.AuthorProviderExclusionRoles(step.Events); len(roles) != 0 {
				t.Errorf("template %q step %q sets exclude_author_provider for %v; built-ins must leave it off",
					tmpl.ID, step.Name, roles)
			}
		}
	}
}
