package automation

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type CredentialCreatorAccess func(context.Context, string, string, string) error

func (s *Service) SetCredentialCreatorAccessChecker(checker CredentialCreatorAccess) {
	s.credentialCreatorAccess = checker
}

func (s *Service) GrantWorkspaceSecret(
	ctx context.Context,
	automationID string,
	req *WorkspaceCredentialGrantRequest,
) (*Automation, error) {
	a, ownerID, ownerUpdatedAt, err := s.workspaceCredentialGrantContext(ctx, automationID)
	if err != nil {
		return nil, err
	}
	if err := validateWorkspaceCredentialGrantRequest(req, ownerUpdatedAt); err != nil {
		return nil, err
	}
	if err := validateCredentialChoiceSettings(a, a.Triggers); err != nil {
		return nil, err
	}
	if err := s.validateWorkspaceCredentialGrantCreator(ctx, a); err != nil {
		return nil, ErrCredentialCreatorUnavailable
	}
	binding, secret, err := s.resolveGrantableWorkspaceCredential(ctx, a, ownerID, req)
	if err != nil {
		return nil, err
	}
	choice := AutomationCredentialChoice{
		AutomationID: automationID, Kind: CredentialChoiceWorkspace, RepositoryID: req.RepositoryID,
		Key: req.Key, SecretID: secret.ID, SourceSecretID: binding.SecretID,
		SourceUpdatedAt: binding.UpdatedAt, WorkspaceOwnerUpdatedAt: &ownerUpdatedAt, CreatorUserID: a.CreatedByUserID,
		GrantorUserID: ownerID, State: CredentialChoiceGranted, SecretName: secret.Name,
	}
	if err := s.store.SaveWorkspaceCredentialGrant(ctx, choice, req.ExpectedAutomationRevision, req.ExpectedCredentialRevision); err != nil {
		return nil, err
	}
	return s.GetAutomation(ctx, automationID)
}

func (s *Service) workspaceCredentialGrantContext(
	ctx context.Context,
	automationID string,
) (*Automation, string, time.Time, error) {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return nil, "", time.Time{}, err
	}
	a, err := s.store.GetAutomation(ctx, automationID)
	if err != nil || a == nil {
		return nil, "", time.Time{}, errors.Join(ErrAutomationNotFound, err)
	}
	ownerID, err := s.requireWorkspaceCredentialOwner(ctx, a.WorkspaceID)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	ownerUpdatedAt, err := s.workspaceOwnerRevision(ctx, a.WorkspaceID)
	return a, ownerID, ownerUpdatedAt, err
}

func validateWorkspaceCredentialGrantRequest(req *WorkspaceCredentialGrantRequest, ownerUpdatedAt time.Time) error {
	if req == nil || req.RepositoryID == "" || req.Key == "" || req.ExpectedCredentialRevision < 0 ||
		req.ExpectedBindingUpdatedAt.IsZero() || req.ExpectedWorkspaceOwnerUpdatedAt.IsZero() {
		return ErrCredentialChoiceUnavailable
	}
	if !ownerUpdatedAt.Equal(req.ExpectedWorkspaceOwnerUpdatedAt) {
		return ErrCredentialSettingsConflict
	}
	return nil
}

func (s *Service) validateWorkspaceCredentialGrantCreator(ctx context.Context, a *Automation) error {
	if s.credentialCreatorAccess == nil {
		return ErrCredentialCreatorUnavailable
	}
	return s.credentialCreatorAccess(ctx, a.WorkspaceID, a.CreatedByUserID, a.ExecutorProfileID)
}

func (s *Service) resolveGrantableWorkspaceCredential(
	ctx context.Context,
	a *Automation,
	ownerID string,
	req *WorkspaceCredentialGrantRequest,
) (taskmodels.RepositorySecretBinding, *secrets.Secret, error) {
	if !automationSelectsRepository(a, req.RepositoryID) || s.credentialRepositoryReader == nil || s.credentialSecretStore == nil {
		return taskmodels.RepositorySecretBinding{}, nil, ErrCredentialChoiceUnavailable
	}
	repository, err := s.credentialRepositoryReader.GetRepository(ctx, req.RepositoryID)
	if err != nil {
		return taskmodels.RepositorySecretBinding{}, nil, ErrCredentialStorageUnavailable
	}
	binding, ok := findRepositorySecretBinding(repository, a.WorkspaceID, req.Key)
	if !ok {
		return taskmodels.RepositorySecretBinding{}, nil, ErrCredentialChoiceUnavailable
	}
	if !binding.UpdatedAt.Equal(req.ExpectedBindingUpdatedAt) {
		return taskmodels.RepositorySecretBinding{}, nil, ErrCredentialSettingsConflict
	}
	secret, err := s.credentialSecretStore.GetForWorkspace(ctx, binding.SecretID, a.WorkspaceID)
	if err != nil {
		return taskmodels.RepositorySecretBinding{}, nil, workspaceGrantSecretReadError(err)
	}
	if !workspaceSecretOwnedBy(secret, ownerID, a.WorkspaceID) {
		return taskmodels.RepositorySecretBinding{}, nil, ErrWorkspaceCredentialGrantDenied
	}
	if _, err := s.credentialSecretStore.RevealForWorkspace(ctx, secret.ID, a.WorkspaceID); err != nil {
		return taskmodels.RepositorySecretBinding{}, nil, workspaceGrantSecretReadError(err)
	}
	return binding, secret, nil
}

