package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

var (
	ErrAutomationCredentialUnavailable = errors.New("automation credential source unavailable")
	ErrAutomationCredentialStorage     = errors.New("automation credential storage unavailable")
)

type AutomationCredentialRequest struct {
	AutomationID       string
	WorkspaceID        string
	AgentProfileID     string
	ExecutorProfileID  string
	RepositoryIDs      []string
	CredentialBindings []AutomationCredentialBinding
}

// AutomationCredentialEnvironmentError contains only the field and visible
// source needed to repair a saved automation.
type AutomationCredentialEnvironmentError struct {
	Field  string
	Source string
	err    error
}

func (e *AutomationCredentialEnvironmentError) Error() string {
	return fmt.Sprintf("automation credential for %s from %s is unavailable", e.Field, e.Source)
}

func (e *AutomationCredentialEnvironmentError) Unwrap() error { return e.err }

type automationCredentialRepositoryStore interface {
	GetRepository(context.Context, string) (*models.Repository, error)
}

type automationCredentialExecutorReader interface {
	GetExecutor(context.Context, string) (*models.Executor, error)
}

// ValidateAutomationCredentialEnvironment resolves the known profile and
// repository sources through the runtime environment merger, then discards
// every resolved value. Dynamic profiles and repositories selected only by a
// future event are validated at launch when their source is known.
func (m *Manager) ValidateAutomationCredentialEnvironment(ctx context.Context, req AutomationCredentialRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	profileInfo, agentConfig, agentDefinitions, err := m.automationAgentCredentialDefinitions(ctx, req.AgentProfileID)
	if err != nil {
		return err
	}
	executorProfile, executorDefinitions, err := m.automationExecutorCredentialDefinitions(ctx, req.ExecutorProfileID)
	if err != nil {
		return err
	}
	definitions := make([]runtimeenv.Definition, 0, len(agentDefinitions)+len(executorDefinitions))
	definitions = append(definitions, agentDefinitions...)
	definitions = append(definitions, executorDefinitions...)

	repositoryDefinitions, err := m.automationRepositoryEnvironmentDefinitions(ctx, req.WorkspaceID, req.RepositoryIDs)
	if err != nil {
		return err
	}
	definitions = append(definitions, repositoryDefinitions...)
	definitions, err = applyAutomationCredentialBindings(req.AgentProfileID, req.ExecutorProfileID, req.CredentialBindings, definitions)
	if err != nil {
		return err
	}
	appendAgentRuntimeDefaults(&definitions, agentConfig)
	appendRequiredCredentialDefinitions(ctx, &definitions, m.credsMgr, agentConfig)
	visibleSecrets, err := m.resolveAutomationCredentialDefinitions(ctx, definitions)
	if err != nil {
		return err
	}
	if err := validateAutomationProfileProviderSecret(ctx, profileInfo, visibleSecrets); err != nil {
		return err
	}
	return m.validateAutomationRemoteCredentials(
		ctx, profileInfo, agentConfig,
		automationCredentialProfileEnvVars(definitions, runtimeenv.OriginAgentProfile),
		automationCredentialProfileEnvVars(definitions, runtimeenv.OriginExecutorProfile),
		executorProfile, visibleSecrets,
	)
}

func (m *Manager) automationAgentCredentialDefinitions(
	ctx context.Context,
	profileID string,
) (*AgentProfileInfo, agents.Agent, []runtimeenv.Definition, error) {
	if profileID == "" || m.profileResolver == nil {
		return nil, nil, nil, nil
	}
	profileInfo, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		switch {
		case errors.Is(err, ErrVirtualProfile):
			return nil, nil, nil, nil
		case isCredentialCancellation(err):
			return nil, nil, nil, err
		default:
			return nil, nil, nil, &AutomationCredentialEnvironmentError{Field: "agent profile", Source: "Agent profile", err: ErrAutomationCredentialUnavailable}
		}
	}
	definitions := make([]runtimeenv.Definition, 0)
	if profileInfo == nil {
		return nil, nil, definitions, nil
	}
	appendProfileDefinitions(&definitions, profileInfo.EnvVars, runtimeenv.OriginAgentProfile)
	var agentConfig agents.Agent
	if m.registry != nil && profileInfo.AgentName != "" {
		agentConfig, _ = m.registry.Get(profileInfo.AgentName)
	}
	return profileInfo, agentConfig, definitions, nil
}

