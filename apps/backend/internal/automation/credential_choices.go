package automation

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type PersonalCredentialBindingValidator interface {
	ValidatePersonalCredentialBindings(context.Context, *Automation, []PersonalCredentialBindingInput) ([]AutomationCredentialChoice, error)
}

type CredentialRepositoryReader interface {
	GetRepository(context.Context, string) (*taskmodels.Repository, error)
}

type CredentialSecretStore interface {
	GetForWorkspace(context.Context, string, string) (*secrets.Secret, error)
	RevealForWorkspace(context.Context, string, string) (string, error)
}

type CredentialOwnerContext func(context.Context, string) (context.Context, error)

type credentialChoiceViewer struct {
	identity      authn.Identity
	hasIdentity   bool
	ownerID       string
	ownerRevision int64
}

func (v credentialChoiceViewer) authenticatedUserIs(userID string) bool {
	return v.hasIdentity && !v.identity.Synthetic && v.identity.UserID == userID
}

func (v credentialChoiceViewer) canGrantWorkspace() bool {
	return v.authenticatedUserIs(v.ownerID) && v.ownerRevision > 0
}

func (s *Service) SetPersonalCredentialBindingValidator(validator PersonalCredentialBindingValidator) {
	s.personalCredentialValidator = validator
}

func (s *Service) SetWorkspaceCredentialGrantDependencies(
	ownerLookup func(context.Context, string) (string, error),
	repositories CredentialRepositoryReader,
	secretStore CredentialSecretStore,
	ownerContext CredentialOwnerContext,
	creatorAccess CredentialCreatorAccess,
) {
	s.workspaceOwnerLookup = ownerLookup
	s.credentialRepositoryReader = repositories
	s.credentialSecretStore = secretStore
	s.credentialOwnerContext = ownerContext
	s.credentialCreatorAccess = creatorAccess
}

func (s *Service) SetWorkspaceCredentialOwnerRevisionLookup(lookup func(context.Context, string) (int64, error)) {
	s.workspaceOwnerRevisionLookup = lookup
}

func (s *Service) SetCredentialCreatorNameLookup(lookup func(context.Context, string) (string, error)) {
	s.credentialCreatorNameLookup = lookup
}

func (s *Service) projectCredentialChoices(ctx context.Context, a *Automation) (*Automation, error) {
	if a == nil {
		return nil, nil
	}
	rows, err := s.store.credentialChoices(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	viewer, err := s.credentialChoiceViewer(ctx, a)
	if err != nil {
		return nil, err
	}
	a.CanManageCredentialChoices = !viewer.hasIdentity || viewer.identity.Synthetic || viewer.identity.UserID == a.CreatedByUserID
	a.CredentialChoices = make([]CredentialChoiceProjection, 0, len(rows))
	seen, err := s.projectSavedCredentialChoices(ctx, a, rows, viewer)
	if err != nil {
		return nil, err
	}
	if err := s.projectCurrentWorkspaceCredentialChoices(ctx, a, viewer, seen); err != nil {
		return nil, err
	}
	a.CanTriggerNow = automationCanTriggerNow(ctx, a)
	return a, nil
}

func (s *Service) credentialChoiceViewer(ctx context.Context, a *Automation) (credentialChoiceViewer, error) {
	identity, hasIdentity := authn.IdentityFromContext(ctx)
	viewer := credentialChoiceViewer{identity: identity, hasIdentity: hasIdentity}
	if !hasIdentity || identity.Synthetic || s.workspaceOwnerLookup == nil {
		return viewer, nil
	}
	ownerID, err := s.workspaceOwnerLookup(ctx, a.WorkspaceID)
	if err != nil {
		return credentialChoiceViewer{}, err
	}
	viewer.ownerID = ownerID
	if s.workspaceOwnerRevisionLookup == nil {
		return viewer, nil
	}
	viewer.ownerRevision, err = s.workspaceOwnerRevisionLookup(ctx, a.WorkspaceID)
	return viewer, err
}

func (s *Service) projectSavedCredentialChoices(
	ctx context.Context,
	a *Automation,
	rows []AutomationCredentialChoice,
	viewer credentialChoiceViewer,
) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		projection, err := s.projectSavedCredentialChoice(ctx, a, row, viewer)
		if err != nil {
			return nil, err
		}
		a.CredentialChoices = append(a.CredentialChoices, projection)
		seen[credentialChoiceProjectionKey(projection.RepositoryID, projection.Key)] = struct{}{}
	}
	return seen, nil
}

