package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/shared"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"

	"go.uber.org/zap"
)

// DB-state constants. These mirror the values stored in the
// tasks.state column and are used in several places (status
// normalisation, gate logic, supersede branches).
const (
	stateTODO       = "TODO"
	stateInProgress = "IN_PROGRESS"
	stateInReview   = "REVIEW"
	stateBlocked    = "BLOCKED"
	stateCompleted  = "COMPLETED"
	stateCancelled  = "CANCELLED"
	stateBacklog    = "BACKLOG"
)

// statusInReviewLowercase is the lowercase canonical form returned to
// clients in the ApprovalsPendingError redirect payload.
const statusInReviewLowercase = "in_review"

// Lowercase canonical status strings returned to / accepted from clients.
// Each pairs with its uppercase DB state above.
const (
	statusTODOLowercase       = "todo"
	statusCancelledLowercase  = "cancelled"
	statusDoneLowercase       = "done"
	statusInProgressLowercase = "in_progress"
	statusReviewLowercase     = "review"
	statusBlockedLowercase    = "blocked"
	statusBacklogLowercase    = "backlog"
)

// userSentinel is the decider_id used for the singleton human user
// in the decisions table. Pulled out as a constant since it is also
// the actor_type label for activity log entries.
const userSentinel = "user"

// Repository is the persistence interface required by DashboardService.
type Repository interface {
	CountPendingApprovals(ctx context.Context, workspaceID string) (int, error)
	ListActivityEntries(ctx context.Context, workspaceID string, limit int) ([]*models.ActivityEntry, error)
	ListActivityEntriesByAction(ctx context.Context, workspaceID, action string, limit int) ([]*models.ActivityEntry, error)
	ListActivityEntriesByTarget(ctx context.Context, workspaceID, targetID string, limit int) ([]*models.ActivityEntry, error)
	ListPendingApprovals(ctx context.Context, workspaceID string) ([]*models.Approval, error)
	ListRuns(ctx context.Context, workspaceID string) ([]*models.Run, error)
	SearchTasks(ctx context.Context, workspaceID, query string, limit int) ([]*sqlite.TaskSearchResult, error)
	ListActivityEntriesByType(ctx context.Context, workspaceID, filterType string, limit int) ([]*models.ActivityEntry, error)
	ListTasksByWorkspace(ctx context.Context, workspaceID string, includeSystem bool) ([]*sqlite.TaskRow, error)
	ListTasksFiltered(ctx context.Context, workspaceID string, opts sqlite.ListTasksOptions) (*sqlite.ListTasksFilteredResult, error)
	GetTaskByID(ctx context.Context, taskID string) (*sqlite.TaskRow, error)
	ListChildTasks(ctx context.Context, parentID string) ([]*sqlite.TaskRow, error)
	ListBlockersForTasks(ctx context.Context, taskIDs []string) (map[string][]string, error)
	ListTaskComments(ctx context.Context, taskID string) ([]*models.TaskComment, error)
	CreateTaskComment(ctx context.Context, comment *models.TaskComment) error
	GetRunsByCommentIDs(ctx context.Context, commentIDs []string) (map[string]sqlite.CommentRunStatus, error)
	UpdateTaskState(ctx context.Context, taskID, state string) error
	GetTaskExecutionFields(ctx context.Context, taskID string) (*sqlite.TaskExecutionFields, error)
	UpdateTaskAssignee(ctx context.Context, taskID, assigneeID string) error
	UpdateTaskPriority(ctx context.Context, taskID, priority string) error
	UpdateTaskProjectID(ctx context.Context, taskID, projectID string) error
	UpdateTaskParentID(ctx context.Context, taskID, parentID string) error
	GetProjectWorkspaceID(ctx context.Context, projectID string) (string, error)
	GetTaskWorkspaceID(ctx context.Context, taskID string) (string, error)
	CountTasksByWorkspace(ctx context.Context, workspaceID string) (int, error)
	QueryRunActivity(ctx context.Context, workspaceID string, days int) ([]sqlite.RunActivityRow, error)
	QueryTaskBreakdown(ctx context.Context, workspaceID string) ([]sqlite.TaskBreakdownRow, error)
	QueryRecentTasks(ctx context.Context, workspaceID string, limit int) ([]sqlite.RecentTaskRow, error)
	QueryRecentSessions(ctx context.Context, workspaceID string, limit int) ([]sqlite.LiveSessionRow, error)
	ListRecentSessionsByAgentBatch(ctx context.Context, agentInstanceIDs []string, perAgentLimit int) (map[string][]sqlite.AgentSessionRow, error)
	CountToolCallMessagesBySession(ctx context.Context, sessionIDs []string) (map[string]int, error)
	GetTasksByIDs(ctx context.Context, ids []string) ([]sqlite.TaskTitleRow, error)
	CreateTaskBlocker(ctx context.Context, blocker *models.TaskBlocker) error
	DeleteTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error
	ListTaskBlockers(ctx context.Context, taskID string) ([]*models.TaskBlocker, error)
	ListTaskParticipants(ctx context.Context, taskID, role string) ([]sqlite.Participant, error)
	ListAllTaskParticipants(ctx context.Context, taskID string) ([]sqlite.Participant, error)
	AddTaskParticipant(ctx context.Context, taskID, agentID, role string) error
	RemoveTaskParticipant(ctx context.Context, taskID, agentID, role string) error
	GetTaskWorkflowStepID(ctx context.Context, taskID string) (string, error)
}

