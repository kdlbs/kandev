package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// AutomationService is the interface the orchestrator uses for automation operations.
type AutomationService interface {
	GetAutomation(ctx context.Context, id string) (*automation.Automation, error)
	RecordRun(ctx context.Context, run *automation.AutomationRun) error
	MarkRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error
	MarkRunSucceededByTaskID(ctx context.Context, taskID string) error
}

// SetAutomationService sets the automation service for handling automation triggers.
func (s *Service) SetAutomationService(svc AutomationService) {
	s.automationService = svc
}

// SetWorktreeManager sets the worktree manager. Used to clean up ephemeral
// worktrees for run-mode automation tasks on completion. Nil-safe — handlers
// skip worktree cleanup when not set.
func (s *Service) SetWorktreeManager(mgr *worktree.Manager) {
	s.worktreeMgr = mgr
}

// subscribeAutomationEvents subscribes to automation-related events on the event bus.
func (s *Service) subscribeAutomationEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.AutomationTriggered, s.handleAutomationTriggered); err != nil {
		s.logger.Error("failed to subscribe to automation.triggered events", zap.Error(err))
	}
}

// handleAutomationTriggered creates a task when an automation trigger fires.
func (s *Service) handleAutomationTriggered(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*automation.AutomationTriggeredEvent)
	if !ok {
		return nil
	}

	s.logger.Info("automation trigger received",
		zap.String("automation_id", evt.AutomationID),
		zap.String("trigger_type", string(evt.TriggerType)))

	if s.automationService == nil || s.reviewTaskCreator == nil {
		s.logger.Warn("automation service or task creator not configured")
		return nil
	}

	go s.createAutomationTask(context.Background(), evt)
	return nil
}

func (s *Service) createAutomationTask(ctx context.Context, evt *automation.AutomationTriggeredEvent) {
	a, err := s.automationService.GetAutomation(ctx, evt.AutomationID)
	if err != nil || a == nil {
		s.logger.Error("failed to load automation for trigger",
			zap.String("automation_id", evt.AutomationID), zap.Error(err))
		s.recordFailedRun(ctx, evt, "automation not found")
		return
	}
	if !a.Enabled {
		s.logger.Debug("automation disabled, skipping",
			zap.String("automation_id", evt.AutomationID))
		return
	}

	// Interpolate prompt with trigger data.
	prompt := automation.InterpolatePrompt(a.Prompt, evt.TriggerType, evt.TriggerData)
	if prompt == "" {
		prompt = fmt.Sprintf("Automation '%s' triggered by %s", a.Name, evt.TriggerType)
	}

	title := s.resolveAutomationTaskTitle(a, evt)
	repositories := s.resolveAutomationRepository(ctx, a, evt)
	if len(repositories) == 0 {
		errMsg := "no repository available — add a repository to the workspace"
		s.logger.Warn("automation skipped: "+errMsg,
			zap.String("automation_id", a.ID),
			zap.String("workspace_id", a.WorkspaceID))
		s.recordFailedRun(ctx, evt, errMsg)
		return
	}

	// Run-mode automations create an ephemeral task hidden from the kanban —
	// the user surfaces them through the AutomationRun row instead. The
	// existing session/launch pipeline still runs against the task. Ephemeral
	// tasks reject a non-empty workflow_id, so we strip workflow fields in
	// run-mode even if the automation row still carries them.
	isRunMode := a.ExecutionMode == automation.ExecutionModeRun
	workflowID := a.WorkflowID
	workflowStepID := a.WorkflowStepID
	if isRunMode {
		workflowID = ""
		workflowStepID = ""
	}
	task, taskErr := s.reviewTaskCreator.CreateReviewTask(ctx, &ReviewTaskRequest{
		WorkspaceID:    a.WorkspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: workflowStepID,
		Title:          title,
		Description:    prompt,
		Repositories:   repositories,
		Metadata: map[string]interface{}{
			"automation_id":       a.ID,
			"automation_name":     a.Name,
			"trigger_id":          evt.TriggerID,
			"trigger_type":        string(evt.TriggerType),
			"agent_profile_id":    a.AgentProfileID,
			"executor_profile_id": a.ExecutorProfileID,
			"execution_mode":      string(a.ExecutionMode),
		},
		IsEphemeral: isRunMode,
		Origin:      models.TaskOriginAutomationRun,
	})
	if taskErr != nil {
		s.logger.Error("failed to create automation task",
			zap.String("automation_id", a.ID), zap.Error(taskErr))
		s.recordFailedRun(ctx, evt, taskErr.Error())
		return
	}

	// Record successful run.
	s.recordSuccessRun(ctx, evt, task.ID)

	// Associate PR with task for github_pr triggers (same as PR Watcher).
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		s.associateAutomationPR(ctx, task.ID, repositories[0].RepositoryID, evt.TriggerData)
	}

	s.logger.Info("created automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID),
		zap.String("execution_mode", string(a.ExecutionMode)),
		zap.String("trigger_type", string(evt.TriggerType)))

	// Auto-start: always for run-mode (the user never sees the task, so no
	// kanban drag triggers it); otherwise honour the workflow step's
	// auto_start_agent on_enter setting.
	if !isRunMode && !s.shouldAutoStartStep(ctx, a.WorkflowStepID) {
		return
	}
	s.autoStartAutomationTask(ctx, a, task, workflowStepID)
}

