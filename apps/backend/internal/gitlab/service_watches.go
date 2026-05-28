package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// eventSource is the `source` field on every bus.Event published by this
// package; consumed by event_handlers_gitlab.go-style subscribers.
const eventSource = "gitlab"

const (
	defaultWatchPollIntervalSec = 300
	minWatchPollIntervalSec     = 60
)

// --- MR Watch ---

// CreateMRWatch records a session→MR watch row. The poller (and topbar) use it
// to discover a freshly-pushed MR off the agent's source branch.
func (s *Service) CreateMRWatch(ctx context.Context, sessionID, taskID, repositoryID, projectPath string, iid int, branch string) (*MRWatch, error) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return nil, fmt.Errorf("gitlab store not configured")
	}
	w := &MRWatch{
		SessionID:    sessionID,
		TaskID:       taskID,
		RepositoryID: repositoryID,
		ProjectPath:  projectPath,
		MRIID:        iid,
		Branch:       branch,
	}
	if err := store.CreateMRWatch(ctx, w); err != nil {
		return nil, fmt.Errorf("create MR watch: %w", err)
	}
	s.publishWatchEvent(ctx, "mr_watch_created", w.ID, sessionID, taskID)
	return w, nil
}

// GetMRWatchBySession fetches the legacy single-repo watch.
func (s *Service) GetMRWatchBySession(ctx context.Context, sessionID string) (*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetMRWatchBySession(ctx, sessionID)
}

// GetMRWatchBySessionAndRepo fetches a watch keyed by (session, repo).
func (s *Service) GetMRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetMRWatchBySessionAndRepo(ctx, sessionID, repositoryID)
}

// ListMRWatchesBySession lists every MR watch on a session.
func (s *Service) ListMRWatchesBySession(ctx context.Context, sessionID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListMRWatchesBySession(ctx, sessionID)
}

// ListMRWatchesByTask lists every MR watch on a task.
func (s *Service) ListMRWatchesByTask(ctx context.Context, taskID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListMRWatchesByTask(ctx, taskID)
}

// ListActiveMRWatches returns every MR watch (used by the poller).
func (s *Service) ListActiveMRWatches(ctx context.Context) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListActiveMRWatches(ctx)
}

// DeleteMRWatch removes a single MR watch.
func (s *Service) DeleteMRWatch(ctx context.Context, id string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.DeleteMRWatch(ctx, id)
}

// errStoreUnavailable is returned by Service methods that require the
// SQLite store to be wired but the runtime didn't manage to create it
// (table migration failure on boot). Distinct error so callers can render
// a "GitLab unconfigured" UI instead of "500 internal error".
var errStoreUnavailable = fmt.Errorf("gitlab store not configured")

// ErrWatchNotFound is returned by Update/Trigger methods when the watch id
// doesn't exist. Sentinel so the HTTP controller can map it to 404 rather
// than 500.
var ErrWatchNotFound = fmt.Errorf("watch not found")

// CheckMRWatch polls a watch once: returns the latest MR status and whether
// the underlying MR moved into a state worth notifying about (new note,
// pipeline transition, approval transition).
func (s *Service) CheckMRWatch(ctx context.Context, watch *MRWatch) (*MRStatus, bool, error) {
	if watch == nil {
		return nil, false, fmt.Errorf("watch is nil")
	}
	client := s.Client()
	if client == nil {
		return nil, false, ErrNoClient
	}
	store := s.requireStore()
	// If we don't yet know an iid, try to find it from the branch.
	if watch.MRIID <= 0 {
		mr, err := client.FindMRByBranch(ctx, watch.ProjectPath, watch.Branch)
		if err != nil || mr == nil {
			now := time.Now().UTC()
			_ = store.UpdateMRWatchTimestamps(ctx, watch.ID, now, watch.LastNoteAt, watch.LastPipelineState, watch.LastApprovalState)
			return nil, false, err
		}
		if err := store.UpdateMRWatchMRIID(ctx, watch.ID, mr.IID); err != nil {
			return nil, false, fmt.Errorf("update MR watch iid: %w", err)
		}
		watch.MRIID = mr.IID
	}
	status, err := client.GetMRStatus(ctx, watch.ProjectPath, watch.MRIID)
	if err != nil {
		return nil, false, err
	}
	notable := watch.LastPipelineState != status.PipelineState ||
		watch.LastApprovalState != status.ApprovalState
	now := time.Now().UTC()
	if err := store.UpdateMRWatchTimestamps(ctx, watch.ID, now, watch.LastNoteAt, status.PipelineState, status.ApprovalState); err != nil {
		return nil, false, fmt.Errorf("record MR watch poll: %w", err)
	}
	if notable {
		s.publishMRFeedbackEvent(ctx, watch, status)
	}
	return status, notable, nil
}

// --- Review Watch ---

