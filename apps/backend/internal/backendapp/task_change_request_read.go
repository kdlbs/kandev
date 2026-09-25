package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	agentprojects "github.com/kandev/kandev/internal/projects"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type taskChangeRequestTaskLookup interface {
	GetTask(context.Context, string) (*models.Task, error)
	GetRepository(context.Context, string) (*models.Repository, error)
}

type taskChangeRequestProviderResult struct {
	Configured     bool
	Available      bool
	Status         string
	ReasonCode     string
	Settings       *mcphandlers.TaskChangeRequestProviderSettings
	ChangeRequests []mcphandlers.TaskChangeRequest
	Errors         []error
}

const taskChangeRequestProviderReadFailedReason = "provider_read_failed"

type taskChangeRequestProviderAdapter interface {
	Name() string
	ReadTaskChangeRequests(context.Context, *models.Task) taskChangeRequestProviderResult
}

type githubTaskChangeRequestService interface {
	ListTaskPRs(context.Context, []string) (map[string][]*github.TaskPR, error)
	GetTaskCIOptionsResponse(context.Context, string) (*github.TaskCIOptionsResponse, error)
	GetWorkspaceAuthStatus(context.Context, string, string) (*github.WorkspaceAuthStatus, error)
}

type gitlabTaskChangeRequestService interface {
	ListTaskMRsByTask(context.Context, string) ([]*gitlab.TaskMR, error)
	GetTaskMRAutomationResponse(context.Context, string) (*gitlab.TaskMRAutomationResponse, error)
	GetStatusForWorkspace(context.Context, string) (*gitlab.Status, error)
}

type taskChangeRequestReader struct {
	tasks     taskChangeRequestTaskLookup
	providers []taskChangeRequestProviderAdapter
}

func newTaskChangeRequestReader(
	tasks *taskservice.Service,
	githubProvider githubTaskChangeRequestService,
	gitlabProvider gitlabTaskChangeRequestService,
) *taskChangeRequestReader {
	providers := []taskChangeRequestProviderAdapter{
		newGitHubTaskChangeRequestAdapter(githubProvider),
		newGitLabTaskChangeRequestAdapter(tasks, gitlabProvider),
	}
	return &taskChangeRequestReader{tasks: tasks, providers: providers}
}

func (r *taskChangeRequestReader) GetTaskChangeRequests(
	ctx context.Context, taskID string,
) (mcphandlers.TaskChangeRequestReadResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return mcphandlers.TaskChangeRequestReadResponse{}, fmt.Errorf("task_id is required")
	}
	if r == nil || r.tasks == nil {
		return mcphandlers.TaskChangeRequestReadResponse{}, fmt.Errorf("task change request task reader is unavailable")
	}
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return mcphandlers.TaskChangeRequestReadResponse{}, err
	}
	if task == nil {
		return mcphandlers.TaskChangeRequestReadResponse{}, fmt.Errorf("task %q not found", taskID)
	}

	providers := append([]taskChangeRequestProviderAdapter(nil), r.providers...)
	sort.SliceStable(providers, func(i, j int) bool { return providers[i].Name() < providers[j].Name() })
	result := mcphandlers.TaskChangeRequestReadResponse{
		TaskID:               taskID,
		Complete:             true,
		ChangeRequests:       []mcphandlers.TaskChangeRequest{},
		ProviderSettings:     []mcphandlers.TaskChangeRequestProviderSettings{},
		ProviderCapabilities: []mcphandlers.TaskChangeRequestProviderCapabilities{},
		Errors:               []mcphandlers.TaskChangeRequestProviderError{},
	}
	for _, provider := range providers {
		appendTaskChangeRequestProviderRead(ctx, task, provider, &result)
	}
	return result, nil
}

