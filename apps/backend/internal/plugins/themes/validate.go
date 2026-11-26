package themes

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const MaxDefinitionBytes = 256 << 10
const BaseDark = "dark"
const BaseLight = "light"

// Schema is the shared authoring and runtime contract.
//
//go:embed schema/v1.json
var Schema []byte

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var doc any
	if err := json.Unmarshal(Schema, &doc); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("https://kandev.local/themes/v1", doc); err != nil {
		return nil, err
	}
	return c.Compile("https://kandev.local/themes/v1")
})

func Parse(data []byte, base string) (*Definition, error) {
	if len(data) > MaxDefinitionBytes {
		return nil, fmt.Errorf("theme_too_large: maximum %d bytes", MaxDefinitionBytes)
	}
	if base != BaseLight && base != BaseDark {
		return nil, fmt.Errorf("theme_base: light or dark required")
	}
	value, err := strictJSON(data)
	if err != nil {
		return nil, err
	}
	schema, err := compiledSchema()
	if err != nil {
		return nil, err
	}
	if err = schema.Validate(value); err != nil {
		return nil, fmt.Errorf("theme_schema: %w", err)
	}
	normalizeColors(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var d Definition
	if err = json.Unmarshal(normalized, &d); err != nil {
		return nil, err
	}
	if d.Base != "" && d.Base != base {
		return nil, fmt.Errorf("theme_base: definition disagrees with manifest")
	}
	d.Base = base
	if err = d.validateOpaque(); err != nil {
		return nil, err
	}
	scopes := 0
	for _, rule := range d.Textmate {
		if list, ok := rule.Scope.([]any); ok {
			scopes += len(list)
		} else {
			scopes++
		}
	}
	if scopes > 4096 {
		return nil, fmt.Errorf("theme_scopes: maximum 4096 scope strings")
	}
	return &d, nil
}

func (d *Definition) validateOpaque() error {
	for group, colors := range map[string]map[string]string{"ui": d.UI, "editor": d.Editor, "terminal": d.Terminal} {
		for key, color := range colors {
			opaque := key == "background" || (group == "ui" && (key == "card" || key == "popover" || key == "sidebar"))
			if opaque && len(color) == 9 && color[7:] != "ff" {
				return fmt.Errorf("theme_alpha: %s.%s must be opaque", group, key)
			}
		}
	}
	return nil
}

func (d *Definition) Digest() string {
	data, _ := json.Marshal(d)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeColors(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if s, ok := child.(string); ok && key != "scope" && strings.HasPrefix(s, "#") {
				s = strings.ToLower(s)
				if len(s) == 4 || len(s) == 5 {
					var b strings.Builder
					b.WriteByte('#')
					for _, c := range s[1:] {
						b.WriteRune(c)
						b.WriteRune(c)
					}
					s = b.String()
				}
				v[key] = s
			} else {
				normalizeColors(child)
			}
		}
	case []any:
		for _, child := range v {
			normalizeColors(child)
		}
	}
}

func strictJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := readJSONValue(decoder, 0)
	if err != nil {
		return nil, fmt.Errorf("theme_json: %w", err)
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("theme_json: trailing content")
	}
	return value, nil
}

func readJSONValue(d *json.Decoder, depth int) (any, error) {
	if depth > 16 {
		return nil, fmt.Errorf("maximum nesting depth exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		return readJSONObject(d, depth)
	case json.Delim('['):
		var list []any
		for d.More() {
			child, err := readJSONValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, child)
		}
		_, err = d.Token()
		return list, err
	default:
		return token, nil
	}
}

func readJSONObject(d *json.Decoder, depth int) (map[string]any, error) {
	value := map[string]any{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("object key required")
		}
		if _, exists := value[key]; exists || key == "__proto__" || key == "constructor" || key == "prototype" {
			return nil, fmt.Errorf("duplicate or forbidden key %q", key)
		}
		child, err := readJSONValue(d, depth+1)
		if err != nil {
			return nil, err
		}
		value[key] = child
	}
	_, err := d.Token()
	return value, err
}
