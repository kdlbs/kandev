package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	// taskPRAgentBadgeLabel names the automation in the chat origin badge so a
	// lifecycle prompt is not mistaken for a user message.
	taskPRAgentBadgeLabel = "PR automation"

	taskPRAgentEventReviewRequested = "review_requested"
	taskPRAgentEventMerged          = "merged"
	taskPRAgentEventClosed          = "closed"

	taskPRAgentReviewRequestedPrompt = "Your review was requested on %s."
	taskPRAgentMergedPrompt          = "The linked pull request %s was merged."
	taskPRAgentClosedPrompt          = "The linked pull request %s was closed without merging."
)

var (
	githubOwnerPattern     = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	githubRepoPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	errTaskPRAgentInactive = errors.New("task PR agent is inactive")
)

type taskPRAgentAutomationService interface {
	GetTaskCIOptionsResponse(ctx context.Context, taskID string) (*github.TaskCIOptionsResponse, error)
	GetTaskCIPRState(ctx context.Context, taskID, repositoryID string, prNumber int) (*github.TaskCIPRAutomationState, error)
	RebindTaskPRReviewer(ctx context.Context, taskID string) (string, bool, error)
	IsReviewRequestedForLogin(ctx context.Context, workspaceID, owner, repo string, prNumber int, login string) (bool, error)
	SetTaskPRReviewRequestState(ctx context.Context, taskID, repositoryID string, prNumber int, requested bool) error
	SetTaskPRObservedState(ctx context.Context, taskID, repositoryID string, prNumber int, state string) error
	RecordTaskPRLifecyclePrompt(ctx context.Context, prompt github.TaskPRLifecyclePrompt) error
}

type taskPRAgentPromptDecision struct {
	Event           string
	ReviewRequested *bool
	ObservedState   string
}

func decideTaskPRAgentPrompt(
	prState string,
	options *github.TaskCIOptionsResponse,
	checkpoint *github.TaskCIPRAutomationState,
	reviewRequested *bool,
) taskPRAgentPromptDecision {
	if options == nil {
		return taskPRAgentPromptDecision{}
	}
	previousState := observedTaskPRState(checkpoint)
	if prState == "merged" || prState == "closed" {
		return decideTaskPRTerminalPrompt(prState, previousState, options, checkpoint)
	}

	decision := taskPRAgentPromptDecision{}
	if previousState != prState {
		decision.ObservedState = prState
	}
	return decideTaskPRReviewPrompt(decision, options, checkpoint, reviewRequested)
}

func observedTaskPRState(checkpoint *github.TaskCIPRAutomationState) string {
	if checkpoint == nil {
		return ""
	}
	return checkpoint.LastObservedPRState
}

func decideTaskPRTerminalPrompt(
	prState, previousState string,
	options *github.TaskCIOptionsResponse,
	checkpoint *github.TaskCIPRAutomationState,
) taskPRAgentPromptDecision {
	if previousState == prState || (checkpoint != nil && checkpoint.LastLifecycleEvent == prState) {
		return taskPRAgentPromptDecision{}
	}
	decision := taskPRAgentPromptDecision{ObservedState: prState}
	if prState == "merged" && options.PromptOnMerged {
		decision.Event = taskPRAgentEventMerged
	}
	if prState == "closed" && options.PromptOnClosed {
		decision.Event = taskPRAgentEventClosed
	}
	return decision
}

func decideTaskPRReviewPrompt(
	decision taskPRAgentPromptDecision,
	options *github.TaskCIOptionsResponse,
	checkpoint *github.TaskCIPRAutomationState,
	reviewRequested *bool,
) taskPRAgentPromptDecision {
	if !options.PromptOnReviewRequested || reviewRequested == nil {
		return decision
	}
	if checkpoint == nil || !checkpoint.ReviewRequestInitialized {
		decision.ReviewRequested = reviewRequested
		return decision
	}
	if checkpoint.LastReviewRequested == *reviewRequested {
		return decision
	}
	decision.ReviewRequested = reviewRequested
	if *reviewRequested {
		decision.Event = taskPRAgentEventReviewRequested
	}
	return decision
}

