package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// Workspace-mode and ordering constants used across handoff plumbing.
// Mirror the string values agents send via the MCP tool surface — keep
// in sync with internal/mcp/handlers/workspace_policy.go.
const (
	workspaceModeInheritParent = "inherit_parent"
	workspaceModeSharedGroup   = "shared_group"
	orderingSequential         = "sequential"
)

// WorkspaceGroupRepo is the slim repository surface HandoffService uses
// to record workspace-group membership for office task handoffs. The
// office sqlite Repository (Phase 1) implements this; tests can supply
// fakes.
type WorkspaceGroupRepo interface {
	CreateWorkspaceGroup(ctx context.Context, g *orchmodels.WorkspaceGroup) error
	GetWorkspaceGroup(ctx context.Context, id string) (*orchmodels.WorkspaceGroup, error)
	GetWorkspaceGroupForTask(ctx context.Context, taskID string) (*orchmodels.WorkspaceGroup, error)
	AddWorkspaceGroupMember(ctx context.Context, groupID, taskID, role string) error
	// Phase 6 surface — cascade release / restore + cleanup status updates.
	ReleaseWorkspaceGroupMember(ctx context.Context, groupID, taskID, reason, cascadeID string) error
	RestoreWorkspaceGroupMemberByCascade(ctx context.Context, taskID, cascadeID string) error
	ListActiveWorkspaceGroupMembers(ctx context.Context, groupID string) ([]orchmodels.WorkspaceGroupMember, error)
	// ListWorkspaceGroupMembers returns ALL members (including
	// released ones). Cleanup uses this to scan every task that ever
	// belonged to the group for lingering executor_running rows.
	ListWorkspaceGroupMembers(ctx context.Context, groupID string) ([]orchmodels.WorkspaceGroupMember, error)
	UpdateWorkspaceGroupCleanupStatus(ctx context.Context, id, status, errStr string, cleanedAt *time.Time) error
	// Materializer hook — flips owned_by_kandev / cleanup_policy
	// atomically on a workspace group when the owner's session
	// produces a worktree on disk.
	MarkWorkspaceMaterialized(ctx context.Context, id string, m orchmodels.MaterializedWorkspace) error
	UpdateWorkspaceGroupRestoreStatus(ctx context.Context, id, status, errStr string) error
}

// SessionWorktreeReader is the slim repository surface HandoffService
// uses to discover a task's primary session + worktree details when
// recording materialization state on a workspace group. Implemented by
// the sqlite task repository.
type SessionWorktreeReader interface {
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error)
	GetTask(ctx context.Context, id string) (*models.Task, error)
	// HasExecutorRunningRow tells cleanup whether a session still has
	// an executors_running row — i.e. an agent is (or recently was)
	// bound to the workspace. Cleanup MUST refuse to delete a
	// materialized workspace while any of the group's member sessions
	// is still active, otherwise the agent's writes get destroyed
	// out from under it (post-review #5).
	HasExecutorRunningRow(ctx context.Context, sessionID string) (bool, error)
}

// RunCanceller stops active sessions / runs for a task before a
// cascade archive / delete touches the task row. Implemented by
// orchestrator.Service.CancelTaskExecution. Optional — when nil the
// cascade proceeds without cancellation (legacy / tests).
type RunCanceller interface {
	CancelTaskExecution(ctx context.Context, taskID, reason string, force bool) error
}

// SetRunCanceller wires the run-canceller used by ArchiveTaskTree /
// DeleteTaskTree to terminate active descendants before archive.
func (s *HandoffService) SetRunCanceller(c RunCanceller) {
	s.runCanceller = c
}

// WorkspacePolicy bundles the workspace-mode fields a coordinator
// supplies (or that get inherited from the parent) when creating a new
// office task. AttachWorkspacePolicy is the consumer.
type WorkspacePolicy struct {
	Mode                  string // inherit_parent | new_workspace | shared_group
	GroupID               string // when Mode == shared_group
	DefaultChildWorkspace string // parent-only: applied to children created later
	DefaultChildOrdering  string // parent-only: sequential | parallel
	// ParentOrdering is the ordering policy resolved from the parent
	// task's metadata. Drives whether AttachWorkspacePolicy adds a
	// blocker chain on the new task. Empty means parallel (no chain).
	ParentOrdering string
}

