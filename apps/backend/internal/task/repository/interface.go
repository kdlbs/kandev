package repository

import (
	"context"
	"time"

	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/statussummary"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var ErrWorkspaceNameMismatch = repoerrors.ErrWorkspaceNameMismatch
var ErrWorkspaceNotFound = repoerrors.ErrWorkspaceNotFound
var ErrTaskNotFound = repoerrors.ErrTaskNotFound
var ErrTaskParentMismatch = repoerrors.ErrTaskParentMismatch
var ErrTaskPlanNotFound = repoerrors.ErrTaskPlanNotFound
var ErrRepositoryNotFound = repoerrors.ErrRepositoryNotFound
var ErrTaskEnvironmentNotFound = repoerrors.ErrTaskEnvironmentNotFound
var ErrWIPLimitExceeded = wfmodels.ErrWIPLimitExceeded
var ErrExternalIDConflict = repoerrors.ErrExternalIDConflict

// WorkspaceRepository handles workspace CRUD.
type WorkspaceRepository interface {
	CreateWorkspace(ctx context.Context, workspace *models.Workspace) error
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
	UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error
	DeleteWorkspace(ctx context.Context, id string) error
	DeleteWorkspaceCascade(ctx context.Context, id string) ([]*models.Task, []*models.Workflow, error)
	DeleteWorkspaceCascadeWithName(ctx context.Context, id, name string) ([]*models.Task, []*models.Workflow, error)
	ListWorkspaces(ctx context.Context) ([]*models.Workspace, error)
}

// TaskRepository handles task CRUD and workflow placement.
// Note: models.TaskRepository is a struct in internal/task/models; no Go conflict exists.
type TaskRepository interface {
	CreateTask(ctx context.Context, task *models.Task) error
	GetTask(ctx context.Context, id string) (*models.Task, error)
	GetTasksByIDs(ctx context.Context, ids []string) ([]*models.Task, error)
	UpdateTask(ctx context.Context, task *models.Task) error
	DeleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error)
	ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error)
	ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error)
	ArchiveTask(ctx context.Context, id string) error
	// ArchiveTaskIfActive is the CAS variant used by office task-handoffs
	// cascade archives. Returns whether the row was updated.
	ArchiveTaskIfActive(ctx context.Context, id, cascadeID string) (bool, error)
	// UnarchiveTaskByCascade clears archived_at only when the task was
	// archived by the named cascade. Returns whether the row was updated.
	UnarchiveTaskByCascade(ctx context.Context, id, cascadeID string) (bool, error)
	// UnarchiveTask clears archived_at only when the task carries no
	// cascade stamp (archived_by_cascade_id empty/NULL) — the CAS keeps a
	// delayed manual unarchive from erasing a newer cascade archive.
	// Cascade-stamped rows are restored via UnarchiveTaskByCascade.
	// Returns whether the row was updated.
	UnarchiveTask(ctx context.Context, id string) (bool, error)
	ListTasksForAutoArchive(ctx context.Context) ([]*models.Task, error)
	// ListArchivedTasksWithActiveSessions returns the IDs of archived tasks
	// (archived_at IS NOT NULL) that still have at least one task_sessions
	// row in an active DB state (CREATED/STARTING/RUNNING/WAITING_FOR_INPUT).
	// Candidate list for the periodic reconciliation sweep that recovers
	// sessions left stranded when finalizeCancelledSessions's bounded
	// in-line retry was exhausted by sustained SQLite writer contention.
	ListArchivedTasksWithActiveSessions(ctx context.Context) ([]string, error)
	ListExpiredQuickChatTasks(ctx context.Context, cutoff time.Time) ([]*models.Task, error)
	DeleteExpiredQuickChatTask(ctx context.Context, id string, cutoff time.Time) (bool, error)
	// CountOpenWatcherCreatedTasks returns the number of open watcher-created
	// tasks for a single watch, identified by the integration's task-metadata
	// key (e.g. "sentry_issue_watch_id") and the watch id. Open = non-archived
	// AND state NOT IN (COMPLETED, FAILED, CANCELLED). Used by the
	// orchestrator's watcher throttle gate to enforce a per-watch cap. Keyed
	// by metadata key (not integration name) so this layer stays agnostic of
	// which integrations exist.
	CountOpenWatcherCreatedTasks(ctx context.Context, metadataKey, watchID string) (int, error)
	// SetTaskMetadataKeyIfPresent rewrites one metadata key only while that
	// key is still present, reporting whether the write landed. The CAS
	// counterpart to an atomic remove: an editor must never re-create a key a
	// concurrent claim just consumed.
	SetTaskMetadataKeyIfPresent(ctx context.Context, taskID, key string, value interface{}) (bool, error)
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error
	// UpdateTaskStateIfSessionState atomically transitions task state only while
	// the named session remains in expectedSessionState and the task is not
	// archived. Returns the pre-update state and whether a row changed.
	UpdateTaskStateIfSessionState(
		ctx context.Context,
		taskID, sessionID string,
		expectedSessionState models.TaskSessionState,
		state v1.TaskState,
	) (v1.TaskState, bool, error)
	// UpdateTaskStateIfCurrentIn atomically transitions state only when the
	// task's current state is in allowed AND the task is not archived
	// (archived_at IS NULL). The archived check is enforced inside the same
	// UPDATE's WHERE clause, not just by a caller's earlier (non-transactional)
	// read, so a late write can never race an ArchiveTask commit that lands
	// between that read and this call. Returns the pre-update state and
	// whether a row was modified.
	UpdateTaskStateIfCurrentIn(ctx context.Context, id string, state v1.TaskState, allowed []v1.TaskState) (v1.TaskState, bool, error)
	// UpdateTaskStateIfNotArchived is UpdateTaskStateIfCurrentIn without the
	// prior-state constraint — for writers (e.g. IN_PROGRESS reconciliation)
	// that legitimately fire from many prior states and only need the
	// archived_at IS NULL guarantee. Same TOCTOU-closing semantics: the
	// archived check is atomic with the write. Returns the pre-update state
	// and whether a row was modified.
	UpdateTaskStateIfNotArchived(ctx context.Context, id string, state v1.TaskState) (v1.TaskState, bool, error)
	CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error)
	CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error)
	AddTaskToWorkflow(ctx context.Context, taskID, workflowID, workflowStepID string, position int) error
	RemoveTaskFromWorkflow(ctx context.Context, taskID, workflowID string) error
	ListTasksByProject(ctx context.Context, projectID string) ([]*models.Task, error)
	ListTasksByAssignee(ctx context.Context, agentInstanceID string) ([]*models.Task, error)
	ListTaskTree(ctx context.Context, workspaceID string, filters models.TaskTreeFilters) ([]*models.Task, error)
	// ListChildren returns non-archived, non-ephemeral child tasks of parentID.
	ListChildren(ctx context.Context, parentID string) ([]*models.Task, error)
	// ListChildrenIncludingArchived returns ALL child tasks of parentID,
	// including archived ones. Used by the office task-handoffs unarchive
	// cascade (phase 6) to walk a previously-archived descendant tree.
	ListChildrenIncludingArchived(ctx context.Context, parentID string) ([]*models.Task, error)
	// ReparentDirectChildren updates every row whose parent_id matches
	// oldParentID, replacing it with newParentID. Used by no-cascade
	// delete so direct children of a deleted task become roots
	// (newParentID="") instead of dangling pointers. Affects archived
	// and active rows alike.
	ReparentDirectChildren(ctx context.Context, oldParentID, newParentID string) error
	// ListSiblings returns non-archived, non-ephemeral sibling tasks for taskID.
	// A task is a sibling of taskID when it shares a non-empty parent_id and
	// the same workspace_id, and is not taskID itself. Root tasks (empty
	// parent_id) intentionally have NO siblings — without a non-empty common
	// parent, every other root in the workspace would falsely match.
	ListSiblings(ctx context.Context, taskID string) ([]*models.Task, error)
	IncrementTaskSequence(ctx context.Context, workspaceID string) (int, error)
	GetWorkspaceTaskPrefix(ctx context.Context, workspaceID string) (prefix, officeWorkflowID string, err error)

	// GetTaskByExternalID returns the task holding (workspaceID, externalID),
	// including archived and unsettled tasks, or ErrTaskNotFound if none does.
	GetTaskByExternalID(ctx context.Context, workspaceID, externalID string) (*models.Task, error)
	// SettleTaskExternalID stamps external_id_settled_at on the task if it
	// still holds externalID and has not already been settled. The predicate
	// includes external_id (not just id) because release clears both columns,
	// so guarding on id alone would wrongly stamp a released row. Returns
	// whether a row was updated.
	SettleTaskExternalID(ctx context.Context, taskID, externalID string, settledAt time.Time) (bool, error)
	// ReleaseTaskExternalID clears external_id and external_id_settled_at on
	// the task holding (workspaceID, externalID), without deleting the task,
	// and bumps updated_at. Returns the task as it exists immediately after
	// the update, or nil if no task held the identity.
	ReleaseTaskExternalID(ctx context.Context, workspaceID, externalID string) (*models.Task, error)
}