func workspaceGrantSecretReadError(err error) error {
	if errors.Is(err, secrets.ErrNotFound) {
		return ErrWorkspaceCredentialGrantDenied
	}
	return ErrCredentialStorageUnavailable
}

func workspaceSecretOwnedBy(secret *secrets.Secret, ownerID, workspaceID string) bool {
	return secret != nil && secret.Scope == secrets.ScopeWorkspace && secret.WorkspaceID == workspaceID &&
		secret.OwnerUserID == ownerID
}

func (s *Service) RevokeWorkspaceSecret(
	ctx context.Context,
	automationID string,
	req *WorkspaceCredentialGrantRequest,
) (*Automation, error) {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return nil, err
	}
	a, err := s.store.GetAutomation(ctx, automationID)
	if err != nil || a == nil {
		return nil, errors.Join(ErrAutomationNotFound, err)
	}
	if _, err := s.requireWorkspaceCredentialOwner(ctx, a.WorkspaceID); err != nil {
		return nil, err
	}
	if req == nil || req.RepositoryID == "" || req.Key == "" || req.ExpectedCredentialRevision <= 0 {
		return nil, ErrCredentialChoiceUnavailable
	}
	if err := s.store.RevokeWorkspaceCredentialGrant(
		ctx, automationID, req.RepositoryID, req.Key,
		req.ExpectedAutomationRevision, req.ExpectedCredentialRevision,
	); err != nil {
		return nil, err
	}
	return s.GetAutomation(ctx, automationID)
}

func (s *Service) ResolveAutomationCredentialBindingValue(
	ctx context.Context,
	taskID, automationID, runID, repositoryID, key string,
) (string, error) {
	a, run, choices, err := s.CredentialChoicesForRun(ctx, runID)
	if err != nil || !protectedCredentialRunMatches(a, run, automationID, taskID) {
		return "", credentialUnavailable(key)
	}
	if err := ValidateCredentialChoiceSettingsForRuntime(a, run.TriggerType); err != nil {
		return "", credentialUnavailable(key)
	}
	selected := workspaceCredentialChoice(choices, repositoryID, key)
	if selected == nil || !workspaceCredentialChoiceGranted(*selected) {
		return "", credentialUnavailable(key)
	}
	return s.resolveWorkspaceCredentialChoiceValue(ctx, a, *selected)
}

func protectedCredentialRunMatches(a *Automation, run *AutomationRun, automationID, taskID string) bool {
	return a != nil && run != nil && a.ID == automationID && run.TaskID == taskID &&
		run.ProtectedCreatorUserID != "" && run.ProtectedCreatorUserID == a.CreatedByUserID
}

func workspaceCredentialChoice(choices []AutomationCredentialChoice, repositoryID, key string) *AutomationCredentialChoice {
	for index := range choices {
		choice := &choices[index]
		if choice.Kind == CredentialChoiceWorkspace && choice.RepositoryID == repositoryID && choice.Key == key {
			return choice
		}
	}
	return nil
}

func workspaceCredentialChoiceGranted(choice AutomationCredentialChoice) bool {
	return choice.State == CredentialChoiceGranted && choice.SecretID != "" && choice.SecretID == choice.SourceSecretID
}

// ResolveAutomationCredentialChoice returns the current approved binding and
// its value so enabled-save validation uses runtime grant authority.
func (s *Service) ResolveAutomationCredentialChoice(
	ctx context.Context,
	a *Automation,
	repositoryID, key string,
) (AutomationCredentialChoice, string, error) {
	if a == nil || repositoryID == "" || key == "" {
		return AutomationCredentialChoice{}, "", credentialUnavailable(key)
	}
	choices, err := s.store.credentialChoices(ctx, a.ID)
	if err != nil {
		return AutomationCredentialChoice{}, "", ErrCredentialStorageUnavailable
	}
	for _, choice := range choices {
		if choice.Kind == CredentialChoiceWorkspace && choice.RepositoryID == repositoryID && choice.Key == key {
			value, err := s.resolveWorkspaceCredentialChoiceValue(ctx, a, choice)
			return choice, value, err
		}
	}
	return AutomationCredentialChoice{}, "", credentialUnavailable(key)
}

