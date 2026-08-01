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

// TaskMRAutomationOptions stores task-level MR lifecycle notification
// preferences. Parallel to github.TaskCIOptions, scoped to the three
// lifecycle switches only — GitLab has no auto-fix/auto-merge automation.
type TaskMRAutomationOptions struct {
	TaskID                  string    `json:"task_id" db:"task_id"`
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
	PromptOnReviewRequested *bool
	PromptOnMerged          *bool
	PromptOnClosed          *bool
}

// HasAny reports whether the patch contains at least one requested field change.
func (p TaskMRAutomationPatch) HasAny() bool {
	return p.PromptOnReviewRequested != nil || p.PromptOnMerged != nil || p.PromptOnClosed != nil
}

// TaskMRAutomationResponse is the HTTP/MCP shape for task MR automation
// options, including the per-MR lifecycle checkpoints for observability.
type TaskMRAutomationResponse struct {
	TaskID                  string                  `json:"task_id"`
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
	LastError                *string    `json:"last_error,omitempty" db:"last_error"`
	CreatedAt                time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at" db:"updated_at"`
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
