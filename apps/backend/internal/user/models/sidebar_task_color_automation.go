package models

import (
	"fmt"
	"unicode/utf8"
)

const (
	SidebarTaskColorDimensionWorkflowStep    = "workflow_step"
	SidebarTaskColorDimensionRepository      = "repository"
	SidebarTaskColorDimensionWorkflow        = "workflow"
	SidebarTaskColorDimensionExecutorProfile = "executor_profile"
	SidebarTaskColorDimensionTaskState       = "task_state"
	SidebarTaskColorDimensionPriority        = "priority"
	SidebarTaskColorDimensionOrigin          = "origin"

	SidebarTaskColorOutputFixed        = "fixed"
	SidebarTaskColorOutputWorkflowStep = "workflow_step"

	MaxSidebarTaskColorAutomationRules           = 50
	MaxSidebarTaskColorAutomationRuleIDBytes     = 64
	MaxSidebarTaskColorAutomationLabelCodePoints = 200
	MaxSidebarTaskColorAutomationIdentityBytes   = 512
	MaxSidebarTaskColorAutomationLocalPathBytes  = 4096
)

// SidebarTaskColorAutomation is a complete personal rule set for deriving
// task-row colors. It is stored inside UserSettings rather than on a task.
type SidebarTaskColorAutomation struct {
	Enabled bool                   `json:"enabled"`
	Rules   []SidebarTaskColorRule `json:"rules"`
}

type SidebarTaskColorRule struct {
	ID        string                    `json:"id"`
	Enabled   bool                      `json:"enabled"`
	Condition SidebarTaskColorCondition `json:"condition"`
	Output    SidebarTaskColorOutput    `json:"output"`
}

type SidebarTaskColorCondition struct {
	Dimension string `json:"dimension"`
	Value     any    `json:"value"`
	Label     string `json:"label"`
}

type SidebarTaskColorOutput struct {
	Kind  string `json:"kind"`
	Color string `json:"color,omitempty"`
}

// DefaultSidebarTaskColorAutomation returns the disabled settings value used
// for new users and malformed stored values.
func DefaultSidebarTaskColorAutomation() SidebarTaskColorAutomation {
	return SidebarTaskColorAutomation{Enabled: false, Rules: []SidebarTaskColorRule{}}
}

// ValidateSidebarTaskColorAutomation validates both the portable rule shape
// and the resource limits applied to user-supplied settings.
func ValidateSidebarTaskColorAutomation(value SidebarTaskColorAutomation) error {
	if len(value.Rules) > MaxSidebarTaskColorAutomationRules {
		return fmt.Errorf("automatic colors: max %d rules allowed", MaxSidebarTaskColorAutomationRules)
	}
	seenIDs := make(map[string]struct{}, len(value.Rules))
	for index, rule := range value.Rules {
		if rule.ID == "" {
			return fmt.Errorf("automatic colors rule %d: rule id is required", index+1)
		}
		if !utf8.ValidString(rule.ID) || len([]byte(rule.ID)) > MaxSidebarTaskColorAutomationRuleIDBytes {
			return fmt.Errorf("automatic colors rule %s: rule id exceeds %d bytes", rule.ID, MaxSidebarTaskColorAutomationRuleIDBytes)
		}
		if _, exists := seenIDs[rule.ID]; exists {
			return fmt.Errorf("automatic colors: duplicate rule id %q", rule.ID)
		}
		seenIDs[rule.ID] = struct{}{}

		if err := validateSidebarTaskColorCondition(rule.Condition, rule.Enabled); err != nil {
			return fmt.Errorf("automatic colors rule %s: %w", rule.ID, err)
		}
		if err := validateSidebarTaskColorOutput(rule.Condition.Dimension, rule.Output); err != nil {
			return fmt.Errorf("automatic colors rule %s: %w", rule.ID, err)
		}
	}
	return nil
}

func validateSidebarTaskColorCondition(condition SidebarTaskColorCondition, enabled bool) error {
	if !isSidebarTaskColorDimension(condition.Dimension) {
		return fmt.Errorf("unknown dimension %q", condition.Dimension)
	}
	if err := validateSidebarTaskColorLabel(condition.Label); err != nil {
		return err
	}
	if condition.Value == nil {
		if enabled {
			return fmt.Errorf("enabled rule must have a target")
		}
		return nil
	}

	switch condition.Dimension {
	case SidebarTaskColorDimensionWorkflowStep:
		return validateObjectTarget(condition.Value, "workspace_id", "step_id", false)
	case SidebarTaskColorDimensionWorkflow:
		return validateObjectTarget(condition.Value, "workspace_id", "workflow_id", false)
	case SidebarTaskColorDimensionRepository:
		return validateRepositoryTarget(condition.Value)
	case SidebarTaskColorDimensionExecutorProfile,
		SidebarTaskColorDimensionTaskState,
		SidebarTaskColorDimensionPriority,
		SidebarTaskColorDimensionOrigin:
		return validateBoundedString(condition.Value, "target", MaxSidebarTaskColorAutomationIdentityBytes)
	default:
		return fmt.Errorf("unknown dimension %q", condition.Dimension)
	}
}

