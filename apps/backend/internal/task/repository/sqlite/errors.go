package sqlite

import "errors"

// ErrTaskNotFound is returned by Repository methods (GetTask, UpdateTask,
// DeleteTask, UpdateTaskState, …) when no row matches the supplied id.
// Callers should classify via errors.Is rather than substring-matching the
// formatted message, which includes the task id and is therefore brittle.
var ErrTaskNotFound = errors.New("task not found")

// ErrOfficeSessionRaceConflict is returned by CreateTaskSession when the
// insert violates the uniq_office_task_session partial unique index — i.e.
// two callers raced past their SELECT-then-INSERT for the same
// (task_id, agent_profile_id) pair. Callers should re-read and reuse the
// winning row rather than treating this as a hard failure.
var ErrOfficeSessionRaceConflict = errors.New("office task session race conflict")
