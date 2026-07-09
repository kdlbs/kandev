package linear

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/watchreset"
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
		Filter:              normalizeFilter(req.Filter),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		PollIntervalSeconds: req.PollIntervalSeconds,
		MaxInflightTasks:    req.MaxInflightTasks,
		SortBy:              req.SortBy,
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
	prevRepositoryID, prevBaseBranch := w.RepositoryID, w.BaseBranch
	applyIssueWatchPatch(w, req)
	if filterIsEmpty(w.Filter) {
		return nil, fmt.Errorf("%w: filter must specify at least one of query, teamKey, stateIds, assigned, priorities, labelIds, creatorId, or estimate range", ErrInvalidConfig)
	}
	if err := validateFilterBounds(w.Filter); err != nil {
		return nil, err
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
	if err := validateSortBy(w.SortBy); err != nil {
		return nil, err
	}
	// Only validate/resolve the binding when its value actually changed. The
	// dialog re-sends repositoryId/baseBranch on every PATCH, and an unchanged
	// binding whose repo was since soft-deleted must not block edits to other
	// fields (prompt, filter, …).
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

// DeleteIssueWatch removes the watch and its dedup rows. Idempotent.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	return s.store.DeleteIssueWatch(ctx, id)
}

// linearIssueWatchResetter is the watchreset.Resetter adapter for a single
// Linear issue watch. Closes over the store and watch ID so the shared
// watchreset.Run helper stays integration-agnostic.
type linearIssueWatchResetter struct {
	store   *Store
	watchID string
}

func (r *linearIssueWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListIssueWatchTaskIDs(ctx, r.watchID)
}

func (r *linearIssueWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetIssueWatchState(ctx, r.watchID)
}

