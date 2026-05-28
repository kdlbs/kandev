package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

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
	if err := s.requireStore().CreateReviewWatch(ctx, rw); err != nil {
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
	return s.requireStore().GetReviewWatch(ctx, id)
}

// ListReviewWatches lists review watches in a workspace.
func (s *Service) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	return s.requireStore().ListReviewWatches(ctx, workspaceID)
}

// ListAllReviewWatches returns every review watch.
func (s *Service) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	return s.requireStore().ListAllReviewWatches(ctx)
}

// UpdateReviewWatch applies a partial update to a review watch.
func (s *Service) UpdateReviewWatch(ctx context.Context, id string, req *UpdateReviewWatchRequest) error {
	rw, err := s.requireStore().GetReviewWatch(ctx, id)
	if err != nil {
		return err
	}
	if rw == nil {
		return fmt.Errorf("review watch not found: %s", id)
	}
	applyReviewWatchPatch(rw, req)
	if req.CleanupPolicy != nil && !IsValidCleanupPolicy(*req.CleanupPolicy) {
		return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
	}
	return s.requireStore().UpdateReviewWatch(ctx, rw)
}

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
		rw.PollIntervalSeconds = *req.PollIntervalSeconds
	}
	if req.CleanupPolicy != nil {
		rw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
}

// DeleteReviewWatch removes a review watch and best-effort reaps any tasks
// it owned (tasks survive when the dedup row dies, so pre-sweep first).
func (s *Service) DeleteReviewWatch(ctx context.Context, id string) error {
	store := s.requireStore()
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
	filter := watch.CustomQuery
	if filter == "" {
		filter = "reviewer_username=" + url.QueryEscape(username)
	}
	// Project filter: when projects are specified, narrow each result;
	// otherwise the API returns all MRs the user can review.
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
		return nil, fmt.Errorf("review watch not found: %s", id)
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
	watches, err := s.requireStore().ListEnabledReviewWatches(ctx)
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

// --- Issue Watch ---

// CreateIssueWatch persists a new issue watch.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
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
	iw := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Projects:            normalizeProjectFilters(req.Projects),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		Labels:              req.Labels,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: interval,
		CleanupPolicy:       NormalizeCleanupPolicy(req.CleanupPolicy),
	}
	if err := s.requireStore().CreateIssueWatch(ctx, iw); err != nil {
		return nil, fmt.Errorf("create issue watch: %w", err)
	}
	go s.initialIssueCheck(context.Background(), iw)
	return iw, nil
}

func (s *Service) initialIssueCheck(ctx context.Context, watch *IssueWatch) {
	issues, err := s.CheckIssueWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial gitlab issue check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, issue := range issues {
		s.publishNewIssueEvent(ctx, watch, issue)
	}
}

// GetIssueWatch returns an issue watch by id.
func (s *Service) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	return s.requireStore().GetIssueWatch(ctx, id)
}

// ListIssueWatches lists issue watches in a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	return s.requireStore().ListIssueWatches(ctx, workspaceID)
}

// ListAllIssueWatches returns every issue watch.
func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.requireStore().ListAllIssueWatches(ctx)
}

// UpdateIssueWatch applies a partial update.
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) error {
	iw, err := s.requireStore().GetIssueWatch(ctx, id)
	if err != nil {
		return err
	}
	if iw == nil {
		return fmt.Errorf("issue watch not found: %s", id)
	}
	applyIssueWatchPatch(iw, req)
	if req.CleanupPolicy != nil && !IsValidCleanupPolicy(*req.CleanupPolicy) {
		return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
	}
	return s.requireStore().UpdateIssueWatch(ctx, iw)
}

func applyIssueWatchPatch(iw *IssueWatch, req *UpdateIssueWatchRequest) {
	if req.WorkflowID != nil {
		iw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		iw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Projects != nil {
		iw.Projects = normalizeProjectFilters(*req.Projects)
	}
	if req.AgentProfileID != nil {
		iw.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		iw.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		iw.Prompt = *req.Prompt
	}
	if req.Labels != nil {
		iw.Labels = *req.Labels
	}
	if req.CustomQuery != nil {
		iw.CustomQuery = *req.CustomQuery
	}
	if req.Enabled != nil {
		iw.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		iw.PollIntervalSeconds = *req.PollIntervalSeconds
	}
	if req.CleanupPolicy != nil {
		iw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
}

// DeleteIssueWatch removes an issue watch and best-effort reaps tasks.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	store := s.requireStore()
	s.mu.RLock()
	deleter := s.taskDeleter
	s.mu.RUnlock()
	if deleter != nil {
		tasks, err := store.ListIssueWatchTasksByWatch(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list issue tasks for pre-delete sweep",
				zap.String("watch_id", id), zap.Error(err))
		} else {
			s.sweepIssueWatchTasksOnDelete(ctx, id, tasks, deleter)
		}
	}
	return store.DeleteIssueWatch(ctx, id)
}

