package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidWorkflowAgentOverrides identifies a create-time override map
	// that cannot be attached to the selected workflow.
	ErrInvalidWorkflowAgentOverrides = errors.New("invalid workflow agent overrides")
	// ErrMalformedWorkflowAgentOverrides identifies a persisted value that no
	// longer conforms to the task routing record.
	ErrMalformedWorkflowAgentOverrides = errors.New("malformed workflow agent overrides")
)

// WorkflowAgentOverrideBinding pins a replacement to one workflow step. The
// source profile is retained so the record remains inspectable after a
// workflow is edited and never has to rematch a current profile grouping.
type WorkflowAgentOverrideBinding struct {
	StepID               string `json:"step_id"`
	SourceProfileID      string `json:"source_profile_id"`
	ReplacementProfileID string `json:"replacement_profile_id"`
}

// WorkflowAgentOverrides is the task-owned routing record expanded from the
// grouped create-dialog choices.
type WorkflowAgentOverrides struct {
	WorkflowID string                         `json:"workflow_id"`
	Steps      []WorkflowAgentOverrideBinding `json:"steps"`
}

// NewWorkflowAgentOverrides validates and canonicalizes bindings. A self
// replacement is equivalent to no override and is omitted from the record.
func NewWorkflowAgentOverrides(workflowID string, bindings []WorkflowAgentOverrideBinding) (*WorkflowAgentOverrides, error) {
	workflowID = strings.TrimSpace(workflowID)
	if len(bindings) == 0 {
		return nil, nil
	}
	if workflowID == "" {
		return nil, fmt.Errorf("%w: workflow_id is required", ErrInvalidWorkflowAgentOverrides)
	}

	normalized := &WorkflowAgentOverrides{WorkflowID: workflowID}
	seenSteps := make(map[string]struct{}, len(bindings))
	for i, binding := range bindings {
		binding.StepID = strings.TrimSpace(binding.StepID)
		binding.SourceProfileID = strings.TrimSpace(binding.SourceProfileID)
		binding.ReplacementProfileID = strings.TrimSpace(binding.ReplacementProfileID)
		if binding.StepID == "" || binding.SourceProfileID == "" || binding.ReplacementProfileID == "" {
			return nil, fmt.Errorf("%w: binding %d requires step_id, source_profile_id, and replacement_profile_id", ErrInvalidWorkflowAgentOverrides, i)
		}
		if _, exists := seenSteps[binding.StepID]; exists {
			return nil, fmt.Errorf("%w: duplicate step_id %q", ErrInvalidWorkflowAgentOverrides, binding.StepID)
		}
		seenSteps[binding.StepID] = struct{}{}
		if binding.SourceProfileID == binding.ReplacementProfileID {
			continue
		}
		normalized.Steps = append(normalized.Steps, binding)
	}
	if len(normalized.Steps) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// ReplacementFor returns the task-specific replacement for a fixed step.
// WorkflowID is part of the lookup key so a stored map is inert after a task
// moves to another workflow until it returns to its original workflow.
func (o *WorkflowAgentOverrides) ReplacementFor(workflowID, stepID string) (string, bool) {
	if o == nil || strings.TrimSpace(workflowID) == "" || o.WorkflowID != workflowID {
		return "", false
	}
	for _, binding := range o.Steps {
		if binding.StepID == stepID {
			return binding.ReplacementProfileID, binding.ReplacementProfileID != ""
		}
	}
	return "", false
}

// Validate checks a decoded record without applying create-time self-mapping
// normalization. Persisted invalid data must surface as a routing error.
func (o *WorkflowAgentOverrides) Validate() error {
	if o == nil {
		return nil
	}
	o.WorkflowID = strings.TrimSpace(o.WorkflowID)
	if o.WorkflowID == "" {
		return fmt.Errorf("%w: workflow_id is required", ErrMalformedWorkflowAgentOverrides)
	}
	seenSteps := make(map[string]struct{}, len(o.Steps))
	for i := range o.Steps {
		binding := &o.Steps[i]
		binding.StepID = strings.TrimSpace(binding.StepID)
		binding.SourceProfileID = strings.TrimSpace(binding.SourceProfileID)
		binding.ReplacementProfileID = strings.TrimSpace(binding.ReplacementProfileID)
		if binding.StepID == "" || binding.SourceProfileID == "" || binding.ReplacementProfileID == "" {
			return fmt.Errorf("%w: binding %d requires step_id, source_profile_id, and replacement_profile_id", ErrMalformedWorkflowAgentOverrides, i)
		}
		if _, exists := seenSteps[binding.StepID]; exists {
			return fmt.Errorf("%w: duplicate step_id %q", ErrMalformedWorkflowAgentOverrides, binding.StepID)
		}
		seenSteps[binding.StepID] = struct{}{}
	}
	if len(o.Steps) == 0 {
		return fmt.Errorf("%w: steps is empty", ErrMalformedWorkflowAgentOverrides)
	}
	return nil
}

// EncodeWorkflowAgentOverrides returns the nullable database representation.
func EncodeWorkflowAgentOverrides(overrides *WorkflowAgentOverrides) (string, error) {
	if overrides == nil {
		return "", nil
	}
	if err := overrides.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(overrides)
	if err != nil {
		return "", fmt.Errorf("encode workflow agent overrides: %w", err)
	}
	return string(data), nil
}

// DecodeWorkflowAgentOverrides reads the nullable database representation.
func DecodeWorkflowAgentOverrides(raw string) (*WorkflowAgentOverrides, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		return nil, nil
	}
	var overrides WorkflowAgentOverrides
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedWorkflowAgentOverrides, err)
	}
	if err := overrides.Validate(); err != nil {
		return nil, err
	}
	return &overrides, nil
}
