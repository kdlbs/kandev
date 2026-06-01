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
	if w.WorkflowID == "" || w.WorkflowStepID == "" {
		return nil, fmt.Errorf("%w: workflowId and workflowStepId cannot be empty", ErrInvalidConfig)
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
	res, err := client.SearchIssues(ctx, s.resolveWatchFilter(ctx, w.Filter), "")
	if err != nil {
		return nil, err
	}
	seen, err := s.store.ListSeenIssueShortIDs(ctx, w.ID)
	if err != nil {
		s.log.Warn("sentry: dedup set fetch failed",
			zap.String("watch_id", w.ID), zap.Error(err))
		seen = nil
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
	// The filter's org/project are intentionally not required: an empty value
	// means "use the install-wide default" (resolved at poll time by
	// resolveWatchFilter), mirroring the "(use step default)" profile options.
	if req.PollIntervalSeconds != 0 {
		if err := validatePollInterval(req.PollIntervalSeconds); err != nil {
			return err
		}
	}
	return nil
}

// resolveWatchFilter fills an empty org/project on the watch filter from the
// install-wide Sentry config defaults. A watch saved with "use default" (empty
// org and/or project) therefore follows whatever the integration is configured
// to use, resolved fresh on every poll — the same idea as the "(use step
// default)" agent/executor profile options. A concrete value on the filter
// always wins; the config is only read when something is missing.
func (s *Service) resolveWatchFilter(ctx context.Context, f SearchFilter) SearchFilter {
	if f.OrgSlug != "" && f.ProjectSlug != "" {
		return f
	}
	cfg, err := s.store.GetConfig(ctx)
	if err != nil || cfg == nil {
		return f
	}
	if f.OrgSlug == "" {
		f.OrgSlug = cfg.DefaultOrgSlug
	}
	if f.ProjectSlug == "" {
		f.ProjectSlug = cfg.DefaultProjectSlug
	}
	return f
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
}
