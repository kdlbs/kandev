package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	cursorcloudruntime "github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (m *cursorCloudAgentManager) prepareCloudLaunch(ctx context.Context, req *executor.LaunchAgentRequest) (cursorcloudruntime.LaunchInput, error) {
	if err := validateCursorCloudLaunchRequest(req); err != nil {
		return cursorcloudruntime.LaunchInput{}, err
	}
	task, session, err := m.loadCloudTaskSession(ctx, req)
	if err != nil {
		return cursorcloudruntime.LaunchInput{}, err
	}
	userID, err := m.resolveCloudLaunchUser(ctx, task)
	if err != nil {
		return cursorcloudruntime.LaunchInput{}, err
	}
	return m.buildCloudLaunchInput(ctx, req, task, session, userID)
}

func validateCursorCloudLaunchRequest(req *executor.LaunchAgentRequest) error {
	if req == nil || req.SessionID == "" || req.TaskID == "" || req.WorkspaceID == "" || req.AgentProfileID == "" || req.TurnID == "" {
		return errors.New("cursor cloud launch identity is incomplete")
	}
	if cursorCloudLaunchHasUnsupportedOptions(req) {
		return errors.New("cursor cloud does not support this task execution mode")
	}
	if req.McpMode != "" && req.McpMode != cursorCloudMCPModeTask && req.McpMode != cursorCloudMCPModeTaskTitlePending {
		return errors.New("cursor cloud only supports normal task conversations")
	}
	if req.McpMode == "" {
		req.McpMode = cursorCloudMCPModeTask
	}
	if strings.TrimSpace(req.TaskDescription) == "" {
		return errors.New("cursor cloud prompt is required")
	}
	return nil
}

func cursorCloudLaunchHasUnsupportedOptions(req *executor.LaunchAgentRequest) bool {
	unsupported := [...]bool{
		req.IsPassthrough, req.IsEphemeral, req.TaskEnvironmentID != "", req.WorkspacePath != "",
		req.WorkspaceReuseRequired, req.UseWorktree, len(req.WorkspaceFolders) > 0, len(req.Repositories) > 1,
		len(req.Attachments) > 0, len(req.Env) > 0, len(req.EnvironmentDefinitions) > 0,
		req.RouteOverride != nil, len(req.AdditionalSkillSlugs) > 0, req.OfficeAgentProfileID != "",
	}
	for _, unsupportedOption := range unsupported {
		if unsupportedOption {
			return true
		}
	}
	return false
}

func (m *cursorCloudAgentManager) loadCloudTaskSession(
	ctx context.Context,
	req *executor.LaunchAgentRequest,
) (*models.Task, *models.TaskSession, error) {
	if err := m.taskService.AuthorizeTaskAccess(ctx, req.TaskID); err != nil {
		return nil, nil, err
	}
	task, err := m.repo.GetTask(ctx, req.TaskID)
	if err != nil || task == nil {
		return nil, nil, errors.New("cursor cloud task is unavailable")
	}
	if task.ArchivedAt != nil {
		return nil, nil, errors.New("cursor cloud cannot start an archived task")
	}
	session, err := m.repo.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil || session.TaskID != task.ID {
		return nil, nil, errors.New("cursor cloud session is unavailable")
	}
	if task.IsFromOffice || task.Autopilot || task.Origin == models.TaskOriginAutomationRun ||
		session.IsPassthrough || isConfigurationSession(session) {
		return nil, nil, errors.New("cursor cloud does not support Office, automation, or configuration sessions")
	}
	return task, session, nil
}

func (m *cursorCloudAgentManager) resolveCloudLaunchUser(ctx context.Context, task *models.Task) (string, error) {
	workspace, err := m.repo.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil {
		return "", errors.New("cursor cloud workspace is unavailable")
	}
	identity, _ := authn.IdentityFromContext(ctx)
	userID := strings.TrimSpace(identity.UserID)
	if userID == "" {
		userID = strings.TrimSpace(workspace.OwnerID)
	}
	if userID == "" {
		return "", errors.New("cursor cloud user identity is unavailable")
	}
	return userID, nil
}

func isConfigurationSession(session *models.TaskSession) bool {
	if session == nil || session.Metadata == nil {
		return false
	}
	value, _ := session.Metadata["config_mode"].(bool)
	return value
}

