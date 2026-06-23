package sentry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/watchreset"
)

// ErrIssueWatchNotFound is returned when GetIssueWatch's caller looks up an ID
// that doesn't exist. Callers map this to HTTP 404.
var ErrIssueWatchNotFound = errors.New("sentry: issue watch not found")

// SetEventBus wires the bus used to publish NewSentryIssueEvent. Optional: if
// unset the poller still runs but observed issues do not become Kandev tasks.
func (s *Service) SetEventBus(eb bus.EventBus) {
	s.mu.Lock()
	s.eventBus = eb
	s.mu.Unlock()
}

// CreateIssueWatch validates the request and persists a new watch row.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if err := validateIssueWatchCreate(req); err != nil {
		return nil, err
	}
	w := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Filter:              normalizeFilter(req.Filter),
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
	return s.store.ListIssueWatches(ctx, workspaceID)
}

// ListAllIssueWatches returns every watch across all workspaces.
func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.store.ListAllIssueWatches(ctx)
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
	return w, nil
}

// UpdateIssueWatch applies a partial update by patching only the fields the
// caller explicitly set, then persists the result.
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) (*IssueWatch, error) {
	w, err := s.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	applyIssueWatchPatch(w, req)
	if err := validateFilter(w.Filter); err != nil {
		return nil, err
	}
	// Only enforce the single-status rule when the caller actually changed the
	// filter. A partial update that leaves the filter untouched (e.g. the
	// enable/disable toggle) must not fail on a legacy multi-status watch.
	if req.Filter != nil {
		if err := validateFilterStatuses(w.Filter); err != nil {
			return nil, err
		}
	}
	if w.WorkflowID == "" || w.WorkflowStepID == "" {
		return nil, fmt.Errorf("%w: workflowId and workflowStepId cannot be empty", ErrInvalidConfig)
	}
	if err := validateMaxInflightTasks(w.MaxInflightTasks); err != nil {
		return nil, err
	}
	if err := validatePollInterval(w.PollIntervalSeconds); err != nil {
		return nil, err
	}
	if err := s.store.UpdateIssueWatch(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// DeleteIssueWatch removes the watch and its dedup rows. Idempotent.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	return s.store.DeleteIssueWatch(ctx, id)
}

// sentryIssueWatchResetter is the watchreset.Resetter adapter for a single
// Sentry issue watch. Closes over the store and watch ID so the shared
// watchreset.Run helper stays integration-agnostic.
type sentryIssueWatchResetter struct {
	store   *Store
	watchID string
}

func (r *sentryIssueWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListIssueWatchTaskIDs(ctx, r.watchID)
}

func (r *sentryIssueWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetIssueWatchState(ctx, r.watchID)
}

// PreviewResetIssueWatch returns how many tasks ResetIssueWatch would
// cascade-delete. Used by the frontend to populate the confirmation dialog.
func (s *Service) PreviewResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	return watchreset.Preview(ctx, &sentryIssueWatchResetter{store: s.store, watchID: watchID})
}

// ResetIssueWatch is destructive: cascade-deletes every task previously
// created by the watch (including archived), wipes the per-watch dedup
// rows, and nulls last_polled_at so the next poll re-imports every
// currently-matching issue. Returns the count of tasks deleted.
func (s *Service) ResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	s.mu.Lock()
	td := s.taskDeleter
	s.mu.Unlock()
	if td == nil {
		return 0, errors.New("sentry: task deleter not wired; reset unavailable")
	}
	res, err := watchreset.Run(ctx,
		&sentryIssueWatchResetter{store: s.store, watchID: watchID},
		td, s.log)
	return res.TasksDeleted, err
}