// CreateReviewWatch persists a new review watch.
func (s *Service) CreateReviewWatch(ctx context.Context, req *CreateReviewWatchRequest) (*ReviewWatch, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if !IsValidCleanupPolicy(req.CleanupPolicy) {
		return nil, fmt.Errorf("invalid cleanup_policy: %q", req.CleanupPolicy)
	}
	interval := req.PollIntervalSeconds
	if interval <= 0 {
		interval = defaultWatchPollIntervalSec
	}
	if interval < minWatchPollIntervalSec {
		interval = minWatchPollIntervalSec
	}
	scope := req.ReviewScope
	if scope == "" {
		scope = ReviewScopeUserAndTeams
	}
	rw := &ReviewWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Projects:            normalizeProjectFilters(req.Projects),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		ReviewScope:         scope,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: interval,
		CleanupPolicy:       NormalizeCleanupPolicy(req.CleanupPolicy),
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	if err := store.CreateReviewWatch(ctx, rw); err != nil {
		return nil, fmt.Errorf("create review watch: %w", err)
	}
	go s.initialReviewCheck(context.Background(), rw)
	return rw, nil
}

func (s *Service) initialReviewCheck(ctx context.Context, watch *ReviewWatch) {
	mrs, err := s.CheckReviewWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial gitlab review check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, mr := range mrs {
		s.publishNewReviewMREvent(ctx, watch, mr)
	}
}

// GetReviewWatch returns a review watch by id.
func (s *Service) GetReviewWatch(ctx context.Context, id string) (*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetReviewWatch(ctx, id)
}

// ListReviewWatches lists review watches in a workspace.
func (s *Service) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListReviewWatches(ctx, workspaceID)
}

// ListAllReviewWatches returns every review watch.
func (s *Service) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListAllReviewWatches(ctx)
}

// UpdateReviewWatch applies a partial update to a review watch.
func (s *Service) UpdateReviewWatch(ctx context.Context, id string, req *UpdateReviewWatchRequest) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	rw, err := store.GetReviewWatch(ctx, id)
	if err != nil {
		return err
	}
	if rw == nil {
		return fmt.Errorf("%w: review watch %s", ErrWatchNotFound, id)
	}
	applyReviewWatchPatch(rw, req)
	if req.CleanupPolicy != nil && !IsValidCleanupPolicy(*req.CleanupPolicy) {
		return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
	}
	return store.UpdateReviewWatch(ctx, rw)
}

