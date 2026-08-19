package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/runtime/envmetrics"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// TaskEnvironmentRepositoryReader supplies durable repository bindings when a
// backend restart reconstructs a workspace-only execution.
type TaskEnvironmentRepositoryReader interface {
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
}

func (m *Manager) resolveStrictEnvironment(
	ctx context.Context,
	executionID string,
	req *LaunchRequest,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
) (map[string]string, error) {
	definitions := append([]runtimeenv.Definition(nil), req.EnvironmentDefinitions...)
	appendMapDefinitions(&definitions, req.Env, runtimeenv.OriginManagedRuntime)
	appendAgentProfileDefinitions(&definitions, profileInfo)

	appendStandardDefinitions(&definitions, executionID, req)
	appendAgentRuntimeDefaults(&definitions, agentConfig)
	appendRequiredCredentialDefinitions(ctx, &definitions, m.credsMgr, agentConfig)
	if req.managedGoCachePath != "" {
		definitions = append(definitions, runtimeenv.Definition{
			Key: "GOCACHE", Literal: req.managedGoCachePath, Origin: runtimeenv.OriginManagedRuntime,
		})
	}

	resolved, records, err := runtimeenv.Resolve(ctx, definitions, m.resolveEnvironmentDefinition)
	if err != nil {
		return nil, fmt.Errorf("resolve task environment: %w", err)
	}
	m.logEnvironmentOverrides(req, records)
	return resolved, nil
}

// logEnvironmentOverrides emits the AC-23 structured log and the AC-24
// envmetrics counter for every OverrideRecord a successful Resolve produced.
// It runs only on Resolve's success path: all-or-nothing governs Resolve's
// own error paths, not a later launch failure, so a failure downstream of a
// successful resolve must not roll these back (F32).
func (m *Manager) logEnvironmentOverrides(req *LaunchRequest, records []runtimeenv.OverrideRecord) {
	for _, record := range records {
		losingOrigins := make([]string, 0, len(record.LosingOrigins))
		losingTiers := make([]int, 0, len(record.LosingOrigins))
		for _, losing := range record.LosingOrigins {
			losingOrigins = append(losingOrigins, losing.Origin)
			losingTiers = append(losingTiers, int(losing.Tier))
			envmetrics.RecordOverrideApplied(
				runtimeenv.JoinOriginLabels(record.WinningOrigins),
				runtimeenv.NormalizeOriginLabel(losing.Origin),
			)
		}
		m.logger.Info("environment override applied",
			zap.String("env_key", record.Key),
			zap.Strings("winning_origins", record.WinningOrigins),
			zap.Int("winning_tier", int(record.WinningTier)),
			zap.Strings("losing_origins", losingOrigins),
			zap.Ints("losing_tiers", losingTiers),
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
		)
	}
}

func appendAgentProfileDefinitions(definitions *[]runtimeenv.Definition, profileInfo *AgentProfileInfo) {
	if profileInfo == nil {
		return
	}
	appendProfileDefinitions(definitions, profileInfo.EnvVars, runtimeenv.OriginAgentProfile)
	if profileInfo.Model != "" {
		*definitions = append(*definitions, runtimeenv.Definition{
			Key: "AGENT_MODEL", Literal: profileInfo.Model, Origin: runtimeenv.OriginManagedRuntime,
		})
	}
	if profileInfo.AutoApprove {
		*definitions = append(*definitions, runtimeenv.Definition{
			Key: "AGENTCTL_AUTO_APPROVE_PERMISSIONS", Literal: "true", Origin: runtimeenv.OriginManagedRuntime,
		})
	}
}

func appendAgentRuntimeDefaults(definitions *[]runtimeenv.Definition, agentConfig agents.Agent) {
	if agentConfig == nil {
		return
	}
	rt := agentConfig.Runtime()
	if rt == nil {
		return
	}
	for key, value := range rt.Env {
		if hasEnvironmentDefinition(*definitions, key) {
			continue
		}
		*definitions = append(*definitions, runtimeenv.Definition{
			Key: key, Literal: value, Origin: runtimeenv.OriginManagedAgentDefaults,
		})
	}
}

func appendRequiredCredentialDefinitions(
	ctx context.Context,
	definitions *[]runtimeenv.Definition,
	credsMgr CredentialsManager,
	agentConfig agents.Agent,
) {
	if credsMgr == nil || agentConfig == nil {
		return
	}
	rt := agentConfig.Runtime()
	if rt == nil {
		return
	}
	for _, key := range rt.RequiredEnv {
		value, err := credsMgr.GetCredentialValue(ctx, key)
		if err != nil || value == "" {
			continue
		}
		*definitions = append(*definitions, runtimeenv.Definition{
			Key: key, Literal: value, Origin: runtimeenv.OriginManagedCredentials,
		})
	}
}

func appendMapDefinitions(definitions *[]runtimeenv.Definition, values map[string]string, origin string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		*definitions = append(*definitions, runtimeenv.Definition{Key: key, Literal: values[key], Origin: origin})
	}
}

