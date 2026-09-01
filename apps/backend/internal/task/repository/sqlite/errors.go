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

// ErrOfficeSessionRaceConflict is classified by isOfficeTaskSessionUniqueViolation
// (office_task_session_uniqueness.go) and returned by every session-create
// method that routes through the unexported createTaskSession —
// CreateTaskSession, CreateTaskSessionWithInitialRuntimeSeed,
// CreateTaskSessionWithWorkspaceBinding,
// CreateTaskSessionWithSharedGroupWorkspaceBinding, and
// CreateOfficeTaskSession — and by every full-row session update that routes
// through the unexported updateTaskSessionWithStateGuard — UpdateTaskSession,
// UpdateTaskSessionWithMetadata, UpdateTaskSessionIfCurrentState, and
// UpdateTaskSessionIfCurrentStateRemovingMetadataKeys — when a write violates
// uniq_office_task_session — i.e. two callers raced past their
// SELECT-then-INSERT (or a resume raced a fresh create) for the same
// (task_id, agent_profile_id) pair while both rows
// are "live" (CREATED, STARTING, RUNNING, IDLE, or WAITING_FOR_INPUT).
// Terminal rows for the same pair are unaffected — the index only
// constrains the live set. Callers should re-read and reuse the winning row
// rather than treating this as a hard failure (see
// executor_office.go's EnsureSessionForAgentWithCreation).
//
// uniq_office_task_session does NOT exist in the schema yet: a table-wide
// version of it broke live kanban-relaunch and workflow-replacement flows,
// which intentionally create a second live session for the same pair (see
// TestCreateStartSession_KanbanRunnerCreatesDistinctSession). Scoping the
// index to office sessions only is tracked as Kandev task
// 05864a73-2dd9-4b15-98ab-35643d9d55e4; until it lands, this sentinel cannot
// fire and the guarded call sites are dead code kept for that follow-up to
// activate.
var ErrOfficeSessionRaceConflict = errors.New("office task session race conflict")

// ErrWorkflowResolutionConflict is returned by UpdateTaskIfWorkflowMatches and
// the workflow-step admission writers when a caller-supplied expected
// workflow id no longer matches the row's persisted workflow_id, checked
// atomically inside the write transaction. See repoerrors.ErrWorkflowResolutionConflict.
var ErrWorkflowResolutionConflict = repoerrors.ErrWorkflowResolutionConflict