func (s *Service) projectSavedCredentialChoice(
	ctx context.Context,
	a *Automation,
	row AutomationCredentialChoice,
	viewer credentialChoiceViewer,
) (CredentialChoiceProjection, error) {
	projection := CredentialChoiceProjection{
		Kind: row.Kind, ProfileID: row.ProfileID, RepositoryID: row.RepositoryID,
		Key: row.Key, State: row.State,
	}
	if row.Kind == CredentialChoiceWorkspace {
		if err := s.projectSavedWorkspaceCredential(ctx, a, row, viewer, &projection); err != nil {
			return CredentialChoiceProjection{}, err
		}
	}
	if viewer.authenticatedUserIs(row.CreatorUserID) && row.Kind == CredentialChoicePersonal {
		projection.SecretID = row.SecretID
		projection.SecretName = row.SecretName
	}
	return projection, nil
}

func (s *Service) projectSavedWorkspaceCredential(
	ctx context.Context,
	a *Automation,
	row AutomationCredentialChoice,
	viewer credentialChoiceViewer,
	projection *CredentialChoiceProjection,
) error {
	stale, err := s.workspaceCredentialChoiceIsStale(ctx, a, row, viewer.ownerID, viewer.ownerRevision)
	if err != nil {
		return err
	}
	if stale && row.State == CredentialChoiceGranted {
		projection.State = CredentialChoiceStale
	}
	canGrant, secretName, _, bindingUpdatedAt, err := s.workspaceCredentialGrantProjection(
		ctx, a, row.RepositoryID, row.Key, viewer.ownerID, viewer.canGrantWorkspace(),
	)
	if err != nil {
		return err
	}
	projection.CanGrant = canGrant
	projection.SecretName = secretName
	projection.BindingUpdatedAt = bindingUpdatedAt
	projection.WorkspaceOwnerRevision = viewer.ownerRevision
	if viewer.authenticatedUserIs(viewer.ownerID) {
		projection.Revision = row.Revision
		projection.CanRevoke = row.Revision > 0 &&
			(row.State == CredentialChoiceGranted || row.State == CredentialChoiceStale)
		if canGrant && s.credentialCreatorNameLookup != nil {
			projection.CreatorName, err = s.credentialCreatorNameLookup(ctx, a.CreatedByUserID)
		}
	}
	return err
}

