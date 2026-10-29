package sessioncapacity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

const inferenceBindingsField = "_inference_bindings"
const maxInferenceBindings = 10000

// InferenceBinding is the active ownership half of the existing capacity row.
// It contains no prompt, environment, credential or transport address.
type InferenceBinding struct {
	Key                    string `json:"key"`
	InstanceID             string `json:"instance_id"`
	OwnerID                string `json:"owner_id"`
	AttemptID              string `json:"attempt_id"`
	PreparationID          string `json:"preparation_id,omitempty"`
	OwnerKind              string `json:"owner_kind"`
	TaskID                 string `json:"task_id,omitempty"`
	SessionID              string `json:"session_id,omitempty"`
	ParentExecutionID      string `json:"parent_execution_id,omitempty"`
	LogicalProfileID       string `json:"logical_profile_id,omitempty"`
	ExecutionProfileID     string `json:"execution_profile_id,omitempty"`
	CallID                 string `json:"call_id,omitempty"`
	ReviewRunID            string `json:"review_run_id,omitempty"`
	AgentID                string `json:"agent_id,omitempty"`
	ExecutorType           string `json:"executor_type,omitempty"`
	Model                  string `json:"model,omitempty"`
	Grade                  string `json:"grade"`
	GradeKnown             bool   `json:"grade_known"`
	State                  string `json:"state"`
	ProviderKey            string `json:"provider_key,omitempty"`
	ProviderKnown          bool   `json:"provider_known"`
	ProviderFamilies       string `json:"provider_families,omitempty"`
	TerminalState          string `json:"terminal_state,omitempty"`
	TerminalPromptDispatch string `json:"terminal_prompt_dispatch,omitempty"`
}

type inferenceBindings struct {
	Version int                         `json:"version"`
	Active  map[string]InferenceBinding `json:"active"`
}

func decodeInferenceBindings(raw []byte, found bool) (map[string]InferenceBinding, error) {
	result := make(map[string]InferenceBinding)
	if !found {
		return result, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("%w: capacity owner JSON", ErrInvalidPersisted)
	}
	encoded, present := fields[inferenceBindingsField]
	if !present {
		return result, nil
	}
	var saved inferenceBindings
	if err := json.Unmarshal(encoded, &saved); err != nil || saved.Version != 1 || saved.Active == nil {
		return nil, fmt.Errorf("%w: inference owner state", ErrInvalidPersisted)
	}
	if len(saved.Active) > maxInferenceBindings {
		return nil, fmt.Errorf("%w: inference owner population exceeds retained bound", ErrInvalidPersisted)
	}
	for key, binding := range saved.Active {
		if !validInferenceBinding(key, binding) {
			return nil, fmt.Errorf("%w: inference owner identity or terminal proof", ErrInvalidPersisted)
		}
	}
	return saved.Active, nil
}

func (s *Store) inferenceCompareAndSwapStore() (rawCompareAndSwapStore, error) {
	if s == nil || s.raw == nil {
		return nil, ErrTargetUnavailable
	}
	cas, ok := s.raw.(rawCompareAndSwapStore)
	if !ok {
		return nil, ErrTargetUnavailable
	}
	return cas, nil
}

func (s *Store) LoadInferenceBindings(ctx context.Context) (map[string]InferenceBinding, error) {
	cas, err := s.inferenceCompareAndSwapStore()
	if err != nil {
		return nil, err
	}
	raw, found, err := cas.GetConsistent(ctx, SettingsKey)
	if err != nil {
		return nil, err
	}
	return decodeInferenceBindings(raw, found)
}

// UpdateInferenceBindings shares the settings row's compare-and-swap owner.
func (s *Store) UpdateInferenceBindings(ctx context.Context, update func(map[string]InferenceBinding) error) error {
	if update == nil {
		return fmt.Errorf("%w: inference owner update function is required", ErrValidation)
	}
	cas, err := s.inferenceCompareAndSwapStore()
	if err != nil {
		return err
	}
	for {
		raw, found, err := cas.GetConsistent(ctx, SettingsKey)
		if err != nil {
			return err
		}
		bindings, err := decodeInferenceBindings(raw, found)
		if err != nil {
			return err
		}
		if err := update(bindings); err != nil {
			return err
		}
		if len(bindings) > maxInferenceBindings {
			return fmt.Errorf("inference ownership population exceeds retained bound")
		}
		next, err := encodeInferenceBindings(raw, found, bindings)
		if err != nil {
			return err
		}
		if found && bytes.Equal(raw, next) {
			return nil
		}
		var expected []byte
		if found {
			expected = raw
		}
		swapped, err := cas.CompareAndSwap(ctx, SettingsKey, expected, next)
		if err != nil {
			return err
		}
		if swapped {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func encodeInferenceBindings(raw []byte, found bool, bindings map[string]InferenceBinding) ([]byte, error) {
	for key, binding := range bindings {
		if !validInferenceBinding(key, binding) {
			return nil, fmt.Errorf("%w: inference owner identity or terminal proof", ErrValidation)
		}
	}
	fields := make(map[string]json.RawMessage)
	if found {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(inferenceBindings{Version: 1, Active: bindings})
	if err != nil {
		return nil, err
	}
	fields[inferenceBindingsField] = encoded
	return json.Marshal(fields)
}

func preserveInferenceBindings(previous, next []byte) ([]byte, error) {
	if len(previous) == 0 {
		return next, nil
	}
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(previous, &before); err != nil {
		return nil, err
	}
	if state, found := before[inferenceBindingsField]; found {
		if _, err := decodeInferenceBindings(previous, true); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(next, &after); err != nil {
			return nil, err
		}
		after[inferenceBindingsField] = state
		return json.Marshal(after)
	}
	return next, nil
}

func inferenceOnlySettings(raw []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 1 {
		return false
	}
	_, found := fields[inferenceBindingsField]
	return found
}

func validInferenceBinding(key string, binding InferenceBinding) bool {
	if key == "" || binding.Key != key || binding.AttemptID == "" || binding.OwnerID == "" || binding.InstanceID == "" {
		return false
	}
	switch binding.TerminalState {
	case "", "terminated", "not_started":
		return true
	default:
		return false
	}
}
