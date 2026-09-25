package service

import (
	"context"
	"errors"
	"maps"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// deferredLaunchPromptCASRetryBudget bounds the read-compare-write retry for
// a concurrent, legitimate deferred_launch writer (the session ceiling's own
// admission machinery) mutating a different field while this edit is
// in flight. Mirrors the orchestrator's own deferredLaunchCASRetryBudget.
const deferredLaunchPromptCASRetryBudget = 3

var (
	// ErrNoPendingDeferredLaunch means the task carries no deferred launch
	// record to edit. Either it never had one, or it has already been consumed
	// by the gate that was waiting or by a direct start.
	ErrNoPendingDeferredLaunch = errors.New("task has no pending deferred launch")
	// ErrDeferredLaunchAlreadyStarted means the task already has an agent
	// session, so its deferred launch prompt can no longer decide anything.
	ErrDeferredLaunchAlreadyStarted = errors.New("task has already started")
	// ErrDeferredLaunchPromptEmpty rejects a blank replacement prompt. Clearing
	// the prompt would make the eventual launch fall back to the description,
	// which is a different operation and never what an edit meant.
	ErrDeferredLaunchPromptEmpty = errors.New("deferred launch prompt must not be empty")
)

// ReadCeilingDeferredLaunch returns the authoritative ceiling record used by
// message admission and task-state reconciliation. An absent record is a
// valid negative result. A repository failure, or a record that explicitly
// claims the ceiling half but cannot be decoded, is returned so a caller
// never changes task state while queue ownership is uncertain.
func (s *Service) ReadCeilingDeferredLaunch(
	ctx context.Context, taskID string,
) (models.CeilingDeferral, bool, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return models.CeilingDeferral{}, false, err
	}
	record, _, err := s.tasks.GetTaskDeferredLaunch(ctx, taskID)
	if err != nil {
		return models.CeilingDeferral{}, false, err
	}
	if record == nil {
		return models.CeilingDeferral{}, false, nil
	}
	ceilingFlag, hasCeilingFlag := record[models.CeilingDeferredKey]
	if !hasCeilingFlag || ceilingFlag != true {
		return models.CeilingDeferral{}, false, nil
	}
	deferral, err := models.ReadCeilingDeferral(record)
	if err != nil {
		return models.CeilingDeferral{}, false, err
	}
	return deferral, true, nil
}

// UpdateDeferredLaunchPrompt rewrites the prompt a task will be launched with
// once its gate opens (dependencies resolve, or WIP capacity frees up).
//
// A deferred launch can sit unfired for hours. In a long-running program the
// brief written at create time goes stale — it was written before the work
// learned most of what mattered — and until now there was no way to correct it:
// update_task_kandev only reached title/description/state, so the only
// workaround was writing "your prompt is stale" into the description and hoping
// the agent read it.
//
// It refuses once the task has started, in either of the two ways that can be
// true, so the caller is never told it changed a prompt that nothing will read:
// the record itself is gone (consumed by the gate or by a direct start), or a
// session already exists on the task.
//
// # Why the write is a value-compared, retried single-key patch
//
// The obvious implementation — read the task, check it has no session, write
// the task back — reintroduces the very bug this file exists to fix. A start
// landing between the check and the write consumes the intent with an atomic
// RemoveTaskMetadataKey, and then the full-row UpdateTask, still holding the
// metadata it read before, RESURRECTS the key. The gate fires a second session
// on a task that is already running, which is exactly the production symptom.
//
// So the write goes through GetTaskDeferredLaunch/SetTaskDeferredLaunchIfUnchanged,
// the session ceiling's own compare-and-set protocol, rather than
// SetTaskMetadataKeyIfPresent: presence-only is not enough, because the ceiling's
// admission machinery (for example the replay sweeper) can legitimately rewrite
// other fields on this same record while this edit is in flight, and a
// presence-only write would silently overwrite that change with the stale copy
// this function read earlier. A lost comparison against an unchanged prior is
// retried, bounded by deferredLaunchPromptCASRetryBudget; a lost comparison
// against a record the retry finds gone means a concurrent start consumed it,
// which reports the same ErrDeferredLaunchAlreadyStarted a presence-only write
// would have. The session check stays because it gives the common case a
// precise error instead of a bare conflict; it is not what makes the update
// safe.
func (s *Service) UpdateDeferredLaunchPrompt(ctx context.Context, taskID, prompt string) (*models.Task, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, ErrDeferredLaunchPromptEmpty
	}
	launch, prior, err := s.tasks.GetTaskDeferredLaunch(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if launch == nil {
		return nil, ErrNoPendingDeferredLaunch
	}
	// The session check runs after establishing the record is present, so a
	// concurrent start's atomic claim landing during this call (rather than
	// before this function started) is caught below by the compare-and-set
	// losing against the prior this function already captured, not
	// misreported as "never had one".
	started, err := s.taskHasAnySession(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if started {
		return nil, ErrDeferredLaunchAlreadyStarted
	}

	for attempt := 0; attempt < deferredLaunchPromptCASRetryBudget; attempt++ {
		updated := make(map[string]interface{}, len(launch)+1)
		maps.Copy(updated, launch)
		updated["prompt"] = prompt
		if payload, ok := launch[models.CeilingLaunchPayloadKey].(map[string]interface{}); ok {
			updatedPayload := make(map[string]interface{}, len(payload)+1)
			maps.Copy(updatedPayload, payload)
			updatedPayload["prompt"] = prompt
			updated[models.CeilingLaunchPayloadKey] = updatedPayload
		}
		stored, lostCompare, err := s.tasks.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil {
			return nil, err
		}
		if stored {
			// Re-read rather than returning the patched in-memory copy: the row
			// now carries whatever else changed while this ran, and handing
			// back a stale snapshot is how the resurrection bug got in.
			task, err := s.tasks.GetTask(ctx, taskID)
			if err != nil {
				return nil, err
			}
			s.PublishTaskUpdated(ctx, task)
			return task, nil
		}
		if !lostCompare {
			return nil, ErrDeferredLaunchAlreadyStarted
		}
		launch, prior, err = s.tasks.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if launch == nil {
			return nil, ErrDeferredLaunchAlreadyStarted
		}
	}
	return nil, ErrDeferredLaunchAlreadyStarted
}

// taskHasAnySession reports whether any session row exists for the task,
// whatever its state. A cancelled or failed session still means the task was
// started, so this is deliberately not filtered by state.
func (s *Service) taskHasAnySession(ctx context.Context, taskID string) (bool, error) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	return len(sessions) > 0, nil
}
