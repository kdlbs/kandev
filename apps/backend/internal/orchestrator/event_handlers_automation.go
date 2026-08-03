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
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const automationDefaultBaseBranch = "main"

// AutomationService is the interface the orchestrator uses for automation operations.
type AutomationService interface {
	GetAutomation(ctx context.Context, id string) (*automation.Automation, error)
	RecordRun(ctx context.Context, run *automation.AutomationRun) error
	MarkRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error
	MarkRunSucceededByTaskID(ctx context.Context, taskID string) error
}

// taskCleaner deletes a task a firing has abandoned.
//
// Deliberately one method wide, and asserted at the call site rather than added
// to repoStore: that interface is implemented by several mocks, and none of
// them should have to grow a method for a path they never take.
type taskCleaner interface {
	DeleteTask(ctx context.Context, id string) error
}

// SetAutomationService sets the automation service for handling automation triggers.
func (s *Service) SetAutomationService(svc AutomationService) {
	s.automationService = svc
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

	// Every automation produces the same kind of run: an ordinary, persistent
	// task tagged with origin=automation_run. The origin — not is_ephemeral —
	// is what keeps it off the kanban and out of task lists, so the task keeps
	// its worktree and stays repliable. Workflow fields are passed through as
	// configured; they are optional and nothing about the row is special-cased
	// on write.
	task, taskErr := s.reviewTaskCreator.CreateReviewTask(ctx, &ReviewTaskRequest{
		WorkspaceID:    a.WorkspaceID,
		WorkflowID:     a.WorkflowID,
		WorkflowStepID: a.WorkflowStepID,
		Title:          title,
		Description:    prompt,
		Repositories:   repositories,
		Metadata: map[string]interface{}{
			"automation_id":                 a.ID,
			"automation_name":               a.Name,
			"trigger_id":                    evt.TriggerID,
			"trigger_type":                  string(evt.TriggerType),
			models.MetaKeyAgentProfileID:    a.AgentProfileID,
			models.MetaKeyExecutorProfileID: a.ExecutorProfileID,
		},
		Origin: models.TaskOriginAutomationRun,
	})
	if taskErr != nil {
		s.logger.Error("failed to create automation task",
			zap.String("automation_id", a.ID), zap.Error(taskErr))
		s.recordFailedRun(ctx, evt, taskErr.Error())
		return
	}

	// The run row is the only record that this firing happened: it carries the
	// concurrency accounting and it is the sole way the work is reachable,
	// since the task is hidden from every board and list. Launching an agent
	// without it would leave an automation running that nothing can see and
	// nothing can finalize, so a failed write stops the firing here — and takes
	// the task with it, or the hidden task outlives the run it belonged to.
	if err := s.recordSuccessRun(ctx, evt, task.ID); err != nil {
		s.logger.Error("failed to record automation run; abandoning the firing",
			zap.String("automation_id", a.ID),
			zap.String("task_id", task.ID),
			zap.Error(err))
		s.deleteAbandonedTask(ctx, a.ID, task.ID)
		return
	}

	// Associate PR with task for github_pr triggers (same as PR Watcher).
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		s.associateAutomationPR(ctx, task.ID, repositories[0].RepositoryID, evt.TriggerData)
	}

	s.logger.Info("created automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID),
		zap.String("trigger_type", string(evt.TriggerType)))

	// Auto-start unconditionally: nobody opens an automation's task to drag
	// it, so the trigger has to be the start signal. A workflow step's
	// auto_start_agent setting is irrelevant here — it describes board
	// behaviour and automation runs never reach the board.
	s.autoStartAutomationTask(ctx, a, task, task.WorkflowStepID)
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
		// The run row was written before the launch, so a start that never
		// happened would otherwise sit at task_created forever and hold a
		// max_concurrent_runs slot — one failed launch would stop the
		// automation permanently, with no completion event coming to free it.
		s.markAutomationRunTerminal(ctx, task.ID, false, err.Error())
		return
	}
	s.logger.Info("auto-started automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID))
}

// resolveAutomationRepository determines the repositories for an
// automation-triggered task. For github_pr triggers, it always extracts
// repo info from the trigger data — the PR's own repo is the only sensible
// choice when responding to a PR event, so the automation's RepositoryIDs
// are ignored. For other triggers (scheduled, webhook), it prefers the
// automation's explicit RepositoryIDs; falls back to the workspace's first
// repository if unset.
func (s *Service) resolveAutomationRepository(
	ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent,
) []ReviewTaskRepository {
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		return s.resolveGitHubPRTriggerRepository(ctx, a.WorkspaceID, evt.TriggerData)
	}
	if len(a.RepositoryIDs) > 0 {
		return s.resolveExplicitRepositories(ctx, a.RepositoryIDs)
	}
	return s.resolveWorkspaceRepository(ctx, a.WorkspaceID)
}