// DecisionStore is the workflow-domain decisions interface required by
// DashboardService. Implementations route to workflow_step_decisions.
// ADR 0005 Wave E moved decisions off the legacy office_task_approval_decisions
// table; this interface lives at the dashboard tier so the package does
// not import the workflow repository directly.
type DecisionStore interface {
	// FindParticipantID resolves the participant id for the natural key
	// (step_id, task_id, role, agent_profile_id). Empty agent_profile_id
	// (singleton user) returns "" without an error.
	FindParticipantID(ctx context.Context, stepID, taskID, role, agentProfileID string) (string, error)
	// RecordStepDecision atomically supersedes any prior active decision
	// matching the natural key (task, step, decider_id, role) — falling back
	// to (task, step, participant_id) when decider info is empty — and
	// inserts a fresh non-superseded row.
	RecordStepDecision(ctx context.Context, d *workflowmodels.WorkflowStepDecision) error
	// ListActiveTaskDecisions returns every non-superseded decision for a
	// task across all steps, oldest first.
	ListActiveTaskDecisions(ctx context.Context, taskID string) ([]*workflowmodels.WorkflowStepDecision, error)
	// SupersedeTaskDecisions marks every active decision for a task as
	// superseded across all steps.
	SupersedeTaskDecisions(ctx context.Context, taskID string) error
}

// SkillLister returns the list of skills for a workspace.
type SkillLister interface {
	ListSkills(ctx context.Context, wsID string) ([]*models.Skill, error)
}

// RoutineLister returns the list of routines for a workspace.
type RoutineLister interface {
	ListRoutines(ctx context.Context, wsID string) ([]*models.Routine, error)
}

// GovernanceSettingsStore persists workspace governance settings.
type GovernanceSettingsStore interface {
	GetRequireApprovalForNewAgents(ctx context.Context, workspaceID string) (bool, error)
	SetRequireApprovalForNewAgents(ctx context.Context, workspaceID string, required bool) error
	GetRequireApprovalForTaskCompletion(ctx context.Context, workspaceID string) (bool, error)
	SetRequireApprovalForTaskCompletion(ctx context.Context, workspaceID string, required bool) error
	GetRequireApprovalForSkillChanges(ctx context.Context, workspaceID string) (bool, error)
	SetRequireApprovalForSkillChanges(ctx context.Context, workspaceID string, required bool) error
}