// MetadataBlock returns the metadata map to persist on the new task. The
// "workspace" sub-map captures the effective mode so launch-time logic
// can read it back without re-deriving from parent state. Returns nil
// when no fields are set so callers don't accidentally overwrite a
// task's existing metadata with an empty workspace block.
func (p WorkspacePolicy) MetadataBlock() map[string]interface{} {
	ws := map[string]interface{}{}
	if p.Mode != "" {
		ws["mode"] = p.Mode
	}
	if p.GroupID != "" {
		ws["group_id"] = p.GroupID
	}
	if p.DefaultChildWorkspace != "" {
		ws["default_child_workspace"] = p.DefaultChildWorkspace
	}
	if p.DefaultChildOrdering != "" {
		ws["default_child_ordering"] = p.DefaultChildOrdering
	}
	if len(ws) == 0 {
		return nil
	}
	return map[string]interface{}{"workspace": ws}
}

// NeedsAttachment returns true when AttachWorkspacePolicy has work to do
// (group membership recording or sequential blocker chaining).
func (p WorkspacePolicy) NeedsAttachment() bool {
	if p.Mode == workspaceModeInheritParent || p.Mode == workspaceModeSharedGroup {
		return true
	}
	if p.ParentOrdering == orderingSequential {
		return true
	}
	return false
}

// RelatedTask is the lightweight projection of a task surfaced through the
// list_related_tasks_kandev MCP tool. Document keys are precomputed so an
// agent can decide what to fetch without a follow-up list call.
type RelatedTask struct {
	ID            string   `json:"id"`
	Identifier    string   `json:"identifier,omitempty"`
	Title         string   `json:"title"`
	State         string   `json:"state"`
	WorkspaceID   string   `json:"workspace_id"`
	ParentID      string   `json:"parent_id,omitempty"`
	AssigneeLabel string   `json:"assignee_label,omitempty"`
	DocumentKeys  []string `json:"document_keys,omitempty"`
}

// RelatedTasks bundles every relation surface for a single task.
type RelatedTasks struct {
	Task      RelatedTask    `json:"task"`
	Parent    *RelatedTask   `json:"parent,omitempty"`
	Children  []*RelatedTask `json:"children"`
	Siblings  []*RelatedTask `json:"siblings"`
	Blockers  []*RelatedTask `json:"blockers"`
	BlockedBy []*RelatedTask `json:"blocked_by"`
}

// HandoffService implements the cross-task context surface (related-task
// listing + document access guards) used by the new MCP tools introduced
// for office task handoffs. It deliberately wraps existing services rather
// than reaching into the repos directly so document writes still go
// through DocumentService and emit the same revision/event side effects.
type HandoffService struct {
	tasks          repository.TaskRepository
	docs           *DocumentService
	docsRepo       repository.DocumentRepository
	blockers       BlockerRepository
	wsGroups       WorkspaceGroupRepo
	sessions       SessionWorktreeReader
	cleaner        WorkspaceCleaner
	runCanceller   RunCanceller
	eventPublisher TaskEventPublisher
	logger         *logger.Logger
	parentLock     parentMutex
}

// TaskEventPublisher abstracts the side-effect of broadcasting task
// lifecycle events. The cascade paths (ArchiveTaskTree, DeleteTaskTree)
// bypass Service.ArchiveTask / Service.DeleteTask — which means they
// also bypass the publishTaskEvent call those wrappers perform. Without
// re-publishing here, the gateway never forwards task.updated /
// task.deleted to WebSocket clients and the kanban board does not
// reflect cascade-archive or cascade-delete operations until a reload.
//
// Implemented by *Service via PublishTaskUpdated / PublishTaskDeleted.
// Optional — when nil the cascade still completes, it just won't emit
// WS events (matches the pre-handoff fallback in the HTTP handler).
type TaskEventPublisher interface {
	PublishTaskUpdated(ctx context.Context, task *models.Task)
	PublishTaskDeleted(ctx context.Context, task *models.Task)
}

