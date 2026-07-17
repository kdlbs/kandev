// config.go implements the operator-facing plugin settings mechanism: the
// manifest's config_schema (a JSON-Schema-like object the plugin author
// declares) drives a settings form in Settings > Plugins > <plugin>, and the
// helpers here mask/merge/validate the values flowing through it.
//
// Secret fields (a schema property with `secret: true` or
// `format: "password"`, e.g. a GitHub PAT) never leave the backend in
// cleartext on the operator API: GetMaskedConfig replaces stored values with
// configSecretMask, and mergeMaskedSecrets treats an incoming masked value as
// "keep what is stored" so re-submitting the form unchanged never clobbers a
// secret. The plugin process itself reads the REAL values via the Host
// GetConfig RPC (host.go) — that is how the configured secret reaches the
// plugin.
package plugins

import (
	"errors"
	"fmt"
	"reflect"
)

// configSecretMask is the placeholder returned in place of a secret config
// value on the operator API, and recognized on write as "leave the stored
// value unchanged". Deliberately implausible as a real credential.
const configSecretMask = "********"

// ErrConfigInvalid marks a config rejected by validateConfigSchema so the
// HTTP layer can map it to 400 instead of 500.
var ErrConfigInvalid = errors.New("plugin config invalid")

// schemaProperties extracts the "properties" object from a config_schema.
// Returns nil (no declared properties, everything permissive) when the
// schema is absent or not shaped like a JSON-Schema object.
func schemaProperties(schema map[string]any) map[string]any {
	props, _ := schema["properties"].(map[string]any)
	return props
}

// secretPropertyKeys returns the set of property names the schema marks as
// secret: `secret: true` or `format: "password"`.
func secretPropertyKeys(schema map[string]any) map[string]bool {
	keys := map[string]bool{}
	for name, raw := range schemaProperties(schema) {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if prop["secret"] == true || prop["format"] == "password" {
			keys[name] = true
		}
	}
	return keys
}

// maskSecrets returns a copy of config with every non-empty secret string
// value replaced by configSecretMask.
func maskSecrets(config map[string]any, schema map[string]any) map[string]any {
	secrets := secretPropertyKeys(schema)
	out := make(map[string]any, len(config))
	for k, v := range config {
		if secrets[k] {
			if s, ok := v.(string); ok && s != "" {
				out[k] = configSecretMask
				continue
			}
		}
		out[k] = v
	}
	return out
}

// mergeMaskedSecrets resolves an incoming config write against the stored
// one: a secret field submitted as the mask placeholder keeps its stored
// value (or is dropped if nothing is stored). Everything else is taken from
// incoming verbatim — the write is a full replace, not a patch.
func mergeMaskedSecrets(incoming, existing map[string]any, schema map[string]any) map[string]any {
	secrets := secretPropertyKeys(schema)
	out := make(map[string]any, len(incoming))
	for k, v := range incoming {
		out[k] = v
	}
	for k := range secrets {
		if out[k] != configSecretMask {
			continue
		}
		if stored, ok := existing[k]; ok {
			out[k] = stored
		} else {
			delete(out, k)
		}
	}
	return out
}

// validateConfigSchema checks config against the author-declared
// config_schema. Deliberately a small JSON-Schema subset — required fields,
// primitive types (string/boolean/number/integer), and enum membership on
// declared properties. Undeclared keys and richer schema constructs are
// permitted: the schema is advisory authoring metadata, not a hard sandbox.
func validateConfigSchema(config map[string]any, schema map[string]any) error {
	if err := checkRequiredKeys(config, schema); err != nil {
		return err
	}
	props := schemaProperties(schema)
	for name, raw := range props {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, present := config[name]
		if !present {
			continue
		}
		if err := checkPropertyValue(name, value, prop); err != nil {
			return err
		}
	}
	return nil
}

// checkRequiredKeys enforces the schema's "required" list.
func checkRequiredKeys(config map[string]any, schema map[string]any) error {
	required, _ := schema["required"].([]any)
	for _, raw := range required {
		name, ok := raw.(string)
		if !ok {
			continue
		}
		if _, present := config[name]; !present {
			return fmt.Errorf("%w: missing required field %q", ErrConfigInvalid, name)
		}
	}
	return nil
}

// checkPropertyValue validates one present value against its declared
// property: primitive type match plus enum membership.
func checkPropertyValue(name string, value any, prop map[string]any) error {
	if typeName, ok := prop["type"].(string); ok {
		if !valueMatchesType(value, typeName) {
			return fmt.Errorf("%w: field %q must be a %s", ErrConfigInvalid, name, typeName)
		}
	}
	if enum, ok := prop["enum"].([]any); ok && len(enum) > 0 {
		for _, allowed := range enum {
			if reflect.DeepEqual(value, allowed) {
				return nil
			}
		}
		return fmt.Errorf("%w: field %q must be one of the declared enum values", ErrConfigInvalid, name)
	}
	return nil
}

// valueMatchesType reports whether value satisfies the JSON-Schema primitive
// typeName. Numeric values may arrive as float64 (JSON decoding) or int
// (YAML round-trip of a stored config); "integer" additionally requires an
// integral float. Unknown type names (object/array/…) are not checked.
func valueMatchesType(value any, typeName string) bool {
	switch typeName {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		return isNumeric(value)
	case "integer":
		if f, ok := value.(float64); ok {
			return f == float64(int64(f))
		}
		return isInt(value)
	default:
		return true
	}
}

func isNumeric(value any) bool {
	if _, ok := value.(float64); ok {
		return true
	}
	return isInt(value)
}

func isInt(value any) bool {
	switch value.(type) {
	case int, int64, uint64:
		return true
	default:
		return false
	}
}