func validateSidebarTaskColorOutput(dimension string, output SidebarTaskColorOutput) error {
	switch output.Kind {
	case SidebarTaskColorOutputFixed:
		if !isSidebarTaskColor(output.Color) {
			return fmt.Errorf("unsupported fixed color %q", output.Color)
		}
	case SidebarTaskColorOutputWorkflowStep:
		if dimension != SidebarTaskColorDimensionWorkflowStep {
			return fmt.Errorf("workflow-step output requires workflow_step dimension")
		}
		if output.Color != "" {
			return fmt.Errorf("workflow-step output cannot include a fixed color")
		}
	default:
		return fmt.Errorf("unknown output kind %q", output.Kind)
	}
	return nil
}

func validateObjectTarget(value any, firstKey, secondKey string, allowExtra bool) error {
	target, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("target must be an object")
	}
	if !allowExtra && len(target) != 2 {
		return fmt.Errorf("target must contain only %s and %s", firstKey, secondKey)
	}
	if err := validateObjectString(target, firstKey, MaxSidebarTaskColorAutomationIdentityBytes); err != nil {
		return err
	}
	if err := validateObjectString(target, secondKey, MaxSidebarTaskColorAutomationIdentityBytes); err != nil {
		return err
	}
	return nil
}

func validateRepositoryTarget(value any) error {
	target, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("repository target must be an object")
	}
	kind, ok := target["kind"].(string)
	if !ok || kind == "" {
		return fmt.Errorf("repository target kind is required")
	}
	switch kind {
	case "workspace":
		return validateWorkspaceRepositoryTarget(target)
	case "provider":
		return validateProviderRepositoryTarget(target)
	case "local":
		return validateLocalRepositoryTarget(target)
	default:
		return fmt.Errorf("unknown repository target kind %q", kind)
	}
}

func validateWorkspaceRepositoryTarget(target map[string]any) error {
	if len(target) != 3 {
		return fmt.Errorf("workspace repository target has an invalid shape")
	}
	if err := validateObjectString(target, "workspace_id", MaxSidebarTaskColorAutomationIdentityBytes); err != nil {
		return err
	}
	return validateObjectString(target, "repository_id", MaxSidebarTaskColorAutomationIdentityBytes)
}

func validateProviderRepositoryTarget(target map[string]any) error {
	if len(target) != 5 {
		return fmt.Errorf("provider repository target has an invalid shape")
	}
	for _, key := range []string{"provider_id", "host", "scope", "provider_repository_id"} {
		if err := validateObjectString(target, key, MaxSidebarTaskColorAutomationIdentityBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalRepositoryTarget(target map[string]any) error {
	if len(target) != 2 {
		return fmt.Errorf("local repository target has an invalid shape")
	}
	path, err := objectString(target, "path")
	if err != nil {
		return err
	}
	if len([]byte(path)) > MaxSidebarTaskColorAutomationLocalPathBytes {
		return fmt.Errorf("path exceeds %d bytes", MaxSidebarTaskColorAutomationLocalPathBytes)
	}
	return nil
}

func validateObjectString(target map[string]any, key string, maxBytes int) error {
	value, err := objectString(target, key)
	if err != nil {
		return err
	}
	return validateStringBytes(value, key, maxBytes)
}

func objectString(target map[string]any, key string) (string, error) {
	value, ok := target[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return value, nil
}

func validateBoundedString(value any, field string, maxBytes int) error {
	stringValue, ok := value.(string)
	if !ok || stringValue == "" {
		return fmt.Errorf("%s must be a non-empty string", field)
	}
	return validateStringBytes(stringValue, field, maxBytes)
}

func validateStringBytes(value, field string, maxBytes int) error {
	if !utf8.ValidString(value) || len([]byte(value)) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return nil
}

func validateSidebarTaskColorLabel(label string) error {
	if !utf8.ValidString(label) || utf8.RuneCountInString(label) > MaxSidebarTaskColorAutomationLabelCodePoints {
		return fmt.Errorf("label exceeds %d code points", MaxSidebarTaskColorAutomationLabelCodePoints)
	}
	return nil
}

func isSidebarTaskColorDimension(value string) bool {
	switch value {
	case SidebarTaskColorDimensionWorkflowStep,
		SidebarTaskColorDimensionRepository,
		SidebarTaskColorDimensionWorkflow,
		SidebarTaskColorDimensionExecutorProfile,
		SidebarTaskColorDimensionTaskState,
		SidebarTaskColorDimensionPriority,
		SidebarTaskColorDimensionOrigin:
		return true
	default:
		return false
	}
}

func isSidebarTaskColor(value string) bool {
	_, ok := sidebarTaskColorValues[value]
	return ok
}

var sidebarTaskColorValues = map[string]struct{}{
	"gray":   {},
	"red":    {},
	"orange": {},
	"yellow": {},
	"green":  {},
	"cyan":   {},
	"blue":   {},
	"indigo": {},
	"purple": {},
	"pink":   {},
}
