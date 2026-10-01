package coordinator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// ErrConversationConflict is returned by OpenConversation when a concurrent
// context/profile change or a losing create race means the caller must retry
// with the coordinator's fresh state
// (docs/specs/coordinator/system-design/copilot.md#conversation-task, steps 4
// and 7).
var ErrConversationConflict = errors.New("coordinator: conversation conflict")

// ErrConversationSessionUnavailable is returned by OpenConversation when
// EnsureSession fails but the coordinator still exists (step 6): the task is
// kept as current, so the caller's next open retries only that step.
var ErrConversationSessionUnavailable = errors.New("coordinator: conversation session unavailable")

// ProfileUnavailableError is returned by OpenConversation when the
// coordinator's agent profile is missing or passthrough, or its executor
// profile is missing (coordinators.md#validation).
type ProfileUnavailableError struct {
	AgentStatus    ProfileStatus
	ExecutorStatus ProfileStatus
}

func (e *ProfileUnavailableError) Error() string {
	return "coordinator: profile unavailable"
}

// ConversationTaskManager is the task-service surface the conversation route
// and its cleanup pass need: create, read, archive and delete a coordinator's
// conversation task, and enumerate every coordinator-origin task
// (docs/specs/coordinator/system-design/copilot.md#conversation-task).
// Satisfied by the task service.
type ConversationTaskManager interface {
	CreateTask(ctx context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error)
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	UpdateTask(ctx context.Context, id string, req *taskservice.UpdateTaskRequest) (*taskmodels.Task, error)
	ArchiveTask(ctx context.Context, id string) error
	DeleteTask(ctx context.Context, id string) error
	ListCoordinatorOriginTasks(ctx context.Context, workspaceID string) ([]*taskmodels.Task, error)
}

// SessionEnsurer is the orchestrator surface the conversation route needs to
// ensure a session for the conversation task exists, without starting an
// agent (copilot.md#conversation-task, step 6). Satisfied by the orchestrator
// service.
type SessionEnsurer interface {
	EnsureSession(ctx context.Context, taskID string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error)
}

// ConversationResult is OpenConversation's success shape. ArchiveState is
// always false: a task the route would otherwise return as archived is
// returned as ErrConversationConflict instead.
type ConversationResult struct {
	TaskID    string
	SessionID string
}

// SetConversationDeps registers the task and session dependencies
// OpenConversation and the conversation cleanup pass need
// (copilot.md#wiring). Callers must also call SetConversationHooks (commonly
// with ArchiveClearedConversationTask and DeleteConversationTasksForCoordinator)
// before ConversationDepsReady is checked by the route registration.
func (s *Service) SetConversationDeps(tasks ConversationTaskManager, sessions SessionEnsurer) {
	s.conversationTasks = tasks
	s.conversationSessions = sessions
}

// ConversationDepsReady reports whether SetConversationDeps has been called.
// registerCoordinatorConversation checks this before registering the
// conversation route, so a missed wiring call 404s instead of running with a
// nil dependency (copilot.md#wiring).
func (s *Service) ConversationDepsReady() bool {
	return s.conversationTasks != nil && s.conversationSessions != nil
}

// ArchiveClearedConversationTask is the ConversationClearedHook
// implementation wireCoordinatorConversation registers: a context/profile
// change already cleared conversation_task_id, so the old task is archived,
// stopping any running turn (copilot.md#conversation-lifecycle).
func (s *Service) ArchiveClearedConversationTask(ctx context.Context, coordinatorID, oldConversationTaskID string) {
	s.archiveClearedConversationTask(ctx, coordinatorID, oldConversationTaskID)
}

// DeleteConversationTasksForCoordinator is the CoordinatorDeletedHook
// implementation wireCoordinatorConversation registers: every task whose
// coordinator_id metadata matches the just-deleted coordinator is deleted,
// current or archived (copilot.md#conversation-cleanup).
func (s *Service) DeleteConversationTasksForCoordinator(ctx context.Context, workspaceID, coordinatorID string) {
	s.deleteConversationTasksForCoordinator(ctx, workspaceID, coordinatorID)
}

