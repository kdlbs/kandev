package shared

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// AgentReader provides read access to agent instances.
// Implemented by the agents feature (and transitionally by office/service.Service).
type AgentReader interface {
	// GetAgentInstance looks up an agent by ID or name.
	GetAgentInstance(ctx context.Context, idOrName string) (*models.AgentInstance, error)
	// ListAgentInstances returns all agent instances for a workspace.
	// An empty wsID returns agents across all workspaces.
	ListAgentInstances(ctx context.Context, wsID string) ([]*models.AgentInstance, error)
	// ListAgentInstancesByIDs returns agent instances whose ids are in `ids`,
	// in unspecified order. Rows missing from the DB are omitted. An empty
	// input returns an empty slice.
	ListAgentInstancesByIDs(ctx context.Context, ids []string) ([]*models.AgentInstance, error)
}

// AgentWriter provides write access to agent status.
// Implemented by the agents feature (and transitionally by office/service.Service).
type AgentWriter interface {
	// UpdateAgentStatusFields persists a new status and optional pause reason for an agent.
	UpdateAgentStatusFields(ctx context.Context, agentID, status, pauseReason string) error
}

// RunQueuer enqueues run requests for agent instances.
// Implemented by the run feature (and transitionally by office/service.Service).
type RunQueuer interface {
	// QueueRun enqueues a run for agentInstanceID with the given reason, payload,
	// and optional idempotency key (empty string disables deduplication).
	QueueRun(ctx context.Context, agentInstanceID, reason, payload, idempotencyKey string) error
}

// ActivityLogger logs activity entries across office features.
// Implemented by the ActivityLoggerImpl in shared/activity.go.
type ActivityLogger interface {
	// LogActivity records an activity entry. Errors are logged but not returned.
	LogActivity(ctx context.Context, wsID, actorType, actorID, action, targetType, targetID, details string)
	// LogActivityWithRun records an activity entry tagged with the originating
	// office run id (and optional session id). Use this from agent-driven
	// mutation paths so the run detail page's Tasks Touched surface can join
	// activity rows back to the run that produced them.
	LogActivityWithRun(ctx context.Context, wsID, actorType, actorID, action, targetType, targetID, details, runID, sessionID string)
}

// CostChecker reads cost data for budget and dashboard features.
// Implemented by the costs feature (and transitionally by office/service.Service).
type CostChecker interface {
	// GetCostSummary returns the total spend in subcents (hundredths of a
	// cent) for a workspace.
	GetCostSummary(ctx context.Context, wsID string) (int64, error)
}

// BudgetChecker evaluates budget policies before or after execution.
// Implemented by the budgets feature (and transitionally by office/service.Service).
type BudgetChecker interface {
	// CheckPreExecutionBudget returns (allowed, reason, error).
	// If allowed is false, the caller should skip execution.
	CheckPreExecutionBudget(ctx context.Context, agentInstanceID, projectID, workspaceID string) (bool, string, error)
}

// SkillReader provides read access to skill definitions.
// Implemented by the skills feature (and transitionally by office/service.Service).
type SkillReader interface {
	// GetSkillFromConfig looks up a skill by ID or slug.
	GetSkillFromConfig(ctx context.Context, idOrSlug string) (*models.Skill, error)
	// ListSkillsFromConfig returns all skills for a workspace.
	// An empty workspaceID returns skills across all workspaces.
	ListSkillsFromConfig(ctx context.Context, workspaceID string) ([]*models.Skill, error)
}

// ProjectReader provides read access to project records.
// Implemented by the projects feature (and transitionally by office/service.Service).
type ProjectReader interface {
	// GetProjectFromConfig looks up a project by ID or name.
	GetProjectFromConfig(ctx context.Context, idOrName string) (*models.Project, error)
	// ListProjectsFromConfig returns all projects for a workspace.
	// An empty workspaceID returns projects across all workspaces.
	ListProjectsFromConfig(ctx context.Context, workspaceID string) ([]*models.Project, error)
}

// RoutineReader provides read access to routine records.
// Implemented by the routines feature (and transitionally by office/service.Service).
type RoutineReader interface {
	// GetRoutineFromConfig looks up a routine by ID or name.
	GetRoutineFromConfig(ctx context.Context, idOrName string) (*models.Routine, error)
	// ListRoutinesFromConfig returns all routines for a workspace.
	// An empty workspaceID returns routines across all workspaces.
	ListRoutinesFromConfig(ctx context.Context, workspaceID string) ([]*models.Routine, error)
}

// PendingPermission is a simplified, office-safe view of a pending
// clarification/permission request for display in the office inbox.
type PendingPermission struct {
	PendingID string
	SessionID string
	TaskID    string
	Prompt    string
	Context   string
	CreatedAt time.Time
}

// PermissionLister provides a snapshot of pending permission/clarification requests.
// Implemented by clarification.Store (or any wrapper around it).
type PermissionLister interface {
	// ListPendingPermissions returns all pending permission requests.
	// The returned slice is a snapshot; callers must not modify its elements.
	ListPendingPermissions() []PendingPermission
}

// ModelPricing carries per-million-token pricing in subcents. Duplicates
// the costs.ModelPricing shape so the shared package doesn't depend on
// the costs package.
type ModelPricing struct {
	InputPerMillion       int64
	CachedReadPerMillion  int64
	CachedWritePerMillion int64
	OutputPerMillion      int64
}

// PricingLookup resolves per-model pricing. Implemented by the
// office/costs/modelsdev package. Returns (zero, false) when the model
// is unknown or the cache hasn't warmed yet.
type PricingLookup interface {
	LookupForModel(ctx context.Context, modelID string) (ModelPricing, bool)
}

// SessionUsageWriter increments the cumulative tokens/cost columns on
// task_sessions when a cost event lands. Implemented by the task repo.
type SessionUsageWriter interface {
	IncrementTaskSessionUsage(ctx context.Context, sessionID string,
		tokensIn, tokensOut, costSubcents int64) error
}
