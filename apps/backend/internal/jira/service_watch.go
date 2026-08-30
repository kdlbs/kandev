package jira

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/watchreset"
)

var ErrIssueWatchNotFound = errors.New("jira: issue watch not found")

// CreateIssueWatch validates the request and persists a new watch row.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if err := validateIssueWatchCreate(req); err != nil {
		return nil, err
	}
	// WorkspaceID comes from the request body, which the query-only integration
	// middleware never authorizes. Without this a caller could plant a watch in
	// another user's workspace that repeatedly spawns tasks with that user's
	// credentials and an attacker-supplied JQL/prompt.
	if err := s.authorizeWorkspaceAccess(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	repositoryID, baseBranch, err := s.resolveRepositoryBinding(ctx, req.WorkspaceID, req.RepositoryID, req.BaseBranch)
	if err != nil {
		return nil, err
	}
	w := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		RepositoryID:        repositoryID,
		BaseBranch:          baseBranch,
		JQL:                 strings.TrimSpace(req.JQL),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		PollIntervalSeconds: req.PollIntervalSeconds,
		MaxInflightTasks:    req.MaxInflightTasks,
		Enabled:             true,
	}
	if req.Enabled != nil {
		w.Enabled = *req.Enabled
	}
	if err := s.store.CreateIssueWatch(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// ListIssueWatches returns the watches configured for a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListIssueWatches(ctx, workspaceID)
}

// ListAllIssueWatches returns every watch the caller may see. For a scoped
// caller that is only their own workspaces' watches; for an identity-less
// internal caller (unscoped) it is every watch, as before auth. The unscoped
// list form is only reachable when workspace_id is omitted, which the query-only
// integration middleware does not authorize — without this filter it leaked
// every workspace's watch config (JQL, repo/agent/profile IDs, spawn prompt).
func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	watches, err := s.store.ListAllIssueWatches(ctx)
	if err != nil {
		return nil, err
	}
	return s.filterIssueWatchesByAccess(ctx, watches)
}

// filterIssueWatchesByAccess keeps only watches whose workspace the caller may
// access. Access decisions are memoized per workspace so a long list costs one
// authorize call per distinct workspace, not one per watch. Only an
// ErrWorkspaceNotFound denial drops a watch; any other authorizer error (a
// transient DB failure, say) is propagated so the caller sees the failure
// rather than a silently truncated 200.
func (s *Service) filterIssueWatchesByAccess(ctx context.Context, watches []*IssueWatch) ([]*IssueWatch, error) {
	decision := make(map[string]bool)
	visible := make([]*IssueWatch, 0, len(watches))
	for _, w := range watches {
		allowed, seen := decision[w.WorkspaceID]
		if !seen {
			switch err := s.authorizeWorkspaceAccess(ctx, w.WorkspaceID); {
			case err == nil:
				allowed = true
			case errors.Is(err, repoerrors.ErrWorkspaceNotFound):
				allowed = false
			default:
				return nil, err
			}
			decision[w.WorkspaceID] = allowed
		}
		if allowed {
			visible = append(visible, w)
		}
	}
	return visible, nil
}

// GetIssueWatch returns a single watch by ID or ErrIssueWatchNotFound.
func (s *Service) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	w, err := s.store.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrIssueWatchNotFound
	}
	if err := s.authorizeWorkspaceAccess(ctx, w.WorkspaceID); err != nil {
		return nil, err
	}
	return w, nil
}

// UpdateIssueWatch applies a partial update by patching only the fields the
// caller explicitly set, then persists the result.
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) (*IssueWatch, error) {
	w, err := s.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	prevRepositoryID, prevBaseBranch := w.RepositoryID, w.BaseBranch
	applyIssueWatchPatch(w, req)
	// Empty-string PATCH writes (`{"workflowId": ""}` etc.) bypass the nil
	// guard in applyIssueWatchPatch — Go's JSON decoder returns a non-nil
	// pointer to "". Guard the post-patch state so a PATCH can't strip a row
	// of the fields the orchestrator needs to create tasks.
	if w.JQL == "" {
		return nil, fmt.Errorf("%w: jql cannot be empty", ErrInvalidConfig)
	}
	if w.WorkflowID == "" || w.WorkflowStepID == "" {
		return nil, fmt.Errorf("%w: workflowId and workflowStepId cannot be empty", ErrInvalidConfig)
	}
	if err := validatePollInterval(w.PollIntervalSeconds); err != nil {
		return nil, err
	}
	if err := validateMaxInflightTasks(w.MaxInflightTasks); err != nil {
		return nil, err
	}
	// Only validate/resolve the binding when its value actually changed. The
	// dialog re-sends repositoryId/baseBranch on every PATCH, and an unchanged
	// binding whose repo was since soft-deleted must not block edits to other
	// fields (prompt, JQL, …).
	if w.RepositoryID != prevRepositoryID || w.BaseBranch != prevBaseBranch {
		repositoryID, baseBranch, err := s.resolveRepositoryBinding(ctx, w.WorkspaceID, w.RepositoryID, w.BaseBranch)
		if err != nil {
			return nil, err
		}
		w.RepositoryID = repositoryID
		w.BaseBranch = baseBranch
	}
	if err := s.store.UpdateIssueWatch(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// DeleteIssueWatch removes the watch and its dedup rows. Idempotent: deleting
// a missing ID is a silent success.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	w, err := s.store.GetIssueWatch(ctx, id)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	if err := s.authorizeWorkspaceAccess(ctx, w.WorkspaceID); err != nil {
		return err
	}
	return s.store.DeleteIssueWatch(ctx, id)
}