func (r *taskChangeRequestReader) ListWorkerChangeRequests(
	ctx context.Context, taskID string,
) ([]agentprojects.WorkerChangeRequest, error) {
	response, err := r.GetTaskChangeRequests(ctx, taskID)
	if err != nil {
		return nil, err
	}
	changes := response.ChangeRequests
	if len(changes) > agentprojects.MaxWorkerChangeRequests {
		changes = changes[:agentprojects.MaxWorkerChangeRequests]
	}
	result := make([]agentprojects.WorkerChangeRequest, 0, len(changes))
	for _, change := range changes {
		result = append(result, agentprojects.WorkerChangeRequest{
			Provider: change.Provider, Number: change.Number, URL: change.URL,
			Title: change.Title, State: change.State,
			Draft: change.Draft != nil && *change.Draft,
		})
	}
	return result, nil
}

func appendTaskChangeRequestProviderRead(
	ctx context.Context,
	task *models.Task,
	provider taskChangeRequestProviderAdapter,
	result *mcphandlers.TaskChangeRequestReadResponse,
) {
	if provider == nil {
		return
	}
	name := provider.Name()
	read := provider.ReadTaskChangeRequests(ctx, task)
	status := normalizeTaskChangeRequestProviderStatus(read.Status, read.Configured, read.Available, len(read.Errors) > 0)
	reasonCode := strings.TrimSpace(read.ReasonCode)
	if len(read.Errors) > 0 {
		status = mcphandlers.TaskChangeRequestProviderStatusFailed
		reasonCode = taskChangeRequestProviderReadFailedReason
	}
	if reasonCode == "" {
		reasonCode = defaultTaskChangeRequestProviderReason(status)
	}
	settings := read.Settings
	if settings == nil {
		settings = &mcphandlers.TaskChangeRequestProviderSettings{}
	}
	settings.Provider = name
	settings.Status = status
	settings.Configured = read.Configured
	settings.Available = read.Available
	settings.ReasonCode = reasonCode
	if settings.PromptScope == "" {
		settings.PromptScope = mcphandlers.TaskChangeRequestPromptScopeTaskProvider
	}
	result.ProviderSettings = append(result.ProviderSettings, *settings)
	result.ProviderCapabilities = append(result.ProviderCapabilities, mcphandlers.TaskChangeRequestProviderCapabilities{
		Provider:     name,
		Status:       status,
		Configured:   read.Configured,
		Available:    read.Available,
		ReasonCode:   reasonCode,
		Capabilities: taskChangeRequestProviderCapabilities(name),
	})
	appendTaskChangeRequests(result, name, read.Available, read.ChangeRequests)
	appendTaskChangeRequestProviderErrors(result, name, read.Errors)
}

func appendTaskChangeRequests(
	result *mcphandlers.TaskChangeRequestReadResponse,
	provider string,
	available bool,
	changes []mcphandlers.TaskChangeRequest,
) {
	for _, change := range changes {
		if change.Provider == "" {
			change.Provider = provider
		}
		decorateTaskChangeRequestCapabilities(&change, available, provider)
		result.ChangeRequests = append(result.ChangeRequests, change)
	}
}

func appendTaskChangeRequestProviderErrors(
	result *mcphandlers.TaskChangeRequestReadResponse, provider string, providerErrors []error,
) {
	for _, providerErr := range providerErrors {
		result.Complete = false
		code, message := sanitizeTaskChangeRequestReadError(providerErr)
		result.Errors = append(result.Errors, mcphandlers.TaskChangeRequestProviderError{
			Provider: provider, Code: code, Message: message,
		})
	}
}

func normalizeTaskChangeRequestProviderStatus(status string, configured, available, failed bool) string {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case mcphandlers.TaskChangeRequestProviderStatusAvailable,
		mcphandlers.TaskChangeRequestProviderStatusUnavailable,
		mcphandlers.TaskChangeRequestProviderStatusFailed:
		if failed {
			return mcphandlers.TaskChangeRequestProviderStatusFailed
		}
		return status
	}
	if failed {
		return mcphandlers.TaskChangeRequestProviderStatusFailed
	}
	if available {
		return mcphandlers.TaskChangeRequestProviderStatusAvailable
	}
	if configured {
		return mcphandlers.TaskChangeRequestProviderStatusUnavailable
	}
	return mcphandlers.TaskChangeRequestProviderStatusUnavailable
}

