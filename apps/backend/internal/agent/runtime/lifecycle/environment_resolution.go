package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/runtime/envmetrics"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	// fallbackOfflineBudgetMinutes mirrors the offline-budget default used
	// when a launch's resolved metadata is absent or unparseable (system
	// design part 2 "Budget configuration": absent or zero means 15
	// minutes). The orchestrator validates offline_budget_minutes before
	// launch, so this fallback only needs to match that default, not
	// re-validate the [1,1440] range.
	fallbackOfflineBudgetMinutes = 15
	// toolTimeoutOverheadMinutes is the margin added to the resolved
	// offline budget when raising an agent's declared tool-timeout key
	// (system design part 2 "Harness tool timeout").
	toolTimeoutOverheadMinutes = 10
	// toolTimeoutMinPaddingMinutes is the minimum margin a higher-precedence
	// override must leave above the resolved offline budget before launch
	// rejects it.
	toolTimeoutMinPaddingMinutes = 1
	millisecondsPerMinute        = 60000
)

// ErrToolTimeoutBelowOfflineBudget is returned when a higher-precedence
// environment source sets an agent's declared tool-timeout key below the
// resolved offline budget plus one minute (system design part 2 "Harness
// tool timeout"): a lower harness timeout would let the harness abort a
// waiting Kandev MCP call before the budget itself ends.
var ErrToolTimeoutBelowOfflineBudget = errors.New("harness tool timeout is below the offline budget")

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
	requestEnv := req.Env
	if req.managedGoCachePath != "" {
		requestEnv = cloneStringMap(requestEnv)
		delete(requestEnv, "GOCACHE")
	}
	appendMapDefinitions(&definitions, requestEnv, runtimeenv.OriginManagedRuntime)
	appendAgentProfileDefinitions(&definitions, profileInfo)

	appendStandardDefinitions(&definitions, executionID, req)
	if err := appendAgentRuntimeDefaults(&definitions, agentConfig, resolveOfflineBudgetMinutes(req)); err != nil {
		return nil, err
	}
	appendRequiredCredentialDefinitions(ctx, &definitions, m.credsMgr, agentConfig)
	if req.managedGoCachePath != "" {
		definitions = append(definitions, runtimeenv.Definition{
			Key: "GOCACHE", Literal: req.managedGoCachePath, Origin: runtimeenv.OriginManagedRuntime,
		})
	}

	resolved, records, err := runtimeenv.Resolve(ctx, definitions, m.resolveEnvironmentDefinition)
	if err != nil {
		var secretErr *runtimeenv.SecretError
		if errors.As(err, &secretErr) && secretErr.Origin == runtimeenv.OriginAgentProfile && profileInfo != nil && profileInfo.ProfileName != "" {
			err = fmt.Errorf("agent profile %q: %w", profileInfo.ProfileName, err)
		}
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

// appendAgentRuntimeDefaults converts each of agentConfig's declared runtime
// env entries into a managed-default definition, skipping any key a
// higher-precedence definition already declared. When the agent declares a
// ToolTimeoutEnvKey (system design part 2 "Harness tool timeout"), it also
// enforces that key against offlineBudgetMinutes: a value this call is about
// to inject is raised to cover the budget, and a pre-existing
// higher-precedence value that would undercut the budget fails the launch
// with ErrToolTimeoutBelowOfflineBudget.
func appendAgentRuntimeDefaults(definitions *[]runtimeenv.Definition, agentConfig agents.Agent, offlineBudgetMinutes int) error {
	if agentConfig == nil {
		return nil
	}
	rt := agentConfig.Runtime()
	if rt == nil {
		return nil
	}
	for key, value := range rt.Env {
		if hasEnvironmentDefinition(*definitions, key) {
			if key == rt.ToolTimeoutEnvKey {
				if err := checkToolTimeoutOverride(*definitions, key, offlineBudgetMinutes); err != nil {
					return err
				}
			}
			continue
		}
		if key == rt.ToolTimeoutEnvKey {
			value = raiseToolTimeoutValue(value, offlineBudgetMinutes)
		}
		*definitions = append(*definitions, runtimeenv.Definition{
			Key: key, Literal: value, Origin: runtimeenv.OriginManagedAgentDefaults,
		})
	}
	return nil
}

// resolveOfflineBudgetMinutes reads the offline budget resolved for this
// launch (lifecycle.MetadataKeyOfflineBudgetMinutes). The orchestrator
// validates and clamps the configured value to [1,1440] before launch,
// so an absent or unparseable value here only needs the same permissive
// default agentctl itself falls back to (system design part 2 "Budget
// configuration").
func resolveOfflineBudgetMinutes(req *LaunchRequest) int {
	if req == nil {
		return fallbackOfflineBudgetMinutes
	}
	raw := getMetadataString(req.Metadata, MetadataKeyOfflineBudgetMinutes)
	minutes, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || minutes <= 0 {
		return fallbackOfflineBudgetMinutes
	}
	return minutes
}

// raiseToolTimeoutValue returns the larger of current (a base-10 integer in
// milliseconds) and the offline budget plus its overhead margin, also in
// milliseconds. A current value that fails to parse is treated as unset so
// the budget-derived floor always applies.
func raiseToolTimeoutValue(current string, offlineBudgetMinutes int) string {
	currentMs, err := strconv.Atoi(strings.TrimSpace(current))
	floorMs := (offlineBudgetMinutes + toolTimeoutOverheadMinutes) * millisecondsPerMinute
	if err == nil && currentMs >= floorMs {
		return current
	}
	return strconv.Itoa(floorMs)
}

// checkToolTimeoutOverride enforces that a higher-precedence definition
// already present for the declared tool-timeout key does not undercut the
// offline budget. A definition whose Literal is empty (secret-backed) or
// non-numeric cannot be checked at this pre-resolve boundary and is left to
// the agent CLI itself.
func checkToolTimeoutOverride(definitions []runtimeenv.Definition, key string, offlineBudgetMinutes int) error {
	thresholdMs := (offlineBudgetMinutes + toolTimeoutMinPaddingMinutes) * millisecondsPerMinute
	for _, definition := range definitions {
		if definition.Key != key || definition.Literal == "" {
			continue
		}
		valueMs, err := strconv.Atoi(strings.TrimSpace(definition.Literal))
		if err != nil || valueMs >= thresholdMs {
			continue
		}
		return fmt.Errorf("%s=%dms is below the offline budget of %d minutes (minimum %d minutes): %w",
			key, valueMs, offlineBudgetMinutes, offlineBudgetMinutes+toolTimeoutMinPaddingMinutes,
			ErrToolTimeoutBelowOfflineBudget)
	}
	return nil
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
