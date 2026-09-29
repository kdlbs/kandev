package coordinator

import (
	"context"
	"errors"
)

var errSettingsNotImplemented = errors.New("coordinator: settings not implemented")

// SettingsError is a 400 the settings save answers with: a closed Code, and
// the Field it is about.
type SettingsError struct {
	Code    string
	Field   string
	Message string
}

func (e *SettingsError) Error() string { return e.Message }

// PolicyDeniedError is the approve re-check refusal: the named action is
// denied by the coordinator's stored policy.
type PolicyDeniedError struct {
	Action Action
}

func (e *PolicyDeniedError) Error() string { return "policy_denied" }

// GetSettings returns the coordinator's policy, revision and effective Watches.
func (s *Service) GetSettings(ctx context.Context, workspaceID, coordinatorID string) (*CoordinatorPhase2, error) {
	return nil, errSettingsNotImplemented
}

// SaveSettings applies a settings PUT body and returns the stored result.
func (s *Service) SaveSettings(ctx context.Context, workspaceID, coordinatorID string, body []byte) (*CoordinatorPhase2, error) {
	return nil, errSettingsNotImplemented
}

// WorkflowDeleted removes the deleted workflow's watch rows.
func (s *Service) WorkflowDeleted(ctx context.Context, workflowID string) error {
	return errSettingsNotImplemented
}