// TaskStatusSummaryRepository stores the bounded task-level projection used by
// list and switcher surfaces. Implementations must compare revisions and the
// semantic payload atomically so retries and concurrent source observations do
// not regress a task or create a revision for a no-op.
type TaskStatusSummaryRepository interface {
	LoadTaskStatusSummaries(ctx context.Context, taskIDs []string) (map[string]*statussummary.TaskStatusSummary, error)
	CompareAndUpdateTaskStatusSummary(ctx context.Context, stored *statussummary.StoredTaskStatusSummary) (bool, error)
	DeleteTaskStatusSummary(ctx context.Context, taskID string) error
}

// TaskActivityRepository reconstructs the bounded task activity timestamp
// from authoritative task, prompt, and turn rows.
type TaskActivityRepository interface {
	LoadTaskLastActivity(ctx context.Context, taskIDs []string) (map[string]time.Time, error)
}

// TaskRepoRepository handles the task↔repository junction table (models.TaskRepository rows).
// Named TaskRepoRepository to reduce reader confusion with the TaskRepository sub-interface above.
type TaskRepoRepository interface {
	CreateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error
	GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error)
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	ListTaskRepositoriesByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*models.TaskRepository, error)
	UpdateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error
	// UpdateTaskRepositoryComparisonTarget atomically replaces or removes the
	// provider-owned comparison target on one exact attachment. When target is
	// nil, expected limits removal to the same provider change when supplied.
	UpdateTaskRepositoryComparisonTarget(ctx context.Context, id string, target *models.ComparisonTarget, expected *models.ComparisonTarget) (*models.TaskRepository, bool, error)
	// UpdateTaskRepositoryBaseBranchAndClearComparisonTarget changes the manual
	// base branch and clears any provider-owned comparison target in one write.
	UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx context.Context, id, baseBranch string) (*models.TaskRepository, bool, error)
	DeleteTaskRepository(ctx context.Context, id string) error
	DeleteTaskRepositoriesByTask(ctx context.Context, taskID string) error
	GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error)
}

