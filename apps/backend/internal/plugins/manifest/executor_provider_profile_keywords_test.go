package manifest

import (
	"strings"
	"testing"
)

func profileSchemaManifest(t *testing.T, field string) (*Manifest, error) {
	t.Helper()
	manifest := validExecutorProviderManifest(`handle: {type: string}`)
	old := "profile_schema: {type: object, properties: {region: {type: string, enum: [eu-west-1, us-east-1]}}}"
	updated := "profile_schema: {type: object, properties: {region: " + field + "}}"
	manifest = strings.Replace(manifest, old, updated, 1)
	parsed, err := Parse([]byte(manifest))
	if err != nil {
		return nil, err
	}
	return parsed, parsed.Validate()
}

func TestValidateExecutorProviderAcceptsProfileKeywords(t *testing.T) {
	for _, field := range []string{
		`{type: string, default: eu-west-1, minLength: 1, maxLength: 32, pattern: "[a-z]{2}-[a-z]+-[0-9]"}`,
		`{type: integer, default: 4, minimum: 1, maximum: 8}`,
		`{type: boolean, default: true}`,
		`{type: string, minLength: 0}`,
	} {
		if _, err := profileSchemaManifest(t, field); err != nil {
			t.Fatalf("field %s rejected: %v", field, err)
		}
	}
}

func TestValidateExecutorProviderRejectsProfileKeywordViolations(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"secret with default", `{type: string, secret: true, default: x}`, "region"},
		{"default violates enum", `{type: string, enum: [a, b], default: c}`, "default"},
		{"default violates pattern", `{type: string, pattern: "[a-z]+", default: ABC}`, "default"},
		{"default wrong type", `{type: integer, default: seven}`, "default"},
		{"bad pattern", `{type: string, pattern: "("}`, "pattern"},
		{"minLength above maxLength", `{type: string, minLength: 9, maxLength: 3}`, "minLength"},
		{"length on integer", `{type: integer, maxLength: 3}`, "maxLength"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := profileSchemaManifest(t, test.field); err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "region") {
				t.Fatalf("Validate() error = %v, want a rejection naming region and containing %q", err, test.want)
			}
		})
	}
}

func TestResourceStateSchemaKeepsItsClosedKeywordSet(t *testing.T) {
	manifest := validExecutorProviderManifest(`handle: {type: string, default: x}`)
	parsed, err := Parse([]byte(manifest))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	if err := parsed.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported schema keyword") {
		t.Fatalf("Validate() error = %v, want the state schema to reject default", err)
	}
}
