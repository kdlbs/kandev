package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"go.uber.org/zap"
)

type taskChangeRequestGitHubService interface {
	githubTaskChangeRequestService
	UpdateTaskCIOptions(context.Context, string, github.TaskCIOptionsPatch) (*github.TaskCIOptionsResponse, error)
}

type taskChangeRequestGitLabService interface {
	gitlabTaskChangeRequestService
	UpdateTaskMRAutomationOptions(context.Context, string, gitlab.TaskMRAutomationPatch) (*gitlab.TaskMRAutomationResponse, error)
}

type taskChangeRequestAutomationCoordinator struct {
	tasks    taskChangeRequestTaskLookup
	github   taskChangeRequestGitHubService
	gitlab   taskChangeRequestGitLabService
	eventBus mcphandlers.EventBus
	logger   *logger.Logger
}

func newTaskChangeRequestAutomationCoordinator(
	tasks taskChangeRequestTaskLookup,
	githubProvider taskChangeRequestGitHubService,
	gitlabProvider taskChangeRequestGitLabService,
	eventBus mcphandlers.EventBus,
	log *logger.Logger,
) *taskChangeRequestAutomationCoordinator {
	if log == nil {
		log = logger.Default()
	}
	return &taskChangeRequestAutomationCoordinator{
		tasks: tasks, github: githubProvider, gitlab: gitlabProvider, eventBus: eventBus, logger: log,
	}
}

type taskChangeRequestAutomationPreflight struct {
	provider string
	changes  []mcphandlers.TaskChangeRequest
	settings *mcphandlers.TaskChangeRequestProviderSettings
}

func (c *taskChangeRequestAutomationCoordinator) UpdateTaskChangeRequestAutomation(
	ctx context.Context,
	taskID string,
	request mcphandlers.TaskChangeRequestAutomationRequest,
) (mcphandlers.TaskChangeRequestAutomationResult, error) {
	result := mcphandlers.TaskChangeRequestAutomationResult{
		TaskID:     strings.TrimSpace(taskID),
		Status:     mcphandlers.TaskChangeRequestAutomationStatusFailed,
		Providers:  []mcphandlers.TaskChangeRequestAutomationProviderResult{},
		StateKnown: false,
	}
	if c == nil || c.tasks == nil {
		return result, errors.New("task change request automation is unavailable")
	}
	if result.TaskID == "" {
		return result, errors.New("task_id is required")
	}
	task, err := c.tasks.GetTask(ctx, result.TaskID)
	if err != nil || task == nil {
		return result, errors.New("task not found")
	}

	providers := automationTargetProviders(request.Target)
	for _, provider := range providers {
		result.Providers = append(result.Providers, mcphandlers.TaskChangeRequestAutomationProviderResult{
			Provider: provider,
			Status:   mcphandlers.TaskChangeRequestAutomationProviderSkipped,
			Affected: []mcphandlers.TaskChangeRequestAutomationIdentity{},
		})
	}

	snapshot, err := c.readTaskChangeRequestSnapshot(ctx, result.TaskID)
	if err != nil {
		return result, errors.New("failed to read task change request state")
	}

	preflights, firstPreflightErr := c.preflightAutomationProviders(request, providers, snapshot, &result)
	if firstPreflightErr != nil {
		return result, firstPreflightErr
	}

	for applied, provider := range providers {
		providerResult := automationProviderResult(&result, provider)
		preflight := preflights[provider]
		if updateErr := c.applyAutomationProvider(ctx, result.TaskID, request, provider, providerResult, preflight); updateErr != nil {
			result.Status = automationStatusAfterFailure(applied)
			result.StateKnown = false
			return result, updateErr
		}
	}

	result.Status = mcphandlers.TaskChangeRequestAutomationStatusApplied
	result.StateKnown = automationProviderStatesKnown(result.Providers)
	return result, nil
}

func (c *taskChangeRequestAutomationCoordinator) readTaskChangeRequestSnapshot(
	ctx context.Context, taskID string,
) (mcphandlers.TaskChangeRequestReadResponse, error) {
	reader := taskChangeRequestReader{
		tasks: c.tasks,
		providers: []taskChangeRequestProviderAdapter{
			newGitHubTaskChangeRequestAdapter(c.github),
			newGitLabTaskChangeRequestAdapter(c.tasks, c.gitlab),
		},
	}
	return reader.GetTaskChangeRequests(ctx, taskID)
}