func (s *Service) resolveWorkspaceCredentialChoiceValue(
	ctx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
) (string, error) {
	key := choice.Key
	if !workspaceCredentialChoiceMatchesAutomation(a, choice) {
		return "", credentialUnavailable(key)
	}
	var value string
	err := s.store.WithWorkspaceCredentialGrant(ctx, a.ID, choice.RepositoryID, key, choice.Revision, func(current AutomationCredentialChoice) error {
		if !workspaceCredentialChoiceMatchesAutomation(a, current) {
			return credentialUnavailable(key)
		}
		var resolveErr error
		value, resolveErr = s.resolveCurrentWorkspaceCredential(ctx, a, current)
		return resolveErr
	})
	if err != nil {
		return "", workspaceCredentialResolutionError(key, err)
	}
	if value == "" {
		return "", credentialUnavailable(key)
	}
	return value, nil
}

func workspaceCredentialChoiceMatchesAutomation(a *Automation, choice AutomationCredentialChoice) bool {
	return a != nil && choice.Kind == CredentialChoiceWorkspace && choice.State == CredentialChoiceGranted &&
		choice.AutomationID == a.ID && choice.RepositoryID != "" && choice.CreatorUserID == a.CreatedByUserID &&
		choice.GrantorUserID != "" && choice.SecretID != "" && choice.SecretID == choice.SourceSecretID
}

func workspaceCredentialResolutionError(key string, err error) error {
	if errors.Is(err, ErrCredentialStorageUnavailable) {
		return ErrCredentialStorageUnavailable
	}
	if errors.Is(err, ErrCredentialChoiceUnavailable) || errors.Is(err, ErrCredentialsUnavailable) {
		return credentialUnavailable(key)
	}
	return ErrCredentialStorageUnavailable
}

// ProtectedAutomationRunUseAccessForTask checks the current authority before
// a protected task accepts another prompt or runtime attachment.
func (s *Service) ProtectedAutomationRunUseAccessForTask(
	ctx context.Context,
	taskID, userID string,
) (bool, bool, error) {
	run, err := s.store.ProtectedRunForTask(ctx, taskID)
	if err != nil || run == nil {
		return false, false, err
	}
	if run.ProtectedCreatorUserID != userID {
		return true, false, nil
	}
	a, err := s.store.GetAutomation(ctx, run.AutomationID)
	if err != nil {
		return true, false, err
	}
	if a == nil {
		return true, false, nil
	}
	choices, err := s.store.credentialChoices(ctx, a.ID)
	if err != nil {
		return true, false, err
	}
	for _, choice := range choices {
		if choice.Kind != CredentialChoiceWorkspace {
			continue
		}
		if err := s.validateCurrentWorkspaceCredentialChoice(ctx, a, choice); err != nil {
			if errors.Is(err, ErrCredentialStorageUnavailable) {
				return true, false, err
			}
			return true, false, nil
		}
	}
	return true, true, nil
}

func (s *Service) validateCurrentWorkspaceCredentialChoice(
	ctx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
) error {
	key := choice.Key
	if a == nil || choice.Kind != CredentialChoiceWorkspace || choice.State != CredentialChoiceGranted ||
		choice.AutomationID != a.ID || choice.RepositoryID == "" || choice.CreatorUserID != a.CreatedByUserID ||
		choice.GrantorUserID == "" || choice.SecretID == "" || choice.SecretID != choice.SourceSecretID {
		return credentialUnavailable(key)
	}
	return s.store.WithWorkspaceCredentialGrant(ctx, a.ID, choice.RepositoryID, key, choice.Revision, func(current AutomationCredentialChoice) error {
		if current.State != CredentialChoiceGranted || current.GrantorUserID == "" ||
			current.CreatorUserID != a.CreatedByUserID || current.SecretID != current.SourceSecretID {
			return credentialUnavailable(key)
		}
		_, _, err := s.currentWorkspaceCredentialSecret(ctx, a, current)
		return err
	})
}

func (s *Service) resolveCurrentWorkspaceCredential(
	ctx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
) (string, error) {
	ownerCtx, _, err := s.currentWorkspaceCredentialSecret(ctx, a, choice)
	if err != nil {
		return "", err
	}
	value, err := s.credentialSecretStore.RevealForWorkspace(ownerCtx, choice.SecretID, a.WorkspaceID)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return "", ErrCredentialChoiceUnavailable
		}
		return "", ErrCredentialStorageUnavailable
	}
	return value, nil
}

func (s *Service) currentWorkspaceCredentialSecret(
	ctx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
) (context.Context, *secrets.Secret, error) {
	ownerCtx, ownerID, err := s.currentWorkspaceCredentialOwner(ctx, a, choice)
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateCurrentWorkspaceCredentialBinding(ownerCtx, a, choice); err != nil {
		return nil, nil, err
	}
	secret, err := s.currentWorkspaceCredentialSecretRecord(ownerCtx, a, choice, ownerID)
	return ownerCtx, secret, err
}