func defaultTaskChangeRequestProviderReason(status string) string {
	switch status {
	case mcphandlers.TaskChangeRequestProviderStatusAvailable:
		return ""
	case mcphandlers.TaskChangeRequestProviderStatusFailed:
		return taskChangeRequestProviderReadFailedReason
	default:
		return "provider_not_configured"
	}
}

func sanitizeTaskChangeRequestReadError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	// Provider and database details stay in the backend log. The MCP caller
	// needs a stable category so it can retry a failed provider read safely.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "provider_read_cancelled", "provider read was cancelled"
	}
	return taskChangeRequestProviderReadFailedReason, "provider read failed"
}

func finalizeTaskChangeRequestProviderResult(result *taskChangeRequestProviderResult) {
	if len(result.Errors) == 0 || result.Status == mcphandlers.TaskChangeRequestProviderStatusUnavailable {
		return
	}
	result.Status = mcphandlers.TaskChangeRequestProviderStatusFailed
	result.ReasonCode = taskChangeRequestProviderReadFailedReason
}

var taskChangeRequestCapabilityNames = []string{
	"link",
	"unlink",
	"replace_same_provider",
	"auto_fix",
	"auto_merge",
	"lifecycle_notifications",
	"custom_auto_fix_prompt",
	"auto_fix_outcome_reporting",
}

func taskChangeRequestProviderCapabilities(provider string) map[string]bool {
	capabilities := make(map[string]bool, len(taskChangeRequestCapabilityNames))
	for _, name := range taskChangeRequestCapabilityNames {
		capabilities[name] = name != "auto_fix_outcome_reporting" || provider == mcphandlers.TaskChangeRequestProviderGitHub
	}
	return capabilities
}

func decorateTaskChangeRequestCapabilities(
	change *mcphandlers.TaskChangeRequest, available bool, provider string,
) {
	if change == nil {
		return
	}
	change.Capabilities = taskChangeRequestProviderCapabilities(provider)
	if change.CapabilityReasons == nil {
		change.CapabilityReasons = map[string]string{}
	}
	if !available {
		for name, supported := range change.Capabilities {
			if supported {
				change.Capabilities[name] = false
				change.CapabilityReasons[name] = "provider_unavailable"
			}
		}
		return
	}
	if change.IdentityStatus == "" {
		if change.RepositoryID == nil {
			change.IdentityStatus = mcphandlers.TaskChangeRequestIdentityUnresolved
		} else {
			change.IdentityStatus = mcphandlers.TaskChangeRequestIdentityResolved
		}
	}
	if change.IdentityStatus == mcphandlers.TaskChangeRequestIdentityResolved {
		return
	}
	reason := "identity_unresolved"
	if change.IdentityStatus == mcphandlers.TaskChangeRequestIdentityAmbiguous {
		reason = "identity_ambiguous"
	}
	for name, supported := range change.Capabilities {
		if supported {
			change.Capabilities[name] = false
			change.CapabilityReasons[name] = reason
		}
	}
}

type githubTaskChangeRequestAdapter struct {
	service githubTaskChangeRequestService
}

func newGitHubTaskChangeRequestAdapter(service githubTaskChangeRequestService) taskChangeRequestProviderAdapter {
	return githubTaskChangeRequestAdapter{service: service}
}

func (a githubTaskChangeRequestAdapter) Name() string {
	return mcphandlers.TaskChangeRequestProviderGitHub
}