// TaskWorkspaceFolderRepository handles canonical non-Git folder attachments.
// It is intentionally separate from TaskRepoRepository to preserve existing
// repository payload and Git-consumer contracts.
type TaskWorkspaceFolderRepository interface {
	ListTaskWorkspaceFolders(ctx context.Context, taskID string) ([]*models.TaskWorkspaceFolder, error)
	ListTaskWorkspaceFoldersByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*models.TaskWorkspaceFolder, error)
	CreateWorkspaceSourceBatch(ctx context.Context, batch *models.WorkspaceSourceBatch) error
	CompensateWorkspaceSourceBatch(ctx context.Context, batch *models.WorkspaceSourceBatch) error
}

// WorkflowRepository handles workflow CRUD.
type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *models.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*models.Workflow, error)
	UpdateWorkflow(ctx context.Context, workflow *models.Workflow) error
	DeleteWorkflow(ctx context.Context, id string) error
	ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*models.Workflow, error)
	ReorderWorkflows(ctx context.Context, workspaceID string, workflowIDs []string) error
}

// MessageRepository handles message persistence and lookups.
type MessageRepository interface {
	CreateMessage(ctx context.Context, message *models.Message) error
	GetMessage(ctx context.Context, id string) (*models.Message, error)
	// GetMessageWithPromptIndex retrieves a message by ID with its computed
	// prompt_index (1-based ordinal among the session's user messages).
	// Used by the idempotent WS replay/response path and user update-event
	// publication; hot-path reads stay on GetMessage.
	GetMessageWithPromptIndex(ctx context.Context, id string) (*models.Message, error)
	GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error)
	GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error)
	GetPermissionMessageByIdentity(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.Message, error)
	FindMessageByPendingID(ctx context.Context, pendingID string) (*models.Message, error)
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*models.Message, error)
	FindMessageByPendingIDAndQuestion(ctx context.Context, sessionID, pendingID, questionID string) (*models.Message, error)
	FindActiveClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*models.Message, error)
	GetPendingActionsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error)
	// ListPendingInteractions returns the durable request rows behind the
	// compact pending-action projection, under the same turn/session authority
	// (ADR 0052). Clarification bundles come back as one row per question.
	ListPendingInteractions(ctx context.Context, filter models.PendingInteractionFilter) ([]*models.Message, error)
	CompleteActiveClarificationBundle(ctx context.Context, pendingID, status string, responses map[string]interface{}) ([]*models.Message, bool, error)
	FinalizeClarificationResponseDelivery(ctx context.Context, pendingID, terminalStatus string, claimedMessages []*models.Message) ([]*models.Message, bool, error)
	RestoreActiveClarificationBundle(ctx context.Context, pendingID, terminalStatus string, claimedMessages []*models.Message) ([]*models.Message, bool, error)
	UpdateMessage(ctx context.Context, message *models.Message) error
	ClaimPermissionResolution(ctx context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error)
	FinalizePermissionResolution(ctx context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error)
	GetPermissionResolutionAudit(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.PermissionResolutionAudit, error)
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	ListMessagesByTurnID(ctx context.Context, turnID string) ([]*models.Message, error)
	ListMessagesPaginated(ctx context.Context, sessionID string, opts models.ListMessagesOptions) ([]*models.Message, bool, error)
	ListMessagesForPlugin(ctx context.Context, filter models.PluginMessageFilter) ([]*models.Message, error)
	SearchMessages(ctx context.Context, sessionID string, opts models.SearchMessagesOptions) ([]*models.Message, error)
	DeleteMessage(ctx context.Context, id string) error
}