// SetSessionReader wires the session/worktree lookup used by the
// materializer hook (MarkOwnerSessionMaterialized). Optional — when
// nil the materializer is a no-op so legacy / test wiring still works.
func (s *HandoffService) SetSessionReader(r SessionWorktreeReader) {
	s.sessions = r
}

// SetWorkspaceCleaner wires the disk-cleanup surface invoked by
// evaluateWorkspaceGroupCleanup when a Kandev-owned group's last active
// member leaves. Optional — when nil the cleanup state machine still
// transitions to cleanup_pending but no disk work is performed.
func (s *HandoffService) SetWorkspaceCleaner(c WorkspaceCleaner) {
	s.cleaner = c
}

// SetTaskEventPublisher wires the publisher used by cascade
// archive/delete to broadcast task.updated / task.deleted events.
// Optional — when nil the cascade silently skips event publishing
// (legacy and test wiring).
func (s *HandoffService) SetTaskEventPublisher(p TaskEventPublisher) {
	s.eventPublisher = p
}

// NewHandoffService creates a HandoffService. blockers and wsGroups may be
// nil — when nil the corresponding methods return empty results /
// no-ops, mirroring the optional wiring used by Service.SetBlockerRepository.
func NewHandoffService(
	tasks repository.TaskRepository,
	docsRepo repository.DocumentRepository,
	docs *DocumentService,
	blockers BlockerRepository,
	wsGroups WorkspaceGroupRepo,
	log *logger.Logger,
) *HandoffService {
	return &HandoffService{
		tasks:    tasks,
		docs:     docs,
		docsRepo: docsRepo,
		blockers: blockers,
		wsGroups: wsGroups,
		logger:   log,
		parentLock: parentMutex{
			locks: make(map[string]*sync.Mutex),
		},
	}
}

// parentMutex serialises sequential-child creation per parent. A naive
// "previous sibling = most recent non-archived" lookup races when two
// create_task_kandev calls arrive concurrently — both could pick the
// same previous sibling and the resulting blocker chain would skip a
// task. Per-parent locking keeps the chain deterministic without a
// schema change.
type parentMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (p *parentMutex) lockFor(parentID string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.locks[parentID]
	if !ok {
		m = &sync.Mutex{}
		p.locks[parentID] = m
	}
	return m
}

// AttachWorkspacePolicy records workspace-group membership and adds a
// sequential blocker edge to the previous non-archived sibling when the
// parent's ordering policy is "sequential". Safe to call multiple times
// for the same task — group membership inserts use INSERT OR IGNORE and
// blocker creation is wrapped in a per-parent mutex to keep the chain
// deterministic under concurrent task creation.
func (s *HandoffService) AttachWorkspacePolicy(ctx context.Context, taskID, parentID string, pol WorkspacePolicy) error {
	if !pol.NeedsAttachment() {
		return nil
	}
	if err := s.attachWorkspaceGroup(ctx, taskID, parentID, pol); err != nil {
		return err
	}
	if pol.ParentOrdering == orderingSequential && parentID != "" {
		if err := s.attachSequentialBlocker(ctx, taskID, parentID); err != nil {
			return err
		}
	}
	return nil
}

func (s *HandoffService) attachWorkspaceGroup(ctx context.Context, taskID, parentID string, pol WorkspacePolicy) error {
	if s.wsGroups == nil {
		return nil
	}
	switch pol.Mode {
	case workspaceModeInheritParent:
		if parentID == "" {
			return errors.New("workspace_mode=" + workspaceModeInheritParent + " requires a parent task")
		}
		group, err := s.lookupOrCreateParentGroup(ctx, parentID)
		if err != nil {
			return err
		}
		if group == nil {
			return nil
		}
		return s.wsGroups.AddWorkspaceGroupMember(ctx, group.ID, taskID, orchmodels.WorkspaceMemberRoleMember)

	case workspaceModeSharedGroup:
		if pol.GroupID == "" {
			return errors.New("workspace_mode=shared_group requires workspace_group_id")
		}
		g, err := s.wsGroups.GetWorkspaceGroup(ctx, pol.GroupID)
		if err != nil {
			return err
		}
		if g == nil {
			return fmt.Errorf("workspace group %s not found", pol.GroupID)
		}
		if g.CleanupStatus != orchmodels.WorkspaceCleanupStatusActive {
			return fmt.Errorf("workspace group %s is not active (cleanup_status=%s)", pol.GroupID, g.CleanupStatus)
		}
		return s.wsGroups.AddWorkspaceGroupMember(ctx, g.ID, taskID, orchmodels.WorkspaceMemberRoleMember)
	}
	return nil
}