// RetryCanceller is the interface used to cancel pending retries when a task is reassigned.
type RetryCanceller interface {
	CancelPendingRetriesForTask(ctx context.Context, taskID string) error
}

// TaskCanceller hard-cancels a task's active execution. Used by the
// reactivity pipeline when a task is moved to "cancelled" status.
type TaskCanceller interface {
	CancelTaskExecution(ctx context.Context, taskID, reason string, force bool) error
}

// SessionTerminator flips the (task, agent) office session row to a terminal
// state. Used when an agent stops being a participant on a task — reassignment,
// reviewer/approver removal, or agent instance deletion. Idempotent: skipping
// the call when no matching row exists or the row is already terminal is the
// caller's responsibility (or the implementation's, depending on flavour).
type SessionTerminator interface {
	TerminateOfficeSession(ctx context.Context, taskID, agentInstanceID, reason string) error
}

// FailureNotifier is invoked when a task's assignee changes so the
// failure-tracking layer can auto-dismiss any inbox entries tied to
// the prior (task, agent) pair. The agent's consecutive-failure
// counter is intentionally NOT reset by reassignment.
type FailureNotifier interface {
	OnAssigneeChanged(ctx context.Context, taskID, oldAgentID string)
}

// FailureInboxRow is a slim, dashboard-package view of one failed
// run or one auto-paused agent. The Office service flattens its
// repo rows into this shape so the dashboard package doesn't need to
// import the office repo types.
type FailureInboxRow struct {
	Kind                string
	ItemID              string
	AgentProfileID      string
	AgentName           string
	TaskID              string
	ErrorMessage        string
	PauseReason         string
	ConsecutiveFailures int
	FailedAt            time.Time
}

// FailureInboxSource exposes the failed-run + auto-paused-agent
// sources used by the inbox.
type FailureInboxSource interface {
	ListFailedRunInboxRows(ctx context.Context, workspaceID, userID string) ([]FailureInboxRow, error)
	ListPausedAgentInboxRows(ctx context.Context, workspaceID, userID string) ([]FailureInboxRow, error)
}

// MarkFixedHandler resolves an inbox dismissal: removes the row and
// re-queues the appropriate run(s).
type MarkFixedHandler interface {
	MarkAgentRunFailedFixed(ctx context.Context, userID, runID string) error
	MarkAgentPausedFixed(ctx context.Context, userID, agentID string) error
}

// TaskReactivityChange is a slim, dashboard-package view of the change
// being applied. Mirrored in scheduler.TaskMutation by the adapter so
// the dashboard package doesn't import scheduler types.
type TaskReactivityChange struct {
	NewStatus     *string
	NewAssigneeID *string
	// PrevAssigneeID is the assignee BEFORE the mutation. Required when
	// NewAssigneeID is set so the pipeline can detect a real change and
	// hand off the previous assignee's session.
	PrevAssigneeID string
	Comment        *TaskReactivityComment
	ReopenIntent   bool
	ResumeIntent   bool
	ActorID        string
	ActorType      string
}

// TaskReactivityComment is the slim view of a comment for the reactivity pipeline.
type TaskReactivityComment struct {
	ID         string
	Body       string
	AuthorType string
	AuthorID   string
}

// TaskReactivityResult summarises pipeline side-effects (kept thin so
// the dashboard package doesn't depend on scheduler types).
type TaskReactivityResult struct {
	InterruptSessionID string
}

// ReactivityApplier runs the office task reactivity pipeline. Implemented
// by an adapter around scheduler.SchedulerService so the dashboard
// package doesn't import the scheduler package.
type ReactivityApplier interface {
	ApplyTaskMutation(
		ctx context.Context,
		taskID string,
		preStatus string,
		change TaskReactivityChange,
	) (*TaskReactivityResult, error)
}