// PreviewResetIssueWatch returns how many tasks ResetIssueWatch would
// cascade-delete. Used by the frontend to populate the confirmation dialog.
func (s *Service) PreviewResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	return watchreset.Preview(ctx, &linearIssueWatchResetter{store: s.store, watchID: watchID})
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
		return 0, errors.New("linear: task deleter not wired; reset unavailable")
	}
	res, err := watchreset.Run(ctx,
		&linearIssueWatchResetter{store: s.store, watchID: watchID},
		td, s.log)
	return res.TasksDeleted, err
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
	client, err := s.clientFor(ctx, w.WorkspaceID)
	if err != nil {
		return nil, err
	}
	seen, err := s.store.ListSeenIssueIdentifiers(ctx, w.ID)
	if err != nil {
		s.log.Warn("linear: dedup set fetch failed",
			zap.String("watch_id", w.ID), zap.Error(err))
		seen = nil
	}
	// Page through matches (bounded by maxPages) so the sort below ranks the
	// whole unseen backlog. Linear only orders by created/updated, so priority
	// ordering must be done here over the full fetched set. Default order is a
	// no-op for sortIssues, so paginating a default watch buys nothing but
	// extra Linear calls and larger dispatch bursts — keep it on the legacy
	// single-page fetch.
	maxPages := issueWatchMaxPages
	if w.SortBy == SortByDefault {
		maxPages = 1
	}
	out := make([]*LinearIssue, 0, issueWatchSearchPageSize)
	pageToken := ""
	for page := 0; page < maxPages; page++ {
		res, err := client.SearchIssues(ctx, w.Filter, pageToken, issueWatchSearchPageSize)
		if err != nil {
			if page == 0 {
				return nil, err
			}
			// A canceled/expired context means the poll is being torn down.
			// Abort instead of publishing issues from an incomplete fetch —
			// dispatch detaches the context, so partial results would still
			// create tasks during shutdown or a canceled manual trigger.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			// Any other later-page error is non-fatal: rank/dispatch what we
			// have; the next poll retries from the top.
			s.log.Debug("linear: issue watch pagination stopped early",
				zap.String("watch_id", w.ID), zap.Int("page", page), zap.Error(err))
			break
		}
		for i := range res.Issues {
			issue := res.Issues[i]
			if _, ok := seen[issue.Identifier]; ok {
				continue
			}
			out = append(out, &issue)
		}
		if res.IsLast || res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	// Order dispatch so the most important issues win the in-flight slots.
	sortIssues(out, w.SortBy)
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
		RepositoryID:      w.RepositoryID,
		BaseBranch:        w.BaseBranch,
		AgentProfileID:    w.AgentProfileID,
		ExecutorProfileID: w.ExecutorProfileID,
		Prompt:            w.Prompt,
		MaxInflightTasks:  w.MaxInflightTasks,
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

// issueWatchMaxPages bounds how many pages CheckIssueWatch scans per poll.
// Combined with issueWatchSearchPageSize this caps per-tick cost at
// issueWatchMaxPages*issueWatchSearchPageSize issues while letting the sort
// rank enough of the backlog that high-priority issues on later pages can
// still win the in-flight slots. A caught-up watch fetches a single page.
const issueWatchMaxPages = 5

// MinIssueWatchPollInterval / MaxIssueWatchPollInterval bound the per-watch
// search re-run cadence.
const (
	MinIssueWatchPollInterval = 60
	MaxIssueWatchPollInterval = 3600
)

// resolveRepositoryBinding validates the watch's optional repository binding
// against its workspace and fills an empty base branch with the repository's
// default branch. An empty repositoryID clears the binding (and forces an empty
// base branch), preserving the historical repo-less behaviour. When no
// RepositoryLookup is wired (unit tests, early boot), the binding is accepted
// as-is and the default-branch fill is skipped.
func (s *Service) resolveRepositoryBinding(ctx context.Context, workspaceID, repositoryID, baseBranch string) (string, string, error) {
	repositoryID = strings.TrimSpace(repositoryID)
	baseBranch = strings.TrimSpace(baseBranch)
	if repositoryID == "" {
		return "", "", nil
	}
	// Reject a non-empty base branch that isn't a safe git ref before it can be
	// persisted and copied into watcher-created tasks (then fail at worktree
	// launch). Empty defers to the repo's default branch below.
	if baseBranch != "" && !securityutil.IsValidBaseBranchRef(baseBranch) {
		return "", "", fmt.Errorf("%w: base branch %q is not a valid git ref", ErrInvalidConfig, baseBranch)
	}
	rl := s.getRepositoryLookup()
	if rl == nil {
		return repositoryID, baseBranch, nil
	}
	repoWorkspace, defaultBranch, ok := rl.GetRepository(ctx, repositoryID)
	if !ok {
		return "", "", fmt.Errorf("%w: repository %q not found", ErrInvalidConfig, repositoryID)
	}
	if repoWorkspace != workspaceID {
		return "", "", fmt.Errorf("%w: repository %q does not belong to this workspace", ErrInvalidConfig, repositoryID)
	}
	if baseBranch == "" {
		baseBranch = defaultBranch
	}
	return repositoryID, baseBranch, nil
}

func validateIssueWatchCreate(req *CreateIssueWatchRequest) error {
	if req.WorkspaceID == "" {
		return fmt.Errorf("%w: workspaceId required", ErrInvalidConfig)
	}
	if req.WorkflowID == "" || req.WorkflowStepID == "" {
		return fmt.Errorf("%w: workflowId and workflowStepId required", ErrInvalidConfig)
	}
	if filterIsEmpty(normalizeFilter(req.Filter)) {
		return fmt.Errorf("%w: filter must specify at least one of query, teamKey, stateIds, assigned, priorities, labelIds, creatorId, or estimate range", ErrInvalidConfig)
	}
	if err := validateFilterBounds(req.Filter); err != nil {
		return err
	}
	if req.PollIntervalSeconds != 0 {
		if err := validatePollInterval(req.PollIntervalSeconds); err != nil {
			return err
		}
	}
	if err := validateMaxInflightTasks(req.MaxInflightTasks); err != nil {
		return err
	}
	if err := validateSortBy(req.SortBy); err != nil {
		return err
	}
	return nil
}

// validateSortBy rejects sort keys outside the known wire set. The empty value
// (Linear default order) is allowed.
func validateSortBy(v IssueSortBy) error {
	switch v {
	case SortByDefault, SortByPriorityDesc, SortByPriorityAsc,
		SortByCreatedDesc, SortByCreatedAsc, SortByUpdatedDesc, SortByUpdatedAsc:
		return nil
	}
	return fmt.Errorf("%w: invalid sortBy %q", ErrInvalidConfig, v)
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

// validateFilterBounds rejects out-of-range values for fields where the wire
// type permits invalid values (e.g. Priority being an unconstrained int).
// Empty / unset fields pass without check.
func validateFilterBounds(f SearchFilter) error {
	for _, p := range f.Priorities {
		if p < 0 || p > 4 {
			return fmt.Errorf("%w: priority must be between 0 and 4", ErrInvalidConfig)
		}
	}
	if err := validateEstimateBound(f.EstimateMin, "estimateMin"); err != nil {
		return err
	}
	if err := validateEstimateBound(f.EstimateMax, "estimateMax"); err != nil {
		return err
	}
	if f.EstimateMin != nil && f.EstimateMax != nil && *f.EstimateMin > *f.EstimateMax {
		return fmt.Errorf("%w: estimateMin cannot be greater than estimateMax", ErrInvalidConfig)
	}
	return nil
}

// validateEstimateBound rejects NaN, ±Inf, and negative values. json.Marshal
// fails on NaN/Inf, so without this check a malformed config would surface as a
// 500 at watch-time instead of a clean validation error at create-time.
func validateEstimateBound(v *float64, name string) error {
	if v == nil {
		return nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
		return fmt.Errorf("%w: %s must be a non-negative number", ErrInvalidConfig, name)
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
		Query:       strings.TrimSpace(f.Query),
		TeamKey:     strings.TrimSpace(f.TeamKey),
		Assigned:    strings.TrimSpace(f.Assigned),
		CreatorID:   strings.TrimSpace(f.CreatorID),
		EstimateMin: f.EstimateMin,
		EstimateMax: f.EstimateMax,
	}
	for _, id := range f.StateIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			out.StateIDs = append(out.StateIDs, id)
		}
	}
	for _, id := range f.LabelIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			out.LabelIDs = append(out.LabelIDs, id)
		}
	}
	seenPriority := map[int]bool{}
	for _, p := range f.Priorities {
		if seenPriority[p] {
			continue
		}
		seenPriority[p] = true
		out.Priorities = append(out.Priorities, p)
	}
	return out
}