// AttachmentRepository stores file-backed prompt attachment descriptors.
// Implementations must keep ownership and aggregate-claim checks in the same
// transaction as state transitions so a retry cannot partially claim a batch.
type AttachmentRepository interface {
	CreateMessageAttachment(ctx context.Context, attachment *models.TaskMessageAttachment) error
	GetMessageAttachment(ctx context.Context, id string) (*models.TaskMessageAttachment, error)
	ListMessageAttachments(ctx context.Context, ids []string) ([]*models.TaskMessageAttachment, error)
	ClaimMessageAttachments(ctx context.Context, ids []string, ownerID, workspaceID, taskID, sessionID string) error
	DeleteClaimedMessageAttachments(ctx context.Context, ids []string, ownerID, taskID, sessionID string) ([]*models.TaskMessageAttachment, error)
	DeleteMessageAttachmentsByTask(ctx context.Context, taskID string) ([]*models.TaskMessageAttachment, error)
	DeleteMessageAttachment(ctx context.Context, id, ownerID string) error
	MarkExpiredMessageAttachments(ctx context.Context, now time.Time) ([]*models.TaskMessageAttachment, error)
}

// TurnRepository handles conversation turn persistence.
type TurnRepository interface {
	CreateTurn(ctx context.Context, turn *models.Turn) error
	DeleteTurnIfUnreferenced(ctx context.Context, sessionID, turnID string) (bool, error)
	// ReconcileUnpublishedPromptTurns repairs or accepts durable prompt
	// reservations and response-delivery claims before startup admits new work.
	// Accepted reservations retain a durable start-event outbox marker for the
	// service to replay before it clears recovery metadata.
	// Every production turn store must provide this recovery boundary; callers
	// fail rather than skip it.
	ReconcileUnpublishedPromptTurns(ctx context.Context) (int, error)
	// ListTurnsPendingStartEvent returns accepted reservations whose durable
	// start-event outbox marker still needs replay before startup admits work.
	ListTurnsPendingStartEvent(ctx context.Context) ([]*models.Turn, error)
	// CreateTurnWithStepStamp creates turn atomically with the
	// workflow-step-at-start stamp: it reads the task's current step and
	// inserts the turn row in the same transaction, taking the same lock
	// readTaskStepInTx takes for step moves, so the stamp reflects a state
	// serialized against concurrent movers of the same task rather than a
	// plain unlocked read taken before the insert. A task-step read failure
	// (missing task, transient error) degrades to an unstamped turn rather
	// than failing turn creation. Returns whether the stamp was applied.
	CreateTurnWithStepStamp(ctx context.Context, turn *models.Turn) (stamped bool, err error)
	GetTurn(ctx context.Context, id string) (*models.Turn, error)
	GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error)
	UpdateTurn(ctx context.Context, turn *models.Turn) error
	// PatchTurnMetadata merges fields into an active or completed turn while
	// preserving unrelated metadata under the session's turn-write authority.
	PatchTurnMetadata(
		ctx context.Context,
		sessionID, turnID string,
		updates map[string]interface{},
	) (bool, time.Time, error)
	// UpdateActiveTurnMetadata merges updates and removes named keys only while
	// the turn is active and belongs to sessionID. Implementations serialize
	// this authority change with other current-turn decisions for the session.
	UpdateActiveTurnMetadata(
		ctx context.Context,
		sessionID, turnID string,
		updates map[string]interface{},
		removeKeys []string,
	) (bool, map[string]interface{}, time.Time, error)
	// ClearTurnPromptDispatchMetadata removes reservation-only metadata from
	// an active or completed turn after its start event has been accepted by
	// the event bus. It preserves unrelated concurrent metadata.
	ClearTurnPromptDispatchMetadata(
		ctx context.Context,
		sessionID, turnID string,
	) (bool, map[string]interface{}, time.Time, error)
	CompleteTurn(ctx context.Context, id string) error
	AbandonTurn(ctx context.Context, id string) error
	CompletePendingToolCallsForTurn(ctx context.Context, turnID string) (int64, error)
	ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error)
}

