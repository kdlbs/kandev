package statussummary

import (
	"strings"
	"time"
)

// RebuildSession is the durable/session-backed portion of a task summary.
// Callers may add live activity values when an in-memory activity provider is
// available; the rest of the fields come from task_sessions and metadata.
type RebuildSession struct {
	ID                  string
	State               string
	IsPrimary           bool
	ForegroundActivity  string
	ActiveSubagentCount int
	ActiveError         *ActiveErrorSummary
}

// RebuildGit is one authoritative Git snapshot. Repository is empty for the
// single-repository shape and is otherwise the stable repository key.
type RebuildGit struct {
	Repository string
	Summary    GitSummary
}

// PullRequestInput is the bounded GitHub state needed to rebuild a task row.
// It deliberately does not expose the full provider record to the summary
// package or to the WebSocket payload.
type PullRequestInput struct {
	Key                   string
	State                 string
	Number                int
	URL                   string
	ReviewState           string
	ChecksState           string
	MergeableState        string
	MergeQueueState       string
	UnresolvedReviewCount int
	PendingReviewCount    int
	RequiredReviews       int
	ChecksTotal           int
	ChecksPassing         int
	AutoFixEnabled        bool
	AutoMergeEnabled      bool
}

// RebuildInput contains the authoritative bounded facts available from
// durable stores and optional live providers. Every source is marked as
// observed by its corresponding boolean so an unavailable optional provider
// does not masquerade as an authoritative empty value.
type RebuildInput struct {
	Sessions         []RebuildSession
	TaskError        *ActiveErrorSummary
	PendingActions   map[string]string
	ActivityObserved bool
	LastActivityAt   *time.Time
	Git              []RebuildGit
	GitObserved      bool
	PullRequests     []PullRequestInput
	PRObserved       bool
	// QueuedPromptCount is the authoritative pending prompt count for the task
	// (all sessions). Supplied by the caller; 0 means nothing is queued.
	QueuedPromptCount int
	Now               time.Time
}

// BuildFromAuthoritative derives the same summary semantics used by the live
// projector, but starts from a batch-loaded snapshot. It is used to repair
// rows created before the projector existed, without subscribing to or
// replaying any session stream.
func BuildFromAuthoritative(input RebuildInput) TaskStatusSummary {
	state := &projectionState{
		sessions:           make(map[string]sessionObservation, len(input.Sessions)),
		pending:            make(map[string]string, len(input.PendingActions)),
		pendingRequests:    make(map[string]pendingRequestIdentity),
		errors:             make(map[string]*ActiveErrorSummary),
		clearedErrorStamps: make(map[string]string),
		git:                make(map[string]GitSummary, len(input.Git)),
		prs:                make(map[string]pullRequestObservation, len(input.PullRequests)),
		pendingObserved:    true,
		activityObserved:   input.ActivityObserved,
		errorsObserved:     true,
		gitObserved:        input.GitObserved,
		prObserved:         input.PRObserved,
	}
	for _, inputSession := range input.Sessions {
		if strings.TrimSpace(inputSession.ID) == "" {
			continue
		}
		state.sessions[inputSession.ID] = sessionObservation{
			id:                  inputSession.ID,
			state:               inputSession.State,
			isPrimary:           inputSession.IsPrimary,
			foregroundActivity:  inputSession.ForegroundActivity,
			activeSubagentCount: maxInt(inputSession.ActiveSubagentCount, 0),
		}
		if inputSession.ActiveError != nil {
			if activeError := normalizeRebuildError(inputSession.ActiveError, input.Now); activeError != nil {
				state.errors[inputSession.ID] = activeError
			}
		}
	}
	if taskError := normalizeRebuildError(input.TaskError, input.Now); taskError != nil {
		state.taskError = taskError
		state.taskErrorObserved = true
	}
	for sessionID, action := range input.PendingActions {
		if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(action) == "" {
			continue
		}
		state.pending[sessionID] = action
	}
	recomputeTaskPending(state)
	for _, git := range input.Git {
		repository := strings.TrimSpace(git.Repository)
		if repository == "" {
			repository = RootRepositoryKey
		}
		state.git[repository] = git.Summary
	}
	applyPullRequestInputs(state, input.PullRequests)
	state.queuedCount = maxInt(input.QueuedPromptCount, 0)
	summary := deriveSummary(state)
	if input.LastActivityAt != nil {
		lastActivityAt := input.LastActivityAt.UTC()
		summary.LastActivityAt = &lastActivityAt
	}
	return summary
}

func normalizeRebuildError(input *ActiveErrorSummary, now time.Time) *ActiveErrorSummary {
	if input == nil {
		return nil
	}
	copy := *input
	if copy.OccurredAt.IsZero() {
		copy.OccurredAt = now.UTC()
	}
	copy.Preview = truncateString(copy.Preview, MaxActiveErrorPreviewBytes)
	copy.SessionID = truncateString(copy.SessionID, maxSessionIDBytes)
	copy.TaskRepositoryID = truncateString(copy.TaskRepositoryID, maxTaskRepositoryIDBytes)
	copy.Category = truncateString(copy.Category, maxActiveErrorCategoryBytes)
	copy.RecoveryActions = normalizeRecoveryActions(copy.RecoveryActions)
	if copy.Stamp == "" {
		copy.Stamp = copy.OccurredAt.UTC().Format(time.RFC3339Nano) + ":" + copy.Preview
	}
	copy.Stamp = truncateString(copy.Stamp, maxActiveErrorStampBytes)
	return &copy
}