func (c *taskChangeRequestAutomationCoordinator) preflightAutomationProviders(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	providers []string,
	snapshot mcphandlers.TaskChangeRequestReadResponse,
	result *mcphandlers.TaskChangeRequestAutomationResult,
) (map[string]taskChangeRequestAutomationPreflight, error) {
	preflights := make(map[string]taskChangeRequestAutomationPreflight, len(providers))
	var firstErr error
	for _, provider := range providers {
		preflight, err := c.preflightProvider(request, provider, snapshot)
		if err == nil {
			preflights[provider] = preflight
			continue
		}
		c.recordAutomationPreflightFailure(result, provider, err)
		if firstErr == nil {
			firstErr = err
		}
	}
	return preflights, firstErr
}

func (c *taskChangeRequestAutomationCoordinator) recordAutomationPreflightFailure(
	result *mcphandlers.TaskChangeRequestAutomationResult, provider string, err error,
) {
	providerResult := automationProviderResult(result, provider)
	providerResult.Status = mcphandlers.TaskChangeRequestAutomationProviderFailed
	providerResult.ErrorCode = "preflight_failed"
	providerResult.ErrorMessage = err.Error()
	c.logger.Warn("task change request automation preflight failed",
		zap.String("task_id", result.TaskID), zap.String("provider", provider), zap.Error(err))
}

func (c *taskChangeRequestAutomationCoordinator) applyAutomationProvider(
	ctx context.Context,
	taskID string,
	request mcphandlers.TaskChangeRequestAutomationRequest,
	provider string,
	providerResult *mcphandlers.TaskChangeRequestAutomationProviderResult,
	preflight taskChangeRequestAutomationPreflight,
) error {
	switch provider {
	case mcphandlers.TaskChangeRequestProviderGitHub:
		response, err := c.updateGitHub(ctx, taskID, request)
		if err != nil {
			c.markAutomationProviderFailure(taskID, providerResult, err)
			return err
		}
		*providerResult = c.resultFromGitHubValue(request, preflight, response)
		c.publishGitHubOptionsUpdated(ctx, response)
		return nil
	case mcphandlers.TaskChangeRequestProviderGitLab:
		response, err := c.updateGitLab(ctx, taskID, request, preflight)
		if err != nil {
			c.markAutomationProviderFailure(taskID, providerResult, err)
			return err
		}
		*providerResult = c.resultFromGitLabValue(request, preflight, response)
		c.publishGitLabOptionsUpdated(ctx, response)
		return nil
	default:
		providerResult.Status = mcphandlers.TaskChangeRequestAutomationProviderFailed
		providerResult.ErrorCode = "unsupported_provider"
		providerResult.ErrorMessage = "provider is not supported"
		return errors.New("provider is not supported")
	}
}

func automationProviderStatesKnown(
	providers []mcphandlers.TaskChangeRequestAutomationProviderResult,
) bool {
	for _, provider := range providers {
		if !provider.StateKnown {
			return false
		}
	}
	return true
}

func automationTargetProviders(target mcphandlers.TaskChangeRequestAutomationTarget) []string {
	if target.Scope == mcphandlers.TaskChangeRequestAutomationScopeAssociation {
		return []string{target.Provider}
	}
	providers := append([]string(nil), target.Providers...)
	sort.Strings(providers)
	return providers
}

func (c *taskChangeRequestAutomationCoordinator) preflightProvider(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	provider string,
	snapshot mcphandlers.TaskChangeRequestReadResponse,
) (taskChangeRequestAutomationPreflight, error) {
	settings := taskChangeRequestProviderSettings(snapshot, provider)
	if settings == nil || settings.Status != mcphandlers.TaskChangeRequestProviderStatusAvailable || !settings.Available {
		return taskChangeRequestAutomationPreflight{}, fmt.Errorf("%s provider is unavailable", provider)
	}
	preflight := taskChangeRequestAutomationPreflight{provider: provider, settings: cloneTaskChangeRequestProviderSettings(settings)}
	for _, change := range snapshot.ChangeRequests {
		if change.Provider == provider {
			preflight.changes = append(preflight.changes, change)
		}
	}
	if request.Target.Scope == mcphandlers.TaskChangeRequestAutomationScopeAssociation {
		return preflightAssociationAutomation(request, provider, preflight)
	}
	return preflightTaskAutomation(request, provider, preflight)
}