// OpenConversation implements
// POST /api/v1/workspaces/:id/coordinators/:cid/conversation
// (copilot.md#conversation-task). It is race-safe: concurrent opens converge
// on one task with one session, or on a 409 the caller retries with fresh
// coordinator state.
func (s *Service) OpenConversation(ctx context.Context, workspaceID, coordinatorID string) (*ConversationResult, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}

	// Step 1: load and validate profiles.
	found, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if err != nil {
		return nil, err
	}
	agentStatus, executorStatus, err := s.validator.ProfileStatus(ctx, found.WorkspaceID, found.AgentProfileID, found.ExecutorProfileID)
	if err != nil {
		return nil, fmt.Errorf("compute profile status: %w", err)
	}
	if agentStatus != ProfileStatusOK || executorStatus != ProfileStatusOK {
		return nil, &ProfileUnavailableError{AgentStatus: agentStatus, ExecutorStatus: executorStatus}
	}

	// Step 2: reuse a live, unarchived current task.
	staleTaskID := ""
	if found.ConversationTaskID != nil {
		staleTaskID = *found.ConversationTaskID
		result, reusable, err := s.reuseCurrentConversation(ctx, coordinatorID, staleTaskID)
		if err != nil || reusable {
			return result, err
		}
	}

	// Step 3: create a new ephemeral conversation task, bound to its tool list.
	metadata := map[string]interface{}{
		taskmodels.MetaKeyCoordinatorID:     coordinatorID,
		taskmodels.MetaKeyAgentProfileID:    found.AgentProfileID,
		taskmodels.MetaKeyExecutorProfileID: found.ExecutorProfileID,
	}
	if err := s.stampToolPolicy(metadata, found, ""); err != nil {
		return nil, err
	}
	created, err := s.conversationTasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:           workspaceID,
		Title:                 "Coordinator: " + found.Name,
		IsEphemeral:           true,
		Origin:                taskmodels.TaskOriginCoordinator,
		Metadata:              metadata,
		AllowReservedMetadata: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create conversation task: %w", err)
	}
	newTaskID := created.Task.ID
	if err := s.bindConversationTask(ctx, created.Task, found, metadata); err != nil {
		s.deleteConversationTaskBestEffort(ctx, newTaskID)
		return nil, fmt.Errorf("bind conversation task: %w", err)
	}

	// Step 4: commit the new task as current, or resolve the race.
	ok, err := s.store.SetConversationTaskID(ctx, coordinatorID, newTaskID, staleTaskID, found.ConfigRevision)
	if err != nil {
		return nil, err
	}
	if !ok {
		return s.resolveConversationCreateRace(ctx, coordinatorID, newTaskID, found.ConfigRevision)
	}
	return s.finishConversationOpen(ctx, coordinatorID, newTaskID)
}

// reuseCurrentConversation returns the current task's conversation when the
// task still exists, is unarchived and its session can take a message;
// reusable is false when the caller must create a fresh task instead.
func (s *Service) reuseCurrentConversation(ctx context.Context, coordinatorID, taskID string) (*ConversationResult, bool, error) {
	task, err := s.conversationTasks.GetTask(ctx, taskID)
	if err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		return nil, false, err
	}
	if err != nil || task == nil || task.ArchivedAt != nil {
		return nil, false, nil
	}
	return s.reuseConversationTask(ctx, coordinatorID, taskID)
}