func (s *Service) currentWorkspaceCredentialOwner(
	ctx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
) (context.Context, string, error) {
	if a == nil {
		return nil, "", ErrCredentialChoiceUnavailable
	}
	if s.workspaceOwnerLookup == nil {
		return nil, "", ErrCredentialStorageUnavailable
	}
	ownerID, err := s.workspaceOwnerLookup(ctx, a.WorkspaceID)
	if err != nil {
		return nil, "", ErrCredentialStorageUnavailable
	}
	ownerUpdatedAt, err := s.workspaceOwnerRevision(ctx, a.WorkspaceID)
	if err != nil {
		return nil, "", err
	}
	if ownerID == "" || ownerID != choice.GrantorUserID || choice.WorkspaceOwnerUpdatedAt == nil ||
		!choice.WorkspaceOwnerUpdatedAt.Equal(ownerUpdatedAt) {
		return nil, "", ErrCredentialChoiceUnavailable
	}
	if s.credentialOwnerContext == nil {
		return nil, "", ErrCredentialChoiceUnavailable
	}
	ownerCtx, err := s.credentialOwnerContext(ctx, ownerID)
	if err != nil {
		return nil, "", ErrCredentialChoiceUnavailable
	}
	return ownerCtx, ownerID, nil
}

func (s *Service) validateCurrentWorkspaceCredentialBinding(
	ownerCtx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
) error {
	if !automationSelectsRepository(a, choice.RepositoryID) || s.credentialRepositoryReader == nil || s.credentialSecretStore == nil {
		return ErrCredentialChoiceUnavailable
	}
	repository, err := s.credentialRepositoryReader.GetRepository(ownerCtx, choice.RepositoryID)
	if err != nil {
		return ErrCredentialStorageUnavailable
	}
	binding, ok := findRepositorySecretBinding(repository, a.WorkspaceID, choice.Key)
	if !ok || binding.SecretID != choice.SourceSecretID || !binding.UpdatedAt.Equal(choice.SourceUpdatedAt) {
		return ErrCredentialChoiceUnavailable
	}
	return nil
}

func (s *Service) currentWorkspaceCredentialSecretRecord(
	ownerCtx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
	ownerID string,
) (*secrets.Secret, error) {
	secret, err := s.credentialSecretStore.GetForWorkspace(ownerCtx, choice.SecretID, a.WorkspaceID)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return nil, ErrCredentialChoiceUnavailable
		}
		return nil, ErrCredentialStorageUnavailable
	}
	if !workspaceSecretOwnedBy(secret, ownerID, a.WorkspaceID) {
		return nil, ErrCredentialChoiceUnavailable
	}
	return secret, nil
}

func (s *Service) requireWorkspaceCredentialOwner(ctx context.Context, workspaceID string) (string, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" || s.workspaceOwnerLookup == nil {
		return "", ErrWorkspaceCredentialGrantDenied
	}
	ownerID, err := s.workspaceOwnerLookup(ctx, workspaceID)
	if err != nil {
		return "", ErrCredentialStorageUnavailable
	}
	if ownerID == "" || identity.UserID != ownerID {
		return "", ErrWorkspaceCredentialGrantDenied
	}
	return ownerID, nil
}

func (s *Service) workspaceOwnerRevision(ctx context.Context, workspaceID string) (time.Time, error) {
	if s.workspaceOwnerUpdatedAtLookup == nil {
		return time.Time{}, ErrCredentialStorageUnavailable
	}
	revision, err := s.workspaceOwnerUpdatedAtLookup(ctx, workspaceID)
	if err != nil {
		return time.Time{}, ErrCredentialStorageUnavailable
	}
	if revision.IsZero() {
		return time.Time{}, ErrCredentialStorageUnavailable
	}
	return revision, nil
}

func automationSelectsRepository(a *Automation, repositoryID string) bool {
	for _, repository := range a.Repositories {
		if repository.RepositoryID == repositoryID {
			return true
		}
	}
	return false
}

func findRepositorySecretBinding(repository *taskmodels.Repository, workspaceID, key string) (taskmodels.RepositorySecretBinding, bool) {
	if repository == nil || repository.WorkspaceID != workspaceID {
		return taskmodels.RepositorySecretBinding{}, false
	}
	var selected taskmodels.RepositorySecretBinding
	for _, binding := range repository.SecretBindings {
		if binding.Key != key {
			continue
		}
		if selected.Key != "" || binding.SecretID == "" {
			return taskmodels.RepositorySecretBinding{}, false
		}
		selected = binding
	}
	return selected, selected.Key != ""
}

func credentialUnavailable(key string) error {
	return &CredentialValidationError{Field: key, Source: "Selected repository"}
}
