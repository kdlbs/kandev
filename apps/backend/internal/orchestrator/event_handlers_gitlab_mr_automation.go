package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/gitlab"
)

// gitlabMRState* are the normalized MR state constants (mirroring
// gitlab.normalizeMRState's output vocabulary: "opened" from the API becomes
// "open"). Declared as a separate constant block from the lifecycle event
// identifiers below — even though "merged"/"closed" coincide as strings —
// so a future rename of one vocabulary cannot silently change the other
// (AC35; CodeRabbit raised the collapsed version of this on PR #2038 and it
// was left unfixed there).
const (
	gitlabMRStateOpen   = "open"
	gitlabMRStateClosed = "closed"
	gitlabMRStateMerged = "merged"
	gitlabMRStateLocked = "locked"
)

const (
	mrAgentEventReviewRequested = "review_requested"
	mrAgentEventMerged          = "merged"
	mrAgentEventClosed          = "closed"

	// taskMRAgentBadgeLabel names the automation in the chat origin badge so
	// a lifecycle prompt is not mistaken for a user message (AC19).
	taskMRAgentBadgeLabel = "MR automation"

	taskMRAgentReviewRequestedPrompt = "Your review was requested on %s."
	taskMRAgentMergedPrompt          = "The linked merge request %s was merged."
	taskMRAgentClosedPrompt          = "The linked merge request %s was closed without merging."

	// mrAutomationOrigin and the metadata keys below are shared between the
	// producer (taskMRAgentPromptMetadata) and any consumer that reads the
	// queued/persisted message metadata (AC36).
	mrAutomationOrigin           = "gitlab_mr_automation"
	mrAutomationMetaOrigin       = "origin"
	mrAutomationMetaEvent        = "event"
	mrAutomationMetaMRURL        = "mr_url"
	mrAutomationMetaMRIID        = "mr_iid"
	mrAutomationMetaProject      = "project_path"
	mrAutomationMetaRepository   = "repository_id"
	mrAutomationStateEventSource = "gitlab_mr_automation_state"
)

var (
	gitlabProjectPathSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	errTaskMRAgentInactive          = errors.New("task MR agent is inactive")
)

// taskMRAgentAutomationService is the narrow lifecycle-automation surface the
// orchestrator needs from the GitLab service. Type-asserted against
// s.gitlabMRAutomation, mirroring taskPRAgentAutomationService.
type taskMRAgentAutomationService interface {
	GetTaskMRAutomationResponse(ctx context.Context, taskID string) (*gitlab.TaskMRAutomationResponse, error)
	GetTaskMRLifecycleState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) (*gitlab.TaskMRLifecycleState, error)
	RebindTaskMRReviewer(ctx context.Context, taskID string) (string, bool, error)
	IsReviewerOnMR(ctx context.Context, taskID, projectPath string, mrIID int, username string) (bool, error)
	SetTaskMRReviewRequestState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, requested bool) error
	SetTaskMRObservedState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, state string) error
	RecordTaskMRLifecyclePrompt(ctx context.Context, prompt gitlab.TaskMRLifecyclePrompt) error
	RecordTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error
}

type taskMRAgentPromptDecision struct {
	Event           string
	ReviewRequested *bool
	ObservedState   string
}

// decideTaskMRAgentPrompt is the pure decision function: given the current
// normalized MR state, the task's switches, the per-MR checkpoint, and the
// freshly observed review-request boolean, it decides whether a lifecycle
// prompt should fire and what checkpoint fields should be stamped either
// way. No I/O — every branch is covered by table-driven tests (AC10-AC16).
func decideTaskMRAgentPrompt(
	mrState string,
	options *gitlab.TaskMRAutomationResponse,
	checkpoint *gitlab.TaskMRLifecycleState,
	reviewRequested *bool,
) taskMRAgentPromptDecision {
	if options == nil {
		return taskMRAgentPromptDecision{}
	}
	previousState := observedTaskMRState(checkpoint)
	if mrState == gitlabMRStateMerged || mrState == gitlabMRStateClosed {
		return decideTaskMRTerminalPrompt(mrState, previousState, options)
	}
	decision := taskMRAgentPromptDecision{}
	if previousState != mrState {
		decision.ObservedState = mrState
	}
	return decideTaskMRReviewPrompt(decision, options, checkpoint, reviewRequested)
}