func filterIsEmpty(f SearchFilter) bool {
	return f.Query == "" &&
		f.TeamKey == "" &&
		f.Assigned == "" &&
		len(f.StateIDs) == 0 &&
		len(f.Priorities) == 0 &&
		len(f.LabelIDs) == 0 &&
		f.CreatorID == "" &&
		f.EstimateMin == nil &&
		f.EstimateMax == nil
}

func applyIssueWatchPatch(w *IssueWatch, req *UpdateIssueWatchRequest) {
	if req.WorkflowID != nil {
		w.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		w.WorkflowStepID = *req.WorkflowStepID
	}
	// RepositoryID / BaseBranch are applied here; UpdateIssueWatch then runs them
	// through resolveRepositoryBinding (workspace check + default-branch fill, or
	// clear when empty). An empty RepositoryID unbinds the watch. Switching to a
	// different repository without an explicit base branch resets the branch so
	// the new repo's default is used instead of carrying the old repo's branch.
	if req.RepositoryID != nil {
		if *req.RepositoryID != w.RepositoryID && req.BaseBranch == nil {
			w.BaseBranch = ""
		}
		w.RepositoryID = *req.RepositoryID
	}
	if req.BaseBranch != nil {
		w.BaseBranch = *req.BaseBranch
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
	// PATCH actually carried the key, so a partial update that omits it
	// leaves the cap intact. Present+null clears the cap; present+int sets it.
	if req.MaxInflightTasks.Present {
		w.MaxInflightTasks = req.MaxInflightTasks.Value
	}
	if req.SortBy != nil {
		w.SortBy = *req.SortBy
	}
}
