package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// GitHubService is the interface the orchestrator uses for GitHub operations.
type GitHubService interface {
	Client() github.Client
	CreatePRWatch(ctx context.Context, sessionID, taskID, owner, repo string, prNumber int, branch string) (*github.PRWatch, error)
	EnsurePRWatch(ctx context.Context, sessionID, taskID, owner, repo, branch string) (*github.PRWatch, error)
	GetPRWatchBySession(ctx context.Context, sessionID string) (*github.PRWatch, error)
	AssociatePRWithTask(ctx context.Context, taskID string, pr *github.PR) (*github.TaskPR, error)
	RecordReviewPRTask(ctx context.Context, watchID, repoOwner, repoName string, prNumber int, prURL, taskID string) error
}

// ReviewTaskCreator creates tasks from review watch events.
type ReviewTaskCreator interface {
	CreateReviewTask(ctx context.Context, req *ReviewTaskRequest) (*models.Task, error)
}

// RepositoryResolver resolves a GitHub repo to a local clone + Repository DB record.
type RepositoryResolver interface {
	ResolveForReview(ctx context.Context, workspaceID, provider, owner, name, defaultBranch string) (repositoryID, baseBranch string, err error)
}

// ReviewTaskRequest contains the data for creating a task from a review watch PR.
type ReviewTaskRequest struct {
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID string
	Title          string
	Description    string
	Metadata       map[string]interface{}
	Repositories   []ReviewTaskRepository
}

// ReviewTaskRepository associates a repository with a review task.
type ReviewTaskRepository struct {
	RepositoryID string
	BaseBranch   string
}

// SetGitHubService sets the GitHub service for PR auto-detection.
func (s *Service) SetGitHubService(ghSvc GitHubService) {
	s.githubService = ghSvc
}

// SetReviewTaskCreator sets the task creator for review watch auto-task creation.
func (s *Service) SetReviewTaskCreator(tc ReviewTaskCreator) {
	s.reviewTaskCreator = tc
}

// SetRepositoryResolver sets the repository resolver for review task creation.
func (s *Service) SetRepositoryResolver(rr RepositoryResolver) {
	s.repositoryResolver = rr
}

// handlePRFeedback logs PR feedback events. WS broadcasting is handled in main.go.
func (s *Service) handlePRFeedback(_ context.Context, event *bus.Event) error {
	feedbackEvt, ok := event.Data.(*github.PRFeedbackEvent)
	if !ok {
		return nil
	}
	s.logger.Debug("received PR feedback event",
		zap.String("session_id", feedbackEvt.SessionID),
		zap.Int("pr_number", feedbackEvt.PRNumber))
	return nil
}

// handleNewReviewPR creates a task for a new PR needing review.
// Auto-start is determined by the workflow step's on_enter actions.
func (s *Service) handleNewReviewPR(ctx context.Context, event *bus.Event) error {
	reviewEvt, ok := event.Data.(*github.NewReviewPREvent)
	if !ok {
		return nil
	}

	pr := reviewEvt.PR
	s.logger.Info("new PR to review detected",
		zap.String("review_watch_id", reviewEvt.ReviewWatchID),
		zap.String("repo", fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName)),
		zap.Int("pr_number", pr.Number))

	if s.reviewTaskCreator == nil {
		s.logger.Warn("review task creator not configured, skipping task creation")
		return nil
	}

	// Use a background context: the parent ctx may be an HTTP request context
	// that gets canceled as soon as the response is sent, but the clone +
	// task creation must survive beyond the request lifetime.
	go s.createReviewTask(context.Background(), reviewEvt)
	return nil
}