// ApprovalRun describes a single run the approval-flow service
// wants to queue for an agent. Dashboard ships these to the scheduler
// via the ApprovalReactivityQueuer adapter so the dashboard package
// doesn't depend on scheduler types.
type ApprovalRun struct {
	AgentID         string
	Reason          string
	TaskID          string
	WorkspaceID     string
	ActorID         string
	ActorType       string
	Role            string // reviewer|approver, when relevant
	DecisionComment string // for changes_requested
}

// ApprovalReactivityQueuer queues approval-flow runs (review
// requested, changes requested, ready to close). Implemented by a
// scheduler adapter; the dashboard package only sees this small
// interface so it doesn't import scheduler types.
type ApprovalReactivityQueuer interface {
	QueueApprovalRuns(ctx context.Context, runs []ApprovalRun) error
}

// RoutingProvider exposes provider-routing operations to the dashboard
// HTTP layer. Implemented by routing.Provider (in the routing package)
// so the dashboard package stays repo-agnostic and the resolver lives
// next to the rest of the routing code.
type RoutingProvider interface {
	GetConfig(ctx context.Context, workspaceID string) (*routing.WorkspaceConfig, []routing.ProviderID, error)
	UpdateConfig(ctx context.Context, workspaceID string, cfg routing.WorkspaceConfig) error
	Retry(ctx context.Context, workspaceID, providerID string) (string, *time.Time, error)
	Health(ctx context.Context, workspaceID string) ([]models.ProviderHealth, error)
	Preview(ctx context.Context, workspaceID string) ([]routing.PreviewItem, error)
	PreviewAgent(ctx context.Context, agentID string) (*routing.PreviewItem, error)
	// AgentOverrides returns the raw routing override blob persisted on
	// the agent's settings JSON. The agent routing UI hydrates from
	// this so toggles + tier override + provider-order override reflect
	// the persisted state on first paint instead of always defaulting
	// to "inherit". Returns the zero blob when the agent has no
	// overrides recorded; an error only on lookup / parse failures.
	AgentOverrides(ctx context.Context, agentID string) (routing.AgentOverrides, error)
}

// RouteAttemptLister is the narrow read-side seam used by the run-detail
// + standalone attempts endpoints to fetch the route_attempts list for a
// run. Implemented by *sqlite.Repository.
type RouteAttemptLister interface {
	ListRouteAttempts(ctx context.Context, runID string) ([]models.RouteAttempt, error)
}

// SettingsProvider reads and writes workspace settings for the dashboard.
// Implemented by a thin adapter around the configloader.ConfigLoader.
type SettingsProvider interface {
	// GetSettings returns the workspace settings for the named workspace.
	// Returns nil, nil when the workspace does not exist.
	GetSettings(workspaceName string) (*WorkspaceSettings, error)
	// UpdatePermissionHandlingMode persists a new permission_handling_mode.
	UpdatePermissionHandlingMode(workspaceName, mode string) error
	// UpdateRecoveryLookbackHours persists the recovery_lookback_hours setting.
	UpdateRecoveryLookbackHours(workspaceName string, hours int) error
}

// WorkspaceSettings is a simplified view of workspace-level settings
// exposed to the dashboard.
type WorkspaceSettings struct {
	Name                             string `json:"name"`
	Description                      string `json:"description,omitempty"`
	PermissionHandlingMode           string `json:"permission_handling_mode"`
	RecoveryLookbackHours            int    `json:"recovery_lookback_hours"`
	RequireApprovalForNewAgents      bool   `json:"require_approval_for_new_agents"`
	RequireApprovalForTaskCompletion bool   `json:"require_approval_for_task_completion"`
	RequireApprovalForSkillChanges   bool   `json:"require_approval_for_skill_changes"`
}

