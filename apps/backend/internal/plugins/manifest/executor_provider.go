package manifest

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	executorSchemaTypeBoolean = "boolean"
	executorSchemaTypeInteger = "integer"
	executorSchemaTypeKeyword = "type"
	executorSchemaTypeNumber  = "number"
	executorSchemaTypeObject  = "object"
)

const (
	// CurrentExecutorProviderContractVersion is the only provider RPC contract
	// version implemented by this host.
	CurrentExecutorProviderContractVersion = 1
	MaxExecutorProviderKeyBytes            = 64
	MaxExecutorProviderSchemaBytes         = 64 << 10
	maxExecutorProviders                   = 16
)

var executorProviderKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

const executorSchemaTitleKeyword = "title"
const executorRetentionBounded = "bounded"

func (m *Manifest) validateExecutorProviders() []error {
	if len(m.ExecutorProviders) == 0 {
		if m.Capabilities.ExecutorProvider {
			return []error{fmt.Errorf("capabilities.executor_provider requires executor_providers")}
		}
		return nil
	}

	var errs []error
	if !m.Capabilities.ExecutorProvider {
		errs = append(errs, fmt.Errorf("executor_providers require capabilities.executor_provider"))
	}
	if !m.IsManaged() {
		errs = append(errs, fmt.Errorf("executor_providers require runtime.type binary"))
	}
	if len(m.ExecutorProviders) > maxExecutorProviders {
		errs = append(errs, fmt.Errorf("executor_providers must declare at most %d providers", maxExecutorProviders))
	}

	seen := make(map[string]struct{}, len(m.ExecutorProviders))
	for index := range m.ExecutorProviders {
		provider := &m.ExecutorProviders[index]
		prefix := fmt.Sprintf("executor_providers[%d]", index)
		if !executorProviderKeyPattern.MatchString(provider.Key) || len(provider.Key) > MaxExecutorProviderKeyBytes {
			errs = append(errs, fmt.Errorf("%s.key must be a lowercase identifier of at most %d bytes", prefix, MaxExecutorProviderKeyBytes))
		} else if _, exists := seen[provider.Key]; exists {
			errs = append(errs, fmt.Errorf("duplicate executor provider key %q", provider.Key))
		} else {
			seen[provider.Key] = struct{}{}
		}
		if !validExecutorProviderLabel(provider.DisplayName, 100) {
			errs = append(errs, fmt.Errorf("%s.display_name must be a non-empty UTF-8 label of at most 100 characters", prefix))
		}
		if !validExecutorProviderLabel(provider.Description, 1024) {
			errs = append(errs, fmt.Errorf("%s.description must be a non-empty UTF-8 label of at most 1024 characters", prefix))
		}
		if provider.ContractVersion != CurrentExecutorProviderContractVersion {
			errs = append(errs, fmt.Errorf("%s.contract_version %d is unsupported; host supports %d", prefix, provider.ContractVersion, CurrentExecutorProviderContractVersion))
		}
		errs = append(errs, validateStateVersions(prefix, provider.SupportedStateVersions)...)
		errs = append(errs, validateExecutorSchema(prefix+".profile_schema", provider.ProfileSchema, true, false)...)
		errs = append(errs, validateExecutorSchema(prefix+".resource_state_schema", provider.ResourceStateSchema, false, true)...)
		errs = append(errs, validateExecutorProviderCapabilities(prefix, &provider.Capabilities)...)
		errs = append(errs, validateLocalizedMessages(prefix, provider.LocalizedMessages)...)
	}
	return errs
}

func validExecutorProviderLabel(value string, maxChars int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxChars
}

func validateStateVersions(prefix string, versions []int) []error {
	if len(versions) == 0 || len(versions) > 16 {
		return []error{fmt.Errorf("%s.supported_state_versions must contain 1-16 versions", prefix)}
	}
	seen := make(map[int]struct{}, len(versions))
	var errs []error
	for _, version := range versions {
		if version < 1 {
			errs = append(errs, fmt.Errorf("%s.supported_state_versions values must be positive", prefix))
			continue
		}
		if _, exists := seen[version]; exists {
			errs = append(errs, fmt.Errorf("%s.supported_state_versions contains duplicate version %d", prefix, version))
		}
		seen[version] = struct{}{}
	}
	return errs
}