// jiraIssueWatchResetter is the watchreset.Resetter adapter for a single
// Jira issue watch. Closes over the store and watch ID so the shared
// watchreset.Run helper stays integration-agnostic.
type jiraIssueWatchResetter struct {
	store   *Store
	watchID string
}

func (r *jiraIssueWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListIssueWatchTaskIDs(ctx, r.watchID)
}

func (r *jiraIssueWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetIssueWatchState(ctx, r.watchID)
}

// PreviewResetIssueWatch returns how many tasks ResetIssueWatch would
// cascade-delete. Used by the frontend to populate the confirmation dialog.
func (s *Service) PreviewResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	if _, err := s.GetIssueWatch(ctx, watchID); err != nil {
		return 0, err
	}
	return watchreset.Preview(ctx, &jiraIssueWatchResetter{store: s.store, watchID: watchID})
}

// ResetIssueWatch is destructive: cascade-deletes every task previously
// created by the watch (including archived), wipes the per-watch dedup
// rows, and nulls last_polled_at so the next poll re-imports every
// currently-matching ticket. Returns the count of tasks deleted.
func (s *Service) ResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	if _, err := s.GetIssueWatch(ctx, watchID); err != nil {
		return 0, err
	}
	s.mu.Lock()
	td := s.taskDeleter
	s.mu.Unlock()
	if td == nil {
		return 0, errors.New("jira: task deleter not wired; reset unavailable")
	}
	res, err := watchreset.Run(ctx,
		&jiraIssueWatchResetter{store: s.store, watchID: watchID},
		td, s.log)
	return res.TasksDeleted, err
}

// CheckIssueWatch runs the watch's JQL once and returns the tickets that
// haven't been turned into tasks yet. last_polled_at is stamped regardless of
// whether the search succeeded — a failing search still counts as "we tried"
// so the UI can show liveness.
func (s *Service) CheckIssueWatch(ctx context.Context, w *IssueWatch) ([]*JiraTicket, error) {
	defer s.stampLastPolled(w.ID)
	client, err := s.clientFor(ctx, w.WorkspaceID)
	if err != nil {
		return nil, err
	}
	// Single page is enough per tick: the next poll picks up anything that
	// overflowed maxResults. Pagination would let one chatty workspace starve
	// the others without any real freshness benefit.
	res, err := client.SearchTicketsForWatch(ctx, w.JQL, "", issueWatchSearchPageSize)
	if err != nil {
		return nil, err
	}
	// Bulk-fetch the dedup set once per call instead of one query per ticket —
	// a broad JQL can return up to issueWatchSearchPageSize tickets, so a
	// per-ticket round-trip would multiply the per-tick cost.
	seen, err := s.store.ListSeenIssueKeys(ctx, w.ID)
	if err != nil {
		s.log.Warn("jira: dedup set fetch failed",
			zap.String("watch_id", w.ID), zap.Error(err))
		return nil, fmt.Errorf("load Jira issue-watch dedup set: %w", err)
	}
	out := make([]*JiraTicket, 0, len(res.Tickets))
	for i := range res.Tickets {
		t := res.Tickets[i]
		if _, ok := seen[t.Key]; ok {
			continue
		}
		out = append(out, &t)
	}
	return out, nil
}

// stampLastPolled writes the current timestamp onto the watch row using a
// fresh background context with a short write deadline. The caller's ctx may
// already be cancelled (typical at poller shutdown), which would cause the
// DB write to fail silently and the watch would look "never polled" on the
// next backend restart. Mirrors the pattern in RecordAuthHealth.
func (s *Service) stampLastPolled(watchID string) {
	ctx, cancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer cancel()
	if err := s.store.UpdateIssueWatchLastPolled(ctx, watchID, time.Now().UTC()); err != nil {
		s.log.Warn("jira: update last_polled_at failed",
			zap.String("watch_id", watchID), zap.Error(err))
	}
}

// publishNewJiraIssueEvent emits the orchestrator-facing event for one freshly
// observed ticket. No-op when the event bus is not wired (tests, early boot).
func (s *Service) publishNewJiraIssueEvent(ctx context.Context, w *IssueWatch, ticket *JiraTicket) {
	s.mu.Lock()
	eb := s.eventBus
	s.mu.Unlock()
	if eb == nil {
		return
	}
	evt := bus.NewEvent(events.JiraNewIssue, "jira", &NewJiraIssueEvent{
		IssueWatchID:      w.ID,
		WorkspaceID:       w.WorkspaceID,
		WorkflowID:        w.WorkflowID,
		WorkflowStepID:    w.WorkflowStepID,
		RepositoryID:      w.RepositoryID,
		BaseBranch:        w.BaseBranch,
		AgentProfileID:    w.AgentProfileID,
		ExecutorProfileID: w.ExecutorProfileID,
		Prompt:            w.Prompt,
		MaxInflightTasks:  w.MaxInflightTasks,
		Issue:             ticket,
	})
	if err := eb.Publish(ctx, events.JiraNewIssue, evt); err != nil {
		s.log.Debug("jira: publish new issue event failed",
			zap.String("watch_id", w.ID), zap.String("issue_key", ticket.Key), zap.Error(err))
	}
}

// issueWatchSearchPageSize caps how many tickets a single CheckIssueWatch call
// pulls from JIRA. 50 is well under the API's 100-result limit and keeps the
// per-tick cost bounded even for very broad JQL queries.
const issueWatchSearchPageSize = 50
