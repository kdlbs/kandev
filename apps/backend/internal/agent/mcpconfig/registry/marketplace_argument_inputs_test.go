package registry

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestMarketplaceUnsupportedArgumentInputsAreUnavailable(t *testing.T) {
	for _, argument := range []string{
		`{"type":"named","name":"--directory","isRequired":true,"format":"filepath"}`,
		`{"type":"positional","name":"directory","isRequired":true}`,
		`{"value":"{tenant}","variables":{"tenant":{"isRequired":true}}}`,
		`{"name":"--token","value":"publisher-example","isSecret":true}`,
	} {
		for _, field := range []string{"runtimeArguments", "packageArguments"} {
			t.Run(field+argument, func(t *testing.T) {
				var entry Entry
				raw := `{"name":"example/tools","packages":[{"registryType":"npm","identifier":"@example/tools","version":"1.0.0","transport":{"type":"stdio"},"` + field + `":[` + argument + `]}]}`
				if err := json.Unmarshal([]byte(raw), &entry); err != nil {
					t.Fatal(err)
				}
				choice := entryChoices(entry)[0]
				_, err := definitionInputForChoice(entry, choice, InstallRequest{WorkspaceID: "workspace"})
				if choice.Selectable || choice.UnsupportedReason == "" || !errors.Is(err, ErrMarketplaceChoiceUnsupported) {
					t.Fatalf("unsupported input offered: %+v %v", choice, err)
				}
			})
		}
	}
}

func TestMarketplaceFixedLiteralArgumentsKeepBoundaries(t *testing.T) {
	var entry Entry
	raw := `{"name":"example/tools","packages":[{"registryType":"npm","identifier":"@example/tools","version":"1.0.0","transport":{"type":"stdio"},"packageArguments":[{"type":"named","name":"--directory","value":"two words"},{"type":"positional","value":"literal"},{"value":"--verbose"}]}]}`
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatal(err)
	}
	choice := entryChoices(entry)[0]
	input, err := definitionInputForChoice(entry, choice, InstallRequest{WorkspaceID: "workspace"})
	if err != nil || !choice.Selectable || !reflect.DeepEqual(input.Configuration.PackageArguments, []string{"--directory", "two words", "literal", "--verbose"}) {
		t.Fatalf("fixed argv: %+v %v", input.Configuration.PackageArguments, err)
	}
}

func TestMarketplaceNestedInputVariablesAreUnavailable(t *testing.T) {
	var entry Entry
	if err := json.Unmarshal([]byte(`{"name":"example/tools","remotes":[{"type":"streamable-http","url":"https://example.test","headers":[{"name":"Authorization","value":"Bearer {token}","variables":{"token":{"isRequired":true,"isSecret":true}}}]}]}`), &entry); err != nil {
		t.Fatal(err)
	}
	choice := entryChoices(entry)[0]
	_, err := definitionInputForChoice(entry, choice, InstallRequest{WorkspaceID: "workspace"})
	if choice.Selectable || !errors.Is(err, ErrMarketplaceChoiceUnsupported) {
		t.Fatalf("nested variable silently accepted: %+v %v", choice, err)
	}
}