// DashboardService provides dashboard, inbox, activity, run, and task-search business logic.
type DashboardService struct {
	repo             Repository
	logger           *logger.Logger
	activity         shared.ActivityLogger
	agents           shared.AgentReader
	costs            shared.CostChecker
	permissions      shared.PermissionLister  // optional; nil means no permission items in inbox
	settingsProvider SettingsProvider         // optional; nil means settings endpoints are unavailable
	governanceStore  GovernanceSettingsStore  // optional; nil means governance settings are unavailable
	eb               bus.EventBus             // optional; nil means no events are published
	retryCanceller   RetryCanceller           // optional; nil means retries are not cancelled on reassign
	taskCanceller    TaskCanceller            // optional; used to hard-cancel sessions on status→cancelled
	sessionTerm      SessionTerminator        // optional; flips office session rows to COMPLETED on participation removal
	reactivity       ReactivityApplier        // optional; runs the office reactivity pipeline on mutations
	approvalQueuer   ApprovalReactivityQueuer // optional; queues approval-flow runs
	skillLister      SkillLister              // optional; nil means skill_count is always 0
	routineLister    RoutineLister            // optional; nil means routine_count is always 0
	failureNotifier  FailureNotifier          // optional; nil means assignee changes don't auto-dismiss inbox entries
	failureInbox     FailureInboxSource       // optional; nil disables the new agent_run_failed / agent_paused_after_failures inbox sources
	markFixed        MarkFixedHandler         // optional; nil disables the dismiss endpoint
	decisions        DecisionStore            // workflow-domain decisions store (ADR 0005 Wave E); nil disables decision endpoints
	routingProvider  RoutingProvider          // optional; nil disables /routing endpoints (503)
	attemptLister    RouteAttemptLister       // optional; nil disables attempt embedding on run-detail responses
}

// SetRoutingProvider wires the provider-routing seam used by the
// /workspaces/:wsId/routing/* endpoints. Optional; when nil, the
// HTTP handlers respond with 503.
func (s *DashboardService) SetRoutingProvider(p RoutingProvider) {
	s.routingProvider = p
}

// SetRouteAttemptLister wires the read-side seam used to embed
// route_attempts on the run-detail response and back the standalone
// GET /runs/:id/attempts endpoint. Optional; nil means the run-detail
// payload omits the RouteAttempts field and /attempts responds 503.
func (s *DashboardService) SetRouteAttemptLister(l RouteAttemptLister) {
	s.attemptLister = l
}

// RoutingProvider returns the wired routing provider, or nil when no
// provider has been set. Exposed so the HTTP handler can short-circuit.
func (s *DashboardService) RoutingProviderImpl() RoutingProvider {
	return s.routingProvider
}

// RouteAttemptListerImpl returns the wired attempts lister, or nil.
func (s *DashboardService) RouteAttemptListerImpl() RouteAttemptLister {
	return s.attemptLister
}

// EventBus returns the wired event bus (may be nil). Exposed so the
// HTTP layer can publish routing_settings_updated without re-reading
// the bus reference at every call site.
func (s *DashboardService) EventBus() bus.EventBus { return s.eb }

// SetDecisionStore wires the workflow-domain decisions store. Required for
// approve/request-changes/list endpoints; the service surfaces an
// internal error if any decision method is invoked without a store wired.
func (s *DashboardService) SetDecisionStore(d DecisionStore) {
	s.decisions = d
}

// SetFailureInboxSource wires the source for the new
// agent_run_failed + agent_paused_after_failures inbox rows.
func (s *DashboardService) SetFailureInboxSource(src FailureInboxSource) {
	s.failureInbox = src
}

// SetMarkFixedHandler wires the resolver for inbox "Mark fixed"
// actions. The handler dismisses the entry and re-queues the
// appropriate run(s).
func (s *DashboardService) SetMarkFixedHandler(h MarkFixedHandler) {
	s.markFixed = h
}

// MarkFixedHandler returns the wired handler (or nil) so the HTTP
// layer can short-circuit when the dependency isn't configured.
func (s *DashboardService) MarkFixedHandler() MarkFixedHandler {
	return s.markFixed
}

