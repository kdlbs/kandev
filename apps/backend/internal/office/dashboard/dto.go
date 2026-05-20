// Package dashboard provides dashboard, inbox, activity, and workspace-level git HTTP handlers.
package dashboard

import "github.com/kandev/kandev/internal/office/models"

// RunActivityDay holds aggregated run outcome counts for a single calendar day.
type RunActivityDay struct {
	Date      string `json:"date"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Other     int    `json:"other"`
}

// TaskBreakdown holds task counts bucketed by status category.
type TaskBreakdown struct {
	Open       int `json:"open"`
	InProgress int `json:"in_progress"`
	Blocked    int `json:"blocked"`
	Done       int `json:"done"`
}

// RecentTaskDTO represents a single recently-updated task for the dashboard.
type RecentTaskDTO struct {
	ID                     string `json:"id"`
	Identifier             string `json:"identifier"`
	Title                  string `json:"title"`
	Status                 string `json:"status"`
	AssigneeAgentProfileID string `json:"assigneeAgentProfileId,omitempty"`
	UpdatedAt              string `json:"updatedAt"`
}

// LiveRunDTO represents a single agent run (active or recently finished).
type LiveRunDTO struct {
	AgentID        string `json:"agentId"`
	AgentName      string `json:"agentName"`
	TaskID         string `json:"taskId"`
	TaskTitle      string `json:"taskTitle"`
	TaskIdentifier string `json:"taskIdentifier"`
	Status         string `json:"status"`
	DurationMs     int64  `json:"durationMs"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt,omitempty"`
}

// LiveRunsResponse wraps a list of live run DTOs.
type LiveRunsResponse struct {
	Runs []LiveRunDTO `json:"runs"`
}

// AgentSummariesResponse wraps the list returned by GET
// /workspaces/:wsId/agent-summaries. The AgentSummary struct (defined in
// service.go) already carries snake_case JSON tags so it serialises
// directly without an extra DTO layer.
type AgentSummariesResponse struct {
	Agents []AgentSummary `json:"agents"`
}

// DashboardResponse wraps the full dashboard data.
type DashboardResponse struct {
	AgentCount         int                     `json:"agent_count"`
	RunningCount       int                     `json:"running_count"`
	PausedCount        int                     `json:"paused_count"`
	ErrorCount         int                     `json:"error_count"`
	TasksInProgress    int                     `json:"tasks_in_progress"`
	OpenTasks          int                     `json:"open_tasks"`
	BlockedTasks       int                     `json:"blocked_tasks"`
	MonthSpendSubcents int64                   `json:"month_spend_subcents"`
	PendingApprovals   int                     `json:"pending_approvals"`
	RecentActivity     []*models.ActivityEntry `json:"recent_activity"`
	TaskCount          int                     `json:"task_count"`
	SkillCount         int                     `json:"skill_count"`
	RoutineCount       int                     `json:"routine_count"`
	RunActivity        []RunActivityDay        `json:"run_activity"`
	TaskBreakdown      TaskBreakdown           `json:"task_breakdown"`
	RecentTasks        []RecentTaskDTO         `json:"recent_tasks"`
	// AgentSummaries is the per-agent card payload, embedded in the
	// dashboard response so the dashboard renders in a single round-trip
	// (Stream A + G of office optimization). Always non-nil; empty array
	// for workspaces with no agents.
	AgentSummaries []AgentSummary `json:"agent_summaries"`
}

// InboxResponse wraps inbox items. TotalCount mirrors len(Items) and is
// included so the sidebar badge can read its count from the same payload
// that drives the inbox page (Stream F of office optimization), avoiding a
// separate /inbox?count=true round-trip.
type InboxResponse struct {
	Items      []*models.InboxItem `json:"items"`
	TotalCount int                 `json:"total_count"`
}

// InboxCountResponse wraps the inbox item count.
type InboxCountResponse struct {
	Count int `json:"count"`
}

// ActivityListResponse wraps a list of activity entries.
type ActivityListResponse struct {
	Activity []*models.ActivityEntry `json:"activity"`
}