func (a githubTaskChangeRequestAdapter) ReadTaskChangeRequests(
	ctx context.Context, task *models.Task,
) taskChangeRequestProviderResult {
	result := taskChangeRequestProviderResult{
		Status:     mcphandlers.TaskChangeRequestProviderStatusUnavailable,
		ReasonCode: "provider_not_configured",
	}
	if a.service == nil || task == nil {
		return result
	}
	a.readGitHubTaskChangeRequests(ctx, task, &result)
	a.readGitHubStatus(ctx, task, &result)
	a.readGitHubAutomationSettings(ctx, task, &result)
	finalizeTaskChangeRequestProviderResult(&result)
	return result
}

func (a githubTaskChangeRequestAdapter) readGitHubTaskChangeRequests(
	ctx context.Context, task *models.Task, result *taskChangeRequestProviderResult,
) {
	byTask, err := a.service.ListTaskPRs(ctx, []string{task.ID})
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	for _, pr := range byTask[task.ID] {
		if pr != nil {
			result.ChangeRequests = append(result.ChangeRequests, githubTaskChangeRequest(pr))
		}
	}
}

func (a githubTaskChangeRequestAdapter) readGitHubStatus(
	ctx context.Context, task *models.Task, result *taskChangeRequestProviderResult,
) {
	authStatus, err := a.service.GetWorkspaceAuthStatus(ctx, task.WorkspaceID, "")
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	if authStatus == nil {
		return
	}
	result.Configured = authStatus.Automation != nil
	result.Available = authStatus.Authenticated
	if !result.Configured {
		return
	}
	result.Status = mcphandlers.TaskChangeRequestProviderStatusAvailable
	if !result.Available {
		result.Status = mcphandlers.TaskChangeRequestProviderStatusUnavailable
		result.ReasonCode = "credentials_unavailable"
	}
}

func (a githubTaskChangeRequestAdapter) readGitHubAutomationSettings(
	ctx context.Context, task *models.Task, result *taskChangeRequestProviderResult,
) {
	if !result.Configured {
		return
	}
	options, err := a.service.GetTaskCIOptionsResponse(ctx, task.ID)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	if options == nil {
		return
	}
	result.Settings = githubTaskChangeRequestSettings(options)
	applyGitHubTaskChangeRequestAutomation(result.ChangeRequests, options)
}

func githubTaskChangeRequest(pr *github.TaskPR) mcphandlers.TaskChangeRequest {
	change := mcphandlers.TaskChangeRequest{
		Provider:       mcphandlers.TaskChangeRequestProviderGitHub,
		Number:         pr.PRNumber,
		URL:            pr.PRURL,
		Title:          pr.PRTitle,
		State:          normalizeTaskChangeRequestState(pr.State),
		ProviderState:  pr.State,
		Draft:          pr.IsDraft,
		BaseRef:        pr.BaseBranch,
		HeadRef:        pr.HeadBranch,
		HeadSHA:        pr.HeadSHA,
		MergedAt:       pr.MergedAt,
		ClosedAt:       pr.ClosedAt,
		UpdatedAt:      taskChangeRequestTimePointer(pr.UpdatedAt),
		IdentityStatus: mcphandlers.TaskChangeRequestIdentityResolved,
	}
	if strings.TrimSpace(pr.RepositoryID) == "" {
		change.IdentityStatus = mcphandlers.TaskChangeRequestIdentityUnresolved
	} else {
		repositoryID := strings.TrimSpace(pr.RepositoryID)
		change.RepositoryID = &repositoryID
	}
	return change
}

func githubTaskChangeRequestSettings(options *github.TaskCIOptionsResponse) *mcphandlers.TaskChangeRequestProviderSettings {
	return &mcphandlers.TaskChangeRequestProviderSettings{
		Provider:               mcphandlers.TaskChangeRequestProviderGitHub,
		AutoFixPromptOverride:  options.AutoFixPromptOverride,
		EffectiveAutoFixPrompt: options.EffectiveAutoFixPrompt,
		UsingDefaultPrompt:     options.UsingDefaultPrompt,
		AutoFixMaxRounds:       options.AutoFixMaxRounds,
		PromptScope:            mcphandlers.TaskChangeRequestPromptScopeTaskProvider,
	}
}