// SessionRepository handles task session lifecycle and workflow-session relationships.
type SessionRepository interface {
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error
	// ClaimPromptableTaskSessionIfActive marks a promptable session RUNNING only
	// while its owning task remains active. The state transition is the prompt
	// claim: archive either wins first or follows normal cancellation semantics.
	ClaimPromptableTaskSessionIfActive(ctx context.Context, id string) (models.PromptableTaskSessionClaim, error)
	ResetTaskSessionBasesForRepository(ctx context.Context, taskID, repositoryID, baseBranch string) (int64, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	CancelActiveTaskSessionsByTaskID(ctx context.Context, taskID, reason string) ([]*models.TaskSession, error)
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
	GetActiveTaskInfoByAgentProfile(ctx context.Context, agentProfileID string) ([]agentdto.ActiveTaskInfo, error)
	HasActiveTaskSessionsByExecutor(ctx context.Context, executorID string) (bool, error)
	HasActiveTaskSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error)
	HasActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (bool, error)
	CountActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (int, error)
	DeleteEphemeralTasksByAgentProfile(ctx context.Context, agentProfileID string) (int64, error)
	DeleteTaskSession(ctx context.Context, id string) error
	GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetPrimarySessionIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
	GetSessionCountsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error)
	GetPrimarySessionInfoByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error)
	// BatchGetSessionsByTaskIDs returns every session for the given task IDs
	// grouped by task ID, ordered by started_at DESC within each task. One
	// query (chunked to stay within SQLite's host-parameter limit) replaces
	// per-task GetSession loops on the task-list path.
	BatchGetSessionsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*models.TaskSession, error)
	SetSessionPrimary(ctx context.Context, sessionID string) error
	UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error
	UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
	SetSessionACPSessionID(ctx context.Context, sessionID, acpSessionID string) (bool, error)
	DismissLastAgentError(ctx context.Context, sessionID string, expected models.LastAgentError, dismissedAt time.Time) (bool, error)
	GetLastAgentMessage(ctx context.Context, sessionID string) (string, error)
	UpdateTaskSessionLastReadMessageID(ctx context.Context, id, messageID string) error
}

// SessionWorktreeRepository exposes session-scoped worktree projections over
// the task environment's repository rows. Sessions reference worktrees only
// through task_sessions.task_environment_id.
type SessionWorktreeRepository interface {
	UpdateTaskSessionWorktreeBranch(ctx context.Context, sessionID, branch string) error
	UpdateTaskSessionWorktreeBranchByRepository(ctx context.Context, sessionID, repositoryID, branch string) error
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskEnvironmentRepo, error)
	ListWorktreesBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]*models.TaskEnvironmentRepo, error)
}

// TaskResourceCleanupRepository persists restart-safe task lifecycle cleanup.
type TaskResourceCleanupRepository interface {
	CreateTaskResourceCleanupJob(ctx context.Context, job *models.TaskResourceCleanupJob) error
	HasActiveTaskResourceCleanupJob(ctx context.Context, taskID string) (bool, error)
	TouchPreparedTaskResourceCleanupJob(ctx context.Context, operationID string) error
	UpdateTaskResourceCleanupSnapshot(ctx context.Context, operationID, snapshot string) error
	// UpdateClaimedTaskResourceCleanupSnapshot persists outcomes produced by one
	// exact running cleanup attempt. A newer retry or cancellation wins when
	// the claim no longer matches.
	UpdateClaimedTaskResourceCleanupSnapshot(ctx context.Context, id string, attempt int, snapshot string) (bool, error)
	GetTaskResourceCleanupJob(ctx context.Context, id string) (*models.TaskResourceCleanupJob, error)
	GetTaskResourceCleanupJobByOperationID(ctx context.Context, operationID string) (*models.TaskResourceCleanupJob, error)
	ListPreparedTaskResourceCleanupJobs(ctx context.Context) ([]*models.TaskResourceCleanupJob, error)
	ListDueTaskResourceCleanupJobs(ctx context.Context, now time.Time, limit int) ([]*models.TaskResourceCleanupJob, error)
	StartPreparedTaskResourceCleanupJob(ctx context.Context, id string) (bool, error)
	MarkTaskResourceCleanupJobRunning(ctx context.Context, id string) (bool, error)
	CompleteClaimedTaskResourceCleanupJob(ctx context.Context, id string, attempt int, state models.TaskResourceCleanupState, lastError string, nextAttemptAt *time.Time) (bool, error)
	CompleteTaskResourceCleanupJob(ctx context.Context, id string, state models.TaskResourceCleanupState, lastError string, nextAttemptAt *time.Time) error
	CancelArchiveTaskResourceCleanupJobs(ctx context.Context, taskID string) error
	ResetRunningTaskResourceCleanupJobs(ctx context.Context) error
}

