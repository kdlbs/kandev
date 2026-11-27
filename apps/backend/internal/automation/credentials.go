package automation

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCredentialsUnavailable         = errors.New("automation_credentials_unavailable")
	ErrCredentialStorageUnavailable   = errors.New("automation credential storage is temporarily unavailable")
	ErrCredentialChoiceUnavailable    = errors.New("automation credential choice is unavailable")
	ErrCredentialCreatorRequired      = errors.New("only the automation creator can change credential choices")
	ErrCredentialCreatorUnavailable   = errors.New("the automation creator is unavailable")
	ErrWorkspaceCredentialGrantDenied = errors.New("only the current workspace owner who owns the bound secret can manage this grant")
	ErrCredentialSettingsConflict     = errors.New("automation credential settings changed; reload and try again")
	ErrCredentialSettingsUnsupported  = errors.New("credential choices require a new automation-history task with fixed profiles")
)

// CredentialValidator checks only the selected repository references supplied
// by the caller. It must use the automation creator's identity and must not
// retain resolved values.
type CredentialValidator interface {
	ValidateAutomationCredentials(ctx context.Context, automation *Automation, repositoryIDs []string) error
}

// CredentialValidationError carries safe field details for editor and run
// recovery. Source values identify a visible profile or repository, never a
// secret record.
type CredentialValidationError struct {
	Field  string
	Source string
}

func (e *CredentialValidationError) Error() string {
	return fmt.Sprintf("%s: %s from %s is unavailable", ErrCredentialsUnavailable, e.Field, e.Source)
}

func (e *CredentialValidationError) Unwrap() error { return ErrCredentialsUnavailable }

func CredentialValidationDetails(err error) map[string]interface{} {
	var credentialErr *CredentialValidationError
	if !errors.As(err, &credentialErr) {
		return nil
	}
	return map[string]interface{}{
		"credential_error_code": ErrCredentialsUnavailable.Error(),
		"field":                 credentialErr.Field,
		"source":                credentialErr.Source,
	}
}

func (s *Service) SetCredentialValidator(validator CredentialValidator) {
	s.credentialValidator = validator
}

// ValidateAutomationCredentials checks current credential access at launch.
// Callers restore the fixed creator identity before they invoke this method.
func (s *Service) ValidateAutomationCredentials(
	ctx context.Context,
	a *Automation,
	repositoryIDs []string,
) error {
	if a == nil || a.TaskMode == TaskModeManagedConversation || s.credentialValidator == nil {
		return nil
	}
	return s.credentialValidator.ValidateAutomationCredentials(ctx, a, repositoryIDs)
}

func (s *Service) validateEnabledCredentials(ctx context.Context, a *Automation, repositoryIDs []string) error {
	if a == nil || !a.Enabled || a.TaskMode == TaskModeManagedConversation {
		return nil
	}
	return s.ValidateAutomationCredentials(ctx, a, repositoryIDs)
}

func automationExecutableUpdate(req *UpdateAutomationRequest) bool {
	return req != nil && (automationTargetUpdate(req) || automationProfileUpdate(req) ||
		automationPromptUpdate(req) || automationPolicyUpdate(req))
}

func automationTargetUpdate(req *UpdateAutomationRequest) bool {
	return req.WorkflowID != nil || req.WorkflowStepID != nil || req.Repositories != nil ||
		req.RepositoryIDs != nil || req.RepositoryMode != nil || req.TaskMode != nil ||
		req.ManagedDestination != nil
}

func automationProfileUpdate(req *UpdateAutomationRequest) bool {
	return req.AgentProfileID != nil || req.ExecutorProfileID != nil || req.MCPSelection != nil ||
		req.PluginTools != nil
}

func automationPromptUpdate(req *UpdateAutomationRequest) bool {
	return req.Prompt != nil || req.TaskTitleTemplate != nil
}

func automationPolicyUpdate(req *UpdateAutomationRequest) bool {
	return req.RetryPolicy != nil || req.ContinuationPolicy != nil || req.MaxConcurrentRuns != nil
}

func automationCandidate(existing *Automation, req *UpdateAutomationRequest) *Automation {
	candidate := *existing
	if req.WorkflowID != nil {
		candidate.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		candidate.WorkflowStepID = *req.WorkflowStepID
	}
	if req.AgentProfileID != nil {
		candidate.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		candidate.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		candidate.Prompt = *req.Prompt
	}
	if req.TaskTitleTemplate != nil {
		candidate.TaskTitleTemplate = *req.TaskTitleTemplate
	}
	if req.TaskMode != nil {
		candidate.TaskMode = *req.TaskMode
	}
	if req.ContinuationPolicy != nil {
		candidate.ContinuationPolicy = *req.ContinuationPolicy
	}
	if req.Enabled != nil {
		candidate.Enabled = *req.Enabled
	}
	return &candidate
}