// RunListResponse wraps a list of run requests. RunListItem is used in
// place of *models.Run so the list surface can expose the task_id that
// is otherwise buried inside the JSON-encoded payload.
type RunListResponse struct {
	Runs []RunListItem `json:"runs"`
}

// RunListItem mirrors models.Run with the JSON keys flattened plus a
// derived task_id pulled from the run payload. The frontend run-queue
// surfaces (and the approval-flow E2E poll) read task_id directly.
type RunListItem struct {
	ID                   string  `json:"id"`
	AgentProfileID       string  `json:"agent_profile_id"`
	Reason               string  `json:"reason"`
	Payload              string  `json:"payload"`
	Status               string  `json:"status"`
	CoalescedCount       int     `json:"coalesced_count"`
	IdempotencyKey       *string `json:"idempotency_key"`
	ContextSnapshot      string  `json:"context_snapshot"`
	RetryCount           int     `json:"retry_count"`
	ScheduledRetryAt     *string `json:"scheduled_retry_at,omitempty"`
	CancelReason         *string `json:"cancel_reason,omitempty"`
	ErrorMessage         string  `json:"error_message,omitempty"`
	RequestedAt          string  `json:"requested_at"`
	ClaimedAt            *string `json:"claimed_at,omitempty"`
	FinishedAt           *string `json:"finished_at,omitempty"`
	TaskID               string  `json:"task_id,omitempty"`
	RoutingBlockedStatus string  `json:"routing_blocked_status,omitempty"`
}