// the ReviewWatch shape (with ReviewScope instead of Labels). The two are
// kept as separate functions so each domain's fields are explicit; merging
// them via generics would obscure the per-type validation that lives in
// CreateXxxWatch.
//
//nolint:dupl // structurally similar to applyIssueWatchPatch but operates on
func applyReviewWatchPatch(rw *ReviewWatch, req *UpdateReviewWatchRequest) {
	if req.WorkflowID != nil {
		rw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		rw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Projects != nil {
		rw.Projects = normalizeProjectFilters(*req.Projects)
	}
	if req.AgentProfileID != nil {
		rw.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		rw.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		rw.Prompt = *req.Prompt
	}
	if req.ReviewScope != nil {
		rw.ReviewScope = *req.ReviewScope
	}
	if req.CustomQuery != nil {
		rw.CustomQuery = *req.CustomQuery
	}
	if req.Enabled != nil {
		rw.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		rw.PollIntervalSeconds = clampPollInterval(*req.PollIntervalSeconds)
	}
	if req.CleanupPolicy != nil {
		rw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
}

// clampPollInterval enforces the same bounds the create path applies (0 → default,
// below minimum → minimum). Used by both review-watch and issue-watch update paths.
func clampPollInterval(seconds int) int {
	if seconds <= 0 {
		return defaultWatchPollIntervalSec
	}
	if seconds < minWatchPollIntervalSec {
		return minWatchPollIntervalSec
	}
	return seconds
}

// DeleteReviewWatch removes a review watch and best-effort reaps any tasks
// it owned (tasks survive when the dedup row dies, so pre-sweep first).
func (s *Service) DeleteReviewWatch(ctx context.Context, id string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	s.mu.RLock()
	deleter := s.taskDeleter
	s.mu.RUnlock()
	if deleter != nil {
		mrTasks, err := store.ListReviewMRTasksByWatch(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list review MR tasks for pre-delete sweep",
				zap.String("watch_id", id), zap.Error(err))
		} else {
			s.sweepReviewWatchTasks(ctx, id, mrTasks, deleter)
		}
	}
	return store.DeleteReviewWatch(ctx, id)
}

func (s *Service) sweepReviewWatchTasks(ctx context.Context, watchID string, tasks []*ReviewMRTask, deleter TaskDeleter) {
	for _, t := range tasks {
		if t.TaskID == "" {
			continue
		}
		if err := deleter.DeleteTask(ctx, t.TaskID); err != nil {
			s.logger.Warn("failed to delete review task during watch cleanup",
				zap.String("watch_id", watchID),
				zap.String("task_id", t.TaskID),
				zap.Error(err))
		}
	}
}

// CheckReviewWatch polls a single review watch and returns newly observed MRs
// not yet tracked. Dedup happens against gitlab_review_mr_tasks.
//
// different domain type (MR vs Issue); extracting a generic helper would
// require type parameters across the dedup-check + store-poll API which
// gives up more clarity than it saves.
//
//nolint:dupl // structurally similar to CheckIssueWatch but operates on a
func (s *Service) CheckReviewWatch(ctx context.Context, watch *ReviewWatch) ([]*MR, error) {
	if watch == nil {
		return nil, fmt.Errorf("watch is nil")
	}
	if !watch.Enabled {
		return nil, nil
	}
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	mrs, err := s.fetchReviewMRs(ctx, watch)
	if err != nil {
		return nil, err
	}
	out := make([]*MR, 0, len(mrs))
	for _, mr := range mrs {
		exists, err := store.HasReviewMRTask(ctx, watch.ID, mr.ProjectPath, mr.IID)
		if err != nil {
			s.logger.Error("check review MR dedup", zap.Error(err))
			continue
		}
		if !exists {
			out = append(out, mr)
		}
	}
	now := time.Now().UTC()
	if err := store.RecordReviewWatchPoll(ctx, watch.ID, now); err != nil {
		s.logger.Warn("record review watch poll", zap.String("watch_id", watch.ID), zap.Error(err))
	}
	return out, nil
}

func (s *Service) fetchReviewMRs(ctx context.Context, watch *ReviewWatch) ([]*MR, error) {
	client := s.Client()
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gitlab username: %w", err)
	}
	if username == "" {
		return nil, fmt.Errorf("no authenticated gitlab user")
	}
	// SearchMRs's buildMRSearchQuery returns customQuery verbatim when
	// non-empty (ignoring filter), so only build the default filter when
	// the watch has no customQuery to pass through.
	filter := ""
	if watch.CustomQuery == "" {
		filter = "reviewer_username=" + url.QueryEscape(username)
	}
	mrs, err := client.SearchMRs(ctx, filter, watch.CustomQuery)
	if err != nil {
		return nil, fmt.Errorf("search MRs: %w", err)
	}
	if len(watch.Projects) == 0 {
		return mrs, nil
	}
	allowed := make(map[string]bool, len(watch.Projects))
	for _, p := range watch.Projects {
		allowed[strings.ToLower(strings.TrimSpace(p.Path))] = true
	}
	out := mrs[:0]
	for _, mr := range mrs {
		if allowed[strings.ToLower(mr.ProjectPath)] {
			out = append(out, mr)
		}
	}
	return out, nil
}

// TriggerReviewWatch runs the watch once on demand. Returns matching MRs.
func (s *Service) TriggerReviewWatch(ctx context.Context, id string) ([]*MR, error) {
	rw, err := s.GetReviewWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if rw == nil {
		return nil, fmt.Errorf("%w: review watch %s", ErrWatchNotFound, id)
	}
	mrs, err := s.CheckReviewWatch(ctx, rw)
	if err != nil {
		return nil, err
	}
	for _, mr := range mrs {
		s.publishNewReviewMREvent(ctx, rw, mr)
	}
	return mrs, nil
}

// TriggerReviewWatchAll runs every enabled watch and aggregates new MRs.
func (s *Service) TriggerReviewWatchAll(ctx context.Context) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	watches, err := store.ListEnabledReviewWatches(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, rw := range watches {
		found, err := s.CheckReviewWatch(ctx, rw)
		if err != nil {
			s.logger.Warn("trigger review watch all", zap.String("watch_id", rw.ID), zap.Error(err))
			continue
		}
		for _, mr := range found {
			s.publishNewReviewMREvent(ctx, rw, mr)
		}
		total += len(found)
	}
	return total, nil
}

// --- Helpers ---

func (s *Service) requireStore() *Store {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	return store
}

// appendLabelsToQuery merges a label list into an existing customQuery string.
// If the query already has a `labels` key the caller's value is kept (we
// don't want to silently double-up). url.ParseQuery is used for an exact
// key match — strings.Contains("labels=") would false-positive on keys
// like `mylabels=` and silently drop the watch's labels.
func appendLabelsToQuery(customQuery string, labels []string) string {
	if parsed, err := url.ParseQuery(customQuery); err == nil && parsed.Has("labels") {
		return customQuery
	}
	encoded := url.QueryEscape(strings.Join(labels, ","))
	if customQuery == "" {
		return "labels=" + encoded
	}
	return customQuery + "&labels=" + encoded
}

func normalizeProjectFilters(in []ProjectFilter) []ProjectFilter {
	if in == nil {
		return []ProjectFilter{}
	}
	out := make([]ProjectFilter, 0, len(in))
	for _, p := range in {
		p.Path = strings.TrimSpace(p.Path)
		if p.Path == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