func preflightAssociationAutomation(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	provider string,
	preflight taskChangeRequestAutomationPreflight,
) (taskChangeRequestAutomationPreflight, error) {
	if request.Target.Provider != provider {
		return taskChangeRequestAutomationPreflight{}, errors.New("association provider does not match target provider")
	}
	matching := make([]mcphandlers.TaskChangeRequest, 0, 1)
	for _, change := range preflight.changes {
		if pointerString(change.RepositoryID) == request.Target.RepositoryID && change.Number == request.Target.Number {
			matching = append(matching, change)
		}
	}
	if len(matching) == 0 {
		return taskChangeRequestAutomationPreflight{}, errors.New("change request is not linked to this task")
	}
	if len(matching) != 1 || matching[0].IdentityStatus != mcphandlers.TaskChangeRequestIdentityResolved {
		return taskChangeRequestAutomationPreflight{}, errors.New("change request identity is unresolved or ambiguous")
	}
	if err := validateAutomationCapabilities(matching[0], request.Patch); err != nil {
		return taskChangeRequestAutomationPreflight{}, err
	}
	preflight.changes = matching
	return preflight, nil
}

func preflightTaskAutomation(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	provider string,
	preflight taskChangeRequestAutomationPreflight,
) (taskChangeRequestAutomationPreflight, error) {
	if hasAutomationSwitchPatch(request.Patch) {
		if len(preflight.changes) == 0 {
			return taskChangeRequestAutomationPreflight{}, fmt.Errorf("%s has no linked change requests", provider)
		}
		for _, change := range preflight.changes {
			if change.IdentityStatus != mcphandlers.TaskChangeRequestIdentityResolved {
				return taskChangeRequestAutomationPreflight{}, fmt.Errorf("%s contains an unresolved or ambiguous change request identity", provider)
			}
			if err := validateAutomationCapabilities(change, request.Patch); err != nil {
				return taskChangeRequestAutomationPreflight{}, err
			}
		}
		return preflight, nil
	}
	if err := validateAutomationProviderCapabilities(preflight.settings, request.Patch); err != nil {
		return taskChangeRequestAutomationPreflight{}, err
	}
	return preflight, nil
}

func taskChangeRequestProviderSettings(
	snapshot mcphandlers.TaskChangeRequestReadResponse, provider string,
) *mcphandlers.TaskChangeRequestProviderSettings {
	for i := range snapshot.ProviderSettings {
		if snapshot.ProviderSettings[i].Provider == provider {
			return &snapshot.ProviderSettings[i]
		}
	}
	return nil
}

func cloneTaskChangeRequestProviderSettings(
	settings *mcphandlers.TaskChangeRequestProviderSettings,
) *mcphandlers.TaskChangeRequestProviderSettings {
	if settings == nil {
		return nil
	}
	copy := *settings
	return &copy
}

func validateAutomationProviderCapabilities(
	settings *mcphandlers.TaskChangeRequestProviderSettings,
	patch mcphandlers.TaskChangeRequestAutomationPatch,
) error {
	if settings == nil {
		return errors.New("provider capabilities are unavailable")
	}
	if patch.AutoFixPromptOverride != nil && !providerSupports(settings.Provider, "custom_auto_fix_prompt") {
		return errors.New("provider does not support custom auto-fix prompts")
	}
	return nil
}

func validateAutomationCapabilities(
	change mcphandlers.TaskChangeRequest,
	patch mcphandlers.TaskChangeRequestAutomationPatch,
) error {
	for _, capability := range automationPatchCapabilities(patch) {
		if !change.Capabilities[capability] {
			reason := change.CapabilityReasons[capability]
			if reason == "" {
				reason = "unsupported"
			}
			return fmt.Errorf("change request cannot use %s: %s", capability, reason)
		}
	}
	return nil
}

func providerSupports(provider, capability string) bool {
	return taskChangeRequestProviderCapabilities(provider)[capability]
}