func (m *Manager) automationExecutorCredentialDefinitions(
	ctx context.Context,
	profileID string,
) (*models.ExecutorProfile, []runtimeenv.Definition, error) {
	if profileID == "" {
		return nil, nil, nil
	}
	if m.executorProfileReader == nil {
		return nil, nil, ErrAutomationCredentialStorage
	}
	profile, err := m.executorProfileReader.GetExecutorProfile(ctx, profileID)
	if err != nil {
		if isCredentialCancellation(err) {
			return nil, nil, err
		}
		return nil, nil, ErrAutomationCredentialStorage
	}
	if profile == nil {
		return nil, nil, &AutomationCredentialEnvironmentError{Field: "executor profile", Source: "Executor profile", err: ErrAutomationCredentialUnavailable}
	}
	definitions := make([]runtimeenv.Definition, 0, len(profile.EnvVars))
	appendProfileDefinitions(&definitions, profile.EnvVars, runtimeenv.OriginExecutorProfile)
	return profile, definitions, nil
}

func (m *Manager) resolveAutomationCredentialDefinitions(
	ctx context.Context,
	definitions []runtimeenv.Definition,
) (secrets.SecretStore, error) {
	visibleSecrets := secrets.NewUserVisibleStore(m.secretStore)
	resolved, _, err := runtimeenv.Resolve(ctx, definitions, func(ctx context.Context, definition runtimeenv.Definition) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if visibleSecrets == nil {
			return "", ErrAutomationCredentialStorage
		}
		if definition.WorkspaceID != "" {
			scoped, ok := visibleSecrets.(secrets.ScopedSecretStore)
			if !ok {
				return "", ErrAutomationCredentialStorage
			}
			return scoped.RevealForWorkspace(ctx, definition.SecretID, definition.WorkspaceID)
		}
		if scoped, ok := visibleSecrets.(secrets.ScopedSecretStore); ok {
			return scoped.RevealGlobal(ctx, definition.SecretID)
		}
		return visibleSecrets.Reveal(ctx, definition.SecretID)
	})
	if err != nil {
		return nil, automationCredentialEnvironmentError(err)
	}
	clear(resolved)
	return visibleSecrets, nil
}

func validateAutomationProfileProviderSecret(
	ctx context.Context,
	profile *AgentProfileInfo,
	visibleSecrets secrets.SecretStore,
) error {
	if profile == nil || profile.ProviderKind != settingsmodels.ProviderKindOpenAICompatible || profile.ProviderAPIKeySecretID == "" {
		return nil
	}
	if visibleSecrets == nil {
		return ErrAutomationCredentialStorage
	}
	key, err := revealAutomationProviderSecret(ctx, visibleSecrets, profile.ProviderAPIKeySecretID)
	if err != nil {
		return automationCredentialSecretError("provider API key", "Agent profile", err)
	}
	if strings.TrimSpace(key) == "" || strings.ContainsRune(key, '\x00') {
		return &AutomationCredentialEnvironmentError{Field: "provider API key", Source: "Agent profile", err: ErrAutomationCredentialUnavailable}
	}
	return nil
}

func revealAutomationProviderSecret(ctx context.Context, store secrets.SecretStore, secretID string) (string, error) {
	if scoped, ok := store.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, secretID)
	}
	return store.Reveal(ctx, secretID)
}

func automationCredentialProfileEnvVars(definitions []runtimeenv.Definition, origin string) []models.ProfileEnvVar {
	entries := make([]models.ProfileEnvVar, 0)
	for _, definition := range definitions {
		if definition.Origin != origin {
			continue
		}
		entries = append(entries, models.ProfileEnvVar{
			Key: definition.Key, Value: definition.Literal, SecretID: definition.SecretID,
		})
	}
	return entries
}