func (s *Service) sweepIssueWatchTasksOnDelete(ctx context.Context, watchID string, tasks []*IssueWatchTask, deleter TaskDeleter) {
	for _, t := range tasks {
		if t.TaskID == "" {
			continue
		}
		if err := deleter.DeleteTask(ctx, t.TaskID); err != nil {
			s.logger.Warn("failed to delete issue task during watch cleanup",
				zap.String("watch_id", watchID),
				zap.String("task_id", t.TaskID),
				zap.Error(err))
		}
	}
}

// CheckIssueWatch polls a single watch and returns new issues.
//
//nolint:dupl // see CheckReviewWatch — same shape, different domain types.
func (s *Service) CheckIssueWatch(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
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
	issues, err := s.fetchIssues(ctx, watch)
	if err != nil {
		return nil, err
	}
	out := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		exists, err := store.HasIssueWatchTask(ctx, watch.ID, issue.ProjectPath, issue.IID)
		if err != nil {
			s.logger.Error("check issue watch dedup", zap.Error(err))
			continue
		}
		if !exists {
			out = append(out, issue)
		}
	}
	now := time.Now().UTC()
	if err := store.RecordIssueWatchPoll(ctx, watch.ID, now); err != nil {
		s.logger.Warn("record issue watch poll", zap.String("watch_id", watch.ID), zap.Error(err))
	}
	return out, nil
}

func (s *Service) fetchIssues(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	client := s.Client()
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gitlab username: %w", err)
	}
	// When a custom_query is set, labels need to be folded into it (the
	// client's buildIssueSearchQuery returns customQuery verbatim and
	// ignores the auxiliary `filter` arg). Otherwise build a default
	// "assigned to me" filter and append labels there.
	filter := ""
	customQuery := watch.CustomQuery
	switch {
	case customQuery != "":
		if len(watch.Labels) > 0 {
			customQuery = appendLabelsToQuery(customQuery, watch.Labels)
		}
	default:
		filter = "assignee_username=" + url.QueryEscape(username)
		if len(watch.Labels) > 0 {
			filter += "&labels=" + url.QueryEscape(strings.Join(watch.Labels, ","))
		}
	}
	issues, err := client.ListIssues(ctx, filter, customQuery)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	if len(watch.Projects) == 0 {
		return issues, nil
	}
	allowed := make(map[string]bool, len(watch.Projects))
	for _, p := range watch.Projects {
		allowed[strings.ToLower(strings.TrimSpace(p.Path))] = true
	}
	out := issues[:0]
	for _, i := range issues {
		if allowed[strings.ToLower(i.ProjectPath)] {
			out = append(out, i)
		}
	}
	return out, nil
}

// TriggerIssueWatch runs the watch once on demand.
func (s *Service) TriggerIssueWatch(ctx context.Context, id string) ([]*Issue, error) {
	iw, err := s.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if iw == nil {
		return nil, fmt.Errorf("issue watch not found: %s", id)
	}
	found, err := s.CheckIssueWatch(ctx, iw)
	if err != nil {
		return nil, err
	}
	for _, issue := range found {
		s.publishNewIssueEvent(ctx, iw, issue)
	}
	return found, nil
}