// lookupOrCreateParentGroup returns the parent task's existing active
// workspace group or creates a fresh one with the parent as owner. The
// MaterializedKind is provisional (single_repo by default) — the
// materializer flips it via MarkWorkspaceMaterialized at launch time.
func (s *HandoffService) lookupOrCreateParentGroup(ctx context.Context, parentID string) (*orchmodels.WorkspaceGroup, error) {
	g, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if g != nil {
		return g, nil
	}
	parent, err := s.tasks.GetTask(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, fmt.Errorf("parent task %s not found", parentID)
	}
	g = &orchmodels.WorkspaceGroup{
		WorkspaceID:      parent.WorkspaceID,
		OwnerTaskID:      parentID,
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
	}
	if err := s.wsGroups.CreateWorkspaceGroup(ctx, g); err != nil {
		return nil, err
	}
	if err := s.wsGroups.AddWorkspaceGroupMember(ctx, g.ID, parentID, orchmodels.WorkspaceMemberRoleOwner); err != nil {
		return nil, err
	}
	return g, nil
}

// attachSequentialBlocker links the new child to the previous non-archived
// sibling under parentID. Held inside a per-parent mutex so concurrent
// creates produce a deterministic chain rather than a tangle.
func (s *HandoffService) attachSequentialBlocker(ctx context.Context, taskID, parentID string) error {
	if s.blockers == nil {
		return nil
	}
	mu := s.parentLock.lockFor(parentID)
	mu.Lock()
	defer mu.Unlock()

	siblings, err := s.tasks.ListChildren(ctx, parentID)
	if err != nil {
		return err
	}
	// Pick the most recently created sibling that is not the new task itself.
	// ListChildren already filters archived/ephemeral.
	var prev string
	for i := len(siblings) - 1; i >= 0; i-- {
		if siblings[i].ID == taskID {
			continue
		}
		prev = siblings[i].ID
		break
	}
	if prev == "" {
		return nil
	}
	return s.blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{
		TaskID:        taskID,
		BlockerTaskID: prev,
	})
}

