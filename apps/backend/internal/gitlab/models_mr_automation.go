package gitlab

import "time"

// mrLifecycleEvent* identify a lifecycle transition delivered to a task's
// agent session. They are declared as a separate constant block from the MR
// *state* constants (mrStateOpen, gitlabStateClosed, gitlabStateMerged,
// gitlabStateLocked in client_helpers.go / constants.go) even though the
// merged/closed strings coincide, so a future rename of one vocabulary can't
// silently change the other.
const (
	mrLifecycleEventReviewRequested = "review_requested"
	mrLifecycleEventMerged          = "merged"
	mrLifecycleEventClosed          = "closed"
)

// TaskMRAutoFixMaxRounds is the server-enforced MR auto-fix loop guard.
// Mirrors github.TaskCIAutoFixMaxRounds.
const TaskMRAutoFixMaxRounds = 10

// TaskMRAutomationOptions stores task-level MR automation preferences:
// the three lifecycle notification switches from #2125, plus auto-fix CI
// and auto-merge (this task). Parallel to github.TaskCIOptions.
type TaskMRAutomationOptions struct {
	TaskID                  string    `json:"task_id" db:"task_id"`
	AutoFixEnabled          bool      `json:"auto_fix_enabled" db:"auto_fix_enabled"`
	AutoMergeEnabled        bool      `json:"auto_merge_enabled" db:"auto_merge_enabled"`
	AutoFixPromptOverride   *string   `json:"auto_fix_prompt_override,omitempty" db:"auto_fix_prompt_override"`
	PromptOnReviewRequested bool      `json:"prompt_on_review_requested" db:"prompt_on_review_requested"`
	PromptOnMerged          bool      `json:"prompt_on_merged" db:"prompt_on_merged"`
	PromptOnClosed          bool      `json:"prompt_on_closed" db:"prompt_on_closed"`
	ReviewReviewerUsername  string    `json:"review_reviewer_username" db:"review_reviewer_username"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}

// TaskMRAutomationPatch is a partial update for task MR automation options.
// ReviewReviewerUsername is intentionally absent — it is server-resolved from
// the workspace's authenticated GitLab user, never client-supplied.
type TaskMRAutomationPatch struct {
	AutoFixEnabled          *bool
	AutoMergeEnabled        *bool
	AutoFixPromptOverride   *string
	PromptOnReviewRequested *bool
	PromptOnMerged          *bool
	PromptOnClosed          *bool
}

// HasAny reports whether the patch contains at least one requested field change.
func (p TaskMRAutomationPatch) HasAny() bool {
	return p.AutoFixEnabled != nil || p.AutoMergeEnabled != nil || p.AutoFixPromptOverride != nil ||
		p.PromptOnReviewRequested != nil || p.PromptOnMerged != nil || p.PromptOnClosed != nil
}

// TaskMRAutomationResponse is the HTTP/MCP shape for task MR automation
// options, including the per-MR lifecycle checkpoints for observability.
type TaskMRAutomationResponse struct {
	TaskID                  string                  `json:"task_id"`
	AutoFixEnabled          bool                    `json:"auto_fix_enabled"`
	AutoMergeEnabled        bool                    `json:"auto_merge_enabled"`
	AutoFixPromptOverride   *string                 `json:"auto_fix_prompt_override"`
	AutoFixMaxRounds        int                     `json:"auto_fix_max_rounds"`
	EffectiveAutoFixPrompt  string                  `json:"effective_auto_fix_prompt"`
	UsingDefaultPrompt      bool                    `json:"using_default_prompt"`
	PromptOnReviewRequested bool                    `json:"prompt_on_review_requested"`
	PromptOnMerged          bool                    `json:"prompt_on_merged"`
	PromptOnClosed          bool                    `json:"prompt_on_closed"`
	ReviewReviewerUsername  string                  `json:"review_reviewer_username"`
	UpdatedAt               time.Time               `json:"updated_at"`
	MRStates                []*TaskMRLifecycleState `json:"mr_states"`
	// WorkspaceID is internal routing metadata (best-effort resolved, may be
	// empty) that lets the websocket broadcaster scope the
	// gitlab.task_mr_options.updated event to the owning workspace instead of
	// falling back to a global broadcast. It must stay JSON-visible (not
	// `json:"-"`): the NATS-backed event bus round-trips event.Data through
	// JSON, so a hidden field would silently vanish before the broadcaster's
	// map-based workspace_id extraction ever saw it, defeating the whole
	// scoping fix in any deployment using NATS. Surfacing it in the HTTP/MCP
	// response is not a leak — it's the task's own workspace.
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// TaskMRAutomationEvaluation is the narrow, internal snapshot consumed by
// lifecycle evaluation. Unlike TaskMRAutomationResponse, it contains only
// the target MR checkpoint rather than every checkpoint for the task.
type TaskMRAutomationEvaluation struct {
	Options    *TaskMRAutomationResponse
	Checkpoint *TaskMRLifecycleState
}

// GetWorkspaceID implements the websocket broadcaster's workspace-routing
// interface (internal/gateway/websocket.extractWorkspaceID) — a struct
// payload's fields are otherwise invisible to that generic extractor.
func (r *TaskMRAutomationResponse) GetWorkspaceID() string {
	if r == nil {
		return ""
	}
	return r.WorkspaceID
}

// TaskMRLifecycleState is the per-MR dedupe/checkpoint row that guards
// against re-firing a lifecycle prompt for a transition already observed.
// Keyed by (task_id, repository_id, project_path, mr_iid) — RepositoryID may
// be empty for single-repo tasks.
type TaskMRLifecycleState struct {
	TaskID                   string     `json:"task_id" db:"task_id"`
	RepositoryID             string     `json:"repository_id" db:"repository_id"`
	ProjectPath              string     `json:"project_path" db:"project_path"`
	MRIID                    int        `json:"mr_iid" db:"mr_iid"`
	ReviewRequestInitialized bool       `json:"review_request_initialized" db:"review_request_initialized"`
	LastReviewRequested      bool       `json:"last_review_requested" db:"last_review_requested"`
	LastObservedState        string     `json:"last_observed_state" db:"last_observed_state"`
	LastLifecycleEvent       string     `json:"last_lifecycle_event" db:"last_lifecycle_event"`
	LastLifecyclePromptAt    *time.Time `json:"last_lifecycle_prompt_at,omitempty" db:"last_lifecycle_prompt_at"`
	LastLifecycleSessionID   *string    `json:"last_lifecycle_session_id,omitempty" db:"last_lifecycle_session_id"`
	// LastError records a lifecycle-delivery failure (evaluate/dispatch), set
	// by the orchestrator and cleared only by a subsequent successful prompt
	// delivery. LastSyncError records a poller sync failure (SyncTaskMRStrict)
	// and is owned exclusively by the poller. The two are intentionally
	// separate columns: a successful sync must not erase an unresolved
	// delivery error, and vice versa.
	LastError     *string `json:"last_error,omitempty" db:"last_error"`
	LastSyncError *string `json:"last_sync_error,omitempty" db:"last_sync_error"`
	// The following back auto-fix CI and auto-merge (this task), mirroring
	// github.TaskCIPRAutomationState's equivalent columns.
	LastFixSignature      string     `json:"last_fix_signature" db:"last_fix_signature"`
	LastFixCheckpointJSON string     `json:"last_fix_checkpoint_json" db:"last_fix_checkpoint_json"`
	LastFixEnqueuedAt     *time.Time `json:"last_fix_enqueued_at,omitempty" db:"last_fix_enqueued_at"`
	LastFixSessionID      *string    `json:"last_fix_session_id,omitempty" db:"last_fix_session_id"`
	AutoFixRoundCount     int        `json:"auto_fix_round_count" db:"auto_fix_round_count"`
	AutoFixExhaustedAt    *time.Time `json:"auto_fix_exhausted_at,omitempty" db:"auto_fix_exhausted_at"`
	LastMergeSignature    string     `json:"last_merge_signature" db:"last_merge_signature"`
	LastMergeAttemptAt    *time.Time `json:"last_merge_attempt_at,omitempty" db:"last_merge_attempt_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskMRLifecyclePrompt records an accepted lifecycle prompt checkpoint.
type TaskMRLifecyclePrompt struct {
	TaskID          string
	RepositoryID    string
	ProjectPath     string
	MRIID           int
	Event           string
	SessionID       string
	PromptedAt      time.Time
	ReviewRequested bool
	ObservedState   string
}

// TaskMRFixAttempt records an auto-fix prompt attempt for a task MR.
// Mirrors github.TaskCIFixAttempt.
type TaskMRFixAttempt struct {
	TaskID         string
	RepositoryID   string
	ProjectPath    string
	MRIID          int
	Signature      string
	CheckpointJSON string
	SessionID      string
	EnqueuedAt     time.Time
	IncrementRound bool
}

// TaskMRMergeAttempt records an auto-merge attempt for a task MR. Mirrors
// github.TaskCIMergeAttempt.
type TaskMRMergeAttempt struct {
	TaskID       string
	RepositoryID string
	ProjectPath  string
	MRIID        int
	Signature    string
	AttemptedAt  time.Time
}