// TriggerIssueWatchAll runs every enabled watch.
func (s *Service) TriggerIssueWatchAll(ctx context.Context) (int, error) {
	watches, err := s.requireStore().ListEnabledIssueWatches(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, iw := range watches {
		found, err := s.CheckIssueWatch(ctx, iw)
		if err != nil {
			s.logger.Warn("trigger issue watch all", zap.String("watch_id", iw.ID), zap.Error(err))
			continue
		}
		for _, issue := range found {
			s.publishNewIssueEvent(ctx, iw, issue)
		}
		total += len(found)
	}
	return total, nil
}

// --- Reservation handles (used by orchestrator-side task creation) ---

// ReserveReviewMRTask atomically claims an MR for task creation.
func (s *Service) ReserveReviewMRTask(ctx context.Context, watchID, projectPath string, iid int, mrURL string) (bool, error) {
	return s.requireStore().ReserveReviewMRTask(ctx, watchID, projectPath, iid, mrURL)
}

// AssignReviewMRTaskID stamps the claim with the freshly-created task id.
func (s *Service) AssignReviewMRTaskID(ctx context.Context, watchID, projectPath string, iid int, taskID string) error {
	return s.requireStore().AssignReviewMRTaskID(ctx, watchID, projectPath, iid, taskID)
}

// ReleaseReviewMRTask undoes a reservation (used after task-create failure).
func (s *Service) ReleaseReviewMRTask(ctx context.Context, watchID, projectPath string, iid int) error {
	return s.requireStore().ReleaseReviewMRTask(ctx, watchID, projectPath, iid)
}

// ReserveIssueWatchTask atomically claims an issue for task creation.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID, projectPath string, iid int, issueURL string) (bool, error) {
	return s.requireStore().ReserveIssueWatchTask(ctx, watchID, projectPath, iid, issueURL)
}

// AssignIssueWatchTaskID stamps the claim with the freshly-created task id.
func (s *Service) AssignIssueWatchTaskID(ctx context.Context, watchID, projectPath string, iid int, taskID string) error {
	return s.requireStore().AssignIssueWatchTaskID(ctx, watchID, projectPath, iid, taskID)
}

// ReleaseIssueWatchTask undoes a reservation.
func (s *Service) ReleaseIssueWatchTask(ctx context.Context, watchID, projectPath string, iid int) error {
	return s.requireStore().ReleaseIssueWatchTask(ctx, watchID, projectPath, iid)
}

// --- Event publishing ---

func (s *Service) publishMRFeedbackEvent(ctx context.Context, watch *MRWatch, status *MRStatus) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	ev := &MRFeedbackEvent{
		SessionID:        watch.SessionID,
		TaskID:           watch.TaskID,
		ProjectPath:      watch.ProjectPath,
		MRIID:            watch.MRIID,
		NewPipelineState: status.PipelineState,
		NewApprovalState: status.ApprovalState,
	}
	if err := eb.Publish(ctx, events.GitLabMRFeedback, bus.NewEvent(events.GitLabMRFeedback, eventSource, ev)); err != nil {
		s.logger.Warn("publish gitlab feedback event", zap.Error(err))
	}
}

func (s *Service) publishNewReviewMREvent(ctx context.Context, watch *ReviewWatch, mr *MR) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	ev := &NewReviewMREvent{
		ReviewWatchID:     watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		MR:                mr,
	}
	if err := eb.Publish(ctx, events.GitLabNewReviewMR, bus.NewEvent(events.GitLabNewReviewMR, eventSource, ev)); err != nil {
		s.logger.Warn("publish gitlab new review MR event", zap.Error(err))
	}
}

func (s *Service) publishNewIssueEvent(ctx context.Context, watch *IssueWatch, issue *Issue) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	ev := &NewIssueEvent{
		IssueWatchID:      watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		Issue:             issue,
	}
	if err := eb.Publish(ctx, events.GitLabNewIssue, bus.NewEvent(events.GitLabNewIssue, eventSource, ev)); err != nil {
		s.logger.Warn("publish gitlab new issue event", zap.Error(err))
	}
}

func (s *Service) publishWatchEvent(ctx context.Context, kind, id, sessionID, taskID string) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	_ = eb.Publish(ctx, events.GitLabWatchEvent, bus.NewEvent(events.GitLabWatchEvent, eventSource, map[string]string{
		"kind":       kind,
		"id":         id,
		"session_id": sessionID,
		"task_id":    taskID,
	}))
}

// --- Helpers ---

func (s *Service) requireStore() *Store {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	return store
}

// appendLabelsToQuery merges a label list into an existing customQuery string.
// If the query already contains `labels=`, it is left alone (the user is
// explicitly controlling the filter and we don't want to silently double-up).
func appendLabelsToQuery(customQuery string, labels []string) string {
	if strings.Contains(customQuery, "labels=") {
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
