package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	UpdateOutcomeRunning     = "running"
	UpdateOutcomeSucceeded   = "succeeded"
	UpdateOutcomeFailed      = "failed"
	UpdateOutcomeInterrupted = "interrupted"
)

// UpdateOutcome is the retained automatic-update result after job logs expire.
type UpdateOutcome struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`
	PreviousVersion string    `json:"previous_version"`
	TargetVersion   string    `json:"target_version"`
	FinishedAt      time.Time `json:"finished_at"`
}

type AutoUpdatePolicy struct {
	RuntimeID        string         `json:"runtime_id"`
	Enabled          bool           `json:"enabled"`
	AttemptedVersion string         `json:"attempted_version,omitempty"`
	Outcome          *UpdateOutcome `json:"outcome,omitempty"`
}

type AutoUpdateStore struct{ settings SettingsStore }

func NewAutoUpdateStore(settings SettingsStore) *AutoUpdateStore {
	return &AutoUpdateStore{settings: settings}
}
func (s *AutoUpdateStore) Get(ctx context.Context, agentID, runtimeID string) (AutoUpdatePolicy, error) {
	policy := AutoUpdatePolicy{RuntimeID: runtimeID}
	if s == nil || s.settings == nil {
		return policy, errors.New("automatic runtime settings unavailable")
	}
	raw, found, err := s.settings.Get(ctx, "agent_runtime.auto."+agentID)
	if err != nil || !found {
		return policy, err
	}
	var saved AutoUpdatePolicy
	if err := json.Unmarshal(raw, &saved); err != nil {
		return policy, err
	}
	if saved.RuntimeID != runtimeID {
		return policy, nil
	}
	return saved, nil
}

func (s *AutoUpdateStore) Save(ctx context.Context, agentID string, policy AutoUpdatePolicy) error {
	if s == nil || s.settings == nil {
		return errors.New("automatic runtime settings unavailable")
	}
	if agentID == "" || policy.RuntimeID == "" {
		return errors.New("automatic runtime identity is required")
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return s.settings.Save(ctx, "agent_runtime.auto."+agentID, raw)
}