func (m *cursorCloudAgentManager) buildCloudLaunchInput(ctx context.Context, req *executor.LaunchAgentRequest, task *models.Task, session *models.TaskSession, userID string) (cursorcloudruntime.LaunchInput, error) {
	executorRecord, executorProfile, secretID, callbackURL, err := m.loadCloudExecutor(ctx, session)
	if err != nil {
		return cursorcloudruntime.LaunchInput{}, err
	}
	modelID, err := m.resolveCloudModel(ctx, req, secretID)
	if err != nil {
		return cursorcloudruntime.LaunchInput{}, err
	}
	repositoryRecord, startingRef, err := m.resolveCloudRepository(ctx, task)
	if err != nil {
		return cursorcloudruntime.LaunchInput{}, err
	}
	repositoryURL := fmt.Sprintf("https://github.com/%s/%s", repositoryRecord.ProviderOwner, repositoryRecord.ProviderName)
	launch := models.ManagedAgentLaunchSnapshot{
		RepositoryID: repositoryRecord.ID, RepositoryURL: repositoryURL, StartingRef: startingRef,
		Model: modelID, CallbackURL: callbackURL, AutoCreatePR: req.AutoCreatePR,
	}
	requestSnapshot := models.ManagedAgentRequestSnapshot{
		Prompt: req.TaskDescription, TurnID: req.TurnID, MCPMode: req.McpMode, RepositoryURL: repositoryURL, StartingRef: startingRef,
		Model: modelID, CallbackURL: callbackURL, AutoCreatePR: req.AutoCreatePR,
	}
	return m.reuseOrCreateCloudBinding(ctx, req, task, session, userID, executorRecord, executorProfile, secretID, launch, requestSnapshot)
}

func (m *cursorCloudAgentManager) loadCloudExecutor(
	ctx context.Context,
	session *models.TaskSession,
) (*models.Executor, *models.ExecutorProfile, string, string, error) {
	executorRecord, err := m.repo.GetExecutor(ctx, session.ExecutorID)
	if err != nil || executorRecord == nil {
		return nil, nil, "", "", errors.New("cursor cloud executor is unavailable")
	}
	if executorRecord.Status != models.ExecutorStatusActive || executorRecord.Type != models.ExecutorTypeCursorCloud {
		return nil, nil, "", "", errors.New("cursor cloud executor is unavailable")
	}
	executorProfile, err := m.repo.GetExecutorProfile(ctx, session.ExecutorProfileID)
	if err != nil || executorProfile == nil || executorProfile.ExecutorID != executorRecord.ID {
		return nil, nil, "", "", errors.New("cursor cloud executor profile is unavailable")
	}
	secretID := strings.TrimSpace(executorProfile.Config[cursorcloud.ExecutorConfigSecretID])
	callbackURL := strings.TrimSpace(executorProfile.Config[cursorcloud.ExecutorConfigCallbackURL])
	if err := secrets.ValidateGlobalReference(ctx, m.secrets, secretID); err != nil {
		return nil, nil, "", "", errors.New("cursor cloud API key reference is unavailable")
	}
	if err := cursorcloud.ValidateCallbackURL(callbackURL); err != nil {
		return nil, nil, "", "", errors.New("cursor cloud callback URL is invalid")
	}
	return executorRecord, executorProfile, secretID, callbackURL, nil
}

func (m *cursorCloudAgentManager) resolveCloudModel(
	ctx context.Context,
	req *executor.LaunchAgentRequest,
	secretID string,
) (string, error) {
	profileInfo, err := m.ResolveAgentProfile(ctx, req.AgentProfileID)
	if err != nil || profileInfo == nil ||
		(profileInfo.AgentID != agents.CursorCloudAgentID && !strings.EqualFold(profileInfo.AgentName, "Cursor Cloud")) {
		return "", errors.New("cursor cloud agent profile is incompatible")
	}
	modelID := strings.TrimSpace(req.ModelOverride)
	if modelID == "" {
		modelID = strings.TrimSpace(profileInfo.Model)
	}
	apiKey, err := m.secrets.Reveal(ctx, secretID)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return "", errors.New("cursor cloud API key is unavailable")
	}
	client, err := cursorcloud.NewRuntimeClient(apiKey, m.enabled())
	if err != nil {
		return "", errors.New("cursor cloud is unavailable")
	}
	if err := cursorcloud.ValidateModel(ctx, client, modelID); err != nil {
		return "", err
	}
	return modelID, nil
}

func (m *cursorCloudAgentManager) resolveCloudRepository(ctx context.Context, task *models.Task) (*models.Repository, string, error) {
	repositories, err := m.repo.ListTaskRepositories(ctx, task.ID)
	if err != nil || len(repositories) != 1 || repositories[0] == nil {
		return nil, "", errors.New("cursor cloud requires exactly one attached GitHub repository")
	}
	taskRepository := repositories[0]
	repositoryRecord, err := m.repo.GetRepository(ctx, taskRepository.RepositoryID)
	if err != nil || !validCursorGitHubRepository(repositoryRecord) {
		return nil, "", errors.New("cursor cloud requires an attached GitHub repository")
	}
	if m.github == nil {
		return nil, "", errors.New("GitHub branch validation is unavailable")
	}
	startingRef := strings.TrimSpace(taskRepository.BaseBranch)
	if startingRef == "" {
		startingRef = strings.TrimSpace(repositoryRecord.DefaultBranch)
	}
	if startingRef == "" {
		return nil, "", errors.New("cursor cloud starting ref is required")
	}
	branches, err := m.github.ListRepoBranchesForWorkspace(ctx, task.WorkspaceID, repositoryRecord.ProviderOwner, repositoryRecord.ProviderName)
	if err != nil || !containsBranch(branches, startingRef) {
		return nil, "", errors.New("cursor cloud starting ref must be a published GitHub branch")
	}
	if folders, folderErr := m.repo.ListTaskWorkspaceFolders(ctx, task.ID); folderErr != nil || len(folders) != 0 {
		return nil, "", errors.New("cursor cloud does not support attached local folders")
	}
	return repositoryRecord, startingRef, nil
}