func (s *Service) projectCurrentWorkspaceCredentialChoices(
	ctx context.Context,
	a *Automation,
	viewer credentialChoiceViewer,
	seen map[string]struct{},
) error {
	for _, selected := range a.Repositories {
		if selected.RepositoryID == "" || s.credentialRepositoryReader == nil {
			continue
		}
		if err := s.projectRepositoryWorkspaceChoices(ctx, a, viewer, selected.RepositoryID, seen); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) projectRepositoryWorkspaceChoices(
	ctx context.Context,
	a *Automation,
	viewer credentialChoiceViewer,
	repositoryID string,
	seen map[string]struct{},
) error {
	repository, err := s.credentialRepositoryReader.GetRepository(ctx, repositoryID)
	if err != nil {
		return err
	}
	if repository == nil || repository.WorkspaceID != a.WorkspaceID {
		return nil
	}
	for _, binding := range repository.SecretBindings {
		key := credentialChoiceProjectionKey(repositoryID, binding.Key)
		if _, exists := seen[key]; exists {
			continue
		}
		canGrant, secretName, isWorkspaceSecret, bindingUpdatedAt, err := s.workspaceCredentialGrantProjection(
			ctx, a, repositoryID, binding.Key, viewer.ownerID, viewer.canGrantWorkspace(),
		)
		if err != nil {
			return err
		}
		if !isWorkspaceSecret {
			continue
		}
		a.CredentialChoices = append(a.CredentialChoices, CredentialChoiceProjection{
			Kind: CredentialChoiceWorkspace, RepositoryID: repositoryID, Key: binding.Key,
			State: CredentialChoiceApproval, SecretName: secretName, CanGrant: canGrant,
			BindingUpdatedAt: bindingUpdatedAt, WorkspaceOwnerRevision: viewer.ownerRevision,
		})
		if canGrant && s.credentialCreatorNameLookup != nil {
			name, nameErr := s.credentialCreatorNameLookup(ctx, a.CreatedByUserID)
			if nameErr != nil {
				return nameErr
			}
			a.CredentialChoices[len(a.CredentialChoices)-1].CreatorName = name
		}
		seen[key] = struct{}{}
	}
	return nil
}

func automationCanTriggerNow(ctx context.Context, a *Automation) bool {
	if a == nil || !a.Enabled {
		return false
	}
	if len(a.CredentialChoices) == 0 {
		return true
	}
	if err := validateCredentialChoiceSettings(a, a.Triggers); err != nil {
		return false
	}
	for _, choice := range a.CredentialChoices {
		if choice.Kind == CredentialChoiceWorkspace && !automationSelectsRepository(a, choice.RepositoryID) {
			continue
		}
		if choice.State != CredentialChoiceSelected && choice.State != CredentialChoiceGranted {
			return false
		}
	}
	identity, ok := authn.IdentityFromContext(ctx)
	return !ok || identity.Synthetic || identity.UserID == a.CreatedByUserID
}

func (s *Service) workspaceCredentialGrantProjection(
	ctx context.Context,
	a *Automation,
	repositoryID, key, ownerID string,
	viewerIsOwner bool,
) (bool, string, bool, time.Time, error) {
	if ownerID == "" || s.credentialOwnerContext == nil || s.credentialSecretStore == nil ||
		s.credentialRepositoryReader == nil || !automationSelectsRepository(a, repositoryID) {
		return false, "", false, time.Time{}, nil
	}
	repository, err := s.credentialRepositoryReader.GetRepository(ctx, repositoryID)
	if err != nil {
		return false, "", false, time.Time{}, err
	}
	binding, ok := findRepositorySecretBinding(repository, a.WorkspaceID, key)
	if !ok {
		return false, "", false, time.Time{}, nil
	}
	ownerCtx, err := s.credentialOwnerContext(ctx, ownerID)
	if err != nil {
		return false, "", false, binding.UpdatedAt, nil
	}
	secret, err := s.credentialSecretStore.GetForWorkspace(ownerCtx, binding.SecretID, a.WorkspaceID)
	if err != nil || secret == nil || secret.Scope != secrets.ScopeWorkspace || secret.WorkspaceID != a.WorkspaceID || secret.OwnerUserID != ownerID {
		return false, "", false, binding.UpdatedAt, nil
	}
	if viewerIsOwner {
		return true, secret.Name, true, binding.UpdatedAt, nil
	}
	return false, "", true, binding.UpdatedAt, nil
}

func credentialChoiceProjectionKey(repositoryID, key string) string {
	return repositoryID + "\x00" + key
}

func (s *Service) workspaceCredentialChoiceIsStale(
	ctx context.Context,
	a *Automation,
	choice AutomationCredentialChoice,
	ownerID string,
	ownerRevision int64,
) (bool, error) {
	if choice.State != CredentialChoiceGranted || ownerID == "" || ownerID != choice.GrantorUserID ||
		choice.WorkspaceOwnerRevision == nil || *choice.WorkspaceOwnerRevision != ownerRevision || s.credentialRepositoryReader == nil {
		return true, nil
	}
	if !automationSelectsRepository(a, choice.RepositoryID) {
		return true, nil
	}
	repository, err := s.credentialRepositoryReader.GetRepository(ctx, choice.RepositoryID)
	if err != nil {
		return false, err
	}
	if repository == nil || repository.WorkspaceID != a.WorkspaceID {
		return true, nil
	}
	binding, exists := findRepositorySecretBinding(repository, a.WorkspaceID, choice.Key)
	return !exists || binding.SecretID != choice.SourceSecretID || !binding.UpdatedAt.Equal(choice.SourceUpdatedAt), nil
}

func (s *Service) hasCredentialChoices(ctx context.Context, automationID string) (bool, error) {
	return s.store.HasCredentialChoices(ctx, automationID)
}

func (s *Service) requireCreatorForCredentialMutation(ctx context.Context, a *Automation) (bool, error) {
	hasChoices, err := s.hasCredentialChoices(ctx, a.ID)
	if err != nil || !hasChoices {
		return hasChoices, err
	}
	return true, requireCredentialCreator(ctx, a)
}

func hasWorkspaceCredentialChoices(choices []CredentialChoiceProjection) bool {
	for _, choice := range choices {
		if choice.Kind == CredentialChoiceWorkspace {
			return true
		}
	}
	return false
}

func workspaceRepositorySelectionChanged(previous, current []AutomationRepository) bool {
	key := func(rows []AutomationRepository) []string {
		keys := make([]string, 0, len(rows))
		for _, row := range rows {
			keys = append(keys, row.RepositoryID+"\x00"+row.BaseBranch)
		}
		sort.Strings(keys)
		return keys
	}
	left, right := key(previous), key(current)
	if len(left) != len(right) {
		return true
	}
	for index := range left {
		if left[index] != right[index] {
			return true
		}
	}
	return false
}

func requireCredentialCreator(ctx context.Context, a *Automation) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return nil
	}
	if a == nil || a.CreatedByUserID == "" || identity.UserID != a.CreatedByUserID {
		return ErrCredentialCreatorRequired
	}
	return nil
}

