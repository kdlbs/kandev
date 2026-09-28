package manifest

import (
	"strings"
	"testing"
)

func TestValidateExecutorProviderRejectsInvalidNumericBounds(t *testing.T) {
	tests := []struct {
		name       string
		field      string
		wantInText string
	}{
		{name: "malformed minimum", field: `{type: number, minimum: low}`, wantInText: "minimum"},
		{name: "bounds on string", field: `{type: string, minimum: 1}`, wantInText: "numeric"},
		{name: "minimum exceeds maximum", field: `{type: integer, minimum: 10, maximum: 2}`, wantInText: "minimum"},
		{name: "malformed maximum", field: `{type: integer, maximum: high}`, wantInText: "maximum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validExecutorProviderManifest(`handle: {type: string}`)
			old := "profile_schema: {type: object, properties: {region: {type: string, enum: [eu-west-1, us-east-1]}}}"
			updated := "profile_schema: {type: object, properties: {region: " + test.field + "}}"
			manifest = strings.Replace(manifest, old, updated, 1)
			parsed, err := Parse([]byte(manifest))
			if err != nil {
				t.Fatalf("Parse(): %v", err)
			}
			if err := parsed.Validate(); err == nil || !strings.Contains(err.Error(), test.wantInText) {
				t.Fatalf("Validate() error = %v, want rejection containing %q", err, test.wantInText)
			}
		})
	}
}

func TestValidateExecutorProviderRequiresClosedResourceStateSchema(t *testing.T) {
	manifest := validExecutorProviderManifest(`handle: {type: string}`)
	manifest = strings.Replace(manifest,
		"resource_state_schema: {type: object, additionalProperties: false, properties:",
		"resource_state_schema: {type: object, properties:", 1)
	parsed, err := Parse([]byte(manifest))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	if err := parsed.Validate(); err == nil || !strings.Contains(err.Error(), "additionalProperties") {
		t.Fatalf("Validate() error = %v, want closed resource-state schema rejection", err)
	}
}
