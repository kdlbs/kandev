package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	issueWatchIDKey = "issue_watch_id"
	issueNumberKey  = "issue_number"
)

// GitHubService is the interface the orchestrator uses for GitHub operations.
// All PR-write methods carry repository_id so multi-repo tasks (where the
// same task spans multiple repos and so multiple PRs) don't collapse to one
// row by colliding on the legacy UNIQUE(session_id) / UNIQUE(task_id, pr_number)
// constraints. Empty repository_id preserves single-repo legacy behavior.
type GitHubService interface {
	Client() github.Client
	CreatePRWatch(ctx context.Context, sessionID, taskID, repositoryID, owner, repo string, prNumber int, branch string) (*github.PRWatch, error)
	EnsurePRWatch(ctx context.Context, sessionID, taskID, repositoryID, owner, repo, branch string) (*github.PRWatch, error)
	GetPRWatchBySession(ctx context.Context, sessionID string) (*github.PRWatch, error)
	GetPRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*github.PRWatch, error)
	UpdatePRWatchBranchIfSearching(ctx context.Context, id, branch string) error
	UpdatePRWatchPRNumber(ctx context.Context, id string, prNumber int) error
	ResetPRWatch(ctx context.Context, id, branch string) error
	AssociatePRWithTask(ctx context.Context, taskID, repositoryID string, pr *github.PR) (*github.TaskPR, error)
	GetTaskPR(ctx context.Context, taskID string) (*github.TaskPR, error)
	ListActivePRWatches(ctx context.Context) ([]*github.PRWatch, error)
	ReserveReviewPRTask(ctx context.Context, watchID, repoOwner, repoName string, prNumber int, prURL string) (bool, error)
	AssignReviewPRTaskID(ctx context.Context, watchID, repoOwner, repoName string, prNumber int, taskID string) error
	ReleaseReviewPRTask(ctx context.Context, watchID, repoOwner, repoName string, prNumber int) error

	// Issue watch dedup operations
	ReserveIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, issueURL string) (bool, error)
	AssignIssueWatchTaskID(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, taskID string) error
	ReleaseIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int) error
}

// ReviewTaskCreator creates tasks from review watch events.
type ReviewTaskCreator interface {
	CreateReviewTask(ctx context.Context, req *ReviewTaskRequest) (*models.Task, error)
}

// IssueTaskCreator creates tasks from issue watch events.
type IssueTaskCreator interface {
	CreateIssueTask(ctx context.Context, req *IssueTaskRequest) (*models.Task, error)
}

// IssueTaskRequest contains the data for creating a task from an issue watch event.
type IssueTaskRequest struct {
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID string
	Title          string
	Description    string
	Metadata       map[string]interface{}
	Repositories   []IssueTaskRepository
}

