package service

import (
	"context"
	"errors"
	"maps"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

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
// # Why the write is a conditional single-key patch
//
// The obvious implementation — read the task, check it has no session, write
// the task back — reintroduces the very bug this file exists to fix. A start
// landing between the check and the write consumes the intent with an atomic
// RemoveTaskMetadataKey, and then the full-row UpdateTask, still holding the
// metadata it read before, RESURRECTS the key. The gate fires a second session
// on a task that is already running, which is exactly the production symptom.
//
// So the write goes through SetTaskMetadataKeyIfPresent, which patches the one
// key and only while it is still there. A concurrent start wins the race and
// this returns ErrDeferredLaunchAlreadyStarted, with nothing written. The
// session check stays because it gives the common case a precise error instead
// of a bare conflict; it is not what makes the update safe.
func (s *Service) UpdateDeferredLaunchPrompt(ctx context.Context, taskID, prompt string) (*models.Task, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, ErrDeferredLaunchPromptEmpty
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	launch, ok := pendingDeferredLaunch(task)
	if !ok {
		return nil, ErrNoPendingDeferredLaunch
	}
	started, err := s.taskHasAnySession(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if started {
		return nil, ErrDeferredLaunchAlreadyStarted
	}

	updated := make(map[string]interface{}, len(launch)+1)
	maps.Copy(updated, launch)
	updated["prompt"] = prompt
	written, err := s.tasks.SetTaskMetadataKeyIfPresent(ctx, taskID, models.MetaKeyDeferredLaunch, updated)
	if err != nil {
		return nil, err
	}
	if !written {
		return nil, ErrDeferredLaunchAlreadyStarted
	}

	// Re-read rather than returning the patched in-memory copy: the row now
	// carries whatever else changed while this ran, and handing back a stale
	// snapshot is how the resurrection bug got in.
	stored, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	s.PublishTaskUpdated(ctx, stored)
	return stored, nil
}

// pendingDeferredLaunch returns the task's deferred launch record when it is
// present and well-formed.
func pendingDeferredLaunch(task *models.Task) (map[string]interface{}, bool) {
	if task == nil || task.Metadata == nil {
		return nil, false
	}
	launch, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	return launch, ok
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
