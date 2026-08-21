package manifest

import (
	"strings"
	"testing"
)

func TestActionAccessDefaultsToAuthenticatedAndAcceptsAdmin(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: connection.get
    scope: workspace
    max_body_bytes: 1024
  - key: connection.set
    scope: workspace
    access: admin
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if got := m.Actions[0].EffectiveAccess(); got != ActionAccessAuthenticated {
		t.Fatalf("default action access = %q, want %q", got, ActionAccessAuthenticated)
	}
	if got := m.Actions[1].EffectiveAccess(); got != ActionAccessAdmin {
		t.Fatalf("explicit action access = %q, want %q", got, ActionAccessAdmin)
	}
}

func TestValidateRejectsUnknownActionAccess(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: connection.set
    scope: workspace
    access: owner
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "access") {
		t.Fatalf("Validate() error = %v, want invalid action access", err)
	}
}