func (s *Service) createReviewTask(ctx context.Context, evt *github.NewReviewPREvent) {
	pr := evt.PR
	repoSlug := fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName)

	s.logger.Debug("creating review task from PR",
		zap.String("repo", repoSlug),
		zap.Int("pr_number", pr.Number),
		zap.String("head_branch", pr.HeadBranch),
		zap.String("base_branch", pr.BaseBranch),
		zap.String("review_watch_id", evt.ReviewWatchID))

	title := fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title)
	description := interpolateReviewPrompt(evt.Prompt, pr)

	// Resolve repository: clone if needed, find-or-create DB record
	repositories := s.resolveReviewRepository(ctx, evt.WorkspaceID, pr)

	task, err := s.reviewTaskCreator.CreateReviewTask(ctx, &ReviewTaskRequest{
		WorkspaceID:    evt.WorkspaceID,
		WorkflowID:     evt.WorkflowID,
		WorkflowStepID: evt.WorkflowStepID,
		Title:          title,
		Description:    description,
		Repositories:   repositories,
		Metadata: map[string]interface{}{
			"review_watch_id": evt.ReviewWatchID,
			"pr_number":       pr.Number,
			"pr_url":          pr.HTMLURL,
			"pr_repo":         repoSlug,
			"pr_author":       pr.AuthorLogin,
			"pr_branch":       pr.HeadBranch,
		},
	})
	if err != nil {
		s.logger.Error("failed to create review task",
			zap.String("review_watch_id", evt.ReviewWatchID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
		return
	}

	// Record dedup entry so this PR won't be picked up again
	if s.githubService != nil {
		if recordErr := s.githubService.RecordReviewPRTask(
			ctx, evt.ReviewWatchID, pr.RepoOwner, pr.RepoName, pr.Number, pr.HTMLURL, task.ID,
		); recordErr != nil {
			s.logger.Error("failed to record review PR task",
				zap.String("task_id", task.ID),
				zap.Int("pr_number", pr.Number),
				zap.Error(recordErr))
		}

		// Associate PR with task so the frontend can display PR info
		if _, assocErr := s.githubService.AssociatePRWithTask(ctx, task.ID, pr); assocErr != nil {
			s.logger.Error("failed to associate PR with review task",
				zap.String("task_id", task.ID),
				zap.Int("pr_number", pr.Number),
				zap.Error(assocErr))
		}
	}

	s.logger.Info("created review task",
		zap.String("task_id", task.ID),
		zap.Int("pr_number", pr.Number),
		zap.String("repo", repoSlug))

	// Check if the target workflow step has auto_start_agent on_enter action.
	// If so, start the task (which launches the agent and triggers processOnEnter).
	// Otherwise, the task sits in the step waiting for user action.
	if !s.shouldAutoStartStep(ctx, evt.WorkflowStepID) {
		return
	}
	s.autoStartReviewTask(ctx, evt, task)
}

// shouldAutoStartStep checks if the workflow step has the OnEnterAutoStartAgent action.
func (s *Service) shouldAutoStartStep(ctx context.Context, stepID string) bool {
	if s.workflowStepGetter == nil || stepID == "" {
		return false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Warn("failed to get workflow step for auto-start check",
			zap.String("step_id", stepID),
			zap.Error(err))
		return false
	}
	return step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent)
}

func (s *Service) autoStartReviewTask(
	ctx context.Context, evt *github.NewReviewPREvent, task *models.Task,
) {
	_, err := s.StartTask(
		ctx,
		task.ID,
		evt.AgentProfileID,
		"",
		evt.ExecutorProfileID,
		0,
		task.Description,
		evt.WorkflowStepID,
		false,
	)
	if err != nil {
		s.logger.Error("failed to auto-start review task",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return
	}
	s.logger.Info("auto-started review task",
		zap.String("task_id", task.ID),
		zap.Int("pr_number", evt.PR.Number))
}

// resolveReviewRepository attempts to resolve and clone the PR's repository.
// Returns a slice with one entry on success, or nil on failure (graceful degradation).
func (s *Service) resolveReviewRepository(ctx context.Context, workspaceID string, pr *github.PR) []ReviewTaskRepository {
	if s.repositoryResolver == nil {
		return nil
	}
	repoSlug := fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName)
	s.logger.Debug("resolving review repository",
		zap.String("repo", repoSlug),
		zap.String("pr_base_branch", pr.BaseBranch),
		zap.String("pr_head_branch", pr.HeadBranch))

	repoID, baseBranch, err := s.repositoryResolver.ResolveForReview(
		ctx, workspaceID, "github", pr.RepoOwner, pr.RepoName, pr.BaseBranch,
	)
	if err != nil {
		s.logger.Warn("failed to resolve repository for review task (continuing without repo)",
			zap.String("repo", repoSlug),
			zap.Error(err))
		return nil
	}
	if repoID == "" {
		return nil
	}
	s.logger.Debug("resolved review repository",
		zap.String("repo", repoSlug),
		zap.String("repo_id", repoID),
		zap.String("base_branch", baseBranch))
	// Use the PR's head branch so the worktree checks out the PR code, not the merge target.
	// The actual base branch is preserved in the TaskPR record for reference.
	return []ReviewTaskRepository{{RepositoryID: repoID, BaseBranch: pr.HeadBranch}}
}

// detectPushAndAssociatePR checks if a push happened and looks for a PR on
// that branch. If no PR is found immediately, retries after a delay to handle
// the case where the user creates the PR on GitHub shortly after pushing.
// The pushTracker entry for this session is always removed when the function returns
// to prevent unbounded growth of the tracker map.
func (s *Service) detectPushAndAssociatePR(
	ctx context.Context, sessionID, taskID, branch string,
) {
	defer s.pushTracker.Delete(sessionID)

	if s.githubService == nil {
		return
	}
	client := s.githubService.Client()
	if client == nil {
		return
	}

	// Check if we already have a watch for this session
	existing, err := s.githubService.GetPRWatchBySession(ctx, sessionID)
	if err == nil && existing != nil {
		return // already watching
	}

	owner, repoName := s.resolveSessionRepo(ctx, sessionID)
	if owner == "" || repoName == "" {
		return
	}

	// Try to find a PR immediately, then retry after delays
	delays := []time.Duration{0, 30 * time.Second, 60 * time.Second}
	for _, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			// Re-check if a watch was created in the meantime (e.g. by CreatePR callback)
			if ex, err := s.githubService.GetPRWatchBySession(ctx, sessionID); err == nil && ex != nil {
				return
			}
		}
		foundPR, findErr := client.FindPRByBranch(ctx, owner, repoName, branch)
		if findErr != nil || foundPR == nil {
			s.logger.Debug("no PR found for branch (will retry)",
				zap.String("branch", branch),
				zap.String("session_id", sessionID),
				zap.Duration("delay", delay))
			continue
		}
		s.associatePRFromPush(ctx, sessionID, taskID, owner, repoName, branch, foundPR)
		return
	}
}