func observedTaskMRState(checkpoint *gitlab.TaskMRLifecycleState) string {
	if checkpoint == nil {
		return ""
	}
	return checkpoint.LastObservedState
}

// decideTaskMRTerminalPrompt fires merged/closed exactly once per observed
// transition into that state. Re-polling an unchanged terminal state is a
// no-op (previousState == mrState); a later reopen-then-reclose cycle
// re-arms naturally because the reopen observation updates previousState
// away from the terminal value first (AC14). locked never reaches this
// function — the caller only routes here for merged/closed (AC15).
func decideTaskMRTerminalPrompt(
	mrState, previousState string,
	options *gitlab.TaskMRAutomationResponse,
) taskMRAgentPromptDecision {
	if previousState == mrState {
		return taskMRAgentPromptDecision{}
	}
	decision := taskMRAgentPromptDecision{ObservedState: mrState}
	switch mrState {
	case gitlabMRStateMerged:
		if options.PromptOnMerged {
			decision.Event = mrAgentEventMerged
		}
	case gitlabMRStateClosed:
		if options.PromptOnClosed {
			decision.Event = mrAgentEventClosed
		}
	}
	return decision
}

// decideTaskMRReviewPrompt fires review_requested only on a false→true edge
// relative to the stored baseline. The very first observation for a given
// checkpoint (ReviewRequestInitialized == false) silently baselines instead
// of firing (AC10); a true→false transition re-arms the edge detector
// without prompting (AC12).
func decideTaskMRReviewPrompt(
	decision taskMRAgentPromptDecision,
	options *gitlab.TaskMRAutomationResponse,
	checkpoint *gitlab.TaskMRLifecycleState,
	reviewRequested *bool,
) taskMRAgentPromptDecision {
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
		decision.Event = mrAgentEventReviewRequested
	}
	return decision
}

// currentTaskMRReviewRequest resolves whether the configured reviewer
// username currently appears on the MR (GitLab's assignment-as-request
// signal — there is no distinct "review requested" API event). Only called
// for open MRs with the switch on and a resolved reviewer; returns (nil,
// nil) otherwise so decideTaskMRReviewPrompt treats it as "nothing to
// evaluate" rather than a false observation.
func currentTaskMRReviewRequest(
	ctx context.Context,
	automation taskMRAgentAutomationService,
	mr *gitlab.TaskMR,
	options *gitlab.TaskMRAutomationResponse,
) (*bool, error) {
	if mr.State != gitlabMRStateOpen || !options.PromptOnReviewRequested ||
		strings.TrimSpace(options.ReviewReviewerUsername) == "" {
		return nil, nil
	}
	requested, err := automation.IsReviewerOnMR(ctx, mr.TaskID, mr.ProjectPath, mr.MRIID, options.ReviewReviewerUsername)
	if err != nil {
		return nil, err
	}
	return &requested, nil
}

func stampTaskMRAgentObservations(
	ctx context.Context,
	automation taskMRAgentAutomationService,
	mr *gitlab.TaskMR,
	decision taskMRAgentPromptDecision,
) error {
	if decision.ObservedState != "" {
		if err := automation.SetTaskMRObservedState(
			ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID, decision.ObservedState,
		); err != nil {
			return err
		}
	}
	if decision.ReviewRequested != nil {
		return automation.SetTaskMRReviewRequestState(
			ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID, *decision.ReviewRequested,
		)
	}
	return nil
}

