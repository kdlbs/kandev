package automation

import (
	"context"

	"github.com/jmoiron/sqlx"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ExportAgentProfileLookup resolves the full agent profile row an
// automation's agent_profile_id references, within the single read
// transaction AC-29 opens for the whole export. Satisfied directly by
// agent/settings/store's concrete repository (GetAgentProfileTx), which
// already returns the three-outcome (value, found, err) shape AC-19
// requires — this interface exists only to let the automation package
// depend on that method without importing the settings store package.
//
// Deliberately not the existing AgentProfileLookup: that interface only
// answers whether a profile exists, and the export needs AgentName/Model/Mode
// to build exportAgentProfile.
type ExportAgentProfileLookup interface {
	GetAgentProfileTx(ctx context.Context, tx *sqlx.Tx, id string) (*settingsmodels.AgentProfile, bool, error)
}

// ExportExecutorProfileLookup resolves the full executor profile row an
// automation's executor_profile_id references, within the export's shared
// transaction. No existing interface in this package answers this at all —
// nothing here validates executor_profile_id today.
type ExportExecutorProfileLookup interface {
	GetExecutorProfileTx(ctx context.Context, tx *sqlx.Tx, id string) (*taskmodels.ExecutorProfile, bool, error)
}

// ExportWorkflowLookup resolves the full workflow row an automation's
// workflow_id references. Deliberately not WorkflowLocator: that interface
// returns only the owning workspace ID for an authorization check, not the
// workflow's name.
type ExportWorkflowLookup interface {
	GetWorkflowTx(ctx context.Context, tx *sqlx.Tx, id string) (*taskmodels.Workflow, bool, error)
}

// ExportWorkflowStepLookup resolves the full workflow step row an
// automation's workflow_step_id references.
type ExportWorkflowStepLookup interface {
	GetStepTx(ctx context.Context, tx *sqlx.Tx, id string) (*workflowmodels.WorkflowStep, bool, error)
}

// ExportRepositoryLookup resolves the full repository row a repository_ids
// entry references. Deliberately not the existing RepositoryLookup: that
// interface returns (workspaceID, defaultBranch, ok) for cross-workspace
// validation, not the repository's name, and its two-valued (value, ok) shape
// cannot distinguish "not found" from "lookup failed" — the collapse AC-19
// forbids for the export path.
type ExportRepositoryLookup interface {
	GetRepositoryTx(ctx context.Context, tx *sqlx.Tx, id string) (*taskmodels.Repository, bool, error)
}
