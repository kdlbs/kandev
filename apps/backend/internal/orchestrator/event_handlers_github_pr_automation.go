package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	taskPRAgentEventReviewRequested = "review_requested"
	taskPRAgentEventMerged          = "merged"
	taskPRAgentEventClosed          = "closed"
)

type taskPRAgentAutomationService interface {
	GetTaskPRByRepoAndNumber(ctx context.Context, taskID, repositoryID string, prNumber int) (*github.TaskPR, error)
	GetTaskCIOptionsResponse(ctx context.Context, taskID string) (*github.TaskCIOptionsResponse, error)
	GetTaskCIPRState(ctx context.Context, taskID, repositoryID string, prNumber int) (*github.TaskCIPRAutomationState, error)
	IsReviewRequestedForLogin(ctx context.Context, owner, repo string, prNumber int, login string) (bool, error)
	SetTaskPRReviewRequestState(ctx context.Context, taskID, repositoryID string, prNumber int, requested bool) error
	SetTaskPRObservedState(ctx context.Context, taskID, repositoryID string, prNumber int, state string) error
	RecordTaskPRLifecyclePrompt(ctx context.Context, prompt github.TaskPRLifecyclePrompt) error
}

type taskPRAgentPromptDecision struct {
	Event           string
	Prompt          string
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
		decision.Prompt = options.EffectiveMergedPrompt
	}
	if prState == "closed" && options.PromptOnClosed {
		decision.Event = taskPRAgentEventClosed
		decision.Prompt = options.EffectiveClosedPrompt
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
		decision.Prompt = options.EffectiveReviewPrompt
	}
	return decision
}

func (s *Service) dispatchTaskPRAgentPrompt(
	ctx context.Context, pr *github.TaskPR, prompt, event string,
) (string, error) {
	session, err := s.resolveCIAutoFixSession(ctx, pr.TaskID, nil)
	if err != nil {
		return "", err
	}
	metadata := taskPRAgentPromptMetadata(pr, event)
	coalesceKey := fmt.Sprintf("github-pr:%s:%d:%s", pr.RepositoryID, pr.PRNumber, event)
	switch session.State {
	case models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning:
		if s.messageQueue == nil {
			return "", fmt.Errorf("message queue is not configured")
		}
		if _, _, err := s.messageQueue.QueueMessageWithCoalesceKey(
			ctx, session.ID, pr.TaskID, prompt, "", messagequeue.QueuedByWorkflow,
			false, nil, metadata, coalesceKey, true,
		); err != nil {
			return "", err
		}
		s.publishQueueStatusEvent(ctx, session.ID)
	case models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
		if err := s.recordTaskPRAgentUserMessage(ctx, session, prompt, metadata); err != nil {
			return "", err
		}
		if _, err := s.PromptTask(ctx, pr.TaskID, session.ID, prompt, "", false, nil, true); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("session is not promptable: %s", session.State)
	}
	return session.ID, nil
}

func (s *Service) recordTaskPRAgentUserMessage(
	ctx context.Context, session *models.TaskSession, prompt string, metadata map[string]interface{},
) error {
	if s.messageCreator == nil {
		return fmt.Errorf("message creator is not configured")
	}
	turnID := s.getActiveTurnID(session.ID)
	if turnID == "" {
		s.startTurnForSession(ctx, session.ID)
		turnID = s.getActiveTurnID(session.ID)
	}
	return s.messageCreator.CreateUserMessage(
		ctx, session.TaskID, prompt, session.ID, turnID, metadata,
	)
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
			ctx, pr.Owner, pr.Repo, pr.PRNumber, options.ReviewReviewerLogin,
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

func renderTaskPRAgentPrompt(template string, pr *github.TaskPR) string {
	repo := pr.Owner + "/" + pr.Repo
	return strings.NewReplacer(
		"{{pr.url}}", pr.PRURL,
		"{{pr.link}}", pr.PRURL,
		"{{pr.number}}", strconv.Itoa(pr.PRNumber),
		"{{pr.title}}", pr.PRTitle,
		"{{pr.repo}}", repo,
		"{{pr.owner}}", pr.Owner,
		"{{pr.name}}", pr.Repo,
		"{{pr.branch}}", pr.HeadBranch,
		"{{pr.base_branch}}", pr.BaseBranch,
		"{{pr.state}}", pr.State,
	).Replace(template)
}

func taskPRAgentPromptMetadata(pr *github.TaskPR, event string) map[string]interface{} {
	metadata := NewUserMessageMeta().WithAutoStart(true).ToMap()
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
	metadata["pr_url"] = pr.PRURL
	return metadata
}