func automationPatchCapabilities(patch mcphandlers.TaskChangeRequestAutomationPatch) []string {
	capabilities := make([]string, 0, 2)
	if patch.AutoFixEnabled != nil {
		capabilities = append(capabilities, "auto_fix")
	}
	if patch.AutoMergeEnabled != nil {
		capabilities = append(capabilities, "auto_merge")
	}
	if patch.PromptOnReviewRequested != nil || patch.PromptOnMerged != nil || patch.PromptOnClosed != nil {
		capabilities = append(capabilities, "lifecycle_notifications")
	}
	if patch.AutoFixPromptOverride != nil {
		capabilities = append(capabilities, "custom_auto_fix_prompt")
	}
	return capabilities
}

func hasAutomationSwitchPatch(patch mcphandlers.TaskChangeRequestAutomationPatch) bool {
	return patch.AutoFixEnabled != nil || patch.AutoMergeEnabled != nil ||
		patch.PromptOnReviewRequested != nil || patch.PromptOnMerged != nil || patch.PromptOnClosed != nil
}

func (c *taskChangeRequestAutomationCoordinator) updateGitHub(
	ctx context.Context,
	taskID string,
	request mcphandlers.TaskChangeRequestAutomationRequest,
) (*github.TaskCIOptionsResponse, error) {
	if c.github == nil {
		return nil, errors.New("github provider is unavailable")
	}
	patch := github.TaskCIOptionsPatch{
		AutoFixEnabled:          request.Patch.AutoFixEnabled,
		AutoMergeEnabled:        request.Patch.AutoMergeEnabled,
		AutoFixPromptOverride:   request.Patch.AutoFixPromptOverride,
		PromptOnReviewRequested: request.Patch.PromptOnReviewRequested,
		PromptOnMerged:          request.Patch.PromptOnMerged,
		PromptOnClosed:          request.Patch.PromptOnClosed,
	}
	if request.Target.Scope == mcphandlers.TaskChangeRequestAutomationScopeAssociation {
		repositoryID := request.Target.RepositoryID
		number := request.Target.Number
		patch.RepositoryID = &repositoryID
		patch.PRNumber = &number
	}
	return c.github.UpdateTaskCIOptions(ctx, taskID, patch)
}

func (c *taskChangeRequestAutomationCoordinator) updateGitLab(
	ctx context.Context,
	taskID string,
	request mcphandlers.TaskChangeRequestAutomationRequest,
	preflight taskChangeRequestAutomationPreflight,
) (*gitlab.TaskMRAutomationResponse, error) {
	if c.gitlab == nil {
		return nil, errors.New("gitlab provider is unavailable")
	}
	patch := gitlab.TaskMRAutomationPatch{
		AutoFixEnabled:          request.Patch.AutoFixEnabled,
		AutoMergeEnabled:        request.Patch.AutoMergeEnabled,
		AutoFixPromptOverride:   request.Patch.AutoFixPromptOverride,
		PromptOnReviewRequested: request.Patch.PromptOnReviewRequested,
		PromptOnMerged:          request.Patch.PromptOnMerged,
		PromptOnClosed:          request.Patch.PromptOnClosed,
	}
	if request.Target.Scope == mcphandlers.TaskChangeRequestAutomationScopeAssociation {
		if len(preflight.changes) != 1 {
			return nil, errors.New("change request identity is not unique")
		}
		change := preflight.changes[0]
		repositoryID := change.ProviderRepositoryID
		projectPath := change.ProviderProjectPath
		number := change.Number
		patch.RepositoryID = &repositoryID
		patch.ProjectPath = &projectPath
		patch.MRIID = &number
	}
	return c.gitlab.UpdateTaskMRAutomationOptions(ctx, taskID, patch)
}

func automationProviderResult(
	result *mcphandlers.TaskChangeRequestAutomationResult, provider string,
) *mcphandlers.TaskChangeRequestAutomationProviderResult {
	for i := range result.Providers {
		if result.Providers[i].Provider == provider {
			return &result.Providers[i]
		}
	}
	result.Providers = append(result.Providers, mcphandlers.TaskChangeRequestAutomationProviderResult{
		Provider: provider, Status: mcphandlers.TaskChangeRequestAutomationProviderSkipped,
		Affected: []mcphandlers.TaskChangeRequestAutomationIdentity{},
	})
	return &result.Providers[len(result.Providers)-1]
}