// GitSnapshotRepository handles git snapshots and session commit records.
type GitSnapshotRepository interface {
	CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error
	GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)
	GetLatestGitSnapshotsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]*models.GitSnapshot, error)
	GetLatestGitSnapshotByTaskEnvironmentID(ctx context.Context, taskEnvironmentID string) (*models.GitSnapshot, error)
	GetLatestGitSnapshotsByTaskEnvironmentIDs(ctx context.Context, taskEnvironmentIDs []string) (map[string]*models.GitSnapshot, error)
	GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx context.Context, taskEnvironmentIDs []string) ([]*models.GitSnapshot, error)
	GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)
	GetGitSnapshotsBySession(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error)
	CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) (bool, error)
	GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error)
	GetLatestSessionCommit(ctx context.Context, sessionID string) (*models.SessionCommit, error)
	DeleteSessionCommit(ctx context.Context, id string) error
}

// RepositoryEntityRepository handles git repository entity CRUD and repository scripts.
// Named RepositoryEntityRepository to avoid conflation with the Repository interface itself;
// mirrors the sqlite/repository_entity.go implementation file.
type RepositoryEntityRepository interface {
	CreateRepository(ctx context.Context, repository *models.Repository) error
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	UpdateRepository(ctx context.Context, repository *models.Repository) error
	DeleteRepository(ctx context.Context, id string) error
	ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error)
	CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error
	GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error)
	UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error
	DeleteRepositoryScript(ctx context.Context, id string) error
	ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error)
	ListScriptsByRepositoryIDs(ctx context.Context, repoIDs []string) (map[string][]*models.RepositoryScript, error)
	GetRepositoryByProviderIdentity(ctx context.Context, identity models.ProviderRepositoryIdentity) (*models.Repository, error)
	// GetRepositoryByLocalPath finds a live repository by workspace and canonical
	// local_path. Returns nil, nil if not found. Used by
	// Service.FindOrCreateRepositoryByLocalPath to check for an existing row by
	// canonical path immediately before insert (serialized via repoResolveMu),
	// instead of relying solely on a batch snapshot that can go stale across
	// concurrent callers within this process. This closes the common
	// single-process race; it is not a substitute for a database-level
	// uniqueness constraint against writers outside this process.
	GetRepositoryByLocalPath(ctx context.Context, workspaceID, localPath string) (*models.Repository, error)
}

// RepositorySetRepository stores named, reusable groups of workspace
// repositories. Membership order is authoritative: writes assign contiguous
// positions from the supplied order, and reads return items in that order with
// soft-deleted and out-of-workspace repositories excluded.
type RepositorySetRepository interface {
	CreateRepositorySet(ctx context.Context, set *models.RepositorySet) error
	GetRepositorySet(ctx context.Context, id string) (*models.RepositorySet, error)
	// GetRepositorySetByName compares the name case-insensitively and returns
	// nil, nil when it is unused, leaving the conflict decision to the caller.
	GetRepositorySetByName(ctx context.Context, workspaceID, name string) (*models.RepositorySet, error)
	ListRepositorySets(ctx context.Context, workspaceID string) ([]*models.RepositorySet, error)
	// ListRepositorySetIDsByRepository reports which sets hold a repository, so a
	// caller can publish their new shape after a deletion prunes membership.
	ListRepositorySetIDsByRepository(ctx context.Context, repositoryID string) ([]string, error)
	// UpdateRepositorySet writes the set's fields and, when repositoryIDs is
	// non-nil, replaces its whole membership in the same transaction so the two
	// cannot land apart. A nil repositoryIDs leaves membership untouched.
	UpdateRepositorySet(ctx context.Context, set *models.RepositorySet, repositoryIDs *[]string) error
	DeleteRepositorySet(ctx context.Context, id string) (bool, error)
}

// RepositoryBranchPolicyRepository stores reusable branch workflows owned by
// a repository. The batch method is an atomic, one-time Gitflow starter.
type RepositoryBranchPolicyRepository interface {
	CreateRepositoryBranchPolicy(ctx context.Context, policy *models.RepositoryBranchPolicy) error
	GetRepositoryBranchPolicy(ctx context.Context, id string) (*models.RepositoryBranchPolicy, error)
	GetRepositoryBranchPolicyByName(ctx context.Context, repositoryID, name string) (*models.RepositoryBranchPolicy, error)
	ListRepositoryBranchPolicies(ctx context.Context, repositoryID string) ([]*models.RepositoryBranchPolicy, error)
	ListRepositoryBranchPoliciesByWorkspace(ctx context.Context, workspaceID string) ([]*models.RepositoryBranchPolicy, error)
	UpdateRepositoryBranchPolicy(ctx context.Context, policy *models.RepositoryBranchPolicy) error
	DeleteRepositoryBranchPolicy(ctx context.Context, id string) (bool, error)
	CreateRepositoryBranchPoliciesIfEmpty(ctx context.Context, repositoryID string, policies []*models.RepositoryBranchPolicy) error
}

