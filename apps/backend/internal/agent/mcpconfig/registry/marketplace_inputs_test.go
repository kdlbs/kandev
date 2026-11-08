package registry

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func TestMarketplaceInputValuesStayWithinReviewedInputs(t *testing.T) {
	choice := Choice{Kind: "package", RegistryType: "npm", Identifier: "@example/tools", Version: "1.0.0",
		EnvironmentVariables: []KeyValueInput{{Name: "REGION", IsRequired: true}, {Name: "API_KEY", IsSecret: true}}}
	entry := Entry{Name: "example/tools"}
	input, err := definitionInputForChoice(entry, choice, InstallRequest{WorkspaceID: "workspace", InputValues: map[string]string{"REGION": "eu west"}})
	if err != nil || input.Configuration.Env["REGION"] != "eu west" {
		t.Fatalf("non-secret input was not configured: %v", err)
	}
	if choice.EnvironmentVariables[0].Value != "" {
		t.Fatal("workspace overrides mutated the cached publisher metadata")
	}
	for _, values := range []map[string]string{{"UNKNOWN": "value"}, {"API_KEY": "literal-secret"}} {
		_, err := definitionInputForChoice(entry, choice, InstallRequest{WorkspaceID: "workspace", InputValues: values})
		if !errors.Is(err, mcpconfig.ErrMCPInvalidDefinition) {
			t.Fatal("unreviewed or secret literal input must be rejected")
		}
	}
}

func TestMarketplaceRequiredInputsCannotBeSkipped(t *testing.T) {
	for _, secret := range []bool{false, true} {
		choice := Choice{Kind: "remote", Transport: "streamable-http", URL: "https://example.test/mcp",
			Headers: []KeyValueInput{{Name: "Authorization", IsRequired: true, IsSecret: secret}}}
		_, err := definitionInputForChoice(Entry{Name: "example/tools", Version: "1"}, choice, InstallRequest{WorkspaceID: "workspace"})
		if err == nil {
			t.Fatalf("required input accepted without configuration (secret=%v)", secret)
		}
	}
}

func TestMarketplaceUnmaterializedVariablesAreUnavailable(t *testing.T) {
	entry := Entry{Name: "example/tools", Remotes: []Remote{{Type: "streamable-http", URL: "https://example.test/{tenant}",
		Variables: map[string]KeyValueInput{"tenant": {IsRequired: true}}}}}
	choice := entryChoices(entry)[0]
	if choice.Selectable || choice.UnsupportedReason == "" {
		t.Fatal("a choice with unmaterialized variables must explain why it is unavailable")
	}
}

func TestMarketplaceRequiredSecretUsesReferenceOnly(t *testing.T) {
	choice := Choice{Kind: "package", RegistryType: "npm", Identifier: "@example/tools", Version: "1.0.0",
		EnvironmentVariables: []KeyValueInput{{Name: "API_KEY", IsRequired: true, IsSecret: true}}}
	input, err := definitionInputForChoice(Entry{Name: "example/tools"}, choice, InstallRequest{
		WorkspaceID: "workspace", SecretBindings: []SecretBindingInput{{InputName: "API_KEY", SecretID: "secret-id"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Configuration.Env["API_KEY"] != "" || len(input.SecretBindings) != 1 || input.SecretBindings[0] != (mcpconfig.MCPSecretBinding{InputName: "API_KEY", SecretID: "secret-id"}) {
		t.Fatal("required secret must persist only its reference")
	}
}