func applyGitHubTaskChangeRequestAutomation(
	changes []mcphandlers.TaskChangeRequest, options *github.TaskCIOptionsResponse,
) {
	optionByIdentity := make(map[string]*github.TaskPRAutomationOptions, len(options.PROptions))
	for _, option := range options.PROptions {
		if option == nil {
			continue
		}
		optionByIdentity[taskChangeRequestIdentityKey(option.RepositoryID, option.PRNumber)] = option
	}
	stateByIdentity := make(map[string]*github.TaskCIPRAutomationState, len(options.PRStates))
	for _, state := range options.PRStates {
		if state == nil {
			continue
		}
		stateByIdentity[taskChangeRequestIdentityKey(state.RepositoryID, state.PRNumber)] = state
	}
	for i := range changes {
		change := &changes[i]
		key := taskChangeRequestIdentityKey(pointerString(change.RepositoryID), change.Number)
		if option := optionByIdentity[key]; option != nil {
			change.Automation = &mcphandlers.TaskChangeRequestAutomation{
				AutoFixEnabled:          boolPointer(option.AutoFixEnabled),
				AutoMergeEnabled:        boolPointer(option.AutoMergeEnabled),
				PromptOnReviewRequested: boolPointer(option.PromptOnReviewRequested),
				PromptOnMerged:          boolPointer(option.PromptOnMerged),
				PromptOnClosed:          boolPointer(option.PromptOnClosed),
			}
		}
		if state := stateByIdentity[key]; state != nil {
			rounds := state.AutoFixRoundCount
			change.AutomationStatus = &mcphandlers.TaskChangeRequestAutomationStatus{
				AutoFixRoundCount: &rounds,
				LastError:         state.LastError,
				StateKnown:        true,
			}
		}
	}
}

type gitlabTaskChangeRequestAdapter struct {
	tasks   taskChangeRequestTaskLookup
	service gitlabTaskChangeRequestService
}

func newGitLabTaskChangeRequestAdapter(
	tasks taskChangeRequestTaskLookup, service gitlabTaskChangeRequestService,
) taskChangeRequestProviderAdapter {
	return gitlabTaskChangeRequestAdapter{tasks: tasks, service: service}
}

func (a gitlabTaskChangeRequestAdapter) Name() string {
	return mcphandlers.TaskChangeRequestProviderGitLab
}

func (a gitlabTaskChangeRequestAdapter) ReadTaskChangeRequests(
	ctx context.Context, task *models.Task,
) taskChangeRequestProviderResult {
	result := taskChangeRequestProviderResult{
		Status:     mcphandlers.TaskChangeRequestProviderStatusUnavailable,
		ReasonCode: "provider_not_configured",
	}
	if a.service == nil || task == nil {
		return result
	}
	a.readGitLabTaskChangeRequests(ctx, task, &result)
	a.readGitLabStatus(ctx, task, &result)
	a.readGitLabAutomationSettings(ctx, task, &result)
	finalizeTaskChangeRequestProviderResult(&result)
	return result
}

func (a gitlabTaskChangeRequestAdapter) readGitLabTaskChangeRequests(
	ctx context.Context, task *models.Task, result *taskChangeRequestProviderResult,
) {
	mrs, err := a.service.ListTaskMRsByTask(ctx, task.ID)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	changes, resolveErr := a.gitLabTaskChangeRequestsWithErrors(ctx, task, mrs)
	result.ChangeRequests = changes
	if resolveErr != nil {
		result.Errors = append(result.Errors, resolveErr)
	}
}