func taskMRAgentLifecyclePrompt(event string, mr *gitlab.TaskMR) (string, error) {
	mrURL, err := canonicalTaskMRURL(mr)
	if err != nil {
		return "", err
	}
	switch event {
	case mrAgentEventReviewRequested:
		return fmt.Sprintf(taskMRAgentReviewRequestedPrompt, mrURL), nil
	case mrAgentEventMerged:
		return fmt.Sprintf(taskMRAgentMergedPrompt, mrURL), nil
	case mrAgentEventClosed:
		return fmt.Sprintf(taskMRAgentClosedPrompt, mrURL), nil
	default:
		return "", fmt.Errorf("unsupported task MR lifecycle event: %s", event)
	}
}

// canonicalTaskMRURL rebuilds the MR URL canonically from the linked row's
// own (host, project_path, mr_iid) rather than trusting any GitLab-supplied
// title/branch/note text — no server-fetched free text ever reaches the
// prompt (AC18). host must be a bare HTTP(S) origin; project_path must be
// two or more `[A-Za-z0-9._-]` segments with no traversal.
func canonicalTaskMRURL(mr *gitlab.TaskMR) (string, error) {
	if mr == nil || mr.MRIID <= 0 {
		return "", fmt.Errorf("invalid GitLab merge request identity")
	}
	host, err := canonicalMRHost(mr.Host)
	if err != nil {
		return "", err
	}
	projectPath, err := canonicalMRProjectPath(mr.ProjectPath)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/-/merge_requests/%d", host, projectPath, mr.MRIID), nil
}

func canonicalMRHost(rawHost string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawHost))
	if err != nil || parsed.Host == "" || parsed.Path != "" || parsed.User != nil {
		return "", fmt.Errorf("invalid GitLab merge request host")
	}
	// Restrict to HTTP(S) so a stored host with a non-web scheme can never
	// produce a notification URL the client would treat as a live link.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid GitLab merge request host: unsupported scheme %q", parsed.Scheme)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func canonicalMRProjectPath(projectPath string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(projectPath), "/")
	if trimmed == "" || strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("invalid GitLab project path")
	}
	segments := strings.Split(trimmed, "/")
	if len(segments) < 2 {
		return "", fmt.Errorf("invalid GitLab project path")
	}
	for _, segment := range segments {
		if !gitlabProjectPathSegmentPattern.MatchString(segment) {
			return "", fmt.Errorf("invalid GitLab project path")
		}
	}
	return trimmed, nil
}

func taskMRAgentPromptMetadata(mr *gitlab.TaskMR, event, mrURL string) map[string]interface{} {
	metadata := NewUserMessageMeta().
		WithAutoStart(true).
		WithWorkflowStep("", taskMRAgentBadgeLabel, "").
		ToMap()
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata[mrAutomationMetaOrigin] = mrAutomationOrigin
	metadata[mrAutomationMetaEvent] = event
	metadata[mrAutomationMetaMRURL] = mrURL
	metadata[mrAutomationMetaMRIID] = mr.MRIID
	metadata[mrAutomationMetaProject] = mr.ProjectPath
	metadata[mrAutomationMetaRepository] = mr.RepositoryID
	return metadata
}

// dispatchTaskMRAgentPrompt resolves a promptable session and queues the
// prompt through the shared durable lifecycle queue. The task is re-read
// immediately before the durable side effect: archive/delete can race the
// MR poll that started this evaluation (AC21).
func (s *Service) dispatchTaskMRAgentPrompt(
	ctx context.Context, mr *gitlab.TaskMR, prompt, event string,
) (string, error) {
	mrURL, err := canonicalTaskMRURL(mr)
	if err != nil {
		return "", err
	}
	session, err := s.resolveTaskPRAgentSession(ctx, mr.TaskID)
	if err != nil {
		return "", err
	}
	task, err := s.repo.GetTask(ctx, mr.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return "", fmt.Errorf("task is no longer active: %s", mr.TaskID)
	}
	metadata := taskMRAgentPromptMetadata(mr, event, mrURL)
	coalesceKey := fmt.Sprintf("gitlab-mr:%s:%s:%d:%s", mr.RepositoryID, mr.ProjectPath, mr.MRIID, event)
	return s.queueAndDrainLifecyclePrompt(ctx, session, mr.TaskID, prompt, metadata, coalesceKey, errTaskMRAgentInactive)
}