// SetFailureNotifier wires the failure-tracking notifier so reassignments
// auto-dismiss any inbox entries tied to the prior (task, agent) pair.
func (s *DashboardService) SetFailureNotifier(n FailureNotifier) {
	s.failureNotifier = n
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(
	repo Repository,
	log *logger.Logger,
	activity shared.ActivityLogger,
	agents shared.AgentReader,
	costs shared.CostChecker,
) *DashboardService {
	return &DashboardService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "office-dashboard")),
		activity: activity,
		agents:   agents,
		costs:    costs,
	}
}

// SetPermissionLister sets the source of pending permission/clarification requests
// used to populate inbox items. Call this after construction when the clarification
// store is available.
func (s *DashboardService) SetPermissionLister(lister shared.PermissionLister) {
	s.permissions = lister
}

// SetSettingsProvider sets the provider used to read and write workspace settings.
// Call this after construction when the config loader is available.
func (s *DashboardService) SetSettingsProvider(p SettingsProvider) {
	s.settingsProvider = p
}

// SetRetryCanceller sets the service used to cancel pending retries when a task is reassigned.
func (s *DashboardService) SetRetryCanceller(c RetryCanceller) {
	s.retryCanceller = c
}

// SetTaskCanceller sets the canceller used to hard-cancel sessions when
// a task is moved to "cancelled".
func (s *DashboardService) SetTaskCanceller(c TaskCanceller) {
	s.taskCanceller = c
}

// SetSessionTerminator wires the office session terminator. Optional; when
// nil, participation removal won't flip the prior agent's session to a
// terminal state (the row stays IDLE/RUNNING and its conversation lingers).
func (s *DashboardService) SetSessionTerminator(t SessionTerminator) {
	s.sessionTerm = t
}

// SetReactivityApplier sets the office reactivity pipeline. When set,
// every property mutation runs through it to queue downstream runs
// (blockers resolved, children completed, comments, mentions, etc.).
func (s *DashboardService) SetReactivityApplier(r ReactivityApplier) {
	s.reactivity = r
}

// SetApprovalReactivityQueuer sets the queuer used to fire approval-flow
// runs (task_changes_requested, task_ready_to_close). When unset, the
// approval-flow service still records decisions but no runs fire.
func (s *DashboardService) SetApprovalReactivityQueuer(q ApprovalReactivityQueuer) {
	s.approvalQueuer = q
}

// SetGovernanceStore sets the store used to read and write governance settings.
func (s *DashboardService) SetGovernanceStore(g GovernanceSettingsStore) {
	s.governanceStore = g
}

// GetRequireApprovalForNewAgents returns the governance setting for new agent approval.
// Implements agents.GovernanceSettingsReader.
func (s *DashboardService) GetRequireApprovalForNewAgents(ctx context.Context, workspaceID string) (bool, error) {
	if s.governanceStore == nil {
		return false, nil
	}
	return s.governanceStore.GetRequireApprovalForNewAgents(ctx, workspaceID)
}

// GetWorkspaceSettings returns workspace settings including governance flags.
// Returns defaults when no settings provider has been configured.
func (s *DashboardService) GetWorkspaceSettings(ctx context.Context, workspaceName, workspaceID string) (*WorkspaceSettings, error) {
	var ws *WorkspaceSettings
	if s.settingsProvider == nil {
		ws = &WorkspaceSettings{Name: workspaceName, PermissionHandlingMode: "human"}
	} else {
		var err error
		ws, err = s.settingsProvider.GetSettings(workspaceName)
		if err != nil {
			return nil, err
		}
		if ws == nil {
			ws = &WorkspaceSettings{Name: workspaceName, PermissionHandlingMode: "human"}
		}
	}
	if s.governanceStore != nil {
		req, _ := s.governanceStore.GetRequireApprovalForNewAgents(ctx, workspaceID)
		ws.RequireApprovalForNewAgents = req
		tc, _ := s.governanceStore.GetRequireApprovalForTaskCompletion(ctx, workspaceID)
		ws.RequireApprovalForTaskCompletion = tc
		sc, _ := s.governanceStore.GetRequireApprovalForSkillChanges(ctx, workspaceID)
		ws.RequireApprovalForSkillChanges = sc
	}
	return ws, nil
}