// IssueTaskRepository associates a repository with an issue task.
type IssueTaskRepository struct {
	RepositoryID string
	BaseBranch   string
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
	RepositoryID   string
	BaseBranch     string
	CheckoutBranch string
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

// SetIssueTaskCreator sets the task creator for issue watch auto-task creation.
func (s *Service) SetIssueTaskCreator(tc IssueTaskCreator) {
	s.issueTaskCreator = tc
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

	if !s.reserveReviewPR(ctx, evt) {
		return
	}

	repositories := s.resolveReviewRepository(ctx, evt.WorkspaceID, pr)
	task, err := s.reviewTaskCreator.CreateReviewTask(ctx, buildReviewTaskRequest(evt, repositories, repoSlug))
	if err != nil {
		s.logger.Error("failed to create review task",
			zap.String("review_watch_id", evt.ReviewWatchID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
		s.releaseReviewPR(ctx, evt)
		return
	}

	s.attachTaskToReservation(ctx, evt, task.ID)

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

// reserveReviewPR atomically claims the dedup slot before the slow clone +
// task creation. Returns false (caller must bail) if another handler already
// holds the slot or if the reserve call errored. A nil githubService means
// dedup is disabled and we always proceed.
func (s *Service) reserveReviewPR(ctx context.Context, evt *github.NewReviewPREvent) bool {
	if s.githubService == nil {
		return true
	}
	pr := evt.PR
	reserved, err := s.githubService.ReserveReviewPRTask(
		ctx, evt.ReviewWatchID, pr.RepoOwner, pr.RepoName, pr.Number, pr.HTMLURL,
	)
	if err != nil {
		s.logger.Error("failed to reserve review PR slot",
			zap.String("review_watch_id", evt.ReviewWatchID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
		return false
	}
	if !reserved {
		s.logger.Debug("review PR already reserved by concurrent handler, skipping",
			zap.String("review_watch_id", evt.ReviewWatchID),
			zap.Int("pr_number", pr.Number))
		return false
	}
	return true
}

// releaseReviewPR removes the reservation when task creation fails so a later
// poll can retry this PR instead of it being blocked by an orphan row.
func (s *Service) releaseReviewPR(ctx context.Context, evt *github.NewReviewPREvent) {
	if s.githubService == nil {
		return
	}
	pr := evt.PR
	if err := s.githubService.ReleaseReviewPRTask(
		ctx, evt.ReviewWatchID, pr.RepoOwner, pr.RepoName, pr.Number,
	); err != nil {
		s.logger.Warn("failed to release review PR reservation after task-create failure",
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
	}
}

// attachTaskToReservation stamps the created task ID onto the reservation and
// associates the PR with the task so the frontend can display PR info.
func (s *Service) attachTaskToReservation(ctx context.Context, evt *github.NewReviewPREvent, taskID string) {
	if s.githubService == nil {
		return
	}
	pr := evt.PR
	if err := s.githubService.AssignReviewPRTaskID(
		ctx, evt.ReviewWatchID, pr.RepoOwner, pr.RepoName, pr.Number, taskID,
	); err != nil {
		s.logger.Error("failed to assign task ID to review PR reservation",
			zap.String("task_id", taskID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
	}
	// Review tasks are single-repo (the PR's own repo). Resolve to that
	// task_repository's repository_id so the resulting TaskPR row is scoped
	// per-repo even though there's only one. Empty falls back to legacy
	// "delete all" semantics which is fine for the 1-repo case.
	repositoryID := s.resolvePrimaryTaskRepositoryID(ctx, taskID)
	if _, err := s.githubService.AssociatePRWithTask(ctx, taskID, repositoryID, pr); err != nil {
		s.logger.Error("failed to associate PR with review task",
			zap.String("task_id", taskID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
	}
}

// buildReviewTaskRequest builds the ReviewTaskRequest payload from an event.
func buildReviewTaskRequest(evt *github.NewReviewPREvent, repositories []ReviewTaskRepository, repoSlug string) *ReviewTaskRequest {
	pr := evt.PR
	return &ReviewTaskRequest{
		WorkspaceID:    evt.WorkspaceID,
		WorkflowID:     evt.WorkflowID,
		WorkflowStepID: evt.WorkflowStepID,
		Title:          fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title),
		Description:    interpolateReviewPrompt(evt.Prompt, pr),
		Repositories:   repositories,
		Metadata: map[string]interface{}{
			"review_watch_id":     evt.ReviewWatchID,
			"pr_number":           pr.Number,
			"pr_url":              pr.HTMLURL,
			"pr_repo":             repoSlug,
			"pr_author":           pr.AuthorLogin,
			"pr_branch":           pr.HeadBranch,
			"agent_profile_id":    evt.AgentProfileID,
			"executor_profile_id": evt.ExecutorProfileID,
		},
	}
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
		nil,
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
	// BaseBranch = repo default branch (e.g. "main") for worktree creation.
	// CheckoutBranch = PR head branch to fetch and checkout after worktree is created.
	return []ReviewTaskRepository{{RepositoryID: repoID, BaseBranch: baseBranch, CheckoutBranch: pr.HeadBranch}}
}

// detectPushAndAssociatePR checks if a push happened and looks for a PR on
// that branch. If no PR is found immediately, retries after a delay to handle
// the case where the user creates the PR on GitHub shortly after pushing.
//
// The pushTracker entry is intentionally NOT deleted on return: leaving the
// stored ahead=0 in place causes subsequent status events to fall into the
// `prevAhead <= 0` skip in trackPushAndAssociatePR, which prevents repeated
// firing on every status event for a synced branch. The tracker is cleaned
// up at session-deletion time via pushTrackerForget.
//
// Multi-repo: repositoryName scopes the lookup so each repo's push is detected
// and associated independently. Empty repositoryName falls back to the
// session's primary repo (legacy single-repo path).
func (s *Service) detectPushAndAssociatePR(
	ctx context.Context, sessionID, taskID, repositoryName, branch string,
) {
	if s.githubService == nil {
		return
	}
	client := s.githubService.Client()
	if client == nil {
		return
	}

	owner, repoName, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, repositoryName)
	if owner == "" || repoName == "" {
		return
	}

	// Check if we already have a watch for this (session, repo).
	// If the watch already has a PR number, the PR was found — nothing to do.
	// If the watch has pr_number=0, it's still searching — do an immediate
	// search (faster than waiting for the 1-minute poller) and update the
	// watch branch if the agent pushed from a different branch.
	existing, err := s.githubService.GetPRWatchBySessionAndRepo(ctx, sessionID, repositoryID)
	if err == nil && existing != nil {
		if existing.PRNumber > 0 {
			return // PR already found and being monitored
		}
		s.searchPRForExistingWatch(ctx, client, existing, sessionID, taskID, branch)
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
			if ex, err := s.githubService.GetPRWatchBySessionAndRepo(ctx, sessionID, repositoryID); err == nil && ex != nil {
				return
			}
		}
		foundPR, findErr := client.FindPRByBranch(ctx, owner, repoName, branch)
		if findErr != nil || foundPR == nil {
			s.logger.Debug("no PR found for branch (will retry)",
				zap.String("branch", branch),
				zap.String("session_id", sessionID),
				zap.String("repository_name", repositoryName),
				zap.Duration("delay", delay))
			continue
		}
		s.logger.Info("PR found after push, associating with task",
			zap.String("session_id", sessionID),
			zap.String("task_id", taskID),
			zap.String("repository_name", repositoryName),
			zap.Int("pr_number", foundPR.Number),
			zap.String("branch", branch))
		s.associatePRFromPushScoped(ctx, sessionID, taskID, owner, repoName, repositoryID, branch, foundPR)
		return
	}
	s.logger.Warn("exhausted all retries, no PR found after push",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("repository_name", repositoryName),
		zap.String("branch", branch))
}

// resolvePushRepo returns (owner, name, repository_id) for the per-repo push
// detection. Multi-repo path: looks up the task_repository whose repository's
// Name matches `repositoryName`. Empty name falls back to the session's
// primary repo (legacy single-repo behaviour).
func (s *Service) resolvePushRepo(
	ctx context.Context, sessionID, taskID, repositoryName string,
) (owner, repo, repositoryID string) {
	if repositoryName == "" {
		o, r := s.resolveSessionRepo(ctx, sessionID)
		return o, r, s.resolvePrimaryTaskRepositoryID(ctx, taskID)
	}
	store, ok := s.repo.(repoStore)
	if !ok {
		return "", "", ""
	}
	links, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return "", "", ""
	}
	for _, link := range links {
		repoObj, err := store.GetRepository(ctx, link.RepositoryID)
		if err != nil || repoObj == nil {
			continue
		}
		if repoObj.Name != repositoryName {
			continue
		}
		if repoObj.ProviderOwner == "" && repoObj.LocalPath != "" {
			if p, o, n := service.ResolveGitRemoteProvider(repoObj.LocalPath); o != "" {
				repoObj.Provider = p
				repoObj.ProviderOwner = o
				repoObj.ProviderName = n
				go s.backfillRepoProvider(store, repoObj)
			}
		}
		return repoObj.ProviderOwner, repoObj.ProviderName, link.RepositoryID
	}
	return "", "", ""
}

// searchPRForExistingWatch handles the case where a PR watch exists with pr_number=0
// (still searching). It updates the watch branch if the agent pushed from a different
// branch, then does a single immediate search so we don't wait for the 1-minute poller.
func (s *Service) searchPRForExistingWatch(
	ctx context.Context, client github.Client, watch *github.PRWatch,
	sessionID, taskID, branch string,
) {
	// Update branch if the agent switched branches since the watch was created.
	// Use the atomic variant to guard against a concurrent PR association.
	if watch.Branch != branch {
		if err := s.githubService.UpdatePRWatchBranchIfSearching(ctx, watch.ID, branch); err != nil {
			s.logger.Warn("failed to update PR watch branch",
				zap.String("watch_id", watch.ID),
				zap.String("old_branch", watch.Branch),
				zap.String("new_branch", branch),
				zap.Error(err))
		}
	}
	// Immediate search — if found, update the existing watch and associate.
	// We must not call associatePRFromPush here because it calls CreatePRWatch,
	// which would fail with a UNIQUE constraint since the watch already exists.
	foundPR, findErr := client.FindPRByBranch(ctx, watch.Owner, watch.Repo, branch)
	if findErr == nil && foundPR != nil {
		if err := s.githubService.UpdatePRWatchPRNumber(ctx, watch.ID, foundPR.Number); err != nil {
			s.logger.Warn("failed to update PR watch number",
				zap.String("watch_id", watch.ID),
				zap.Int("pr_number", foundPR.Number),
				zap.Error(err))
		}
		// Use the watch's own repository_id so the association lands on the
		// correct per-repo TaskPR row (matters once multi-repo watches exist;
		// for legacy single-repo watches this is empty and matches the old
		// "delete all" behavior).
		if _, err := s.githubService.AssociatePRWithTask(ctx, taskID, watch.RepositoryID, foundPR); err != nil {
			s.logger.Error("failed to associate PR with task",
				zap.String("task_id", taskID),
				zap.Int("pr_number", foundPR.Number),
				zap.Error(err))
		}
		s.logger.Info("auto-detected PR from push (existing watch)",
			zap.String("session_id", sessionID),
			zap.Int("pr_number", foundPR.Number),
			zap.String("branch", branch))
	}
}

// resolveSessionRepo looks up the repository owner and name for a session.
// If provider info is missing but a local path exists, it detects from the git remote
// and backfills the DB record for future calls.
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
	if repoObj.ProviderOwner == "" && repoObj.LocalPath != "" {
		if p, o, n := service.ResolveGitRemoteProvider(repoObj.LocalPath); o != "" {
			repoObj.Provider = p
			repoObj.ProviderOwner = o
			repoObj.ProviderName = n
			go s.backfillRepoProvider(store, repoObj)
		}
	}
	return repoObj.ProviderOwner, repoObj.ProviderName
}

// backfillRepoProvider persists auto-detected provider info to the DB.
func (s *Service) backfillRepoProvider(store repoStore, repo *models.Repository) {
	if err := store.UpdateRepository(context.Background(), repo); err != nil {
		s.logger.Warn("failed to backfill repository provider info",
			zap.String("repository_id", repo.ID),
			zap.Error(err))
	} else {
		s.logger.Info("backfilled repository provider info from git remote",
			zap.String("repository_id", repo.ID),
			zap.String("provider", repo.Provider),
			zap.String("owner", repo.ProviderOwner),
			zap.String("name", repo.ProviderName))
	}
}

// ensureSessionPRWatch creates one PRWatch (pr_number=0) per repository in
// the task so the poller will search GitHub for each repo's PR. Runs as a
// background goroutine.
//
// Multi-repo: each repository gets its own watch row because the table is
// keyed UNIQUE(session_id, repository_id). Without this, only the primary
// repo's PR was ever discovered — secondary repos in a multi-repo task
// silently dropped their associations.
//
// `fallbackBranch` is used only when a task_repository has no checkout_branch
// AND the session worktree for that repo has no branch (rare; ensures
// backwards compatibility with single-repo callers that pass the resolved
// branch directly).
func (s *Service) ensureSessionPRWatch(ctx context.Context, taskID, sessionID, fallbackBranch string) {
	if s.githubService == nil {
		return
	}
	targets := s.resolveSessionWatchTargets(ctx, taskID, sessionID, fallbackBranch)
	for _, t := range targets {
		if _, err := s.githubService.EnsurePRWatch(
			ctx, sessionID, taskID, t.RepositoryID, t.Owner, t.Repo, t.Branch,
		); err != nil {
			s.logger.Warn("failed to ensure PR watch for session",
				zap.String("session_id", sessionID),
				zap.String("repository_id", t.RepositoryID),
				zap.String("branch", t.Branch),
				zap.Error(err))
		}
	}
}

// sessionWatchTarget describes one (session, repo) pair that should have a
// PR watch. Built by walking the task's repositories and matching each to
// the session's worktree (branch) and the repo (owner/name).
type sessionWatchTarget struct {
	RepositoryID string
	Owner        string
	Repo         string
	Branch       string
}

// resolveSessionWatchTargets walks the task's repositories and produces one
// target per repo whose owner/name and branch resolve. Skips repos for which
// any of those fields are empty — they can't be searched on GitHub.
//
// Branch resolution per repo (matches the legacy single-repo
// resolvePRWatchBranch priority so PR-review-style synthetic worktree
// branches don't override the task's real branch):
//
//  1. task_repository.checkout_branch (set at task creation — authoritative)
//  2. caller's fallbackBranch, but only for the primary repo (legacy signature)
//  3. session worktree's branch (last-resort guess from the worktree)
//
// Applying the fallback to non-primary repos would assign every repo the same
// branch and corrupt cross-repo watches.
func (s *Service) resolveSessionWatchTargets(
	ctx context.Context, taskID, sessionID, fallbackBranch string,
) []sessionWatchTarget {
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil
	}
	taskRepos, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil || len(taskRepos) == 0 {
		return nil
	}
	branchByRepo := s.branchByRepoForSession(ctx, sessionID)

	var targets []sessionWatchTarget
	for _, tr := range taskRepos {
		owner, repoName := s.resolveRepoOwnerName(ctx, store, tr.RepositoryID)
		if owner == "" || repoName == "" {
			continue
		}
		branch := strings.TrimSpace(tr.CheckoutBranch)
		if branch == "" && tr.Position == 0 {
			branch = fallbackBranch
		}
		if branch == "" {
			branch = branchByRepo[tr.RepositoryID]
		}
		if branch == "" {
			continue
		}
		targets = append(targets, sessionWatchTarget{
			RepositoryID: tr.RepositoryID,
			Owner:        owner,
			Repo:         repoName,
			Branch:       branch,
		})
	}
	return targets
}

// branchByRepoForSession returns repository_id → worktree_branch for every
// worktree on the session. Empty map if the session can't be loaded — caller
// falls back to the task_repository's configured branch.
func (s *Service) branchByRepoForSession(ctx context.Context, sessionID string) map[string]string {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil
	}
	out := make(map[string]string, len(session.Worktrees))
	for _, wt := range session.Worktrees {
		if wt.WorktreeBranch == "" || wt.RepositoryID == "" {
			continue
		}
		out[wt.RepositoryID] = wt.WorktreeBranch
	}
	return out
}

// resolveRepoOwnerName returns (owner, name) for a repository, backfilling
// provider info from the local git remote when missing. Mirrors what
// resolveTaskRepo does for the primary repo, but works against any repo id.
func (s *Service) resolveRepoOwnerName(
	ctx context.Context, store repoStore, repositoryID string,
) (string, string) {
	if repositoryID == "" {
		return "", ""
	}
	repoObj, err := store.GetRepository(ctx, repositoryID)
	if err != nil || repoObj == nil {
		return "", ""
	}
	if repoObj.ProviderOwner == "" && repoObj.LocalPath != "" {
		if p, o, n := service.ResolveGitRemoteProvider(repoObj.LocalPath); o != "" {
			repoObj.Provider = p
			repoObj.ProviderOwner = o
			repoObj.ProviderName = n
			go s.backfillRepoProvider(store, repoObj)
		}
	}
	if repoObj.Provider != "github" {
		return "", ""
	}
	return repoObj.ProviderOwner, repoObj.ProviderName
}

// resolvePrimaryTaskRepositoryID returns the primary task_repository's
// repository_id for a task, or "" on miss / error. Used to scope PR
// watches and TaskPR rows to the correct per-repo row in multi-repo tasks
// (and to the single repo's row in single-repo tasks). Empty preserves the
// legacy single-repo behavior at the store layer.
func (s *Service) resolvePrimaryTaskRepositoryID(ctx context.Context, taskID string) string {
	store, ok := s.repo.(repoStore)
	if !ok {
		return ""
	}
	taskRepo, err := store.GetPrimaryTaskRepository(ctx, taskID)
	if err != nil || taskRepo == nil {
		return ""
	}
	return taskRepo.RepositoryID
}

func (s *Service) resolvePRWatchBranch(ctx context.Context, taskID, sessionID, fallback string) string {
	store, ok := s.repo.(repoStore)
	if !ok {
		return fallback
	}

	taskRepo, err := store.GetPrimaryTaskRepository(ctx, taskID)
	if err == nil && taskRepo != nil && strings.TrimSpace(taskRepo.CheckoutBranch) != "" {
		return strings.TrimSpace(taskRepo.CheckoutBranch)
	}

	if fallback != "" {
		return fallback
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	for _, wt := range session.Worktrees {
		if wt.WorktreeBranch != "" {
			return wt.WorktreeBranch
		}
	}
	return ""
}

// resolveTaskRepo looks up the GitHub owner and repo name for a task's primary repository.
// If provider info is missing but a local path exists, it detects from the git remote
// and backfills the DB record for future calls.
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
	if repoObj.ProviderOwner == "" && repoObj.LocalPath != "" {
		if p, o, n := service.ResolveGitRemoteProvider(repoObj.LocalPath); o != "" {
			repoObj.Provider = p
			repoObj.ProviderOwner = o
			repoObj.ProviderName = n
			go s.backfillRepoProvider(store, repoObj)
		}
	}
	if repoObj.Provider != "github" {
		return "", ""
	}
	return repoObj.ProviderOwner, repoObj.ProviderName
}

// associatePRFromPushScoped creates the PR watch and task-PR association
// after push detection, scoped to a specific repository_id. Multi-repo callers
// pass the per-repo id resolved from the git event's repository_name; the
// legacy single-repo path passes the primary task_repository id.
func (s *Service) associatePRFromPushScoped(
	ctx context.Context, sessionID, taskID, owner, repoName, repositoryID, branch string, pr *github.PR,
) {
	if _, watchErr := s.githubService.CreatePRWatch(
		ctx, sessionID, taskID, repositoryID, owner, repoName, pr.Number, branch,
	); watchErr != nil {
		s.logger.Error("failed to create PR watch on push detection",
			zap.String("session_id", sessionID),
			zap.String("repository_id", repositoryID),
			zap.Error(watchErr))
	}

	if _, assocErr := s.githubService.AssociatePRWithTask(ctx, taskID, repositoryID, pr); assocErr != nil {
		s.logger.Error("failed to associate PR with task on push detection",
			zap.String("task_id", taskID),
			zap.String("repository_id", repositoryID),
			zap.Error(assocErr))
	}

	s.logger.Info("auto-detected PR from push",
		zap.String("session_id", sessionID),
		zap.String("repository_id", repositoryID),
		zap.Int("pr_number", pr.Number),
		zap.String("branch", branch))
}

// CheckSessionPR checks if a PR exists for a session's branch and associates it
// if found. This provides an on-demand alternative to the background poller,
// allowing the frontend to trigger immediate PR detection.
func (s *Service) CheckSessionPR(ctx context.Context, taskID, sessionID string) (bool, error) {
	if s.githubService == nil {
		return false, nil
	}

	// Check if a PR is already associated with this task
	existing, err := s.githubService.GetTaskPR(ctx, taskID)
	if err == nil && existing != nil {
		return true, nil
	}

	// Resolve the GitHub owner/repo from the task's repository
	owner, repoName := s.resolveTaskRepo(ctx, taskID)
	if owner == "" || repoName == "" {
		return false, nil
	}

	branch := s.resolvePRWatchBranch(ctx, taskID, sessionID, "")
	if branch == "" {
		return false, nil
	}

	// Ensure a PR watch exists so the background poller will keep checking
	repositoryID := s.resolvePrimaryTaskRepositoryID(ctx, taskID)
	if _, watchErr := s.githubService.EnsurePRWatch(ctx, sessionID, taskID, repositoryID, owner, repoName, branch); watchErr != nil {
		s.logger.Warn("failed to ensure PR watch during check",
			zap.String("session_id", sessionID),
			zap.Error(watchErr))
	}

	// Try to find the PR immediately
	client := s.githubService.Client()
	if client == nil {
		return false, nil
	}
	pr, findErr := client.FindPRByBranch(ctx, owner, repoName, branch)
	if findErr != nil || pr == nil {
		return false, nil
	}

	// Found a PR — associate it with the task
	s.associatePRFromPushScoped(ctx, sessionID, taskID, owner, repoName, repositoryID, branch, pr)
	return true, nil
}

// ListTasksNeedingPRWatch returns task-session pairs that have worktree branches
// but no existing PR watch. This satisfies the github.TaskBranchProvider interface.
func (s *Service) ListTasksNeedingPRWatch(ctx context.Context) ([]github.TaskBranchInfo, error) {
	if s.githubService == nil {
		return nil, nil
	}
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil, nil
	}
	return s.buildTaskBranchList(ctx, store)
}

// ResolveBranchForSession returns the current branch for a task+session.
// This is used by the poller to detect branch renames on existing PR watches.
func (s *Service) ResolveBranchForSession(ctx context.Context, taskID, sessionID string) string {
	return s.resolvePRWatchBranch(ctx, taskID, sessionID, "")
}

// buildTaskBranchList walks sessions × their repositories and emits one
// TaskBranchInfo per (session, repository) that doesn't already have a PR
// watch. Multi-repo: previously dedup was keyed by sessionID, which silently
// dropped non-primary repos as soon as the primary one got a watch.
func (s *Service) buildTaskBranchList(ctx context.Context, store repoStore) ([]github.TaskBranchInfo, error) {
	sessions, err := store.ListSessionsWithBranches(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions with branches: %w", err)
	}
	if len(sessions) == 0 {
		return nil, nil
	}

	// Batch fetch existing watches to avoid N+1 queries.
	watchedKeys := s.buildWatchedSessionRepoSet(ctx)

	var result []github.TaskBranchInfo
	for _, sess := range sessions {
		// Pass sess.Branch as fallback for the primary repo (legacy callers
		// stored the worktree branch on SessionBranchInfo); per-repo branches
		// come from session.Worktrees inside resolveSessionWatchTargets.
		targets := s.resolveSessionWatchTargets(ctx, sess.TaskID, sess.SessionID, sess.Branch)
		for _, t := range targets {
			if watchedKeys[watchedSessionRepoKey(sess.SessionID, t.RepositoryID)] {
				continue
			}
			result = append(result, github.TaskBranchInfo{
				TaskID:       sess.TaskID,
				SessionID:    sess.SessionID,
				RepositoryID: t.RepositoryID,
				Owner:        t.Owner,
				Repo:         t.Repo,
				Branch:       t.Branch,
			})
		}
	}
	return result, nil
}

// buildWatchedSessionRepoSet returns the set of (session_id, repository_id)
// pairs that already have a PR watch row. Replaces the older session-only set
// which collapsed multi-repo sessions into one bucket.
func (s *Service) buildWatchedSessionRepoSet(ctx context.Context) map[string]bool {
	watches, err := s.githubService.ListActivePRWatches(ctx)
	if err != nil {
		s.logger.Debug("failed to list PR watches for reconciliation", zap.Error(err))
		return nil
	}
	set := make(map[string]bool, len(watches))
	for _, w := range watches {
		set[watchedSessionRepoKey(w.SessionID, w.RepositoryID)] = true
	}
	return set
}

// watchedSessionRepoKey builds the per-(session, repo) dedup key.
func watchedSessionRepoKey(sessionID, repositoryID string) string {
	return sessionID + "|" + repositoryID
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
	if _, err := s.eventBus.Subscribe(events.GitHubNewIssue, s.handleNewIssue); err != nil {
		s.logger.Error("failed to subscribe to github.new_issue events", zap.Error(err))
	}
}

// handleNewIssue creates a task for a new GitHub issue matching an issue watch.
func (s *Service) handleNewIssue(ctx context.Context, event *bus.Event) error {
	issueEvt, ok := event.Data.(*github.NewIssueEvent)
	if !ok {
		return nil
	}

	issue := issueEvt.Issue
	s.logger.Info("new issue detected from watch",
		zap.String(issueWatchIDKey, issueEvt.IssueWatchID),
		zap.String("repo", fmt.Sprintf("%s/%s", issue.RepoOwner, issue.RepoName)),
		zap.Int(issueNumberKey, issue.Number))

	if s.issueTaskCreator == nil {
		s.logger.Warn("issue task creator not configured, skipping task creation")
		return nil
	}

	go s.createIssueTask(context.Background(), issueEvt)
	return nil
}

func (s *Service) createIssueTask(ctx context.Context, evt *github.NewIssueEvent) {
	issue := evt.Issue
	repoSlug := fmt.Sprintf("%s/%s", issue.RepoOwner, issue.RepoName)

	if !s.reserveIssueWatch(ctx, evt) {
		return
	}

	repositories := s.resolveIssueRepository(ctx, evt.WorkspaceID, issue)

	req := &IssueTaskRequest{
		WorkspaceID:    evt.WorkspaceID,
		WorkflowID:     evt.WorkflowID,
		WorkflowStepID: evt.WorkflowStepID,
		Title:          fmt.Sprintf("Issue #%d: %s", issue.Number, issue.Title),
		Description:    interpolateIssuePrompt(evt.Prompt, issue),
		Metadata: map[string]interface{}{
			issueWatchIDKey:       evt.IssueWatchID,
			issueNumberKey:        issue.Number,
			"issue_url":           issue.HTMLURL,
			"issue_repo":          repoSlug,
			"issue_author":        issue.AuthorLogin,
			"agent_profile_id":    evt.AgentProfileID,
			"executor_profile_id": evt.ExecutorProfileID,
		},
		Repositories: repositories,
	}

	task, err := s.issueTaskCreator.CreateIssueTask(ctx, req)
	if err != nil {
		s.logger.Error("failed to create issue task",
			zap.String(issueWatchIDKey, evt.IssueWatchID),
			zap.Int(issueNumberKey, issue.Number),
			zap.Error(err))
		s.releaseIssueWatch(ctx, evt)
		return
	}

	s.attachIssueTaskToReservation(ctx, evt, task.ID)

	s.logger.Info("created issue task",
		zap.String("task_id", task.ID),
		zap.Int(issueNumberKey, issue.Number),
		zap.String("repo", repoSlug))

	if !s.shouldAutoStartStep(ctx, evt.WorkflowStepID) {
		return
	}
	s.autoStartIssueTask(ctx, evt, task)
}

func (s *Service) reserveIssueWatch(ctx context.Context, evt *github.NewIssueEvent) bool {
	if s.githubService == nil {
		return true
	}
	issue := evt.Issue
	reserved, err := s.githubService.ReserveIssueWatchTask(
		ctx, evt.IssueWatchID, issue.RepoOwner, issue.RepoName, issue.Number, issue.HTMLURL,
	)
	if err != nil {
		s.logger.Error("failed to reserve issue watch slot",
			zap.String(issueWatchIDKey, evt.IssueWatchID),
			zap.Int(issueNumberKey, issue.Number),
			zap.Error(err))
		return false
	}
	if !reserved {
		s.logger.Debug("issue already reserved by concurrent handler, skipping",
			zap.String(issueWatchIDKey, evt.IssueWatchID),
			zap.Int(issueNumberKey, issue.Number))
		return false
	}
	return true
}

func (s *Service) releaseIssueWatch(ctx context.Context, evt *github.NewIssueEvent) {
	if s.githubService == nil {
		return
	}
	issue := evt.Issue
	if err := s.githubService.ReleaseIssueWatchTask(
		ctx, evt.IssueWatchID, issue.RepoOwner, issue.RepoName, issue.Number,
	); err != nil {
		s.logger.Warn("failed to release issue watch reservation after task-create failure",
			zap.Int(issueNumberKey, issue.Number),
			zap.Error(err))
	}
}

func (s *Service) attachIssueTaskToReservation(ctx context.Context, evt *github.NewIssueEvent, taskID string) {
	if s.githubService == nil {
		return
	}
	issue := evt.Issue
	if err := s.githubService.AssignIssueWatchTaskID(
		ctx, evt.IssueWatchID, issue.RepoOwner, issue.RepoName, issue.Number, taskID,
	); err != nil {
		s.logger.Error("failed to assign task ID to issue watch reservation",
			zap.String("task_id", taskID),
			zap.Int(issueNumberKey, issue.Number),
			zap.Error(err))
	}
}

// resolveIssueRepository attempts to resolve and clone the issue's repository.
// Returns a slice with one entry on success, or nil on failure (graceful degradation).
func (s *Service) resolveIssueRepository(ctx context.Context, workspaceID string, issue *github.Issue) []IssueTaskRepository {
	if s.repositoryResolver == nil {
		return nil
	}
	repoSlug := fmt.Sprintf("%s/%s", issue.RepoOwner, issue.RepoName)
	s.logger.Debug("resolving issue repository", zap.String("repo", repoSlug))

	repoID, baseBranch, err := s.repositoryResolver.ResolveForReview(
		ctx, workspaceID, "github", issue.RepoOwner, issue.RepoName, "",
	)
	if err != nil {
		s.logger.Warn("failed to resolve repository for issue task (continuing without repo)",
			zap.String("repo", repoSlug),
			zap.Error(err))
		return nil
	}
	if repoID == "" {
		return nil
	}
	s.logger.Debug("resolved issue repository",
		zap.String("repo", repoSlug),
		zap.String("repo_id", repoID),
		zap.String("base_branch", baseBranch))
	return []IssueTaskRepository{{RepositoryID: repoID, BaseBranch: baseBranch}}
}

func (s *Service) autoStartIssueTask(
	ctx context.Context, evt *github.NewIssueEvent, task *models.Task,
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
		nil,
	)
	if err != nil {
		s.logger.Error("failed to auto-start issue task",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return
	}
	s.logger.Info("auto-started issue task",
		zap.String("task_id", task.ID),
		zap.Int(issueNumberKey, evt.Issue.Number))
}

// interpolateReviewPrompt replaces {{pr.*}} placeholders in the prompt template with actual PR values.
// When the prompt template is empty (user didn't configure a custom prompt), it uses the
// embedded default that provides useful PR context to the agent.
func interpolateReviewPrompt(promptTemplate string, pr *github.PR) string {
	if promptTemplate == "" {
		promptTemplate = promptcfg.Get("pr-review-watch-default")
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

// interpolateIssuePrompt replaces {{issue.*}} placeholders in the prompt template with actual issue values.
func interpolateIssuePrompt(promptTemplate string, issue *github.Issue) string {
	if promptTemplate == "" {
		promptTemplate = promptcfg.Get("issue-watch-default")
	}
	repoSlug := fmt.Sprintf("%s/%s", issue.RepoOwner, issue.RepoName)
	labelsStr := strings.Join(issue.Labels, ", ")
	replacer := strings.NewReplacer(
		"{{issue.link}}", issue.HTMLURL,
		"{{issue.number}}", strconv.Itoa(issue.Number),
		"{{issue.title}}", issue.Title,
		"{{issue.author}}", issue.AuthorLogin,
		"{{issue.repo}}", repoSlug,
		"{{issue.labels}}", labelsStr,
		"{{issue.body}}", issue.Body,
	)
	return replacer.Replace(promptTemplate)
}
