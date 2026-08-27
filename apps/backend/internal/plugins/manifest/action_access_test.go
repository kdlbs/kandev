package manifest

import (
	"fmt"
	"strings"
	"testing"
)

func TestActionAccessDefaultsToAuthenticatedAndAcceptsAdmin(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
min_kandev_version: "0.91.1"
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

func TestValidateAdminActionRequiresSupportingKandevMinimum(t *testing.T) {
	tests := []struct {
		name       string
		minimum    string
		wantReject bool
	}{
		{name: "missing", wantReject: true},
		{name: "older release", minimum: "0.91.0", wantReject: true},
		{name: "malformed", minimum: "main", wantReject: true},
		{name: "first supporting release", minimum: "0.91.1"},
		{name: "tagged first supporting release", minimum: "v0.91.1"},
		{name: "later release", minimum: "0.92.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := validManifestYAML
			if tt.minimum != "" {
				yaml += fmt.Sprintf("\nmin_kandev_version: %q\n", tt.minimum)
			}
			yaml += `
actions:
  - key: connection.set
    scope: workspace
    access: admin
    max_body_bytes: 1024
`
			m, err := Parse([]byte(yaml))
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			validationErr := m.Validate()
			if tt.wantReject {
				if validationErr == nil || !strings.Contains(validationErr.Error(), "requires min_kandev_version >= 0.91.1") {
					t.Fatalf("Validate() error = %v, want admin action minimum-version rejection", validationErr)
				}
				return
			}
			if validationErr != nil {
				t.Fatalf("Validate() unexpected error: %v", validationErr)
			}
		})
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
