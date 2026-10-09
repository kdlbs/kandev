package lifecycle

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	pluginExecutorSchemaTypeInteger = "integer"
	pluginExecutorSchemaTypeNumber  = "number"
	pluginExecutorSchemaTypeKeyword = "type"
)

func validatePluginExecutorResourceForProvider(provider models.ExecutorProvider, resource *pluginsdk.ExecutorResourceDescriptor) error {
	if err := validatePluginExecutorResource(resource); err != nil {
		return err
	}
	if err := validatePluginExecutorResourceState(provider.ResourceStateSchema, resource.GetStateJson()); err != nil {
		return err
	}
	for _, supported := range provider.SupportedStateVersions {
		if supported > 0 && uint32(supported) == resource.GetStateVersion() {
			return validatePluginExecutorResourceExpiry(provider.Capabilities, resource)
		}
	}
	return errors.New("provider resource state version is unsupported")
}

func validatePluginExecutorResourceState(schema map[string]any, rawState string) error {
	properties, required, err := pluginExecutorResourceStateSchema(schema)
	if err != nil {
		return err
	}
	if rawState == "" {
		rawState = "{}"
	}
	var state map[string]any
	if err := json.Unmarshal([]byte(rawState), &state); err != nil || state == nil {
		return errors.New("provider resource state is not an object")
	}
	for name := range state {
		if _, declared := properties[name]; !declared {
			return errors.New("provider resource state contains an undeclared field")
		}
	}
	for _, name := range required {
		if _, present := state[name]; !present {
			return errors.New("provider resource state is missing a required field")
		}
	}
	for name, value := range state {
		if err := validatePluginExecutorResourceStateValue(value, properties[name]); err != nil {
			return err
		}
	}
	return nil
}

func pluginExecutorResourceStateSchema(schema map[string]any) (map[string]any, []string, error) {
	root, ok := pluginExecutorSchemaObject(schema)
	if !ok || root[pluginExecutorSchemaTypeKeyword] != "object" || root["additionalProperties"] != false {
		return nil, nil, errors.New("provider resource state schema is not a closed object")
	}
	properties, ok := pluginExecutorSchemaObject(root["properties"])
	if !ok || len(properties) == 0 {
		return nil, nil, errors.New("provider resource state schema has no fields")
	}
	required, ok := pluginExecutorSchemaStrings(root["required"])
	if root["required"] != nil && !ok {
		return nil, nil, errors.New("provider resource state schema has invalid required fields")
	}
	for _, name := range required {
		if _, exists := properties[name]; !exists {
			return nil, nil, errors.New("provider resource state schema requires an undeclared field")
		}
	}
	return properties, required, nil
}

func validatePluginExecutorResourceStateValue(value any, rawSchema any) error {
	field, ok := pluginExecutorSchemaObject(rawSchema)
	if !ok || field["secret"] == true {
		return errors.New("provider resource state schema contains an invalid field")
	}
	typeName, ok := field[pluginExecutorSchemaTypeKeyword].(string)
	if !ok || !pluginExecutorStateValueMatchesType(value, typeName) {
		return errors.New("provider resource state field has an invalid type")
	}
	if !pluginExecutorStateValueInEnum(value, field["enum"], typeName) {
		return errors.New("provider resource state field is outside its enum")
	}
	if err := validatePluginExecutorStateBounds(value, field, typeName); err != nil {
		return err
	}
	return nil
}

func validatePluginExecutorStateBounds(value any, schema map[string]any, typeName string) error {
	minimumRaw, hasMinimum := schema["minimum"]
	maximumRaw, hasMaximum := schema["maximum"]
	minimum, validMinimum := pluginExecutorSchemaNumber(minimumRaw)
	maximum, validMaximum := pluginExecutorSchemaNumber(maximumRaw)
	if (hasMinimum && !validMinimum) || (hasMaximum && !validMaximum) {
		return errors.New("provider resource state schema has an invalid numeric bound")
	}
	if (!hasMinimum && !hasMaximum) || typeName != pluginExecutorSchemaTypeNumber && typeName != pluginExecutorSchemaTypeInteger {
		return nil
	}
	numeric, _ := pluginExecutorSchemaNumber(value)
	if hasMinimum && numeric < minimum || hasMaximum && numeric > maximum {
		return errors.New("provider resource state field is outside its numeric bounds")
	}
	return nil
}

func validatePluginExecutorResourceExpiry(base models.ExecutorProviderCapabilities, resource *pluginsdk.ExecutorResourceDescriptor) error {
	retention := pluginExecutorEffectiveRetention(base.Retention, resource.GetCapabilities().GetRetention(), []string{resource.GetRetention()})
	if retention != pluginExecutorRetentionBounded {
		return nil
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, resource.GetExpiresAt())
	if err != nil {
		return errors.New("bounded provider resource requires an absolute expiry")
	}
	maximum := pluginExecutorEffectiveMaximumLifetime(base.MaximumLifetimeSecs, resource.GetCapabilities().GetMaximumLifetimeSeconds())
	if maximum <= 0 || expiresAt.After(time.Now().Add(time.Duration(maximum)*time.Second)) {
		return errors.New("provider resource expiry exceeds its maximum lifetime")
	}
	return nil
}

func pluginExecutorStateValueMatchesType(value any, typeName string) bool {
	switch typeName {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case pluginExecutorSchemaTypeNumber:
		_, ok := pluginExecutorSchemaNumber(value)
		return ok
	case pluginExecutorSchemaTypeInteger:
		number, ok := pluginExecutorSchemaNumber(value)
		return ok && math.Trunc(number) == number
	default:
		return false
	}
}

func pluginExecutorStateValueInEnum(value, rawEnum any, typeName string) bool {
	if rawEnum == nil {
		return true
	}
	values, ok := pluginExecutorSchemaValues(rawEnum)
	if !ok || len(values) == 0 {
		return false
	}
	for _, candidate := range values {
		if pluginExecutorStateEnumValueEqual(value, candidate, typeName) {
			return true
		}
	}
	return false
}

func pluginExecutorStateEnumValueEqual(left, right any, typeName string) bool {
	if typeName == pluginExecutorSchemaTypeNumber || typeName == pluginExecutorSchemaTypeInteger {
		leftNumber, leftOK := pluginExecutorSchemaNumber(left)
		rightNumber, rightOK := pluginExecutorSchemaNumber(right)
		return leftOK && rightOK && leftNumber == rightNumber
	}
	return left == right
}

func pluginExecutorSchemaObject(value any) (map[string]any, bool) {
	switch object := value.(type) {
	case map[string]any:
		return object, true
	case map[any]any:
		result := make(map[string]any, len(object))
		for key, value := range object {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			result[name] = value
		}
		return result, true
	default:
		return nil, false
	}
}

func pluginExecutorSchemaStrings(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []any:
		result := make([]string, len(values))
		for index, value := range values {
			name, ok := value.(string)
			if !ok {
				return nil, false
			}
			result[index] = name
		}
		return result, true
	default:
		return nil, false
	}
}

func pluginExecutorSchemaValues(value any) ([]any, bool) {
	switch values := value.(type) {
	case []any:
		return values, true
	case []string:
		result := make([]any, len(values))
		for index := range values {
			result[index] = values[index]
		}
		return result, true
	default:
		return nil, false
	}
}

func pluginExecutorSchemaNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint64:
		return float64(number), true
	case float64:
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	case float32:
		return float64(number), !math.IsNaN(float64(number)) && !math.IsInf(float64(number), 0)
	default:
		return 0, false
	}
}