// reuseConversationTask ensures the current conversation task's session. A
// session in a terminal state can never take another message, so the task is
// archived and reusable is false: the caller then creates a fresh task.
func (s *Service) reuseConversationTask(ctx context.Context, coordinatorID, taskID string) (*ConversationResult, bool, error) {
	autoStart := false
	resp, err := s.conversationSessions.EnsureSession(ctx, taskID, orchestrator.EnsureSessionOptions{
		AutoStart:        &autoStart,
		ActivationSource: orchestrator.LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		return nil, false, s.handleConversationSessionFailure(ctx, coordinatorID, taskID, err)
	}
	if !isTerminalSessionState(resp.State) {
		result, err := s.confirmConversationTask(ctx, coordinatorID, taskID, resp.SessionID)
		return result, true, err
	}
	if err := s.conversationTasks.ArchiveTask(ctx, taskID); err != nil {
		s.logger.Warn("archive conversation task with ended session failed",
			zap.String("coordinator_id", coordinatorID), zap.String("task_id", taskID),
			zap.String("session_state", resp.State), zap.Error(err))
	}
	return nil, false, nil
}

func isTerminalSessionState(state string) bool {
	switch taskmodels.TaskSessionState(state) {
	case taskmodels.TaskSessionStateFailed, taskmodels.TaskSessionStateCancelled, taskmodels.TaskSessionStateCompleted:
		return true
	}
	return false
}

// resolveConversationCreateRace implements step 4's zero-rows-updated branch:
// re-read the coordinator, delete the task this call just created, and
// converge on whichever task actually won.
func (s *Service) resolveConversationCreateRace(ctx context.Context, coordinatorID, createdTaskID string, expectedConfigRevision int64) (*ConversationResult, error) {
	reread, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		// Step 5: the coordinator was deleted between steps 1 and 4.
		s.deleteConversationTaskBestEffort(ctx, createdTaskID)
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.deleteConversationTaskBestEffort(ctx, createdTaskID)
	if reread.ConfigRevision != expectedConfigRevision {
		// A context/profile change was saved while this open created its task
		// under the earlier configuration, even if the reference is NULL on
		// both sides: no task from that stale configuration to converge on.
		return nil, ErrConversationConflict
	}
	if reread.ConversationTaskID == nil {
		// A concurrent context/profile change cleared the reference after our
		// stale read: no task exists to converge on.
		return nil, ErrConversationConflict
	}
	return s.finishConversationOpen(ctx, coordinatorID, *reread.ConversationTaskID)
}

// finishConversationOpen implements steps 6 and 7: ensure the session, then
// re-confirm the coordinator still names this task as current.
func (s *Service) finishConversationOpen(ctx context.Context, coordinatorID, taskID string) (*ConversationResult, error) {
	autoStart := false
	resp, err := s.conversationSessions.EnsureSession(ctx, taskID, orchestrator.EnsureSessionOptions{
		AutoStart:        &autoStart,
		ActivationSource: orchestrator.LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		return nil, s.handleConversationSessionFailure(ctx, coordinatorID, taskID, err)
	}
	return s.confirmConversationTask(ctx, coordinatorID, taskID, resp.SessionID)
}

// handleConversationSessionFailure implements step 6's error handling: a
// coordinator that is now gone is answered like step 7's deleted-coordinator
// case; otherwise the task is left as current and the caller sees a 502 to
// retry.
func (s *Service) handleConversationSessionFailure(ctx context.Context, coordinatorID, taskID string, cause error) error {
	_, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		s.deleteConversationTaskBestEffort(ctx, taskID)
		return ErrNotFound
	}
	s.logger.Warn("ensure conversation session failed",
		zap.String("coordinator_id", coordinatorID), zap.String("task_id", taskID), zap.Error(cause))
	return fmt.Errorf("%w: %v", ErrConversationSessionUnavailable, cause)
}

// confirmConversationTask implements step 7: a coordinator that is now gone
// is deleted and 404; a mismatch (a race archived or replaced this task) is
// ErrConversationConflict. A concurrent opener can archive this exact task
// (its own EnsureSession call observed a terminal session) without yet having
// repointed ConversationTaskID, since that only happens on its later
// create+CAS: the ConversationTaskID match alone cannot see that, so the task
// is re-read too and an archived result is treated the same as a mismatch.
func (s *Service) confirmConversationTask(ctx context.Context, coordinatorID, taskID, sessionID string) (*ConversationResult, error) {
	reread, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		s.deleteConversationTaskBestEffort(ctx, taskID)
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if reread.ConversationTaskID == nil || *reread.ConversationTaskID != taskID {
		return nil, ErrConversationConflict
	}
	task, err := s.conversationTasks.GetTask(ctx, taskID)
	if err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		return nil, err
	}
	if err != nil || task == nil || task.ArchivedAt != nil {
		return nil, ErrConversationConflict
	}
	return &ConversationResult{TaskID: taskID, SessionID: sessionID}, nil
}

// deleteConversationTaskBestEffort deletes taskID, counting
// taskrepo.ErrTaskNotFound as done. Any other failure is logged at warn and
// swallowed: the startup cleanup pass will retry it.
func (s *Service) deleteConversationTaskBestEffort(ctx context.Context, taskID string) {
	if err := s.conversationTasks.DeleteTask(ctx, taskID); err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		s.logger.Warn("failed to delete conversation task", zap.String("task_id", taskID), zap.Error(err))
	}
}

// archiveClearedConversationTask is the ConversationClearedHook: a
// context/profile change already cleared conversation_task_id, so the old
// task is archived, stopping any running turn (copilot.md#conversation-
// lifecycle).
func (s *Service) archiveClearedConversationTask(ctx context.Context, coordinatorID, oldConversationTaskID string) {
	err := s.conversationTasks.ArchiveTask(ctx, oldConversationTaskID)
	if err == nil || errors.Is(err, taskrepo.ErrTaskNotFound) || errors.Is(err, taskservice.ErrTaskAlreadyArchived) {
		return
	}
	s.logger.Warn("failed to archive cleared conversation task",
		zap.String("coordinator_id", coordinatorID), zap.String("task_id", oldConversationTaskID), zap.Error(err))
}