// resolveSessionRepo looks up the repository owner and name for a session.
func (s *Service) resolveSessionRepo(ctx context.Context, sessionID string) (string, string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RepositoryID == "" {
		return "", ""
	}
	store, ok := s.repo.(repoStore)
	if !ok {
		return "", ""
	}
	repoObj, err := store.GetRepository(ctx, session.RepositoryID)
	if err != nil || repoObj == nil {
		return "", ""
	}
	return repoObj.ProviderOwner, repoObj.ProviderName
}

// ensureSessionPRWatch creates a PRWatch (pr_number=0) for a session's branch
// so the poller will search for a PR on GitHub. Runs as a background goroutine.
func (s *Service) ensureSessionPRWatch(ctx context.Context, taskID, sessionID, branch string) {
	if s.githubService == nil || branch == "" {
		return
	}

	owner, repoName := s.resolveTaskRepo(ctx, taskID)
	if owner == "" || repoName == "" {
		return
	}

	if _, err := s.githubService.EnsurePRWatch(ctx, sessionID, taskID, owner, repoName, branch); err != nil {
		s.logger.Warn("failed to ensure PR watch for session",
			zap.String("session_id", sessionID),
			zap.String("branch", branch),
			zap.Error(err))
	}
}

// resolveTaskRepo looks up the GitHub owner and repo name for a task's primary repository.
func (s *Service) resolveTaskRepo(ctx context.Context, taskID string) (string, string) {
	store, ok := s.repo.(repoStore)
	if !ok {
		return "", ""
	}
	taskRepo, err := store.GetPrimaryTaskRepository(ctx, taskID)
	if err != nil || taskRepo == nil {
		return "", ""
	}
	repoObj, err := store.GetRepository(ctx, taskRepo.RepositoryID)
	if err != nil || repoObj == nil {
		return "", ""
	}
	if repoObj.Provider != "github" {
		return "", ""
	}
	return repoObj.ProviderOwner, repoObj.ProviderName
}

// associatePRFromPush creates the PR watch and task-PR association after push detection.
func (s *Service) associatePRFromPush(
	ctx context.Context, sessionID, taskID, owner, repoName, branch string, pr *github.PR,
) {
	if _, watchErr := s.githubService.CreatePRWatch(
		ctx, sessionID, taskID, owner, repoName, pr.Number, branch,
	); watchErr != nil {
		s.logger.Error("failed to create PR watch on push detection",
			zap.String("session_id", sessionID), zap.Error(watchErr))
	}

	if _, assocErr := s.githubService.AssociatePRWithTask(ctx, taskID, pr); assocErr != nil {
		s.logger.Error("failed to associate PR with task on push detection",
			zap.String("task_id", taskID), zap.Error(assocErr))
	}

	s.logger.Info("auto-detected PR from push",
		zap.String("session_id", sessionID),
		zap.Int("pr_number", pr.Number),
		zap.String("branch", branch))
}

// subscribeGitHubEvents subscribes to GitHub-related events on the event bus.
func (s *Service) subscribeGitHubEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.GitHubPRFeedback, s.handlePRFeedback); err != nil {
		s.logger.Error("failed to subscribe to github.pr_feedback events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.GitHubNewReviewPR, s.handleNewReviewPR); err != nil {
		s.logger.Error("failed to subscribe to github.new_pr_to_review events", zap.Error(err))
	}
}

// interpolateReviewPrompt replaces {{pr.*}} placeholders in the prompt template with actual PR values.
func interpolateReviewPrompt(promptTemplate string, pr *github.PR) string {
	if promptTemplate == "" {
		return fmt.Sprintf("Pull Request ready for review: %s", pr.HTMLURL)
	}
	repoSlug := fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName)
	replacer := strings.NewReplacer(
		"{{pr.link}}", pr.HTMLURL,
		"{{pr.number}}", strconv.Itoa(pr.Number),
		"{{pr.title}}", pr.Title,
		"{{pr.author}}", pr.AuthorLogin,
		"{{pr.repo}}", repoSlug,
		"{{pr.branch}}", pr.HeadBranch,
		"{{pr.base_branch}}", pr.BaseBranch,
	)
	return replacer.Replace(promptTemplate)
}