// RepositorySecretBindingRepository stores normalized repository environment
// references. It is optional on RepositoryEntityRepository to keep legacy
// adapters source-compatible while the SQLite implementation rolls out.
type RepositorySecretBindingRepository interface {
	ListRepositorySecretBindings(ctx context.Context, repositoryID string) ([]*models.RepositorySecretBinding, error)
	ListRepositorySecretBindingsByRepositoryIDs(ctx context.Context, repositoryIDs []string) (map[string][]*models.RepositorySecretBinding, error)
	ReplaceRepositorySecretBindings(ctx context.Context, repositoryID string, bindings []models.RepositorySecretBinding) error
}

// RepositorySecretBindingMutator adds atomic repository-plus-binding writes.
type RepositorySecretBindingMutator interface {
	RepositorySecretBindingRepository
	CreateRepositoryWithSecretBindings(ctx context.Context, repository *models.Repository, bindings []models.RepositorySecretBinding) error
	UpdateRepositoryWithSecretBindings(ctx context.Context, repository *models.Repository, bindings []models.RepositorySecretBinding) error
}

// RepositoryCleanupRepository performs guarded deletion of repositories
// created during workspace-source attachment rollback.
type RepositoryCleanupRepository interface {
	// DeleteRepositoryIfUnreferenced soft-deletes a repository only when no
	// task_repositories row currently adopts it. The predicate is part of the
	// mutation so rollback cleanup cannot delete a repository another task won.
	DeleteRepositoryIfUnreferenced(ctx context.Context, id string) (bool, error)
}

// ExecutorRepository handles executor CRUD, executor profiles, and running state.
type ExecutorRepository interface {
	CreateExecutor(ctx context.Context, executor *models.Executor) error
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	UpdateExecutor(ctx context.Context, executor *models.Executor) error
	DeleteExecutor(ctx context.Context, id string) error
	ListExecutors(ctx context.Context) ([]*models.Executor, error)
	CreateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
	UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error
	DeleteExecutorProfile(ctx context.Context, id string) error
	ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error)
	ListAllExecutorProfiles(ctx context.Context) ([]*models.ExecutorProfile, error)
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	ListExecutorsRunningByTaskID(ctx context.Context, taskID string) ([]*models.ExecutorRunning, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error
	// HasExecutorRunningRow returns true if a row exists for the session.
	// Used to decide "session has been launched at least once" without loading the full row.
	HasExecutorRunningRow(ctx context.Context, sessionID string) (bool, error)
	// UpdateResumeToken performs a CAS-style narrow update of resume_token + last_message_uuid
	// scoped to the row's current agent_execution_id. If the row's agent_execution_id no longer
	// matches expectedExecID (i.e. a new execution has taken over), returns models.ErrExecutionRotated
	// and writes nothing. Use when persisting state from a specific execution that may have been
	// replaced concurrently — typically resume tokens emitted by ACP session events.
	UpdateResumeToken(ctx context.Context, sessionID, expectedExecID, resumeToken, lastMessageUUID string) error
	// UpdateExecutorRunningStatus performs a narrow status update on the row.
	// Used when the agent process is intentionally not being started (prepare-only
	// launch) so the row doesn't sit on the misleading default "starting" forever.
	// Returns models.ErrExecutorRunningNotFound if no row exists for the session.
	UpdateExecutorRunningStatus(ctx context.Context, sessionID, status string) error
	// RepairExecutorRunningDead repairs a row in place to reflect a dead backing
	// process (status=stopped, local_pid cleared, last_seen re-stamped) while
	// preserving resume_token/worktree/endpoint. Used by cleanup paths to honor
	// the resume-safety invariant instead of deleting a resumable row.
	// Returns models.ErrExecutorRunningNotFound if no row exists for the session.
	RepairExecutorRunningDead(ctx context.Context, sessionID string) error
}

// EnvironmentRepository handles environment CRUD.
type EnvironmentRepository interface {
	CreateEnvironment(ctx context.Context, environment *models.Environment) error
	GetEnvironment(ctx context.Context, id string) (*models.Environment, error)
	UpdateEnvironment(ctx context.Context, environment *models.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	ListEnvironments(ctx context.Context) ([]*models.Environment, error)
}

// TaskEnvironmentRepository handles per-task execution environment instances
// and their per-repository child rows.
type TaskEnvironmentRepository interface {
	CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	DeleteTaskEnvironment(ctx context.Context, id string) error
	DeleteTaskEnvironmentsByTask(ctx context.Context, taskID string) error
	CreateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error)
	UpdateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	DeleteTaskEnvironmentRepo(ctx context.Context, id string) error
	DeleteTaskEnvironmentReposByEnv(ctx context.Context, envID string) error
}

// ReviewRepository handles session file review records.
type ReviewRepository interface {
	UpsertSessionFileReview(ctx context.Context, review *models.SessionFileReview) error
	GetSessionFileReviews(ctx context.Context, sessionID string) ([]*models.SessionFileReview, error)
	DeleteSessionFileReviews(ctx context.Context, sessionID string) error
}

