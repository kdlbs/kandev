package linear

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
var ErrIssueWatchNotFound = errors.New("linear: issue watch not found")

// SetEventBus wires the bus used to publish NewLinearIssueEvent. Optional: if
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
	if filterIsEmpty(w.Filter) {
		return nil, fmt.Errorf("%w: filter must specify at least one of query, teamKey, stateIds, or assigned", ErrInvalidConfig)
	}
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
// stolen by a concurrent reserver. We query the seen-set and return unseen
// identifiers, but we do NOT insert the dedup row here — that happens in the
// orchestrator's ReserveIssueWatchTask via INSERT OR IGNORE. If the manual
// /trigger endpoint and the poller tick fire for the same watch in quick
// succession, both calls can see the same identifier as unseen before either
// has reserved it. The duplicate publish is harmless (the second reserver
// loses the race and bails) but the goroutine work is wasted. Same pattern as
// the JIRA watcher.
func (s *Service) CheckIssueWatch(ctx context.Context, w *IssueWatch) ([]*LinearIssue, error) {
	defer s.stampWatchLastPolled(w.ID)
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	res, err := client.SearchIssues(ctx, w.Filter, "", issueWatchSearchPageSize)
	if err != nil {
		return nil, err
	}
	seen, err := s.store.ListSeenIssueIdentifiers(ctx, w.ID)
	if err != nil {
		s.log.Warn("linear: dedup set fetch failed",
			zap.String("watch_id", w.ID), zap.Error(err))
		seen = nil
	}
	out := make([]*LinearIssue, 0, len(res.Issues))
	for i := range res.Issues {
		issue := res.Issues[i]
		if _, ok := seen[issue.Identifier]; ok {
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
		s.log.Warn("linear: update last_polled_at failed",
			zap.String("watch_id", watchID), zap.Error(err))
	}
}

// publishNewLinearIssueEvent emits the orchestrator-facing event for one
// freshly-observed issue. No-op when the event bus is not wired (tests, early
// boot).
func (s *Service) publishNewLinearIssueEvent(ctx context.Context, w *IssueWatch, issue *LinearIssue) {
	s.mu.Lock()
	eb := s.eventBus
	s.mu.Unlock()
	if eb == nil {
		return
	}
	evt := bus.NewEvent(events.LinearNewIssue, "linear", &NewLinearIssueEvent{
		IssueWatchID:      w.ID,
		WorkspaceID:       w.WorkspaceID,
		WorkflowID:        w.WorkflowID,
		WorkflowStepID:    w.WorkflowStepID,
		AgentProfileID:    w.AgentProfileID,
		ExecutorProfileID: w.ExecutorProfileID,
		Prompt:            w.Prompt,
		Issue:             issue,
	})
	if err := eb.Publish(ctx, events.LinearNewIssue, evt); err != nil {
		s.log.Debug("linear: publish new issue event failed",
			zap.String("watch_id", w.ID), zap.String("identifier", issue.Identifier), zap.Error(err))
	}
}

// issueWatchSearchPageSize caps how many issues a single CheckIssueWatch call
// pulls from Linear. Mirrors the Jira watcher's limit so per-tick cost stays
// bounded for very broad filters.
const issueWatchSearchPageSize = 50

// MinIssueWatchPollInterval / MaxIssueWatchPollInterval bound the per-watch
// search re-run cadence.
const (
	MinIssueWatchPollInterval = 60
	MaxIssueWatchPollInterval = 3600
)

func validateIssueWatchCreate(req *CreateIssueWatchRequest) error {
	if req.WorkspaceID == "" {
		return fmt.Errorf("%w: workspaceId required", ErrInvalidConfig)
	}
	if req.WorkflowID == "" || req.WorkflowStepID == "" {
		return fmt.Errorf("%w: workflowId and workflowStepId required", ErrInvalidConfig)
	}
	if filterIsEmpty(normalizeFilter(req.Filter)) {
		return fmt.Errorf("%w: filter must specify at least one of query, teamKey, stateIds, or assigned", ErrInvalidConfig)
	}
	if req.PollIntervalSeconds != 0 {
		if err := validatePollInterval(req.PollIntervalSeconds); err != nil {
			return err
		}
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

// normalizeFilter trims string fields and drops empty stateIds entries, so a
// filter that looks empty after normalization fails the at-least-one check
// instead of slipping through with whitespace.
func normalizeFilter(f SearchFilter) SearchFilter {
	out := SearchFilter{
		Query:    strings.TrimSpace(f.Query),
		TeamKey:  strings.TrimSpace(f.TeamKey),
		Assigned: strings.TrimSpace(f.Assigned),
	}
	for _, id := range f.StateIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			out.StateIDs = append(out.StateIDs, id)
		}
	}
	return out
}

func filterIsEmpty(f SearchFilter) bool {
	return f.Query == "" && f.TeamKey == "" && f.Assigned == "" && len(f.StateIDs) == 0
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
