package toolschema

import (
	"encoding/json"
	"fmt"
	"net/url"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Compile compiles a JSON Schema document using the same draft as Kandev's
// MCP boundary. Callers can use the returned schema for both install-time and
// invocation-time validation.
func Compile(name string, document map[string]any) (*jsonschema.Schema, error) {
	if len(document) == 0 {
		return nil, fmt.Errorf("schema must not be empty")
	}
	if document["type"] != "object" {
		return nil, fmt.Errorf("schema root type must be object")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	resourceURL := "https://kandev.local/mcp/schemas/" + url.PathEscape(name)
	if err := compiler.AddResource(resourceURL, document); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

func Marshal(document map[string]any) ([]byte, error) {
	return json.Marshal(document)
}