func (a gitlabTaskChangeRequestAdapter) readGitLabStatus(
	ctx context.Context, task *models.Task, result *taskChangeRequestProviderResult,
) {
	status, err := a.service.GetStatusForWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	if status == nil {
		return
	}
	if connectionErr := strings.TrimSpace(status.ConnectionError); connectionErr != "" {
		result.Errors = append(result.Errors, errors.New(connectionErr))
	}
	result.Configured = strings.TrimSpace(status.AuthMethod) != "" && strings.TrimSpace(status.AuthMethod) != "none"
	result.Configured = result.Configured || status.TokenConfigured || strings.TrimSpace(status.Username) != ""
	result.Available = status.Authenticated
	if !result.Configured {
		return
	}
	result.Status = mcphandlers.TaskChangeRequestProviderStatusAvailable
	if !result.Available {
		result.Status = mcphandlers.TaskChangeRequestProviderStatusUnavailable
		result.ReasonCode = "credentials_unavailable"
	}
}

func (a gitlabTaskChangeRequestAdapter) readGitLabAutomationSettings(
	ctx context.Context, task *models.Task, result *taskChangeRequestProviderResult,
) {
	if !result.Configured {
		return
	}
	options, err := a.service.GetTaskMRAutomationResponse(ctx, task.ID)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	if options == nil {
		return
	}
	result.Settings = gitlabTaskChangeRequestSettings(options)
	applyGitLabTaskChangeRequestAutomation(result.ChangeRequests, options)
}

func (a gitlabTaskChangeRequestAdapter) gitLabTaskChangeRequests(
	ctx context.Context, task *models.Task, mrs []*gitlab.TaskMR,
) []mcphandlers.TaskChangeRequest {
	changes, _ := a.gitLabTaskChangeRequestsWithErrors(ctx, task, mrs)
	return changes
}

func (a gitlabTaskChangeRequestAdapter) gitLabTaskChangeRequestsWithErrors(
	ctx context.Context, task *models.Task, mrs []*gitlab.TaskMR,
) ([]mcphandlers.TaskChangeRequest, error) {
	changes := make([]mcphandlers.TaskChangeRequest, 0, len(mrs))
	pathsByIdentity := make(map[string]map[string]struct{})
	var resolveErrs []error
	for _, mr := range mrs {
		if mr == nil {
			continue
		}
		change, err := a.gitLabTaskChangeRequestWithError(ctx, task, mr)
		if err != nil {
			resolveErrs = append(resolveErrs, err)
		}
		change.ProviderProjectPath = strings.Trim(strings.TrimSpace(mr.ProjectPath), "/")
		change.ProviderRepositoryID = strings.TrimSpace(mr.RepositoryID)
		changes = append(changes, change)
		if change.RepositoryID != nil {
			key := taskChangeRequestIdentityKey(pointerString(change.RepositoryID), change.Number)
			if pathsByIdentity[key] == nil {
				pathsByIdentity[key] = make(map[string]struct{})
			}
			pathsByIdentity[key][change.ProviderProjectPath] = struct{}{}
		}
	}
	for i := range changes {
		change := &changes[i]
		if change.RepositoryID == nil {
			continue
		}
		key := taskChangeRequestIdentityKey(pointerString(change.RepositoryID), change.Number)
		if len(pathsByIdentity[key]) > 1 {
			change.IdentityStatus = mcphandlers.TaskChangeRequestIdentityAmbiguous
		}
	}
	return changes, errors.Join(resolveErrs...)
}

func (a gitlabTaskChangeRequestAdapter) gitLabTaskChangeRequest(
	ctx context.Context, task *models.Task, mr *gitlab.TaskMR,
) mcphandlers.TaskChangeRequest {
	change, _ := a.gitLabTaskChangeRequestWithError(ctx, task, mr)
	return change
}