func (m *cursorCloudAgentManager) reuseOrCreateCloudBinding(
	ctx context.Context,
	req *executor.LaunchAgentRequest,
	task *models.Task,
	session *models.TaskSession,
	userID string,
	executorRecord *models.Executor,
	executorProfile *models.ExecutorProfile,
	secretID string,
	launch models.ManagedAgentLaunchSnapshot,
	requestSnapshot models.ManagedAgentRequestSnapshot,
) (cursorcloudruntime.LaunchInput, error) {
	if binding, getErr := m.repo.GetManagedAgentBindingBySession(ctx, session.ID); getErr == nil {
		operation, operationErr := m.repo.GetManagedAgentOperationByPromptTurnID(ctx, cursorcloudruntime.InitialPromptTurnID(session.ID))
		if !cloudLaunchMatches(binding, operation, operationErr, task, userID, executorRecord, executorProfile, launch, requestSnapshot) {
			return cursorcloudruntime.LaunchInput{}, errors.New("cursor cloud session is already bound to another launch")
		}
		return cursorcloudruntime.LaunchInput{Binding: binding, Operation: operation}, nil
	} else if !errors.Is(getErr, repository.ErrManagedAgentBindingNotFound) {
		return cursorcloudruntime.LaunchInput{}, fmt.Errorf("load Cursor Cloud session binding: %w", getErr)
	}
	binding := &models.ManagedAgentBinding{
		ID: uuid.NewString(), SessionID: session.ID, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		UserID: userID, ExecutionID: uuid.NewString(), ProviderKind: string(agentruntime.RuntimeCursorCloud),
		ExecutorID: executorRecord.ID, ExecutorProfileID: executorProfile.ID, CredentialRef: secretID,
		RemoteAgentID: "bc-" + uuid.NewString(), Lifecycle: models.ManagedAgentBindingCreating, Launch: launch,
	}
	operation := &models.ManagedAgentOperation{
		ID: uuid.NewString(), BindingID: binding.ID, PromptTurnID: cursorcloudruntime.InitialPromptTurnID(session.ID),
		Kind: models.ManagedAgentOperationCreate, RequestDigest: cursorCloudRequestDigest(requestSnapshot),
		RequestSnapshot: requestSnapshot,
	}
	return cursorcloudruntime.LaunchInput{Binding: binding, Operation: operation}, nil
}

func cloudLaunchMatches(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	operationErr error,
	task *models.Task,
	userID string,
	executorRecord *models.Executor,
	executorProfile *models.ExecutorProfile,
	launch models.ManagedAgentLaunchSnapshot,
	requestSnapshot models.ManagedAgentRequestSnapshot,
) bool {
	return operationErr == nil && operation != nil && operation.RequestSnapshot == requestSnapshot &&
		operation.RequestDigest == cursorCloudRequestDigest(requestSnapshot) && binding.TaskID == task.ID &&
		binding.WorkspaceID == task.WorkspaceID && binding.UserID == userID &&
		binding.ExecutorID == executorRecord.ID && binding.ExecutorProfileID == executorProfile.ID && binding.Launch == launch
}

func validCursorGitHubRepository(repository *models.Repository) bool {
	if repository == nil || !strings.EqualFold(repository.Provider, "github") ||
		strings.TrimSpace(repository.ProviderOwner) == "" || strings.TrimSpace(repository.ProviderName) == "" {
		return false
	}
	host := strings.TrimSpace(repository.ProviderHost)
	return host == "" || strings.EqualFold(host, "github.com")
}

func containsBranch(branches []github.RepoBranch, name string) bool {
	for _, branch := range branches {
		if branch.Name == name {
			return true
		}
	}
	return false
}

func cursorCloudRequestDigest(snapshot models.ManagedAgentRequestSnapshot) string {
	encoded := strings.Join([]string{snapshot.Prompt, snapshot.TurnID, snapshot.MCPMode, snapshot.RepositoryURL, snapshot.StartingRef, snapshot.Model, snapshot.CallbackURL, fmt.Sprint(snapshot.AutoCreatePR)}, "\x00")
	digest := sha256.Sum256([]byte(encoded))
	return hex.EncodeToString(digest[:])
}