func (s *Service) dispatchTaskPRAgentPrompt(
	ctx context.Context, pr *github.TaskPR, prompt, event string,
) (string, error) {
	prURL, err := canonicalTaskPRURL(pr)
	if err != nil {
		return "", err
	}
	session, err := s.resolveTaskPRAgentSession(ctx, pr.TaskID)
	if err != nil {
		return "", err
	}
	// Recheck immediately before the durable side effect: archive/delete can
	// race the PR poll that started this lifecycle evaluation.
	task, err := s.repo.GetTask(ctx, pr.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return "", fmt.Errorf("task is no longer active: %s", pr.TaskID)
	}
	if s.messageQueue == nil {
		return "", fmt.Errorf("message queue is not configured")
	}
	metadata := taskPRAgentPromptMetadata(pr, event, prURL)
	coalesceKey := fmt.Sprintf("github-pr:%s:%d:%s", pr.RepositoryID, pr.PRNumber, event)
	if _, _, accepted, err := s.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, session.ID, pr.TaskID, prompt, "", messagequeue.QueuedByWorkflow,
		false, nil, metadata, coalesceKey, true,
	); err != nil {
		return "", err
	} else if !accepted {
		return "", errTaskPRAgentInactive
	}
	s.publishQueueStatusEvent(ctx, session.ID)
	if ciAutomationSessionCanReceivePrompt(session) &&
		(session.State == models.TaskSessionStateWaitingForInput || session.State == models.TaskSessionStateIdle) {
		s.drainQueuedMessageForPromptableSession(ctx, session.ID)
	}
	return session.ID, nil
}

func (s *Service) resolveTaskPRAgentSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session.IsPrimary && ciAutomationSessionCanReceivePrompt(session) {
			return session, nil
		}
	}
	for _, session := range sessions {
		if ciAutomationSessionCanReceivePrompt(session) {
			return session, nil
		}
	}
	return nil, fmt.Errorf("no promptable agent session for task: %s", taskID)
}

func currentTaskPRReviewRequest(
	ctx context.Context,
	automation taskPRAgentAutomationService,
	pr *github.TaskPR,
	options *github.TaskCIOptionsResponse,
) (*bool, error) {
	if pr.State != "open" || !options.PromptOnReviewRequested ||
		strings.TrimSpace(options.ReviewReviewerLogin) == "" {
		return nil, nil
	}
	requested := false
	if pr.PendingReviewCount > 0 {
		var err error
		requested, err = automation.IsReviewRequestedForLogin(
			ctx, pr.WorkspaceID, pr.Owner, pr.Repo, pr.PRNumber, options.ReviewReviewerLogin,
		)
		if err != nil {
			return nil, err
		}
	}
	return &requested, nil
}

func stampTaskPRAgentObservations(
	ctx context.Context,
	automation taskPRAgentAutomationService,
	pr *github.TaskPR,
	decision taskPRAgentPromptDecision,
) error {
	if decision.ObservedState != "" {
		if err := automation.SetTaskPRObservedState(
			ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber, decision.ObservedState,
		); err != nil {
			return err
		}
	}
	if decision.ReviewRequested != nil {
		return automation.SetTaskPRReviewRequestState(
			ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber, *decision.ReviewRequested,
		)
	}
	return nil
}

func taskPRAgentLifecyclePrompt(event string, pr *github.TaskPR) (string, error) {
	prURL, err := canonicalTaskPRURL(pr)
	if err != nil {
		return "", err
	}
	switch event {
	case taskPRAgentEventReviewRequested:
		return fmt.Sprintf(taskPRAgentReviewRequestedPrompt, prURL), nil
	case taskPRAgentEventMerged:
		return fmt.Sprintf(taskPRAgentMergedPrompt, prURL), nil
	case taskPRAgentEventClosed:
		return fmt.Sprintf(taskPRAgentClosedPrompt, prURL), nil
	default:
		return "", fmt.Errorf("unsupported task PR lifecycle event: %s", event)
	}
}

func canonicalTaskPRURL(pr *github.TaskPR) (string, error) {
	if pr == nil || !githubOwnerPattern.MatchString(pr.Owner) || !githubRepoPattern.MatchString(pr.Repo) || pr.PRNumber <= 0 {
		return "", fmt.Errorf("invalid GitHub pull request identity")
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", pr.Owner, pr.Repo, pr.PRNumber), nil
}

func taskPRAgentPromptMetadata(pr *github.TaskPR, event, prURL string) map[string]interface{} {
	// The automation badge must survive delivery: the queue ghost derives its
	// origin cue from queued_by, but the persisted chat message only has this
	// metadata, so tag it explicitly or the prompt reads as user-typed.
	metadata := NewUserMessageMeta().
		WithAutoStart(true).
		WithWorkflowStep("", taskPRAgentBadgeLabel, "").
		ToMap()
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["origin"] = "github_pr_automation"
	metadata["automation_kind"] = event
	metadata["source"] = "github_pr_automation"
	metadata["event"] = event
	metadata["repository_id"] = pr.RepositoryID
	metadata["owner"] = pr.Owner
	metadata["repo"] = pr.Repo
	metadata["pr_number"] = pr.PRNumber
	metadata["pr_url"] = prURL
	return metadata
}
