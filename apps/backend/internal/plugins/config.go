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
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/secrets"
)

// configSecretMask is the placeholder returned in place of a secret config
// value on the operator API, and recognized on write as "leave the stored
// value unchanged". Deliberately implausible as a real credential.
const configSecretMask = "********"

// isSecretNotFound reports whether err is the secrets layer's "absent entry"
// signal (secrets.ErrNotFound) rather than a genuine backend failure. Kept as
// one helper so the several call sites that must tell "absent" apart from a
// real backend error share one definition (host.go's GetSecret/DeleteSecret,
// service.go's vault rollback).
func isSecretNotFound(err error) bool {
	return errors.Is(err, secrets.ErrNotFound)
}

// pluginVaultIDPrefix is the reserved id namespace every plugin-owned vault
// entry lives under: "plugin:<plugin_id>:...". Uninstall deletes the whole
// prefix.
const pluginVaultIDPrefix = "plugin:"

// pluginSecretKeyPattern bounds the keys plugins may use with the
// GetSecret/SetSecret/DeleteSecret Host RPCs: a single sane identifier, so
// a key can never smuggle separators into the vault-id namespace.
var pluginSecretKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// pluginVaultPrefix returns the vault-id prefix owned by pluginID.
func pluginVaultPrefix(pluginID string) string {
	return pluginVaultIDPrefix + pluginID + ":"
}

// pluginSecretID is the vault id for a plugin-owned secret stored via the
// SetSecret Host RPC.
func pluginSecretID(pluginID, key string) string {
	return pluginVaultPrefix(pluginID) + "secret:" + key
}

// pluginConfigSecretID is the vault id backing a secret config_schema field
// saved from the plugin settings page. Distinct sub-namespace from
// pluginSecretID so a plugin's own SetSecret keys can never collide with
// config-managed entries.
func pluginConfigSecretID(pluginID, field string) string {
	return pluginVaultPrefix(pluginID) + "config:" + field
}

// configVaultRef is the value persisted in <id>.config.yml in place of a
// secret config field's cleartext: "vault:" + the exact vault id holding
// the value. isConfigVaultRef only treats a value as a reference when it
// equals the ref derived for that specific plugin+field, so a cleartext
// secret that merely starts with "vault:" can never be misread.
func configVaultRef(pluginID, field string) string {
	return "vault:" + pluginConfigSecretID(pluginID, field)
}

// isConfigVaultRef reports whether value is the vault reference marker for
// pluginID's config field.
func isConfigVaultRef(pluginID, field string, value any) bool {
	s, ok := value.(string)
	return ok && s == configVaultRef(pluginID, field)
}

// hasPluginVaultPrefix reports whether vault id belongs to pluginID's
// namespace.
func hasPluginVaultPrefix(id, pluginID string) bool {
	return strings.HasPrefix(id, pluginVaultPrefix(pluginID))
}

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

// maskSecrets returns a copy of config with every set secret value replaced
// by configSecretMask — regardless of runtime type, since masking is the
// primary cleartext boundary on the operator API and a schema may declare a
// non-string property secret (or a stored value may be malformed). Only
// zero values (nil, "", false, 0) pass through, so the UI can distinguish
// "not set" from "set".
func maskSecrets(config map[string]any, schema map[string]any) map[string]any {
	secrets := secretPropertyKeys(schema)
	out := make(map[string]any, len(config))
	for k, v := range config {
		if secrets[k] && !isZeroConfigValue(v) {
			out[k] = configSecretMask
			continue
		}
		out[k] = v
	}
	return out
}

// isZeroConfigValue reports whether v is an unset/zero config value that
// maskSecrets leaves unmasked: nil, empty string, false, or numeric zero.
func isZeroConfigValue(v any) bool {
	switch value := v.(type) {
	case nil:
		return true
	case string:
		return value == ""
	case bool:
		return !value
	default:
		if f, ok := numericValue(v); ok {
			return f == 0
		}
		return false
	}
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
// config_schema for pluginID. Deliberately a small JSON-Schema subset —
// required fields, primitive types (string/boolean/number/integer), and enum
// membership on declared properties. Undeclared keys and richer schema
// constructs are permitted: the schema is advisory authoring metadata, not a
// hard sandbox.
//
// A secret field that was left unchanged carries its own vault reference (an
// internal storage marker) rather than a user value — the cleartext was
// already validated when it was first set. Such a value is skipped, so a
// field declaring both `secret: true` and e.g. `enum: [...]` does not 400 on
// every save that keeps the mask (the ref is not one of the enum values).
func validateConfigSchema(pluginID string, config map[string]any, schema map[string]any) error {
	if err := checkRequiredKeys(config, schema); err != nil {
		return err
	}
	if err := checkSecretFieldsAreStrings(config, schema); err != nil {
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
		if isConfigVaultRef(pluginID, name, value) {
			continue
		}
		if err := checkPropertyValue(name, value, prop); err != nil {
			return err
		}
	}
	return nil
}

// checkSecretFieldsAreStrings rejects a non-string value on any declared
// secret field. Secrets are vault-backed and the vault stores strings; a
// numeric/boolean secret would otherwise skip vault storage in
// storeConfigSecrets and persist in cleartext — the exact invariant this
// feature exists to uphold. Credentials are strings in practice, so this is
// a contract, not a limitation: authors wanting a secret value declare it
// `type: string`. nil (unset) is allowed.
func checkSecretFieldsAreStrings(config map[string]any, schema map[string]any) error {
	for field := range secretPropertyKeys(schema) {
		value, present := config[field]
		if !present || value == nil {
			continue
		}
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%w: secret field %q must be a string", ErrConfigInvalid, field)
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
			if enumValueMatches(value, allowed) {
				return nil
			}
		}
		return fmt.Errorf("%w: field %q must be one of the declared enum values", ErrConfigInvalid, name)
	}
	return nil
}

// enumValueMatches compares a submitted value against a declared enum
// entry. Numeric values are normalized first: the manifest YAML decodes an
// enum entry like 5 as int, while the same value submitted over HTTP JSON
// arrives as float64 — reflect.DeepEqual would wrongly reject that valid
// selection. Everything else falls back to DeepEqual.
func enumValueMatches(value, allowed any) bool {
	vf, vOK := numericValue(value)
	af, aOK := numericValue(allowed)
	if vOK && aOK {
		return vf == af
	}
	return reflect.DeepEqual(value, allowed)
}

// numericValue converts the numeric types a config value can realistically
// carry (JSON float64, YAML int/int64/uint64) to float64 for comparison.
func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
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