// UpdatePermissionHandlingMode updates the permission_handling_mode for a workspace.
func (s *DashboardService) UpdatePermissionHandlingMode(workspaceName, mode string) error {
	if s.settingsProvider == nil {
		return fmt.Errorf("settings provider not configured")
	}
	return s.settingsProvider.UpdatePermissionHandlingMode(workspaceName, mode)
}

// UpdateRecoveryLookbackHours updates the recovery_lookback_hours setting for a workspace.
func (s *DashboardService) UpdateRecoveryLookbackHours(workspaceName string, hours int) error {
	if s.settingsProvider == nil {
		return fmt.Errorf("settings provider not configured")
	}
	return s.settingsProvider.UpdateRecoveryLookbackHours(workspaceName, hours)
}

// UpdateGovernanceApprovalFlags updates the three governance approval toggle flags.
func (s *DashboardService) UpdateGovernanceApprovalFlags(
	ctx context.Context, workspaceID string,
	newAgents, taskCompletion, skillChanges *bool,
) {
	if s.governanceStore == nil {
		return
	}
	if newAgents != nil {
		if err := s.governanceStore.SetRequireApprovalForNewAgents(ctx, workspaceID, *newAgents); err != nil {
			s.logger.Warn("failed to update require_approval_for_new_agents", zap.Error(err))
		}
	}
	if taskCompletion != nil {
		if err := s.governanceStore.SetRequireApprovalForTaskCompletion(ctx, workspaceID, *taskCompletion); err != nil {
			s.logger.Warn("failed to update require_approval_for_task_completion", zap.Error(err))
		}
	}
	if skillChanges != nil {
		if err := s.governanceStore.SetRequireApprovalForSkillChanges(ctx, workspaceID, *skillChanges); err != nil {
			s.logger.Warn("failed to update require_approval_for_skill_changes", zap.Error(err))
		}
	}
}

// ListBlockersForChildren returns a map of child task ID → blocker IDs for a set of children.
func (s *DashboardService) ListBlockersForChildren(ctx context.Context, childIDs []string) (map[string][]string, error) {
	if len(childIDs) == 0 {
		return map[string][]string{}, nil
	}
	return s.repo.ListBlockersForTasks(ctx, childIDs)
}

// SetEventBus configures the event bus used to publish task status change events.
// Call after construction when the event bus is available.
func (s *DashboardService) SetEventBus(eb bus.EventBus) {
	s.eb = eb
}

// SetSkillLister sets the provider used to count skills for the dashboard.
func (s *DashboardService) SetSkillLister(sl SkillLister) {
	s.skillLister = sl
}

// SetRoutineLister sets the provider used to count routines for the dashboard.
func (s *DashboardService) SetRoutineLister(rl RoutineLister) {
	s.routineLister = rl
}

// ListActivityFiltered returns activity entries filtered by optional type.
func (s *DashboardService) ListActivityFiltered(ctx context.Context, wsID, filterType string, limit int) ([]*models.ActivityEntry, error) {
	if filterType == "" || filterType == "all" {
		return s.repo.ListActivityEntries(ctx, wsID, limit)
	}
	return s.repo.ListActivityEntriesByType(ctx, wsID, filterType, limit)
}

// ListActivityForTarget returns activity entries scoped to one target entity.
func (s *DashboardService) ListActivityForTarget(ctx context.Context, wsID, targetID string, limit int) ([]*models.ActivityEntry, error) {
	if targetID == "" {
		return s.ListActivityFiltered(ctx, wsID, "", limit)
	}
	return s.repo.ListActivityEntriesByTarget(ctx, wsID, targetID, limit)
}

// ListRuns returns run requests for a workspace.
func (s *DashboardService) ListRuns(ctx context.Context, wsID string) ([]*models.Run, error) {
	return s.repo.ListRuns(ctx, wsID)
}