// AgentRunSummaryDTO is one row in the per-agent paginated runs list.
// It mirrors models.Run with snake_case JSON keys plus a derived
// short id (first 8 chars) so the frontend doesn't need to recompute
// the truncation in every cell.
type AgentRunSummaryDTO struct {
	ID           string  `json:"id"`
	IDShort      string  `json:"id_short"`
	Reason       string  `json:"reason"`
	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
	TaskID       string  `json:"task_id,omitempty"`
	// CommentID is set for runs triggered by a task comment so the
	// frontend can deeplink the row to the originating comment.
	CommentID string `json:"comment_id,omitempty"`
	// RoutineID is set for runs triggered by a routine cron fire so
	// the frontend can deeplink the row to the routine.
	RoutineID   string `json:"routine_id,omitempty"`
	RequestedAt string `json:"requested_at"`
	ClaimedAt   string `json:"claimed_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	DurationMs  int64  `json:"duration_ms,omitempty"`
}

// AgentRunsListResponse wraps the paginated runs list. NextCursor is
// the requested_at of the last row in the current page (RFC3339);
// empty when this is the final page.
type AgentRunsListResponse struct {
	Runs       []AgentRunSummaryDTO `json:"runs"`
	NextCursor string               `json:"next_cursor"`
	NextID     string               `json:"next_id,omitempty"`
}

// RunCostSummaryDTO is the per-run token + cost rollup surfaced in
// the run detail header. CostSubcents stores hundredths of a cent.
type RunCostSummaryDTO struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CachedTokens int64 `json:"cached_tokens"`
	CostSubcents int64 `json:"cost_subcents"`
}

// RunInvocationDTO carries the per-run invocation details: the
// adapter family + working directory + optional command/env/prompt
// context. Populated best-effort from the agent instance + payload.
type RunInvocationDTO struct {
	Adapter    string            `json:"adapter,omitempty"`
	Model      string            `json:"model,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Command    string            `json:"command,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

// RunSessionDTO carries the session ids associated with a run.
// SessionIDBefore / SessionIDAfter are placeholders today (we don't
// track them explicitly); v1 surfaces only the run's claimed session
// id.
type RunSessionDTO struct {
	SessionID       string `json:"session_id,omitempty"`
	SessionIDBefore string `json:"session_id_before,omitempty"`
	SessionIDAfter  string `json:"session_id_after,omitempty"`
}

// RunEventDTO mirrors models.RunEvent with stable JSON keys for the
// frontend events log.
type RunEventDTO struct {
	Seq       int    `json:"seq"`
	EventType string `json:"event_type"`
	Level     string `json:"level"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

// RunDetailResponse is the per-run aggregate returned by
// GET /agents/:id/runs/:runId. It composes the run row with the cost
// rollup, session ids, invocation panel, tasks-touched ids, and the
// events log so the run detail page can render in a single
// round-trip.
//
// PR 1 of office-heartbeat-rework added the AssembledPrompt /
// SummaryInjected / ResultJSON / ContextSnapshot / OutputSummary
// fields so the run-detail UI can render the exact prompt the agent
// received plus the structured adapter output. Today these are
// populated for every dispatch (the persistence is unconditional);
// the prepended-summary slice (SummaryInjected) only fires on
// taskless runs and stays empty for the normal task-bound path.
type RunDetailResponse struct {
	ID           string            `json:"id"`
	IDShort      string            `json:"id_short"`
	AgentID      string            `json:"agent_id"`
	Reason       string            `json:"reason"`
	Status       string            `json:"status"`
	CancelReason *string           `json:"cancel_reason,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	TaskID       string            `json:"task_id,omitempty"`
	RequestedAt  string            `json:"requested_at"`
	ClaimedAt    string            `json:"claimed_at,omitempty"`
	FinishedAt   string            `json:"finished_at,omitempty"`
	DurationMs   int64             `json:"duration_ms,omitempty"`
	Costs        RunCostSummaryDTO `json:"costs"`
	Session      RunSessionDTO     `json:"session"`
	Invocation   RunInvocationDTO  `json:"invocation"`
	Runtime      RunRuntimeDTO     `json:"runtime"`
	TasksTouched []string          `json:"tasks_touched"`
	Events       []RunEventDTO     `json:"events"`
	// AssembledPrompt is the final prompt string the agent received,
	// captured at dispatch. Empty for runs that pre-date PR 1.
	AssembledPrompt string `json:"assembled_prompt,omitempty"`
	// SummaryInjected is the continuation-summary slice prepended to
	// the prompt at dispatch (taskless runs only). Empty for task-
	// bound runs.
	SummaryInjected string `json:"summary_injected,omitempty"`
	// ResultJSON is the structured adapter output captured at run
	// completion. Defaults to "{}" for runs that never received one.
	ResultJSON string `json:"result_json,omitempty"`
	// ContextSnapshot mirrors runs.context_snapshot — the runtime
	// context the run dispatcher built at claim time. Surfaced for
	// debugging.
	ContextSnapshot string `json:"context_snapshot,omitempty"`
	// OutputSummary mirrors runs.output_summary — the free-form
	// agent output captured at run finish. Kept alongside ResultJSON
	// because legacy adapters populate this and not result_json.
	OutputSummary string `json:"output_summary,omitempty"`
	// Routing carries the provider-routing snapshot for the run +
	// every recorded route attempt (skipped + launched). Empty when
	// the run did not go through the routing path.
	Routing *RunRouting `json:"routing,omitempty"`
}

type RunRuntimeDTO struct {
	Capabilities  map[string]interface{} `json:"capabilities"`
	InputSnapshot map[string]interface{} `json:"input_snapshot"`
	SessionID     string                 `json:"session_id,omitempty"`
	Skills        []RunSkillDTO          `json:"skills"`
}

type RunSkillDTO struct {
	SkillID          string `json:"skill_id"`
	Version          string `json:"version"`
	ContentHash      string `json:"content_hash"`
	MaterializedPath string `json:"materialized_path"`
}

// TaskSearchResultDTO represents a single task in search results.
type TaskSearchResultDTO struct {
	ID                     string `json:"id"`
	WorkspaceID            string `json:"workspaceId"`
	Identifier             string `json:"identifier"`
	Title                  string `json:"title"`
	Description            string `json:"description,omitempty"`
	Status                 string `json:"status"`
	Priority               string `json:"priority"`
	ParentID               string `json:"parentId,omitempty"`
	ProjectID              string `json:"projectId,omitempty"`
	AssigneeAgentProfileID string `json:"assigneeAgentProfileId,omitempty"`
	Labels                 string `json:"labels,omitempty"`
	CreatedAt              string `json:"createdAt"`
	UpdatedAt              string `json:"updatedAt"`
}

// TaskSearchResponse wraps search results.
type TaskSearchResponse struct {
	Tasks []*TaskSearchResultDTO `json:"tasks"`
}

// LabelDTO represents a label attached to an issue.
type LabelDTO struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// TaskDTO represents an office task for the frontend.
type TaskDTO struct {
	ID                     string         `json:"id"`
	WorkspaceID            string         `json:"workspaceId"`
	Identifier             string         `json:"identifier"`
	Title                  string         `json:"title"`
	Description            string         `json:"description,omitempty"`
	Status                 string         `json:"status"`
	Priority               string         `json:"priority"`
	ParentID               string         `json:"parentId,omitempty"`
	ProjectID              string         `json:"projectId,omitempty"`
	AssigneeAgentProfileID string         `json:"assigneeAgentProfileId,omitempty"`
	Labels                 []LabelDTO     `json:"labels"`
	Children               []*TaskDTO     `json:"children,omitempty"`
	BlockedBy              []string       `json:"blockedBy,omitempty"`
	Reviewers              []string       `json:"reviewers"`
	Approvers              []string       `json:"approvers"`
	Decisions              []*DecisionDTO `json:"decisions,omitempty"`
	CreatedAt              string         `json:"createdAt"`
	UpdatedAt              string         `json:"updatedAt"`
	// IsSystem flags tasks that live in a kandev-managed system
	// workflow (today: standing coordination; future: routine-fired).
	// The Office Tasks UI hides these by default and surfaces a small
	// "System" badge when the dev toggle reveals them.
	IsSystem bool `json:"isSystem,omitempty"`
}

// TaskListResponse wraps a list of tasks.
type TaskListResponse struct {
	Tasks []*TaskDTO `json:"tasks"`
}

// TimelineEventDTO represents a single chronological event in a task timeline.
type TimelineEventDTO struct {
	Type string `json:"type"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	At   string `json:"at"`
}

// TaskResponse wraps a single task with its timeline.
type TaskResponse struct {
	Task     *TaskDTO           `json:"task"`
	Timeline []TimelineEventDTO `json:"timeline"`
}

// CommentDTO represents a single task comment.
type CommentDTO struct {
	ID         string `json:"id"`
	TaskID     string `json:"taskId"`
	AuthorType string `json:"authorType"`
	AuthorID   string `json:"authorId"`
	Body       string `json:"body"`
	Source     string `json:"source"`
	CreatedAt  string `json:"createdAt"`
	// Per-comment run lifecycle. Only populated for user comments
	// whose comment_created subscriber queued a task_comment run
	// (idempotency_key = "task_comment:<comment_id>"). The frontend
	// reads these to render a Queued / Working / Failed badge on the
	// triggering comment.
	RunID     string `json:"runId,omitempty"`
	RunStatus string `json:"runStatus,omitempty"`
	RunError  string `json:"runError,omitempty"`
}

// CommentListResponse wraps a list of comments.
type CommentListResponse struct {
	Comments []*CommentDTO `json:"comments"`
}

// CommentResponse wraps a single comment.
type CommentResponse struct {
	Comment *CommentDTO `json:"comment"`
}

// CreateCommentRequest is the request body for creating a comment.
type CreateCommentRequest struct {
	Body       string `json:"body"`
	AuthorType string `json:"author_type"`
}

// UpdateWorkspaceSettingsRequest is the request body for updating workspace settings.
type UpdateWorkspaceSettingsRequest struct {
	Name                             *string `json:"name"`
	Description                      *string `json:"description"`
	PermissionHandlingMode           *string `json:"permission_handling_mode"`
	RecoveryLookbackHours            *int    `json:"recovery_lookback_hours"`
	RequireApprovalForNewAgents      *bool   `json:"require_approval_for_new_agents"`
	RequireApprovalForTaskCompletion *bool   `json:"require_approval_for_task_completion"`
	RequireApprovalForSkillChanges   *bool   `json:"require_approval_for_skill_changes"`
}

// WorkspaceSettingsResponse wraps the workspace settings for GET /settings.
type WorkspaceSettingsResponse struct {
	Settings *WorkspaceSettings `json:"settings"`
}

// GitCloneRequest is the request body for cloning a workspace from a git repo.
type GitCloneRequest struct {
	RepoURL       string `json:"repoUrl"`
	Branch        string `json:"branch"`
	WorkspaceName string `json:"workspaceName"`
}

// GitPushRequest is the request body for pushing workspace changes.
type GitPushRequest struct {
	Message string `json:"message"`
}

// GitStatusResponse wraps the git status for a workspace.
type GitStatusResponse struct {
	IsGit       bool   `json:"is_git"`
	Branch      string `json:"branch,omitempty"`
	IsDirty     bool   `json:"is_dirty"`
	HasRemote   bool   `json:"has_remote"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	CommitCount int    `json:"commit_count"`
}

// DocumentDTO represents a task document for the frontend.
type DocumentDTO struct {
	ID         string `json:"id"`
	TaskID     string `json:"task_id"`
	Key        string `json:"key"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Content    string `json:"content,omitempty"`
	AuthorKind string `json:"author_kind"`
	AuthorName string `json:"author_name"`
	Filename   string `json:"filename,omitempty"`
	MimeType   string `json:"mime_type,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// DocumentRevisionDTO represents a single revision of a task document.
type DocumentRevisionDTO struct {
	ID                 string  `json:"id"`
	TaskID             string  `json:"task_id"`
	DocumentKey        string  `json:"document_key"`
	RevisionNumber     int     `json:"revision_number"`
	Title              string  `json:"title"`
	Content            string  `json:"content"`
	AuthorKind         string  `json:"author_kind"`
	AuthorName         string  `json:"author_name"`
	RevertOfRevisionID *string `json:"revert_of_revision_id,omitempty"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

// DocumentListResponse wraps a list of task documents.
type DocumentListResponse struct {
	Documents []*DocumentDTO `json:"documents"`
}

// DocumentResponse wraps a single task document.
type DocumentResponse struct {
	Document *DocumentDTO `json:"document"`
}

// DocumentRevisionListResponse wraps a list of document revisions.
type DocumentRevisionListResponse struct {
	Revisions []*DocumentRevisionDTO `json:"revisions"`
}

// DocumentRevisionResponse wraps a single document revision.
type DocumentRevisionResponse struct {
	Revision *DocumentRevisionDTO `json:"revision"`
}

// CreateOrUpdateDocumentRequest is the request body for creating or updating a document.
type CreateOrUpdateDocumentRequest struct {
	Type       string `json:"type"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	AuthorKind string `json:"author_kind"`
	AuthorName string `json:"author_name"`
}

// PermissionMeta describes a single permission for frontend rendering.
type PermissionMeta struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"` // "bool" or "int"
}

// MetaResponse contains all office metadata for the frontend.
type MetaResponse struct {
	Statuses           []models.StatusMeta               `json:"statuses"`
	Priorities         []models.PriorityMeta             `json:"priorities"`
	Roles              []models.RoleMeta                 `json:"roles"`
	ExecutorTypes      []models.ExecutorTypeMeta         `json:"executorTypes"`
	SkillSourceTypes   []models.SkillSourceTypeMeta      `json:"skillSourceTypes"`
	ProjectStatuses    []models.ProjectStatusMeta        `json:"projectStatuses"`
	AgentStatuses      []models.AgentStatusMeta          `json:"agentStatuses"`
	RoutineRunStatuses []models.RoutineRunStatusMeta     `json:"routineRunStatuses"`
	InboxItemTypes     []models.InboxItemTypeMeta        `json:"inboxItemTypes"`
	Permissions        []PermissionMeta                  `json:"permissions"`
	PermissionDefaults map[string]map[string]interface{} `json:"permissionDefaults"`
}