func validateExecutorProviderCapabilities(prefix string, capabilities *ExecutorProviderCapabilities) []error {
	if capabilities.Retention == "" {
		capabilities.Retention = "unknown"
	}
	switch capabilities.Retention {
	case "unknown", "ephemeral", executorRetentionBounded, "persistent":
	default:
		return []error{fmt.Errorf("%s.capabilities.retention must be unknown, ephemeral, bounded, or persistent", prefix)}
	}
	if capabilities.MaximumLifetimeSecs < 0 || capabilities.MaximumLifetimeSecs > 365*24*60*60 {
		return []error{fmt.Errorf("%s.capabilities.maximum_lifetime_seconds must be between 0 and 31536000", prefix)}
	}
	if capabilities.Retention == executorRetentionBounded && capabilities.MaximumLifetimeSecs == 0 {
		return []error{fmt.Errorf("%s.capabilities.maximum_lifetime_seconds is required for bounded retention", prefix)}
	}
	if capabilities.Retention != executorRetentionBounded && capabilities.MaximumLifetimeSecs != 0 {
		return []error{fmt.Errorf("%s.capabilities.maximum_lifetime_seconds requires bounded retention", prefix)}
	}
	return nil
}

func validateLocalizedMessages(prefix string, messages map[string]string) []error {
	if len(messages) > 32 {
		return []error{fmt.Errorf("%s.localized_messages must contain at most 32 references", prefix)}
	}
	var errs []error
	for key, value := range messages {
		if !executorProviderKeyPattern.MatchString(key) || strings.TrimSpace(value) == "" || len(value) > 128 {
			errs = append(errs, fmt.Errorf("%s.localized_messages contains an invalid message reference", prefix))
		}
	}
	return errs
}

// validateExecutorSchema accepts the scalar-only schema subset shared by the
// provider form and its non-secret persisted resource state.
func validateExecutorSchema(prefix string, schema map[string]any, allowSecrets, requireClosed bool) []error {
	if len(schema) == 0 {
		return []error{fmt.Errorf("%s is required", prefix)}
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return []error{fmt.Errorf("%s must be JSON-compatible", prefix)}
	}
	if len(encoded) > MaxExecutorProviderSchemaBytes {
		return []error{fmt.Errorf("%s must be at most %d bytes", prefix, MaxExecutorProviderSchemaBytes)}
	}
	root, ok := stringMap(schema)
	if !ok {
		return []error{fmt.Errorf("%s must be an object schema", prefix)}
	}
	if err := validateExecutorSchemaRoot(prefix, root, requireClosed); err != nil {
		return []error{err}
	}
	properties, ok := stringMap(root["properties"])
	if !ok || len(properties) == 0 || len(properties) > 64 {
		return []error{fmt.Errorf("%s.properties must contain 1-64 fields", prefix)}
	}
	if err := validateExecutorSchemaProperties(prefix, properties, allowSecrets); err != nil {
		return []error{err}
	}
	if err := validateExecutorSchemaRequired(prefix, root["required"], properties); err != nil {
		return []error{err}
	}
	return nil
}

func validateExecutorSchemaRoot(prefix string, root map[string]any, requireClosed bool) error {
	for key := range root {
		if key != executorSchemaTypeKeyword && key != executorSchemaTitleKeyword && key != "description" && key != "properties" && key != "required" && key != "additionalProperties" {
			return fmt.Errorf("%s contains unsupported schema keyword %q", prefix, key)
		}
	}
	if root[executorSchemaTypeKeyword] != executorSchemaTypeObject {
		return fmt.Errorf("%s.type must be object", prefix)
	}
	additional, present := root["additionalProperties"]
	if requireClosed && !present {
		return fmt.Errorf("%s.additionalProperties must be false", prefix)
	}
	if present {
		closed, ok := additional.(bool)
		if !ok || closed {
			return fmt.Errorf("%s.additionalProperties must be false", prefix)
		}
	}
	return nil
}

func validateExecutorSchemaProperties(prefix string, properties map[string]any, allowSecrets bool) error {
	for name, raw := range properties {
		if !executorProviderKeyPattern.MatchString(name) {
			return fmt.Errorf("%s.properties contains an invalid field name", prefix)
		}
		property, ok := stringMap(raw)
		if !ok {
			return fmt.Errorf("%s.properties.%s must be a scalar field schema", prefix, name)
		}
		if err := validateExecutorSchemaProperty(prefix+".properties."+name, property, allowSecrets); err != nil {
			return err
		}
	}
	return nil
}