// CheckIssueWatch runs the watch's filter once and returns the issues that
// haven't been turned into tasks yet. last_polled_at is stamped regardless of
// whether the search succeeded — a failing search still counts as "we tried".
//
// Concurrency note: callers must tolerate being handed an issue that gets
// stolen by a concurrent reserver. The duplicate publish is harmless — the
// second reserver loses the INSERT OR IGNORE race in the orchestrator and
// bails. Same pattern as the Linear / Jira watchers.
func (s *Service) CheckIssueWatch(ctx context.Context, w *IssueWatch) ([]*SentryIssue, error) {
	defer s.stampWatchLastPolled(w.ID)
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	// Intentionally reads only the first page per tick (bounded-page-per-tick
	// invariant, matching the Linear/Jira watchers). SearchIssues sorts results
	// by first-seen descending (sort=new) so newly created issues reliably land
	// on page one and are not missed by the single-page read.
	res, err := client.SearchIssues(ctx, w.Filter, "")
	if err != nil {
		return nil, err
	}
	seen, err := s.store.ListSeenIssueShortIDs(ctx, w.ID)
	if err != nil {
		// Skip this tick rather than treat a failed dedup read as "nothing seen":
		// a nil map would let the whole page (up to 100 issues) publish as events.
		// The next tick retries with a working dedup set.
		return nil, fmt.Errorf("load dedup set for watch %s: %w", w.ID, err)
	}
	out := make([]*SentryIssue, 0, len(res.Issues))
	for i := range res.Issues {
		issue := res.Issues[i]
		if _, ok := seen[issue.ShortID]; ok {
			continue
		}
		out = append(out, &issue)
	}
	return out, nil
}

// stampWatchLastPolled writes the current timestamp using a fresh background
// context with a short write deadline, so a cancelled caller ctx (e.g. shutdown)
// doesn't drop the liveness record.
func (s *Service) stampWatchLastPolled(watchID string) {
	ctx, cancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer cancel()
	if err := s.store.UpdateIssueWatchLastPolled(ctx, watchID, time.Now().UTC()); err != nil {
		s.log.Warn("sentry: update last_polled_at failed",
			zap.String("watch_id", watchID), zap.Error(err))
	}
}

// ReserveIssueWatchTask exposes the dedup store API to the orchestrator's
// WatcherSource implementation.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID, shortID, issueURL string) (bool, error) {
	return s.store.ReserveIssueWatchTask(ctx, watchID, shortID, issueURL)
}

// AssignIssueWatchTaskID exposes the dedup store API to the orchestrator's
// WatcherSource implementation.
func (s *Service) AssignIssueWatchTaskID(ctx context.Context, watchID, shortID, taskID string) error {
	return s.store.AssignIssueWatchTaskID(ctx, watchID, shortID, taskID)
}

// ReleaseIssueWatchTask exposes the dedup store API to the orchestrator's
// WatcherSource implementation.
func (s *Service) ReleaseIssueWatchTask(ctx context.Context, watchID, shortID string) error {
	return s.store.ReleaseIssueWatchTask(ctx, watchID, shortID)
}

// publishNewSentryIssueEvent emits the orchestrator-facing event for one
// freshly-observed issue. No-op when the event bus is not wired (tests, early
// boot).
func (s *Service) publishNewSentryIssueEvent(ctx context.Context, w *IssueWatch, issue *SentryIssue) {
	s.mu.Lock()
	eb := s.eventBus
	s.mu.Unlock()
	if eb == nil {
		return
	}
	evt := bus.NewEvent(events.SentryNewIssue, "sentry", &NewSentryIssueEvent{
		IssueWatchID:      w.ID,
		WorkspaceID:       w.WorkspaceID,
		WorkflowID:        w.WorkflowID,
		WorkflowStepID:    w.WorkflowStepID,
		AgentProfileID:    w.AgentProfileID,
		ExecutorProfileID: w.ExecutorProfileID,
		Prompt:            w.Prompt,
		MaxInflightTasks:  w.MaxInflightTasks,
		Issue:             issue,
	})
	if err := eb.Publish(ctx, events.SentryNewIssue, evt); err != nil {
		s.log.Warn("sentry: publish new issue event failed",
			zap.String("watch_id", w.ID), zap.String("short_id", issue.ShortID), zap.Error(err))
	}
}

