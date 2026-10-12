package toolschema

import (
	"encoding/json"
	"fmt"
	"net/url"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// rootCombinators are the JSON Schema combinator keywords a downstream model
// provider's function-calling validator rejects at the root of a tool schema.
// Nested use below a property is portable and stays allowed.
var rootCombinators = []string{"oneOf", "allOf", "anyOf"}

// CheckPortableRoot rejects a schema document whose root declares oneOf, allOf,
// or anyOf. It is shared by every schema compile seam so the portable subset is
// enforced identically for plugin and built-in tools.
func CheckPortableRoot(document map[string]any) error {
	for _, keyword := range rootCombinators {
		if _, ok := document[keyword]; ok {
			return fmt.Errorf("schema root must not declare %q", keyword)
		}
	}
	return nil
}

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
	if err := CheckPortableRoot(document); err != nil {
		return nil, err
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