// DocumentRepository handles task document CRUD and revision history.
// Documents generalize plans: each document is identified by a unique key within a task.
type DocumentRepository interface {
	CreateDocument(ctx context.Context, doc *models.TaskDocument) error
	GetDocument(ctx context.Context, taskID, key string) (*models.TaskDocument, error)
	UpdateDocument(ctx context.Context, doc *models.TaskDocument) error
	DeleteDocument(ctx context.Context, taskID, key string) error
	ListDocuments(ctx context.Context, taskID string) ([]*models.TaskDocument, error)

	// Revision history
	InsertDocumentRevision(ctx context.Context, rev *models.TaskDocumentRevision) error
	GetLatestDocumentRevision(ctx context.Context, taskID, key string) (*models.TaskDocumentRevision, error)
	ListDocumentRevisions(ctx context.Context, taskID, key string, limit int) ([]*models.TaskDocumentRevision, error)
	GetDocumentRevision(ctx context.Context, id string) (*models.TaskDocumentRevision, error)
	NextDocumentRevisionNumber(ctx context.Context, taskID, key string) (int, error)
	// WriteDocumentRevision atomically upserts the HEAD document and writes/merges a revision
	// in a single transaction. Pass a non-nil coalesceLatestID to merge into an existing revision;
	// otherwise a new revision is appended with revision_number computed inside the tx.
	WriteDocumentRevision(ctx context.Context, head *models.TaskDocument, rev *models.TaskDocumentRevision, coalesceLatestID *string) error
}

// PlanRepository handles task plan CRUD and its revision history.
type PlanRepository interface {
	CreateTaskPlan(ctx context.Context, plan *models.TaskPlan) error
	GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error)
	UpdateTaskPlan(ctx context.Context, plan *models.TaskPlan) error
	MarkTaskPlanImplementationStarted(ctx context.Context, taskID, sessionID, actor string) (*models.TaskPlan, error)
	DeleteTaskPlan(ctx context.Context, taskID string) error

	// Revision history
	InsertTaskPlanRevision(ctx context.Context, rev *models.TaskPlanRevision) error
	UpdateTaskPlanRevision(ctx context.Context, rev *models.TaskPlanRevision) error
	GetTaskPlanRevision(ctx context.Context, id string) (*models.TaskPlanRevision, error)
	GetLatestTaskPlanRevision(ctx context.Context, taskID string) (*models.TaskPlanRevision, error)
	ListTaskPlanRevisions(ctx context.Context, taskID string, limit int) ([]*models.TaskPlanRevision, error)
	NextTaskPlanRevisionNumber(ctx context.Context, taskID string) (int, error)
	// WritePlanRevision atomically upserts the HEAD plan and writes/merges a revision in a
	// single transaction. Pass a non-nil coalesceLatestID to merge into an existing revision;
	// otherwise a new revision is appended with revision_number computed inside the tx.
	//
	// preserveTitle and preserveCreatedBy gate the HEAD upsert's ON CONFLICT branch only: when
	// true, an existing row keeps its stored title / created_by rather than taking head's value.
	// They have no effect on a fresh insert, which always uses head's value. Callers set a flag
	// only when the value they would otherwise overwrite with could not be read (see
	// docs/specs/tasks/system-design/plan-write-consistency.md, "Existing behavior that must
	// change"); every other caller passes false.
	WritePlanRevision(ctx context.Context, head *models.TaskPlan, rev *models.TaskPlanRevision, coalesceLatestID *string, preserveTitle, preserveCreatedBy bool) error
}

// SubagentContextRepository persists the durable, queryable record of a
// subagent (Task tool) invocation. See
// docs/specs/agents/requirements/subagent-context-persistence.md.
type SubagentContextRepository interface {
	// UpsertSubagentContext inserts or merges one subagent invocation row,
	// keyed on (task_session_id, tool_call_id). A single atomic statement —
	// no read-then-write.
	UpsertSubagentContext(ctx context.Context, sc *models.SubagentContext) error
	ListSubagentContextsBySession(ctx context.Context, sessionID string) ([]*models.SubagentContext, error)
	ListSubagentContextsByTurn(ctx context.Context, turnID string) ([]*models.SubagentContext, error)
}

// UsageRepository serves the task-cost-ledger read surface
// (docs/specs/task-cost-ledger/spec.md AC-18, AC-19, AC-20): per-task and
// per-session aggregate totals over task_usage_events. The ledger write path
// (CreateTaskUsageEvent, ListTaskUsageEvents) is deliberately not part of
// this interface - it is consumed only by internal/task/usage's own narrow
// Repository interface, never through the Service layer.
type UsageRepository interface {
	GetTaskUsageTotals(ctx context.Context, taskID string) (*models.TaskUsageTotals, error)
	GetSessionUsageTotals(ctx context.Context, sessionID string) (*models.TaskUsageTotals, error)
}
