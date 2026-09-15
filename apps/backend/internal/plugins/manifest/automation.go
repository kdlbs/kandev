package manifest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const automationStringType = "string"

// AutomationCondition contributes a host-rendered, externally signed event condition.
type AutomationCondition struct {
	VerificationHeaders []string       `yaml:"verification_headers" json:"verification_headers"`
	Key                 string         `yaml:"key" json:"key"`
	AdapterKey          string         `yaml:"adapter_key" json:"adapter_key"`
	Access              string         `yaml:"access" json:"access"`
	ConfigVersion       int            `yaml:"config_version" json:"config_version"`
	LabelKey            string         `yaml:"label_key,omitempty" json:"label_key,omitempty"`
	DescriptionKey      string         `yaml:"description_key,omitempty" json:"description_key,omitempty"`
	Label               string         `yaml:"label" json:"label"`
	Description         string         `yaml:"description" json:"description"`
	ConfigSchema        map[string]any `yaml:"config_schema" json:"config_schema"`
	DefaultConfig       map[string]any `yaml:"default_config" json:"default_config"`
}

func (m *Manifest) validateAutomationConditions() []error {
	var errs []error
	seen := map[string]bool{}
	if len(m.AutomationConditions) > 32 {
		errs = append(errs, fmt.Errorf("at most 32 automation conditions are allowed"))
	}
	for _, c := range m.AutomationConditions {
		if !actionKeyPattern.MatchString(c.Key) || seen[c.Key] {
			errs = append(errs, fmt.Errorf("invalid or duplicate automation condition key %q", c.Key))
		}
		seen[c.Key] = true
		if !actionKeyPattern.MatchString(c.AdapterKey) || c.Access != "public" || c.ConfigVersion != 1 || c.Label == "" || len(c.Label) > 200 {
			errs = append(errs, fmt.Errorf("automation condition %q requires an adapter, explicit public access, schema version 1 and label", c.Key))
		}
		if !validVerificationHeaders(c.VerificationHeaders) {
			errs = append(errs, fmt.Errorf("invalid automation verification headers for %q", c.Key))
		}
		if !validAutomationSchema(c.ConfigSchema) {
			errs = append(errs, fmt.Errorf("automation condition %q requires an object config schema", c.Key))
		}
	}
	return errs
}

// Deliberately small schema dialect: no scripts, remote refs, passwords, or arbitrary widgets.
func validAutomationSchema(schema map[string]any) bool {
	raw, err := json.Marshal(schema)
	if err != nil || len(raw) > 65536 || schema["type"] != "object" {
		return false
	}
	for key := range schema {
		if !slices.Contains([]string{"type", "properties", "required", "additionalProperties"}, key) {
			return false
		}
	}
	if value, exists := schema["additionalProperties"]; exists && value != false {
		return false
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok && schema["properties"] != nil {
		return false
	}
	if len(props) > 32 {
		return false
	}
	for key, value := range props {
		field, ok := value.(map[string]any)
		if !ok || !actionKeyPattern.MatchString(key) || !validAutomationField(field) {
			return false
		}
	}
	return validAutomationRequired(schema, props)
}
func validAutomationRequired(schema, props map[string]any) bool {
	if value, exists := schema["required"]; exists {
		required, ok := value.([]any)
		if !ok {
			return false
		}
		for _, value := range required {
			key, ok := value.(string)
			if !ok || props[key] == nil {
				return false
			}
		}
	}
	return true
}
func validAutomationField(field map[string]any) bool {
	for key := range field {
		if key != "type" && key != "title" && key != "title_key" && key != "enum" && key != "items" {
			return false
		}
	}
	for _, key := range []string{"title", "title_key"} {
		if value, exists := field[key]; exists {
			str, ok := value.(string)
			if !ok || len(str) > 200 {
				return false
			}
		}
	}
	return validAutomationFieldType(field)
}
func validAutomationFieldType(field map[string]any) bool {
	switch field["type"] {
	case "array":
		items, ok := field["items"].(map[string]any)
		return ok && len(items) == 1 && items["type"] == automationStringType && field["enum"] == nil
	case "boolean":
		return field["enum"] == nil && field["items"] == nil
	case automationStringType:
		if field["items"] != nil {
			return false
		}
		return validAutomationEnum(field)

	default:
		return false
	}
}

func validVerificationHeaders(headers []string) bool {
	if len(headers) > 16 {
		return false
	}
	for _, header := range headers {
		if header != strings.ToLower(header) || !strings.HasPrefix(header, "x-") || strings.HasPrefix(header, "x-kandev-") || header == "x-webhook-secret" || len(header) > 100 {
			return false
		}
		for _, c := range header {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

func validAutomationEnum(field map[string]any) bool {
	value, exists := field["enum"]
	if !exists {
		return true
	}
	options, ok := value.([]any)
	if !ok || len(options) == 0 || len(options) > 100 {
		return false
	}
	for _, value := range options {
		str, ok := value.(string)
		if !ok || len(str) > 4096 {
			return false
		}
	}
	return true
}