// resolveExplicitRepositories loads each repository named by the
// automation's RepositoryIDs and produces one ReviewTaskRepository entry
// per resolvable ID, in order. Each task repository gets pinned to its own
// default branch — automations have no way to specify a checkout branch
// yet. An ID that fails to load is skipped (with a warning) rather than
// aborting the whole firing, so one bad repository doesn't sink the others.
func (s *Service) resolveExplicitRepositories(
	ctx context.Context, repositoryIDs []string,
) []ReviewTaskRepository {
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil
	}
	resolved := make([]ReviewTaskRepository, 0, len(repositoryIDs))
	for _, repositoryID := range repositoryIDs {
		repo, err := store.GetRepository(ctx, repositoryID)
		if err != nil || repo == nil {
			s.logger.Warn("failed to load explicit automation repository",
				zap.String("repository_id", repositoryID), zap.Error(err))
			continue
		}
		baseBranch := repo.DefaultBranch
		if baseBranch == "" {
			baseBranch = automationDefaultBaseBranch
		}
		resolved = append(resolved, ReviewTaskRepository{
			RepositoryID:   repo.ID,
			BaseBranch:     baseBranch,
			CheckoutBranch: baseBranch,
		})
	}
	return resolved
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
		baseBranch = automationDefaultBaseBranch
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
			return taskservice.TruncateTaskTitle(title)
		}
	}
	// Fall back to trigger type default from registry.
	if info := automation.GetTriggerTypeInfo(evt.TriggerType); info != nil && info.DefaultTaskTitle != "" {
		title := automation.InterpolatePrompt(info.DefaultTaskTitle, evt.TriggerType, evt.TriggerData)
		if title != "" {
			return taskservice.TruncateTaskTitle(title)
		}
	}
	return taskservice.TruncateTaskTitle(fmt.Sprintf("[Auto] %s", a.Name))
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

// recordSuccessRun writes the row that makes a firing visible and countable.
// It reports failure rather than logging it: the caller must not launch an
// agent for a run nothing can see.
// deleteAbandonedTask removes a task whose run row was never written.
//
// A failure here is reported rather than swallowed: the task is then genuinely
// orphaned — hidden by its origin, pointed at by no run — and only a human
// reading this line will know it is there.
func (s *Service) deleteAbandonedTask(ctx context.Context, automationID, taskID string) {
	cleaner, ok := s.repo.(taskCleaner)
	if !ok {
		return
	}
	if err := cleaner.DeleteTask(ctx, taskID); err != nil {
		s.logger.Error("failed to delete the task of an unrecorded automation run",
			zap.String("automation_id", automationID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func (s *Service) recordSuccessRun(
	ctx context.Context, evt *automation.AutomationTriggeredEvent, taskID string,
) error {
	if s.automationService == nil {
		return nil
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
	return s.automationService.RecordRun(ctx, run)
}

// handleAutomationTurnComplete closes out an automation run on the stream
// complete event. ACP agents can stay alive after stop_reason=end_turn, so
// waiting for agent.completed would leave the AutomationRun stuck at
// task_created and keep the executor alive.
func (s *Service) handleAutomationTurnComplete(
	ctx context.Context, taskID, sessionID string, session *models.TaskSession, stopReason string, isError bool, errMsg string,
) bool {
	if taskID == "" || sessionID == "" {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return false
	}
	// Keyed on origin alone. Automation tasks are no longer ephemeral, and
	// gating on IsEphemeral here would leave every run stuck at task_created,
	// holding a max_concurrent_runs slot until a human archived it.
	if task.Origin != models.TaskOriginAutomationRun {
		return false
	}

	cancelled := stopReason == stopReasonCancelled
	success := !isError && stopReason != "error" && !cancelled
	if cancelled && errMsg == "" {
		errMsg = stopReasonCancelled
	}
	// A successful run parks in WAITING_FOR_INPUT, which is where the ordinary
	// agent.completed path leaves a finished turn. COMPLETED is terminal and
	// explicitly not resumable ("create a new session instead"), so using it
	// here would make every finished run unrepliable — the opposite of "a run
	// is a thread, not a receipt". Success is recorded on the AutomationRun
	// row; the session state only says whether the conversation can continue.
	nextState := models.TaskSessionStateWaitingForInput
	if cancelled {
		nextState = models.TaskSessionStateCancelled
	} else if !success {
		nextState = models.TaskSessionStateFailed
	}
	s.updateTaskSessionState(ctx, taskID, sessionID, nextState, errMsg, false, session)
	s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
	s.stopAutomationAgent(ctx, taskID, sessionID, session)
	// The worktree is deliberately left in place: the files a run writes are
	// usually the point of running it, and an agent that ends by asking a
	// question needs a workspace in which to be answered.
	return true
}

// finalizeAutomationRun flips an automation run's row from task_created →
// succeeded|failed when its agent terminates, so max_concurrent_runs frees up
// without anyone archiving anything. Keyed on origin — every automation task
// carries origin=automation_run and none of them are ephemeral. Regular tasks
// are untouched, and no worktree is torn down.
func (s *Service) finalizeAutomationRun(ctx context.Context, taskID string, success bool, errMsg string) {
	if taskID == "" {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	if task.Origin != models.TaskOriginAutomationRun {
		return
	}

	s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
}

func (s *Service) markAutomationRunTerminal(ctx context.Context, taskID string, success bool, errMsg string) {
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
}

func (s *Service) stopAutomationAgent(
	ctx context.Context, taskID, sessionID string, session *models.TaskSession,
) {
	if s.agentManager == nil {
		return
	}
	executionID := ""
	if session != nil {
		executionID = session.AgentExecutionID
	}
	if executionID == "" {
		running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to resolve automation execution for turn-complete stop",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
		if running != nil {
			executionID = running.AgentExecutionID
		}
	}
	if executionID == "" {
		return
	}
	if err := s.agentManager.StopAgent(ctx, executionID, false); err != nil {
		s.logger.Warn("failed to stop automation agent on turn complete",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", executionID),
			zap.Error(err))
	}
}