func validateCredentialChoiceSettings(a *Automation, triggers []AutomationTrigger) error {
	if a == nil || a.TaskMode != TaskModeAutomationRun ||
		a.ContinuationPolicy != "" && a.ContinuationPolicy != ContinuationPolicyNewTask ||
		a.AgentProfileID == "" || a.ExecutorProfileID == "" {
		return ErrCredentialSettingsUnsupported
	}
	for _, trigger := range triggers {
		if trigger.Type != TriggerTypeScheduled && trigger.Type != TriggerTypeManual {
			return ErrCredentialSettingsUnsupported
		}
	}
	return nil
}

func ValidateCredentialChoiceSettingsForRuntime(a *Automation, triggerType TriggerType) error {
	if err := validateCredentialChoiceSettings(a, a.Triggers); err != nil {
		return err
	}
	if triggerType != TriggerTypeManual && triggerType != TriggerTypeScheduled {
		return ErrCredentialSettingsUnsupported
	}
	return nil
}

func (s *Service) validatePersonalBindings(
	ctx context.Context,
	a *Automation,
	inputs []PersonalCredentialBindingInput,
) ([]AutomationCredentialChoice, error) {
	if err := requireCredentialCreator(ctx, a); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return []AutomationCredentialChoice{}, nil
	}
	if err := validateCredentialChoiceSettings(a, a.Triggers); err != nil {
		return nil, err
	}
	if s.personalCredentialValidator == nil {
		return nil, ErrCredentialChoiceUnavailable
	}
	choices, err := s.personalCredentialValidator.ValidatePersonalCredentialBindings(ctx, a, inputs)
	if err != nil {
		return nil, err
	}
	if len(choices) != len(inputs) {
		return nil, ErrCredentialChoiceUnavailable
	}
	return choices, nil
}

func (s *Service) CredentialChoicesForRun(ctx context.Context, runID string) (*Automation, *AutomationRun, []AutomationCredentialChoice, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil {
		return nil, run, nil, err
	}
	a, err := s.store.GetAutomation(ctx, run.AutomationID)
	if err != nil || a == nil {
		return nil, run, nil, errors.Join(ErrCredentialChoiceUnavailable, err)
	}
	choices, err := decodeRunCredentialChoiceSnapshot(a.ID, run.CredentialChoicesSnapshot)
	return a, run, choices, err
}