func (c *taskChangeRequestAutomationCoordinator) markAutomationProviderFailure(
	taskID string,
	providerResult *mcphandlers.TaskChangeRequestAutomationProviderResult,
	err error,
) {
	providerResult.Status = mcphandlers.TaskChangeRequestAutomationProviderFailed
	providerResult.StateKnown = false
	providerResult.ErrorCode = "provider_update_failed"
	providerResult.ErrorMessage = "provider update failed; resulting state is unknown"
	c.logger.Error("task change request automation provider update failed",
		zap.String("task_id", taskID), zap.String("provider", providerResult.Provider), zap.Error(err))
}

func automationStatusAfterFailure(applied int) string {
	if applied > 0 {
		return mcphandlers.TaskChangeRequestAutomationStatusPartial
	}
	return mcphandlers.TaskChangeRequestAutomationStatusFailed
}

func (c *taskChangeRequestAutomationCoordinator) resultFromGitHubValue(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	preflight taskChangeRequestAutomationPreflight,
	response *github.TaskCIOptionsResponse,
) mcphandlers.TaskChangeRequestAutomationProviderResult {
	providerResult := mcphandlers.TaskChangeRequestAutomationProviderResult{
		Provider:   mcphandlers.TaskChangeRequestProviderGitHub,
		Status:     mcphandlers.TaskChangeRequestAutomationProviderApplied,
		Affected:   []mcphandlers.TaskChangeRequestAutomationIdentity{},
		StateKnown: response != nil,
	}
	if response == nil {
		providerResult.ErrorCode = "readback_unknown"
		providerResult.ErrorMessage = "provider update completed without a readable result"
		return providerResult
	}
	providerResult.ResultingSettings = githubTaskChangeRequestSettings(response)
	copyProviderSettingsState(providerResult.ResultingSettings, preflight.settings)
	providerResult.ResultingChangeRequests = cloneTaskChangeRequests(preflight.changes)
	applyGitHubTaskChangeRequestAutomation(providerResult.ResultingChangeRequests, response)
	if hasAutomationSwitchPatch(request.Patch) {
		providerResult.Affected = githubAffectedIdentities(request, preflight, response)
	}
	return providerResult
}

func (c *taskChangeRequestAutomationCoordinator) resultFromGitLabValue(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	preflight taskChangeRequestAutomationPreflight,
	response *gitlab.TaskMRAutomationResponse,
) mcphandlers.TaskChangeRequestAutomationProviderResult {
	providerResult := mcphandlers.TaskChangeRequestAutomationProviderResult{
		Provider:   mcphandlers.TaskChangeRequestProviderGitLab,
		Status:     mcphandlers.TaskChangeRequestAutomationProviderApplied,
		Affected:   []mcphandlers.TaskChangeRequestAutomationIdentity{},
		StateKnown: response != nil,
	}
	if response == nil {
		providerResult.ErrorCode = "readback_unknown"
		providerResult.ErrorMessage = "provider update completed without a readable result"
		return providerResult
	}
	providerResult.ResultingSettings = gitlabTaskChangeRequestSettings(response)
	copyProviderSettingsState(providerResult.ResultingSettings, preflight.settings)
	providerResult.ResultingChangeRequests = cloneTaskChangeRequests(preflight.changes)
	applyGitLabTaskChangeRequestAutomation(providerResult.ResultingChangeRequests, response)
	if hasAutomationSwitchPatch(request.Patch) {
		providerResult.Affected = gitlabAffectedIdentities(request, preflight, response)
	}
	return providerResult
}

func copyProviderSettingsState(
	destination *mcphandlers.TaskChangeRequestProviderSettings,
	source *mcphandlers.TaskChangeRequestProviderSettings,
) {
	if destination == nil || source == nil {
		return
	}
	destination.Status = source.Status
	destination.Configured = source.Configured
	destination.Available = source.Available
	destination.ReasonCode = source.ReasonCode
}

func cloneTaskChangeRequests(changes []mcphandlers.TaskChangeRequest) []mcphandlers.TaskChangeRequest {
	cloned := make([]mcphandlers.TaskChangeRequest, len(changes))
	copy(cloned, changes)
	return cloned
}