func validateExecutorSchemaRequired(prefix string, raw any, properties map[string]any) error {
	if raw == nil {
		return nil
	}
	values, ok := stringSlice(raw)
	if !ok || len(values) > len(properties) {
		return fmt.Errorf("%s.required must list declared fields", prefix)
	}
	seen := make(map[string]struct{}, len(values))
	for _, name := range values {
		if _, exists := properties[name]; !exists {
			return fmt.Errorf("%s.required references an undeclared field", prefix)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%s.required contains a duplicate field", prefix)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validateExecutorSchemaProperty(prefix string, property map[string]any, allowSecrets bool) error {
	for key := range property {
		if key != executorSchemaTypeKeyword && key != executorSchemaTitleKeyword && key != "description" && key != "enum" && key != "secret" && key != "format" && key != "minimum" && key != "maximum" {
			return fmt.Errorf("%s contains unsupported schema keyword %q", prefix, key)
		}
	}
	typeName, err := executorSchemaPropertyType(prefix, property[executorSchemaTypeKeyword])
	if err != nil {
		return err
	}
	if err := validateExecutorSchemaSecret(prefix, property, allowSecrets, typeName); err != nil {
		return err
	}
	if err := validateExecutorSchemaEnum(prefix, property["enum"], typeName); err != nil {
		return err
	}
	if err := validateExecutorSchemaNumericBounds(prefix, property, typeName); err != nil {
		return err
	}
	return nil
}

func validateExecutorSchemaNumericBounds(prefix string, property map[string]any, typeName string) error {
	minimumRaw, hasMinimum := property["minimum"]
	maximumRaw, hasMaximum := property["maximum"]
	if !hasMinimum && !hasMaximum {
		return nil
	}
	if typeName != executorSchemaTypeNumber && typeName != executorSchemaTypeInteger {
		return fmt.Errorf("%s numeric bounds require a number or integer field", prefix)
	}
	minimum, hasMinimum := schemaNumericValue(minimumRaw)
	maximum, hasMaximum := schemaNumericValue(maximumRaw)
	if _, present := property["minimum"]; present && !hasMinimum {
		return fmt.Errorf("%s.minimum must be a finite number", prefix)
	}
	if _, present := property["maximum"]; present && !hasMaximum {
		return fmt.Errorf("%s.maximum must be a finite number", prefix)
	}
	if hasMinimum && hasMaximum && minimum > maximum {
		return fmt.Errorf("%s.minimum must not exceed maximum", prefix)
	}
	return nil
}

func schemaNumericValue(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func executorSchemaPropertyType(prefix string, raw any) (string, error) {
	typeName, ok := raw.(string)
	if !ok || (typeName != automationStringType && typeName != executorSchemaTypeBoolean && typeName != executorSchemaTypeNumber && typeName != executorSchemaTypeInteger) {
		return "", fmt.Errorf("%s.type must be string, boolean, number, or integer", prefix)
	}
	return typeName, nil
}

func validateExecutorSchemaSecret(prefix string, property map[string]any, allowSecrets bool, typeName string) error {
	raw, present := property["secret"]
	if !present {
		return nil
	}
	secret, ok := raw.(bool)
	if !ok {
		return fmt.Errorf("%s.secret must be a boolean", prefix)
	}
	if secret && (!allowSecrets || typeName != automationStringType) {
		return fmt.Errorf("%s cannot declare secret state", prefix)
	}
	return nil
}

func validateExecutorSchemaEnum(prefix string, raw any, typeName string) error {
	if raw == nil {
		return nil
	}
	values, ok := anySlice(raw)
	if !ok || len(values) == 0 || len(values) > 64 {
		return fmt.Errorf("%s.enum must contain 1-64 scalar values", prefix)
	}
	for _, value := range values {
		if !scalarMatchesType(value, typeName) {
			return fmt.Errorf("%s.enum values must match the field type", prefix)
		}
	}
	return nil
}

func stringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			out[name] = value
		}
		return out, true
	default:
		return nil, false
	}
}

func stringSlice(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, len(typed))
		for index, value := range typed {
			name, ok := value.(string)
			if !ok {
				return nil, false
			}
			out[index] = name
		}
		return out, true
	default:
		return nil, false
	}
}

func anySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		out := make([]any, len(typed))
		for index, value := range typed {
			out[index] = value
		}
		return out, true
	default:
		return nil, false
	}
}

func scalarMatchesType(value any, typeName string) bool {
	switch typeName {
	case automationStringType:
		_, ok := value.(string)
		return ok
	case executorSchemaTypeBoolean:
		_, ok := value.(bool)
		return ok
	case executorSchemaTypeNumber:
		switch value.(type) {
		case float64, int, int64, uint64:
			return true
		default:
			return false
		}
	case executorSchemaTypeInteger:
		switch value.(type) {
		case int, int64, uint64:
			return true
		default:
			return false
		}
	default:
		return false
	}
}