func (m *Manager) automationRepositoryEnvironmentDefinitions(
	ctx context.Context,
	workspaceID string,
	repositoryIDs []string,
) ([]runtimeenv.Definition, error) {
	if len(repositoryIDs) == 0 {
		return nil, nil
	}
	reader, ok := m.executorProfileReader.(automationCredentialRepositoryStore)
	if !ok {
		return nil, ErrAutomationCredentialStorage
	}
	definitions := make([]runtimeenv.Definition, 0)
	seen := make(map[string]struct{}, len(repositoryIDs))
	for _, repositoryID := range repositoryIDs {
		if repositoryID == "" {
			continue
		}
		if _, exists := seen[repositoryID]; exists {
			continue
		}
		seen[repositoryID] = struct{}{}
		repository, err := reader.GetRepository(ctx, repositoryID)
		if err != nil {
			if isCredentialCancellation(err) {
				return nil, err
			}
			return nil, ErrAutomationCredentialStorage
		}
		if repository == nil || repository.WorkspaceID != workspaceID {
			return nil, &AutomationCredentialEnvironmentError{Field: "repository secret", Source: "Selected repository", err: ErrAutomationCredentialUnavailable}
		}
		origin := runtimeenv.RepositoryOrigin(repository.Name)
		for _, binding := range repository.SecretBindings {
			if strings.TrimSpace(binding.Key) == "" || strings.TrimSpace(binding.SecretID) == "" {
				return nil, &AutomationCredentialEnvironmentError{Field: "repository secret", Source: origin, err: ErrAutomationCredentialUnavailable}
			}
			definitions = append(definitions, runtimeenv.Definition{
				Key: binding.Key, SecretID: binding.SecretID, Origin: origin, WorkspaceID: workspaceID,
				RepositoryID: repository.ID, BindingUpdatedAt: binding.UpdatedAt,
			})
		}
	}
	return definitions, nil
}

func automationCredentialEnvironmentError(err error) error {
	var secretErr *runtimeenv.SecretError
	if errors.As(err, &secretErr) {
		return automationCredentialSecretError(secretErr.Key, secretErr.Origin, secretErr.Unwrap())
	}
	var conflictErr *runtimeenv.ConflictError
	if errors.As(err, &conflictErr) {
		return &AutomationCredentialEnvironmentError{
			Field: conflictErr.Key, Source: strings.Join(conflictErr.Origins, ", "), err: ErrAutomationCredentialUnavailable,
		}
	}
	if isCredentialCancellation(err) {
		return err
	}
	if errors.Is(err, ErrAutomationCredentialStorage) {
		return ErrAutomationCredentialStorage
	}
	return ErrAutomationCredentialStorage
}

func automationCredentialSecretError(field, source string, err error) error {
	if isCredentialCancellation(err) {
		return err
	}
	if errors.Is(err, secrets.ErrNotFound) || errors.Is(err, secrets.ErrWorkspaceAccessDenied) {
		return &AutomationCredentialEnvironmentError{Field: field, Source: source, err: ErrAutomationCredentialUnavailable}
	}
	return ErrAutomationCredentialStorage
}

func (m *Manager) validateAutomationRemoteCredentials(
	ctx context.Context,
	profile *AgentProfileInfo,
	agentConfig agents.Agent,
	profileEnvVars []models.ProfileEnvVar,
	executorEnvVars []models.ProfileEnvVar,
	executorProfile *models.ExecutorProfile,
	secretStore secrets.SecretStore,
) error {
	if profile == nil || executorProfile == nil || agentConfig == nil || executorProfile.ExecutorID == "" {
		return nil
	}
	reader, ok := m.executorProfileReader.(automationCredentialExecutorReader)
	if !ok {
		return nil
	}
	executor, err := reader.GetExecutor(ctx, executorProfile.ExecutorID)
	if err != nil || executor == nil || !models.IsRemoteExecutorType(executor.Type) {
		return nil
	}
	if err := remoteauth.ValidateCredentialAdmission(ctx, remoteauth.AdmissionInput{
		AgentID:         agentConfig.ID(),
		ProfileEnvVars:  profileEnvVars,
		ExecutorEnvVars: executorEnvVars,
		ExecutorConfig:  executorProfile.Config,
		Catalog:         remoteauth.BuildCatalog([]agents.Agent{agentConfig}),
		SecretStore:     secretStore,
	}); err != nil {
		if isCredentialCancellation(err) {
			return err
		}
		return &AutomationCredentialEnvironmentError{Field: "remote authentication", Source: "Executor profile", err: ErrAutomationCredentialUnavailable}
	}
	return nil
}

func isCredentialCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