func githubAffectedIdentities(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	preflight taskChangeRequestAutomationPreflight,
	response *github.TaskCIOptionsResponse,
) []mcphandlers.TaskChangeRequestAutomationIdentity {
	if request.Target.Scope == mcphandlers.TaskChangeRequestAutomationScopeAssociation {
		return []mcphandlers.TaskChangeRequestAutomationIdentity{{
			Provider: request.Target.Provider, RepositoryID: request.Target.RepositoryID, Number: request.Target.Number,
		}}
	}
	identities := make([]mcphandlers.TaskChangeRequestAutomationIdentity, 0, len(response.PROptions))
	for _, option := range response.PROptions {
		if option == nil {
			continue
		}
		identities = append(identities, mcphandlers.TaskChangeRequestAutomationIdentity{
			Provider:     mcphandlers.TaskChangeRequestProviderGitHub,
			RepositoryID: option.RepositoryID,
			Number:       option.PRNumber,
		})
	}
	if len(identities) == 0 {
		for _, change := range preflight.changes {
			if change.RepositoryID != nil {
				identities = append(identities, mcphandlers.TaskChangeRequestAutomationIdentity{
					Provider: change.Provider, RepositoryID: *change.RepositoryID, Number: change.Number,
				})
			}
		}
	}
	return identities
}

func gitlabAffectedIdentities(
	request mcphandlers.TaskChangeRequestAutomationRequest,
	preflight taskChangeRequestAutomationPreflight,
	response *gitlab.TaskMRAutomationResponse,
) []mcphandlers.TaskChangeRequestAutomationIdentity {
	if request.Target.Scope == mcphandlers.TaskChangeRequestAutomationScopeAssociation {
		return []mcphandlers.TaskChangeRequestAutomationIdentity{{
			Provider: request.Target.Provider, RepositoryID: request.Target.RepositoryID, Number: request.Target.Number,
		}}
	}
	byRawIdentity := make(map[string]mcphandlers.TaskChangeRequest, len(preflight.changes))
	for _, change := range preflight.changes {
		key := gitLabAutomationIdentityKey(change.ProviderRepositoryID, change.ProviderProjectPath, change.Number)
		byRawIdentity[key] = change
	}
	identities := make([]mcphandlers.TaskChangeRequestAutomationIdentity, 0, len(response.MROptions))
	for _, option := range response.MROptions {
		if option == nil {
			continue
		}
		change, ok := byRawIdentity[gitLabAutomationIdentityKey(option.RepositoryID, option.ProjectPath, option.MRIID)]
		repositoryID := option.RepositoryID
		if ok && change.RepositoryID != nil {
			repositoryID = *change.RepositoryID
		}
		identities = append(identities, mcphandlers.TaskChangeRequestAutomationIdentity{
			Provider:     mcphandlers.TaskChangeRequestProviderGitLab,
			RepositoryID: repositoryID,
			Number:       option.MRIID,
		})
	}
	return identities
}

func gitLabAutomationIdentityKey(repositoryID, projectPath string, number int) string {
	return fmt.Sprintf("%s|%s|%d", strings.TrimSpace(repositoryID), strings.Trim(strings.TrimSpace(projectPath), "/"), number)
}

func (c *taskChangeRequestAutomationCoordinator) publishGitHubOptionsUpdated(
	ctx context.Context, response *github.TaskCIOptionsResponse,
) {
	if c.eventBus == nil || response == nil {
		return
	}
	event := bus.NewEvent(events.GitHubTaskCIOptionsUpdated, "mcp", response)
	if err := c.eventBus.Publish(ctx, events.GitHubTaskCIOptionsUpdated, event); err != nil {
		c.logger.Warn("failed to publish MCP PR automation options update", zap.Error(err))
	}
}

func (c *taskChangeRequestAutomationCoordinator) publishGitLabOptionsUpdated(
	ctx context.Context, response *gitlab.TaskMRAutomationResponse,
) {
	if c.eventBus == nil || response == nil {
		return
	}
	event := bus.NewEvent(events.GitLabTaskMROptionsUpdated, "mcp", response)
	if err := c.eventBus.Publish(ctx, events.GitLabTaskMROptionsUpdated, event); err != nil {
		c.logger.Warn("failed to publish MCP MR automation options update", zap.Error(err))
	}
}