func validateIssueWatchCreate(req *CreateIssueWatchRequest) error {
	if req.WorkspaceID == "" {
		return fmt.Errorf("%w: workspaceId required", ErrInvalidConfig)
	}
	if req.WorkflowID == "" || req.WorkflowStepID == "" {
		return fmt.Errorf("%w: workflowId and workflowStepId required", ErrInvalidConfig)
	}
	nf := normalizeFilter(req.Filter)
	if err := validateFilter(nf); err != nil {
		return err
	}
	if err := validateFilterStatuses(nf); err != nil {
		return err
	}
	if err := validateMaxInflightTasks(req.MaxInflightTasks); err != nil {
		return err
	}
	if req.PollIntervalSeconds != 0 {
		if err := validatePollInterval(req.PollIntervalSeconds); err != nil {
			return err
		}
	}
	return nil
}

// validateMaxInflightTasks rejects non-positive caps. A nil pointer is
// "uncapped" and explicitly allowed.
func validateMaxInflightTasks(v *int) error {
	if v == nil {
		return nil
	}
	if *v <= 0 {
		return fmt.Errorf("%w: maxInflightTasks must be a positive integer", ErrInvalidConfig)
	}
	return nil
}

// validateFilter requires the minimum identity for a Sentry search: an org and
// a project. Other fields (environment, levels, statuses, query, statsPeriod)
// are optional.
func validateFilter(f SearchFilter) error {
	if f.OrgSlug == "" {
		return fmt.Errorf("%w: filter.orgSlug is required", ErrInvalidConfig)
	}
	if f.ProjectSlug == "" {
		return fmt.Errorf("%w: filter.projectSlug is required", ErrInvalidConfig)
	}
	return nil
}

// validateFilterStatuses rejects a filter that carries more than one status.
// Sentry has no OR form for the `is:` keyword (unlike levels, which use the
// `level:[...]` bracket syntax), so two `is:` tokens would AND-combine into a
// query that silently matches nothing. Applied when a filter is created or
// changed — never against an unchanged stored filter, so a legacy multi-status
// watch can still be paused/resumed without first being edited.
func validateFilterStatuses(f SearchFilter) error {
	if len(f.Statuses) > 1 {
		return fmt.Errorf("%w: filter.statuses must contain at most one status because Sentry has no OR form for the is keyword", ErrInvalidConfig)
	}
	return nil
}

func validatePollInterval(seconds int) error {
	if seconds < MinIssueWatchPollInterval || seconds > MaxIssueWatchPollInterval {
		return fmt.Errorf("%w: pollIntervalSeconds must be between %d and %d",
			ErrInvalidConfig, MinIssueWatchPollInterval, MaxIssueWatchPollInterval)
	}
	return nil
}

// normalizeFilter trims string fields and drops empty list entries so a filter
// that looks empty after normalization fails the minimum-identity check
// instead of slipping through with whitespace.
func normalizeFilter(f SearchFilter) SearchFilter {
	out := SearchFilter{
		OrgSlug:     strings.TrimSpace(f.OrgSlug),
		ProjectSlug: strings.TrimSpace(f.ProjectSlug),
		Environment: strings.TrimSpace(f.Environment),
		Query:       strings.TrimSpace(f.Query),
		StatsPeriod: strings.TrimSpace(f.StatsPeriod),
	}
	for _, v := range f.Levels {
		v = strings.TrimSpace(v)
		if v != "" {
			out.Levels = append(out.Levels, v)
		}
	}
	for _, v := range f.Statuses {
		v = strings.TrimSpace(v)
		if v != "" {
			out.Statuses = append(out.Statuses, v)
		}
	}
	return out
}

func applyIssueWatchPatch(w *IssueWatch, req *UpdateIssueWatchRequest) {
	if req.WorkflowID != nil {
		w.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		w.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Filter != nil {
		w.Filter = normalizeFilter(*req.Filter)
	}
	if req.AgentProfileID != nil {
		w.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		w.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		w.Prompt = *req.Prompt
	}
	if req.Enabled != nil {
		w.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		w.PollIntervalSeconds = *req.PollIntervalSeconds
	}
	// MaxInflightTasks is tri-state (optional.Int): only apply it when the
	// field was present in the payload. Absent leaves the cap unchanged; null
	// clears it to uncapped; a value sets the cap.
	if req.MaxInflightTasks.Present {
		w.MaxInflightTasks = req.MaxInflightTasks.Value
	}
}