// deleteConversationTasksForCoordinator is the CoordinatorDeletedHook: every
// task whose coordinator_id metadata matches the just-deleted coordinator is
// deleted, current or archived (copilot.md#conversation-cleanup).
func (s *Service) deleteConversationTasksForCoordinator(ctx context.Context, workspaceID, coordinatorID string) {
	tasks, err := s.conversationTasks.ListCoordinatorOriginTasks(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("delete conversation tasks for coordinator: list failed",
			zap.String("coordinator_id", coordinatorID), zap.Error(err))
		return
	}
	for _, task := range tasks {
		if conversationTaskCoordinatorID(task) != coordinatorID {
			continue
		}
		s.deleteConversationTaskBestEffort(ctx, task.ID)
	}
}

// CleanupConversationTasks is the startup pass (copilot.md#conversation-
// cleanup): it walks every coordinator-origin task across all workspaces
// created before t0, in id order, repairing anything a crash or lost race
// left dangling. Idempotent: running it twice changes nothing the second
// time. t0 must be recorded before the coordinator routes register, so a task
// the conversation route is still creating (between steps 3 and 4) is never
// touched.
func (s *Service) CleanupConversationTasks(ctx context.Context, t0 time.Time) {
	tasks, err := s.conversationTasks.ListCoordinatorOriginTasks(ctx, "")
	if err != nil {
		s.logger.Warn("conversation cleanup: list coordinator origin tasks failed", zap.Error(err))
		return
	}
	for _, task := range tasks {
		if !task.CreatedAt.Before(t0) {
			continue
		}
		s.cleanupOneConversationTask(ctx, task)
	}
}

// cleanupOneConversationTask applies the startup pass's per-task rule: a task
// whose coordinator is gone is deleted; an unarchived task that is not its
// coordinator's current conversation task is archived; anything else is left
// alone.
func (s *Service) cleanupOneConversationTask(ctx context.Context, task *taskmodels.Task) {
	coordinatorID := conversationTaskCoordinatorID(task)
	if isDreamTask(task) && coordinatorID != "" {
		if _, err := s.store.GetCoordinatorByID(ctx, coordinatorID); !errors.Is(err, ErrNotFound) {
			return
		}
	}
	if coordinatorID == "" {
		s.deleteConversationTaskBestEffort(ctx, task.ID)
		return
	}
	found, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		s.deleteConversationTaskBestEffort(ctx, task.ID)
		return
	}
	if err != nil {
		s.logger.Warn("conversation cleanup: get coordinator failed", zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	isCurrent := found.ConversationTaskID != nil && *found.ConversationTaskID == task.ID
	if task.ArchivedAt != nil || isCurrent {
		return
	}
	if err := s.conversationTasks.ArchiveTask(ctx, task.ID); err != nil &&
		!errors.Is(err, taskrepo.ErrTaskNotFound) && !errors.Is(err, taskservice.ErrTaskAlreadyArchived) {
		s.logger.Warn("conversation cleanup: archive failed", zap.String("task_id", task.ID), zap.Error(err))
	}
}

// conversationTaskCoordinatorID reads a coordinator-origin task's
// coordinator_id metadata, or "" if absent or not a string (copilot.md
// #conversation-cleanup: coordinator_id is read from the metadata in Go, so
// the query has no dialect-specific JSON).
func conversationTaskCoordinatorID(task *taskmodels.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	id, _ := task.Metadata[taskmodels.MetaKeyCoordinatorID].(string)
	return id
}

// isDreamTask reports whether a coordinator-origin task is a shadow dream
// episode, which is not a conversation and is archived by the dream tick.
func isDreamTask(task *taskmodels.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	purpose, _ := task.Metadata[taskmodels.MetaKeyCoordinatorPurpose].(string)
	return purpose == taskmodels.CoordinatorPurposeDream
}

// archiveConversation archives the conversation task a committed
// resetConversation cleared. An empty id names no task and does nothing.
func (s *Service) archiveConversation(ctx context.Context, coordinatorID, taskID string) {
	if taskID == "" {
		return
	}
	s.archiveClearedConversationTask(ctx, coordinatorID, taskID)
}

// resetConversation runs inside the caller's coordinator lock: it clears the
// conversation task, increments config_revision and returns the previous task
// id, which the caller archives after commit through archiveConversation.
func (s *Service) resetConversation(ctx context.Context, exec coordinatorExec, coordinatorID string) (string, error) {
	return s.store.resetConversation(ctx, exec, coordinatorID)
}