// ListRelated returns the parent, children, siblings, blockers, and
// blocked-by tasks for taskID. Document keys are populated for the task
// itself plus every related task so an agent can shop for documents in
// one call.
func (s *HandoffService) ListRelated(ctx context.Context, taskID string) (*RelatedTasks, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	self, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if self == nil {
		return nil, errors.New("task not found")
	}

	out := &RelatedTasks{
		Task:      s.toRelated(ctx, self),
		Children:  []*RelatedTask{},
		Siblings:  []*RelatedTask{},
		Blockers:  []*RelatedTask{},
		BlockedBy: []*RelatedTask{},
	}

	if self.ParentID != "" {
		parent, err := s.tasks.GetTask(ctx, self.ParentID)
		if err == nil && parent != nil {
			ref := s.toRelated(ctx, parent)
			out.Parent = &ref
		}
	}

	children, err := s.tasks.ListChildren(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out.Children = s.toRelatedSlice(ctx, children)

	siblings, err := s.tasks.ListSiblings(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out.Siblings = s.toRelatedSlice(ctx, siblings)

	if s.blockers != nil {
		bs, err := s.blockers.ListTaskBlockers(ctx, taskID)
		if err == nil {
			ids := make([]string, 0, len(bs))
			for _, b := range bs {
				ids = append(ids, b.BlockerTaskID)
			}
			out.Blockers = s.toRelatedByIDs(ctx, ids)
		}
		bbIDs, err := s.blockers.ListTasksBlockedBy(ctx, taskID)
		if err == nil {
			out.BlockedBy = s.toRelatedByIDs(ctx, bbIDs)
		}
	}

	return out, nil
}

// GetDocumentForCaller fetches a document on targetTaskID after enforcing
// the read access rule for currentTaskID. Returns ErrAccessDenied when
// the rule fails.
func (s *HandoffService) GetDocumentForCaller(ctx context.Context, currentTaskID, targetTaskID, key string) (*models.TaskDocument, error) {
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	if targetTaskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	ok, err := canReadDocuments(ctx, repoTaskLookupAdapter{r: s.tasks}, blockerLookupAdapter{repo: s.blockers}, currentTaskID, targetTaskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}
	return s.docs.GetDocument(ctx, targetTaskID, key)
}

// ListDocumentsForCaller returns the document HEADs (no content) for
// targetTaskID after the read-access guard.
func (s *HandoffService) ListDocumentsForCaller(ctx context.Context, currentTaskID, targetTaskID string) ([]*models.TaskDocument, error) {
	if targetTaskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	ok, err := canReadDocuments(ctx, repoTaskLookupAdapter{r: s.tasks}, blockerLookupAdapter{repo: s.blockers}, currentTaskID, targetTaskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}
	docs, err := s.docs.ListDocuments(ctx, targetTaskID)
	if err != nil {
		return nil, err
	}
	// Strip Content from the projection so callers cannot accidentally
	// surface bodies through the listing tool. The MCP layer should treat
	// the listing as metadata-only.
	out := make([]*models.TaskDocument, 0, len(docs))
	for _, d := range docs {
		copy := *d
		copy.Content = ""
		out = append(out, &copy)
	}
	return out, nil
}

// WriteDocumentForCaller upserts a document on targetTaskID through
// DocumentService after the write-access guard. authorKind/authorName
// follow DocumentService defaults when empty.
func (s *HandoffService) WriteDocumentForCaller(
	ctx context.Context,
	currentTaskID, targetTaskID, key, docType, title, content, authorKind, authorName string,
) (*models.TaskDocument, error) {
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	if targetTaskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	ok, err := canWriteDocuments(ctx, repoTaskLookupAdapter{r: s.tasks}, currentTaskID, targetTaskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}
	return s.docs.CreateOrUpdateDocument(ctx, targetTaskID, key, docType, title, content, authorKind, authorName)
}

// toRelated converts a Task into the RelatedTask projection, populating
// document keys when the task has any. Errors fetching documents are
// logged and the keys list is left empty.
func (s *HandoffService) toRelated(ctx context.Context, t *models.Task) RelatedTask {
	rt := RelatedTask{
		ID:            t.ID,
		Identifier:    t.Identifier,
		Title:         t.Title,
		State:         string(t.State),
		WorkspaceID:   t.WorkspaceID,
		ParentID:      t.ParentID,
		AssigneeLabel: t.AssigneeAgentProfileID,
	}
	if s.docsRepo != nil {
		docs, err := s.docsRepo.ListDocuments(ctx, t.ID)
		if err == nil {
			for _, d := range docs {
				rt.DocumentKeys = append(rt.DocumentKeys, d.Key)
			}
		}
	}
	return rt
}

func (s *HandoffService) toRelatedSlice(ctx context.Context, tasks []*models.Task) []*RelatedTask {
	out := make([]*RelatedTask, 0, len(tasks))
	for _, t := range tasks {
		rt := s.toRelated(ctx, t)
		out = append(out, &rt)
	}
	return out
}

func (s *HandoffService) toRelatedByIDs(ctx context.Context, ids []string) []*RelatedTask {
	out := make([]*RelatedTask, 0, len(ids))
	for _, id := range ids {
		t, err := s.tasks.GetTask(ctx, id)
		if err != nil || t == nil {
			continue
		}
		rt := s.toRelated(ctx, t)
		out = append(out, &rt)
	}
	return out
}