func (s *Service) autoStartAutomationTask(ctx context.Context, a *automation.Automation, task *models.Task, workflowStepID string) {
	_, err := s.StartTask(
		ctx,
		task.ID,
		a.AgentProfileID,
		"",
		a.ExecutorProfileID,
		"",
		task.Description,
		workflowStepID,
		false,
		true,
		nil,
	)
	if err != nil {
		s.logger.Error("failed to auto-start automation task",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	s.logger.Info("auto-started automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID))
}

// resolveAutomationRepository determines the repository for an automation-triggered task.
// For github_pr triggers, it always extracts repo info from the trigger data —
// the PR's own repo is the only sensible choice when responding to a PR event,
// so an explicit RepositoryID on the automation is ignored.
// For other triggers (scheduled, webhook), it prefers the automation's explicit
// RepositoryID; falls back to the workspace's first repository if unset.
func (s *Service) resolveAutomationRepository(
	ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent,
) []ReviewTaskRepository {
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		return s.resolveGitHubPRTriggerRepository(ctx, a.WorkspaceID, evt.TriggerData)
	}
	if a.RepositoryID != "" {
		return s.resolveExplicitRepository(ctx, a.RepositoryID)
	}
	return s.resolveWorkspaceRepository(ctx, a.WorkspaceID)
}

// resolveExplicitRepository loads the repository named by the automation's
// RepositoryID field and produces a single ReviewTaskRepository entry. The
// task gets pinned to the repo's default branch; the automation has no way
// to specify a checkout branch yet.
func (s *Service) resolveExplicitRepository(
	ctx context.Context, repositoryID string,
) []ReviewTaskRepository {
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil
	}
	repo, err := store.GetRepository(ctx, repositoryID)
	if err != nil || repo == nil {
		s.logger.Warn("failed to load explicit automation repository",
			zap.String("repository_id", repositoryID), zap.Error(err))
		return nil
	}
	baseBranch := repo.DefaultBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	return []ReviewTaskRepository{{
		RepositoryID:   repo.ID,
		BaseBranch:     baseBranch,
		CheckoutBranch: baseBranch,
	}}
}

// resolveGitHubPRTriggerRepository extracts repo owner/name from PR trigger data
// and resolves it via the repository resolver.
func (s *Service) resolveGitHubPRTriggerRepository(
	ctx context.Context, workspaceID string, triggerData json.RawMessage,
) []ReviewTaskRepository {
	if s.repositoryResolver == nil {
		return nil
	}
	var data struct {
		Repo       string `json:"repo"`
		HeadBranch string `json:"head_branch"`
		BaseBranch string `json:"base_branch"`
	}
	if err := json.Unmarshal(triggerData, &data); err != nil || data.Repo == "" {
		return nil
	}
	parts := strings.SplitN(data.Repo, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	owner, name := parts[0], parts[1]
	repoID, baseBranch, err := s.repositoryResolver.ResolveForReview(
		ctx, workspaceID, "github", owner, name, data.BaseBranch,
	)
	if err != nil || repoID == "" {
		s.logger.Warn("failed to resolve PR trigger repository",
			zap.String("repo", data.Repo), zap.Error(err))
		return nil
	}
	return []ReviewTaskRepository{{
		RepositoryID:   repoID,
		BaseBranch:     baseBranch,
		CheckoutBranch: data.HeadBranch,
	}}
}

// resolveWorkspaceRepository looks up the workspace's first repository
// and returns it for task creation. This mirrors how the review watch
// auto-resolves repos — no manual selector needed.
func (s *Service) resolveWorkspaceRepository(
	ctx context.Context, workspaceID string,
) []ReviewTaskRepository {
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil
	}
	repos, err := store.ListRepositories(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("failed to list workspace repositories", zap.Error(err))
		return nil
	}
	if len(repos) == 0 {
		s.logger.Warn("workspace has no repositories",
			zap.String("workspace_id", workspaceID))
		return nil
	}
	repo := repos[0]
	baseBranch := repo.DefaultBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	return []ReviewTaskRepository{{
		RepositoryID: repo.ID,
		BaseBranch:   baseBranch,
	}}
}

// resolveAutomationTaskTitle builds the task title from the automation's template or falls back to a default.
func (s *Service) resolveAutomationTaskTitle(a *automation.Automation, evt *automation.AutomationTriggeredEvent) string {
	if a.TaskTitleTemplate != "" {
		title := automation.InterpolatePrompt(a.TaskTitleTemplate, evt.TriggerType, evt.TriggerData)
		if title != "" {
			return title
		}
	}
	// Fall back to trigger type default from registry.
	if info := automation.GetTriggerTypeInfo(evt.TriggerType); info != nil && info.DefaultTaskTitle != "" {
		title := automation.InterpolatePrompt(info.DefaultTaskTitle, evt.TriggerType, evt.TriggerData)
		if title != "" {
			return title
		}
	}
	return fmt.Sprintf("[Auto] %s", a.Name)
}

// associateAutomationPR links a task to a GitHub PR using trigger data.
func (s *Service) associateAutomationPR(ctx context.Context, taskID, repositoryID string, triggerData json.RawMessage) {
	if s.githubService == nil {
		return
	}
	var data struct {
		Number      float64 `json:"number"`
		Title       string  `json:"title"`
		HTMLURL     string  `json:"html_url"`
		AuthorLogin string  `json:"author_login"`
		Repo        string  `json:"repo"`
		HeadBranch  string  `json:"head_branch"`
		BaseBranch  string  `json:"base_branch"`
		Body        string  `json:"body"`
		Draft       bool    `json:"draft"`
		State       string  `json:"state"`
	}
	if err := json.Unmarshal(triggerData, &data); err != nil || data.Repo == "" {
		return
	}
	parts := strings.SplitN(data.Repo, "/", 2)
	if len(parts) != 2 {
		return
	}
	pr := &github.PR{
		Number:      int(data.Number),
		Title:       data.Title,
		HTMLURL:     data.HTMLURL,
		AuthorLogin: data.AuthorLogin,
		RepoOwner:   parts[0],
		RepoName:    parts[1],
		HeadBranch:  data.HeadBranch,
		BaseBranch:  data.BaseBranch,
		Body:        data.Body,
		Draft:       data.Draft,
		State:       data.State,
	}
	if _, err := s.githubService.AssociatePRWithTask(ctx, taskID, repositoryID, pr); err != nil {
		s.logger.Error("failed to associate PR with automation task",
			zap.String("task_id", taskID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
	}
}

func (s *Service) recordFailedRun(ctx context.Context, evt *automation.AutomationTriggeredEvent, errMsg string) {
	if s.automationService == nil {
		return
	}
	run := &automation.AutomationRun{
		AutomationID: evt.AutomationID,
		TriggerID:    evt.TriggerID,
		TriggerType:  evt.TriggerType,
		Status:       automation.RunStatusFailed,
		DedupKey:     evt.DedupKey,
		TriggerData:  evt.TriggerData,
		ErrorMessage: errMsg,
	}
	if recordErr := s.automationService.RecordRun(ctx, run); recordErr != nil {
		s.logger.Error("failed to record automation run", zap.Error(recordErr))
	}
}

func (s *Service) recordSuccessRun(ctx context.Context, evt *automation.AutomationTriggeredEvent, taskID string) {
	if s.automationService == nil {
		return
	}
	run := &automation.AutomationRun{
		AutomationID: evt.AutomationID,
		TriggerID:    evt.TriggerID,
		TriggerType:  evt.TriggerType,
		TaskID:       taskID,
		Status:       automation.RunStatusTaskCreated,
		DedupKey:     evt.DedupKey,
		TriggerData:  evt.TriggerData,
	}
	if recordErr := s.automationService.RecordRun(ctx, run); recordErr != nil {
		s.logger.Error("failed to record automation run", zap.Error(recordErr))
	}
}

// finalizeAutomationRunIfEphemeral closes out a run-mode automation run when
// its agent terminates. For ephemeral automation tasks it (a) flips the
// AutomationRun row from task_created → succeeded|failed, and (b) reaps the
// per-run worktree immediately. Regular tasks are untouched.
func (s *Service) finalizeAutomationRunIfEphemeral(ctx context.Context, taskID, sessionID string, success bool, errMsg string) {
	if taskID == "" {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	if !task.IsEphemeral || task.Origin != models.TaskOriginAutomationRun {
		return
	}

	if s.automationService != nil {
		var markErr error
		if success {
			markErr = s.automationService.MarkRunSucceededByTaskID(ctx, taskID)
		} else {
			markErr = s.automationService.MarkRunFailedByTaskID(ctx, taskID, errMsg)
		}
		if markErr != nil {
			s.logger.Warn("failed to update automation run terminal status",
				zap.String("task_id", taskID),
				zap.Bool("success", success),
				zap.Error(markErr))
		}
	}

	s.reapAutomationWorktree(ctx, taskID, sessionID)
}

// reapAutomationWorktree removes the ephemeral worktree (and its branch)
// associated with a finished run-mode automation session.
func (s *Service) reapAutomationWorktree(ctx context.Context, taskID, sessionID string) {
	if s.worktreeMgr == nil || sessionID == "" {
		return
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.WorktreeID == "" {
		return
	}
	if err := s.worktreeMgr.RemoveByID(ctx, running.WorktreeID, true); err != nil {
		s.logger.Warn("failed to reap automation worktree",
			zap.String("task_id", taskID),
			zap.String("worktree_id", running.WorktreeID),
			zap.Error(err))
		return
	}
	s.logger.Info("reaped ephemeral automation worktree",
		zap.String("task_id", taskID),
		zap.String("worktree_id", running.WorktreeID))
}