// SearchTasks searches for tasks matching the query string in a workspace.
func (s *DashboardService) SearchTasks(ctx context.Context, wsID, query string, limit int) ([]*sqlite.TaskSearchResult, error) {
	return s.repo.SearchTasks(ctx, wsID, query, limit)
}

// ListTasks returns non-archived tasks for a workspace. When
// includeSystem is false (default for the Office Tasks UI) tasks
// belonging to kandev-managed system workflows are excluded.
func (s *DashboardService) ListTasks(ctx context.Context, wsID string, includeSystem bool) ([]*sqlite.TaskRow, error) {
	return s.repo.ListTasksByWorkspace(ctx, wsID, includeSystem)
}

// ListTasksFiltered returns a paginated, optionally filtered slice of
// non-archived tasks for a workspace (Stream E of office optimization).
// See sqlite.ListTasksOptions for the supported filters; sort field is
// validated against an allow-list at the repository tier.
func (s *DashboardService) ListTasksFiltered(
	ctx context.Context, wsID string, opts sqlite.ListTasksOptions,
) (*sqlite.ListTasksFilteredResult, error) {
	return s.repo.ListTasksFiltered(ctx, wsID, opts)
}

// GetTask returns a single task by ID. Returns nil, nil when not found.
func (s *DashboardService) GetTask(ctx context.Context, taskID string) (*sqlite.TaskRow, error) {
	return s.repo.GetTaskByID(ctx, taskID)
}

// ListChildTasks returns direct child tasks of a parent task.
func (s *DashboardService) ListChildTasks(ctx context.Context, parentID string) ([]*sqlite.TaskRow, error) {
	return s.repo.ListChildTasks(ctx, parentID)
}

// ListComments returns all comments for a task.
func (s *DashboardService) ListComments(ctx context.Context, taskID string) ([]*models.TaskComment, error) {
	return s.repo.ListTaskComments(ctx, taskID)
}

// GetRunsByCommentIDs returns the per-comment run snapshot for the
// supplied comment ids. Comments without a matching run are absent
// from the map. Used by the comments handler to attach run lifecycle
// state (id, status, error) to each comment DTO.
func (s *DashboardService) GetRunsByCommentIDs(
	ctx context.Context, commentIDs []string,
) (map[string]sqlite.CommentRunStatus, error) {
	return s.repo.GetRunsByCommentIDs(ctx, commentIDs)
}

// CreateComment creates a new comment on a task and runs the reactivity
// pipeline so the assignee (and any @-mentioned agents) wake up.
func (s *DashboardService) CreateComment(ctx context.Context, comment *models.TaskComment) error {
	if err := s.repo.CreateTaskComment(ctx, comment); err != nil {
		return err
	}
	s.publishCommentCreated(ctx, comment)
	s.runReactivityForComment(ctx, comment)
	return nil
}

// ListStatusChanges returns status-change timeline events for a task, sourced
// from the office activity log.  Returns an empty slice (never nil) on any
// error so callers can treat it as best-effort.
func (s *DashboardService) ListStatusChanges(ctx context.Context, workspaceID, taskID string) ([]TimelineEvent, error) {
	entries, err := s.repo.ListActivityEntriesByTarget(ctx, workspaceID, taskID, 50)
	if err != nil {
		return []TimelineEvent{}, nil
	}
	var evts []TimelineEvent
	for _, e := range entries {
		if e.Action != "task_status_changed" {
			continue
		}
		var details map[string]string
		if jsonErr := json.Unmarshal([]byte(e.Details), &details); jsonErr != nil {
			continue
		}
		ev := TimelineEvent{
			From: details["old_status"],
			To:   details["new_status"],
			At:   e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if ev.To == "" {
			continue
		}
		evts = append(evts, ev)
	}
	if evts == nil {
		evts = []TimelineEvent{}
	}
	return evts, nil
}
