// Package profilesnapshot builds the provider-facing profile snapshot. The
// string config is canonical; the typed and reference forms are derived from
// it only when the provider negotiated the matching capability.
package profilesnapshot

import (
	"fmt"
	"math"
	"strconv"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Capability names that switch on the additive snapshot fields.
const (
	CapabilityTypedProfileValues = "typed_profile_values"
	CapabilitySecretReferences   = "secret_references"
)

// Input is everything the snapshot is derived from.
type Input struct {
	ProfileID    string
	Config       map[string]string
	SecretValues map[string]string
	// SecretRefs maps a secret property to the store ID the profile holds.
	SecretRefs map[string]string
	Schema     map[string]any
	// Capabilities lists the negotiated capability names.
	Capabilities []string
}

// InvalidTypeError reports a stored value that does not parse as its schema
// type. It carries the property name and never the value.
type InvalidTypeError struct {
	Field string
}

func (e *InvalidTypeError) Error() string {
	return fmt.Sprintf("invalid_type: profile field %q does not match its schema type", e.Field)
}

// Build returns the snapshot for one operation.
func Build(in Input) (*pluginsdk.ExecutorProfileSnapshot, error) {
	snapshot := &pluginsdk.ExecutorProfileSnapshot{ProfileId: in.ProfileID, Config: in.Config, SecretValues: in.SecretValues}
	properties, _ := in.Schema["properties"].(map[string]any)
	if hasCapability(in.Capabilities, CapabilityTypedProfileValues) {
		typed, err := typedConfig(in.Config, properties)
		if err != nil {
			return nil, err
		}
		snapshot.TypedConfig = typed
	}
	if hasCapability(in.Capabilities, CapabilitySecretReferences) {
		snapshot.SecretRefs = secretRefs(in, properties)
	}
	return snapshot, nil
}

func hasCapability(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func typedConfig(config map[string]string, properties map[string]any) (map[string]*pluginsdk.ExecutorConfigValue, error) {
	typed := make(map[string]*pluginsdk.ExecutorConfigValue, len(config))
	for key, text := range config {
		property, declared := properties[key].(map[string]any)
		if !declared {
			continue
		}
		if secret, _ := property["secret"].(bool); secret {
			continue
		}
		value, ok := typedValue(text, property["type"])
		if !ok {
			return nil, &InvalidTypeError{Field: key}
		}
		typed[key] = value
	}
	return typed, nil
}

func typedValue(text string, typeName any) (*pluginsdk.ExecutorConfigValue, bool) {
	switch typeName {
	case "string":
		return &pluginsdk.ExecutorConfigValue{Value: &pluginsdk.ExecutorConfigValue_StringValue{StringValue: text}}, true
	case "boolean":
		parsed, err := strconv.ParseBool(text)
		if err != nil || (text != "true" && text != "false") {
			return nil, false
		}
		return &pluginsdk.ExecutorConfigValue{Value: &pluginsdk.ExecutorConfigValue_BoolValue{BoolValue: parsed}}, true
	case "integer":
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, false
		}
		return &pluginsdk.ExecutorConfigValue{Value: &pluginsdk.ExecutorConfigValue_IntValue{IntValue: parsed}}, true
	case "number":
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, false
		}
		return &pluginsdk.ExecutorConfigValue{Value: &pluginsdk.ExecutorConfigValue_DoubleValue{DoubleValue: parsed}}, true
	}
	return nil, false
}

func secretRefs(in Input, properties map[string]any) map[string]*pluginsdk.ExecutorSecretRef {
	refs := make(map[string]*pluginsdk.ExecutorSecretRef, len(in.SecretRefs))
	for key, id := range in.SecretRefs {
		property, declared := properties[key].(map[string]any)
		if !declared || id == "" || in.SecretValues[key] == "" {
			continue
		}
		if secret, _ := property["secret"].(bool); secret {
			refs[key] = &pluginsdk.ExecutorSecretRef{Id: id}
		}
	}
	return refs
}