func (a gitlabTaskChangeRequestAdapter) gitLabTaskChangeRequestWithError(
	ctx context.Context, task *models.Task, mr *gitlab.TaskMR,
) (mcphandlers.TaskChangeRequest, error) {
	change := mcphandlers.TaskChangeRequest{
		Provider:            mcphandlers.TaskChangeRequestProviderGitLab,
		Number:              mr.MRIID,
		URL:                 mr.MRURL,
		Title:               mr.MRTitle,
		State:               normalizeTaskChangeRequestState(mr.State),
		ProviderState:       mr.State,
		ProviderProjectPath: strings.Trim(strings.TrimSpace(mr.ProjectPath), "/"),
		BaseRef:             mr.BaseBranch,
		HeadRef:             mr.HeadBranch,
		HeadSHA:             mr.HeadSHA,
		Draft:               boolPointer(mr.Draft),
		MergedAt:            mr.MergedAt,
		ClosedAt:            mr.ClosedAt,
		UpdatedAt:           taskChangeRequestTimePointer(mr.UpdatedAt),
		IdentityStatus:      mcphandlers.TaskChangeRequestIdentityUnresolved,
	}
	if repositoryID, identityStatus, err := a.resolveGitLabRepositoryIdentityWithError(ctx, task, mr); err != nil {
		return change, err
	} else if repositoryID != "" {
		change.RepositoryID = &repositoryID
		change.IdentityStatus = identityStatus
	} else {
		change.IdentityStatus = identityStatus
	}
	return change, nil
}

func (a gitlabTaskChangeRequestAdapter) resolveGitLabRepositoryIdentity(
	ctx context.Context, task *models.Task, mr *gitlab.TaskMR,
) (string, string) {
	repositoryID, identityStatus, _ := a.resolveGitLabRepositoryIdentityWithError(ctx, task, mr)
	return repositoryID, identityStatus
}

func (a gitlabTaskChangeRequestAdapter) resolveGitLabRepositoryIdentityWithError(
	ctx context.Context, task *models.Task, mr *gitlab.TaskMR,
) (string, string, error) {
	if repositoryID := strings.TrimSpace(mr.RepositoryID); repositoryID != "" {
		return repositoryID, mcphandlers.TaskChangeRequestIdentityResolved, nil
	}
	if a.tasks == nil || task == nil {
		return "", mcphandlers.TaskChangeRequestIdentityUnresolved, nil
	}
	candidates, err := a.gitLabRepositoryCandidatesWithError(ctx, task, mr)
	if err != nil {
		return "", mcphandlers.TaskChangeRequestIdentityUnresolved, err
	}
	switch len(candidates) {
	case 0:
		return "", mcphandlers.TaskChangeRequestIdentityUnresolved, nil
	case 1:
		for repositoryID := range candidates {
			return repositoryID, mcphandlers.TaskChangeRequestIdentityResolved, nil
		}
	default:
		return "", mcphandlers.TaskChangeRequestIdentityAmbiguous, nil
	}
	return "", mcphandlers.TaskChangeRequestIdentityUnresolved, nil
}

func (a gitlabTaskChangeRequestAdapter) gitLabRepositoryCandidates(
	ctx context.Context, task *models.Task, mr *gitlab.TaskMR,
) map[string]struct{} {
	candidates, _ := a.gitLabRepositoryCandidatesWithError(ctx, task, mr)
	return candidates
}

func (a gitlabTaskChangeRequestAdapter) gitLabRepositoryCandidatesWithError(
	ctx context.Context, task *models.Task, mr *gitlab.TaskMR,
) (map[string]struct{}, error) {
	candidates := make(map[string]struct{})
	var lookupErrs []error
	for _, taskRepository := range task.Repositories {
		if taskRepository == nil || strings.TrimSpace(taskRepository.RepositoryID) == "" {
			continue
		}
		repository, err := a.tasks.GetRepository(ctx, taskRepository.RepositoryID)
		if err != nil {
			lookupErrs = append(lookupErrs, err)
			continue
		}
		if repository == nil || repository.WorkspaceID != task.WorkspaceID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(repository.Provider), mcphandlers.TaskChangeRequestProviderGitLab) {
			continue
		}
		if normalizeProviderHost(repository.ProviderHost) != normalizeProviderHost(mr.Host) {
			continue
		}
		if !sameGitLabProjectPath(repository, mr.ProjectPath) {
			continue
		}
		candidates[repository.ID] = struct{}{}
	}
	return candidates, errors.Join(lookupErrs...)
}