func appendProfileDefinitions(definitions *[]runtimeenv.Definition, values []models.ProfileEnvVar, origin string) {
	for _, value := range values {
		if strings.TrimSpace(value.Key) == "" {
			continue
		}
		definition := runtimeenv.Definition{Key: value.Key, Literal: value.Value, SecretID: value.SecretID, Origin: origin}
		if definition.SecretID == "" && definition.Literal == "" {
			continue
		}
		*definitions = append(*definitions, definition)
	}
}

func appendStandardDefinitions(definitions *[]runtimeenv.Definition, executionID string, req *LaunchRequest) {
	values := map[string]string{
		"KANDEV_INSTANCE_ID":          executionID,
		"KANDEV_TASK_ID":              req.TaskID,
		"KANDEV_SESSION_ID":           req.SessionID,
		"KANDEV_AGENT_PROFILE_ID":     req.AgentProfileID,
		"KANDEV_EXECUTION_PROFILE_ID": executionProfileID(req),
	}
	for key, value := range values {
		if value == "" {
			continue
		}
		*definitions = append(*definitions, runtimeenv.Definition{Key: key, Literal: value, Origin: runtimeenv.OriginManagedRuntime})
	}
}

func hasEnvironmentDefinition(definitions []runtimeenv.Definition, key string) bool {
	for _, definition := range definitions {
		if definition.Key == key {
			return true
		}
	}
	return false
}

func approvedSecretEnvironmentKeys(definitions []runtimeenv.Definition) []string {
	keys := make(map[string]struct{})
	for _, definition := range definitions {
		if definition.WorkspaceID != "" && strings.TrimSpace(definition.Key) != "" {
			keys[definition.Key] = struct{}{}
		}
	}
	approved := make([]string, 0, len(keys))
	for key := range keys {
		approved = append(approved, key)
	}
	sort.Strings(approved)
	return approved
}

func (m *Manager) resolveEnvironmentDefinition(ctx context.Context, definition runtimeenv.Definition) (string, error) {
	if definition.SecretID == "" {
		return definition.Literal, nil
	}
	if m.secretStore == nil {
		return "", errors.New("secret store unavailable")
	}
	if definition.WorkspaceID != "" {
		scoped, ok := m.secretStore.(secrets.ScopedSecretStore)
		if !ok {
			return "", errors.New("workspace-scoped secret storage is unavailable")
		}
		return scoped.RevealForWorkspace(ctx, definition.SecretID, definition.WorkspaceID)
	}
	return revealGlobalSecret(ctx, m.secretStore, definition.SecretID)
}

func (m *Manager) repositoryEnvironmentDefinitions(ctx context.Context, taskID, workspaceID string) ([]runtimeenv.Definition, error) {
	reader, ok := m.executorProfileReader.(TaskEnvironmentRepositoryReader)
	if !ok || taskID == "" {
		return nil, nil
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, errors.New("workspace id is required to resolve repository environment")
	}
	taskRepositories, err := reader.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task repositories: %w", err)
	}
	definitions := make([]runtimeenv.Definition, 0)
	for _, taskRepository := range taskRepositories {
		if taskRepository == nil || taskRepository.RepositoryID == "" {
			continue
		}
		repository, err := reader.GetRepository(ctx, taskRepository.RepositoryID)
		if err != nil {
			return nil, fmt.Errorf("load repository %s: %w", taskRepository.RepositoryID, err)
		}
		if repository == nil {
			return nil, fmt.Errorf("repository %s is unavailable", taskRepository.RepositoryID)
		}
		if repository.WorkspaceID != workspaceID {
			return nil, errors.New("repository environment belongs to a different workspace")
		}
		origin := runtimeenv.RepositoryOrigin(repository.Name)
		for _, binding := range repository.SecretBindings {
			if strings.TrimSpace(binding.Key) == "" || strings.TrimSpace(binding.SecretID) == "" {
				return nil, fmt.Errorf("repository %s has an invalid secret binding", repository.ID)
			}
			definitions = append(definitions, runtimeenv.Definition{
				Key: binding.Key, SecretID: binding.SecretID, Origin: origin, WorkspaceID: workspaceID,
			})
		}
	}
	return definitions, nil
}

func (m *Manager) executorProfileEnvironmentDefinitions(ctx context.Context, profileID string) ([]runtimeenv.Definition, error) {
	if m.executorProfileReader == nil || profileID == "" {
		return nil, nil
	}
	profile, err := m.executorProfileReader.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("load executor profile: %w", err)
	}
	if profile == nil {
		return nil, nil
	}
	definitions := make([]runtimeenv.Definition, 0, len(profile.EnvVars))
	for _, value := range profile.EnvVars {
		if strings.TrimSpace(value.Key) == "" {
			continue
		}
		if value.SecretID == "" && value.Value == "" {
			continue
		}
		definitions = append(definitions, runtimeenv.Definition{
			Key: value.Key, Literal: value.Value, SecretID: value.SecretID, Origin: runtimeenv.OriginExecutorProfile,
		})
	}
	return definitions, nil
}
