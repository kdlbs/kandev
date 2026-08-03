package models

import (
	"fmt"
	"strings"
)

// OnEnterConfigureSession applies a conditional configuration to the task's
// original session without selecting or creating another session.
const OnEnterConfigureSession OnEnterActionType = "configure_session"

// ConfigureSessionOperation describes the effect of a conditional session
// configuration rule.
type ConfigureSessionOperation string

const (
	ConfigureSessionSet             ConfigureSessionOperation = "set"
	ConfigureSessionKeep            ConfigureSessionOperation = "keep"
	ConfigureSessionRestoreOriginal ConfigureSessionOperation = "restore_original"
)

// ConfigureSessionRule is the typed form of a configure_session action rule.
// The persisted workflow contract remains the existing action Config JSON map;
// this type is used after validation at runtime boundaries.
type ConfigureSessionRule struct {
	AgentName     string
	Operation     ConfigureSessionOperation
	Model         string
	ConfigOptions map[string]string
}

// ParseConfigureSessionRules validates and decodes a configure_session action.
func ParseConfigureSessionRules(action OnEnterAction) ([]ConfigureSessionRule, error) {
	if action.Type != OnEnterConfigureSession {
		return nil, fmt.Errorf("action type %q is not configure_session", action.Type)
	}
	if action.Config == nil {
		return nil, fmt.Errorf("configure_session requires a rules array")
	}
	rawRules, ok := action.Config["rules"]
	if !ok {
		return nil, fmt.Errorf("configure_session requires a rules array")
	}

	var items []interface{}
	switch typed := rawRules.(type) {
	case []interface{}:
		items = typed
	case []map[string]interface{}:
		items = make([]interface{}, len(typed))
		for i := range typed {
			items[i] = typed[i]
		}
	case []ConfigureSessionRule:
		items = make([]interface{}, len(typed))
		for i := range typed {
			items[i] = typed[i]
		}
	default:
		return nil, fmt.Errorf("configure_session rules has unexpected type %T", rawRules)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("configure_session requires at least one rule")
	}

	rules := make([]ConfigureSessionRule, 0, len(items))
	seenFamilies := make(map[string]struct{}, len(items))
	for index, item := range items {
		rule, err := parseConfigureSessionRule(item)
		if err != nil {
			return nil, fmt.Errorf("configure_session rule %d: %w", index, err)
		}
		if _, exists := seenFamilies[rule.AgentName]; exists {
			return nil, fmt.Errorf("configure_session has duplicate agent_name %q", rule.AgentName)
		}
		seenFamilies[rule.AgentName] = struct{}{}
		rules = append(rules, rule)
	}
	return rules, nil
}

// ValidateWorkflowStep validates all on-enter invariants that must hold for
// both API writes and workflow imports.
func ValidateWorkflowStep(step *WorkflowStep) error {
	if step == nil {
		return fmt.Errorf("workflow step is required")
	}
	return ValidateStepEvents(step.Events, step.AgentProfileID != "")
}

// ValidateStepEvents validates on-enter action invariants for a step shape
// that does not carry a full WorkflowStep, such as a portable export.
func ValidateStepEvents(events StepEvents, hasAgentProfile bool) error {
	configureCount := 0
	for _, action := range events.OnEnter {
		switch action.Type {
		case OnEnterConfigureSession:
			configureCount++
			if hasAgentProfile {
				return fmt.Errorf("configure_session cannot be combined with agent_profile_id")
			}
			if _, err := ParseConfigureSessionRules(action); err != nil {
				return err
			}
		case OnEnterSetSessionMode:
			if mode, _ := action.Config["mode"].(string); strings.TrimSpace(mode) == "" {
				return fmt.Errorf("set_session_mode requires a non-empty string \"mode\" config")
			}
		}
	}
	if configureCount > 1 {
		return fmt.Errorf("workflow step may contain at most one configure_session action")
	}
	return nil
}

func parseConfigureSessionRule(raw interface{}) (ConfigureSessionRule, error) {
	if typed, ok := raw.(ConfigureSessionRule); ok {
		return validateConfigureSessionRule(typed, typed.Model != "", typed.ConfigOptions != nil)
	}
	values, ok := raw.(map[string]interface{})
	if !ok {
		return ConfigureSessionRule{}, fmt.Errorf("must be an object")
	}

	agentName, ok := values["agent_name"].(string)
	if !ok || strings.TrimSpace(agentName) == "" {
		return ConfigureSessionRule{}, fmt.Errorf("agent_name must be a non-empty string")
	}
	operationValue, ok := values["operation"].(string)
	if !ok || strings.TrimSpace(operationValue) == "" {
		return ConfigureSessionRule{}, fmt.Errorf("operation must be a non-empty string")
	}

	rule := ConfigureSessionRule{
		AgentName: strings.TrimSpace(agentName),
		Operation: ConfigureSessionOperation(operationValue),
	}
	modelValue, hasModel := values["model"]
	if hasModel {
		model, ok := modelValue.(string)
		if !ok {
			return ConfigureSessionRule{}, fmt.Errorf("model must be a string")
		}
		rule.Model = model
	}
	optionsValue, hasOptions := values["config_options"]
	if hasOptions {
		options, err := parseConfigureSessionOptions(optionsValue)
		if err != nil {
			return ConfigureSessionRule{}, err
		}
		rule.ConfigOptions = options
	}
	return validateConfigureSessionRule(rule, hasModel, hasOptions)
}

func parseConfigureSessionOptions(raw interface{}) (map[string]string, error) {
	options := make(map[string]string)
	switch typed := raw.(type) {
	case map[string]string:
		for key, value := range typed {
			options[key] = value
		}
	case map[string]interface{}:
		for key, value := range typed {
			stringValue, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("config_options[%q] must be a string", key)
			}
			options[key] = stringValue
		}
	default:
		return nil, fmt.Errorf("config_options must be an object")
	}
	return options, nil
}

func validateConfigureSessionRule(rule ConfigureSessionRule, hasModel, hasOptions bool) (ConfigureSessionRule, error) {
	rule.AgentName = strings.TrimSpace(rule.AgentName)
	rule.Model = strings.TrimSpace(rule.Model)
	if rule.AgentName == "" {
		return ConfigureSessionRule{}, fmt.Errorf("agent_name must be a non-empty string")
	}
	if rule.ConfigOptions == nil {
		rule.ConfigOptions = map[string]string{}
	}
	for key, value := range rule.ConfigOptions {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return ConfigureSessionRule{}, fmt.Errorf("config_options keys and values must be non-empty strings")
		}
	}

	switch rule.Operation {
	case ConfigureSessionSet:
		if strings.TrimSpace(rule.Model) == "" && len(rule.ConfigOptions) == 0 {
			return ConfigureSessionRule{}, fmt.Errorf("set requires a non-empty model or config_options")
		}
	case ConfigureSessionKeep, ConfigureSessionRestoreOriginal:
		if hasModel || hasOptions || strings.TrimSpace(rule.Model) != "" || len(rule.ConfigOptions) > 0 {
			return ConfigureSessionRule{}, fmt.Errorf("%s does not accept model or config_options", rule.Operation)
		}
	default:
		return ConfigureSessionRule{}, fmt.Errorf("operation %q must be set, keep, or restore_original", rule.Operation)
	}
	return rule, nil
}
