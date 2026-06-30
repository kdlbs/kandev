package sqlite

import (
	"errors"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ErrTaskNotFound is returned by Repository methods (GetTask, UpdateTask,
// DeleteTask, UpdateTaskState, …) when no row matches the supplied id.
// Callers should classify via errors.Is rather than substring-matching the
// formatted message, which includes the task id and is therefore brittle.
var ErrTaskNotFound = errors.New("task not found")

// ErrWorkspaceNotFound is returned by Repository workspace methods when no row
// matches the supplied id.
var ErrWorkspaceNotFound = repoerrors.ErrWorkspaceNotFound

// ErrNoPrimarySession is returned by GetPrimarySessionByTaskID when the task
// has no primary session row. Callers should use errors.Is to distinguish this
// "not found" case from genuine backend/DB errors.
var ErrNoPrimarySession = errors.New("no primary session")

// ErrOfficeSessionRaceConflict is returned by CreateTaskSession when the
// insert violates the uniq_office_task_session partial unique index — i.e.
// two callers raced past their SELECT-then-INSERT for the same
// (task_id, agent_profile_id) pair. Callers should re-read and reuse the
// winning row rather than treating this as a hard failure.
var ErrOfficeSessionRaceConflict = errors.New("office task session race conflict")