func sameGitLabProjectPath(repository *models.Repository, projectPath string) bool {
	projectPath = strings.Trim(strings.TrimSpace(projectPath), "/")
	if projectPath == "" {
		return false
	}
	for _, path := range []string{
		strings.Trim(strings.TrimSpace(repository.ProviderOwner)+"/"+strings.TrimSpace(repository.ProviderName), "/"),
		strings.Trim(strings.TrimSpace(repository.ProviderScope)+"/"+strings.TrimSpace(repository.ProviderName), "/"),
	} {
		if path != "/" && path == projectPath {
			return true
		}
	}
	return false
}

func gitlabTaskChangeRequestSettings(options *gitlab.TaskMRAutomationResponse) *mcphandlers.TaskChangeRequestProviderSettings {
	return &mcphandlers.TaskChangeRequestProviderSettings{
		Provider:               mcphandlers.TaskChangeRequestProviderGitLab,
		AutoFixPromptOverride:  options.AutoFixPromptOverride,
		EffectiveAutoFixPrompt: options.EffectiveAutoFixPrompt,
		UsingDefaultPrompt:     options.UsingDefaultPrompt,
		AutoFixMaxRounds:       options.AutoFixMaxRounds,
		PromptScope:            mcphandlers.TaskChangeRequestPromptScopeTaskProvider,
	}
}

func applyGitLabTaskChangeRequestAutomation(
	changes []mcphandlers.TaskChangeRequest, options *gitlab.TaskMRAutomationResponse,
) {
	for i := range changes {
		change := &changes[i]
		for _, option := range options.MROptions {
			if option == nil || option.MRIID != change.Number {
				continue
			}
			if gitLabAutomationRepositoryIDMatches(option.RepositoryID, change) &&
				strings.Trim(strings.TrimSpace(option.ProjectPath), "/") == change.ProviderProjectPath {
				change.Automation = &mcphandlers.TaskChangeRequestAutomation{
					AutoFixEnabled:          boolPointer(option.AutoFixEnabled),
					AutoMergeEnabled:        boolPointer(option.AutoMergeEnabled),
					PromptOnReviewRequested: boolPointer(option.PromptOnReviewRequested),
					PromptOnMerged:          boolPointer(option.PromptOnMerged),
					PromptOnClosed:          boolPointer(option.PromptOnClosed),
				}
				break
			}
		}
		for _, state := range options.MRStates {
			if state == nil || state.MRIID != change.Number {
				continue
			}
			if gitLabAutomationRepositoryIDMatches(state.RepositoryID, change) &&
				strings.Trim(strings.TrimSpace(state.ProjectPath), "/") == change.ProviderProjectPath {
				rounds := state.AutoFixRoundCount
				lastError := state.LastError
				if lastError == nil {
					lastError = state.LastSyncError
				}
				change.AutomationStatus = &mcphandlers.TaskChangeRequestAutomationStatus{
					AutoFixRoundCount: &rounds,
					LastError:         lastError,
					StateKnown:        true,
				}
				break
			}
		}
	}
}

func gitLabAutomationRepositoryIDMatches(repositoryID string, change *mcphandlers.TaskChangeRequest) bool {
	if change == nil {
		return false
	}
	repositoryID = strings.TrimSpace(repositoryID)
	if repositoryID == strings.TrimSpace(change.ProviderRepositoryID) {
		return true
	}
	return repositoryID == strings.TrimSpace(pointerString(change.RepositoryID))
}

func normalizeTaskChangeRequestState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open", "opened":
		return "open"
	case "closed":
		return "closed"
	case "merged":
		return "merged"
	default:
		return "unknown"
	}
}

func normalizeProviderHost(host string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(host)), "/")
}

func taskChangeRequestTimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}

func taskChangeRequestIdentityKey(repositoryID string, number int) string {
	return fmt.Sprintf("%s#%d", repositoryID, number)
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boolPointer(value bool) *bool {
	copy := value
	return &copy
}
