package sqlite

import (
	"errors"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ErrTaskNotFound is returned by Repository methods (GetTask, UpdateTask,
// DeleteTask, UpdateTaskState, …) when no row matches the supplied id.
// Callers should classify via errors.Is rather than substring-matching the
// formatted message, which includes the task id and is therefore brittle.
var ErrTaskNotFound = repoerrors.ErrTaskNotFound

// ErrMessageNotFound is returned when a message cursor or around target no
// longer exists. Callers should classify it with errors.Is.
var ErrMessageNotFound = repoerrors.ErrMessageNotFound

// ErrWorkspaceNotFound is returned by Repository workspace methods when no row
// matches the supplied id.
var ErrWorkspaceNotFound = repoerrors.ErrWorkspaceNotFound

// ErrTaskPlanNotFound is returned by Repository task-plan methods when no row
// matches the supplied task id.
var ErrTaskPlanNotFound = repoerrors.ErrTaskPlanNotFound

// ErrTaskEnvironmentNotFound is returned when no task environment row matches
// the supplied id. Callers should classify it with errors.Is.
var ErrTaskEnvironmentNotFound = repoerrors.ErrTaskEnvironmentNotFound

// ErrNoPrimarySession is returned by GetPrimarySessionByTaskID when the task
// has no primary session row. Callers should use errors.Is to distinguish this
// "not found" case from genuine backend/DB errors.
var ErrNoPrimarySession = errors.New("no primary session")

// ErrExternalIDConflict is returned by CreateTask (and its
// admission/capacity variants) when the insert violates
// uniq_tasks_external_id — the TOCTOU backstop for the create sequence's
// step-3 lookup. Callers should re-read by (workspace_id, external_id) and
// return the winner rather than treating this as a hard failure.
var ErrExternalIDConflict = repoerrors.ErrExternalIDConflict

// ErrOfficeSessionRaceConflict is returned by CreateOfficeTaskSession (and,
// for repositories that don't implement the office creator interface, by the
// equivalent in-process fallback in executor_office.go's
// persistOfficeSessionFallback) when an in-transaction guard finds a row
// already live for the same (task_id, agent_profile_id) pair — i.e. two
// callers raced past their lookup-then-create for the same pair while both
// rows are "live" (any state other than COMPLETED, FAILED, or CANCELLED).
// The guard is a SELECT ... WHERE ... AND state NOT IN (...) check inside
// CreateOfficeTaskSession's own transaction (PostgreSQL's row lock and
// SQLite's single writer connection make that check-then-insert atomic
// against concurrent callers), not a database constraint — deliberately
// scoped to the office creation path only, never table-wide: a table-wide
// unique index on this pair broke live kanban-relaunch and
// workflow-replacement flows, which intentionally create a second live
// session for the same pair (see
// TestCreateStartSession_KanbanRunnerCreatesDistinctSession). Terminal rows
// for the same pair are unaffected — the guard only refuses when a live row
// exists. Callers should re-read and reuse the winning row rather than
// treating this as a hard failure (see executor_office.go's
// EnsureSessionForAgentWithCreation).
var ErrOfficeSessionRaceConflict = errors.New("office task session race conflict")

// ErrWorkflowResolutionConflict is returned by UpdateTaskIfWorkflowMatches and
// the workflow-step admission writers when a caller-supplied expected
// workflow id no longer matches the row's persisted workflow_id, checked
// atomically inside the write transaction. See repoerrors.ErrWorkflowResolutionConflict.
var ErrWorkflowResolutionConflict = repoerrors.ErrWorkflowResolutionConflict
