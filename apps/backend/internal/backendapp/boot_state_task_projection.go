package backendapp

import (
	"context"

	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func (b bootStateBuilder) taskDTOWithSessionInfo(ctx context.Context, task *taskmodels.Task) taskdto.TaskDTO {
	if task == nil {
		return taskdto.TaskDTO{}
	}
	dtos := b.taskDTOsWithSessionInfo(ctx, []*taskmodels.Task{task})
	if len(dtos) == 0 {
		return taskdto.FromTask(task)
	}
	return dtos[0]
}

func (b bootStateBuilder) taskDTOsWithSessionInfo(ctx context.Context, tasks []*taskmodels.Task) []taskdto.TaskDTO {
	if len(tasks) == 0 {
		return []taskdto.TaskDTO{}
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	statusSummaries, summaryErr := b.p.taskSvc.GetTaskStatusSummaries(ctx, taskIDs)
	if summaryErr != nil {
		b.logBootError("batch task status summaries", summaryErr)
		statusSummaries = map[string]*statussummary.TaskStatusSummary{}
	}
	sessionObservationsByTask, err := b.p.taskSvc.BatchGetTaskSessionSummaryObservations(ctx, taskIDs)
	if err != nil {
		b.logBootError("batch task detail sessions", err)
		return taskDTOsWithStatusSummaries(tasks, statusSummaries)
	}
	sessionsByTask := bootSessionModelsFromSummaryObservations(sessionObservationsByTask)
	primaryInfoByTask := bootPrimarySessionModelsFromSummaryObservations(sessionObservationsByTask)
	pendingActionsBySession, pendingErr := b.bootPendingActionsForInputCapableSessions(ctx, sessionsByTask)
	if pendingErr != nil {
		b.logBootError("get task detail pending actions", pendingErr)
		pendingActionsBySession = map[string]taskmodels.TaskPendingAction{}
	}
	if summaryErr == nil && pendingErr == nil {
		reconciledSummaries, reconcileErr := b.p.taskSvc.ReconcileTaskStatusSummaries(
			ctx, tasks, sessionsByTask, pendingActionsBySession, statusSummaries,
		)
		statusSummaries = reconciledSummaries
		if reconcileErr != nil {
			b.logBootError("reconcile task status summaries", reconcileErr)
		}
	}
	// Stamp the authoritative per-task queued prompt count onto every summary so
	// the boot payload shows the sidebar badge on first paint, matching the
	// shared list/snapshot assembly. Best-effort: a counter failure omits the
	// badge instead of failing the boot payload.
	queuedByTask, queuedErr := b.p.taskSvc.CountPendingQueuedByTaskIDs(ctx, taskIDs)
	if queuedErr != nil {
		b.logBootError("queued prompt counts", queuedErr)
	}
	// Dependency state is derived per read (never stored, so the auto-start gate
	// can never read a stale value). One batched call for the whole boot payload.
	dependencyViews := b.p.taskSvc.BuildDependencyViews(ctx, tasks)
	// The boot payload is a board-snapshot projection path, so it must run the
	// runner-mutability evaluation itself rather than rely on
	// FromTaskWithSessionInfo's fail-closed default.
	runnerViews := b.p.taskSvc.BuildRunnerMutabilityViews(ctx, tasks)
	result := make([]taskdto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		sessions := sessionsByTask[task.ID]
		primarySessionID, sessionCount := bootPrimarySessionIDAndCount(sessions)
		info := bootSessionInfo(primaryInfoByTask[task.ID])
		dto := taskdto.FromTaskWithSessionInfo(
			task,
			primarySessionID,
			sessionCount,
			info.reviewStatus,
			info.executorID,
			info.executorProfileID,
			info.executorType,
			info.executorName,
			info.agentName,
			info.agentProfileID,
			info.workingDirectory,
			info.sessionState,
			bootPendingActionPtr(info.sessionID, pendingActionsBySession),
		)
		dto.TaskPendingAction = bootTaskPendingActionPtr(sessions, pendingActionsBySession)
		// Stamp the task-level MOST-ACTIVE-WINS activity aggregate so the board
		// card and task list show the background-running affordance on first paint
		// / in a second tab, without holding the task's full session set client-side
		// No-op when no session is running.
		if b.p.orchestratorSvc != nil {
			taskdto.EnrichTaskForegroundActivity(&dto, sessions, b.p.orchestratorSvc)
			taskdto.EnrichTaskParkedProjection(&dto, b.p.orchestratorSvc)
		}
		taskdto.EnrichTaskDependencies(&dto, bootDependencyProjection(dependencyViews[task.ID]), task)
		taskdto.EnrichTaskRunnerMutability(&dto, bootRunnerMutabilityProjection(runnerViews[task.ID]))
		taskdto.EnrichTaskStatusSummary(&dto, task.ID, statusSummaries)
		stampBootQueuedPromptCount(&dto, queuedByTask, queuedErr)
		result = append(result, dto)
	}
	return result
}

func bootSessionModelsFromSummaryObservations(
	observations map[string][]*taskmodels.TaskSessionSummaryObservation,
) map[string][]*taskmodels.TaskSession {
	sessionsByTask := make(map[string][]*taskmodels.TaskSession, len(observations))
	for taskID, taskObservations := range observations {
		sessions := make([]*taskmodels.TaskSession, 0, len(taskObservations))
		for _, observation := range taskObservations {
			sessions = append(sessions, observation.ToTaskSession())
		}
		sessionsByTask[taskID] = sessions
	}
	return sessionsByTask
}

func bootPrimarySessionModelsFromSummaryObservations(
	observations map[string][]*taskmodels.TaskSessionSummaryObservation,
) map[string]*taskmodels.TaskSession {
	primaryByTask := make(map[string]*taskmodels.TaskSession, len(observations))
	for taskID, taskObservations := range observations {
		for _, observation := range taskObservations {
			if observation != nil && observation.IsPrimary {
				primaryByTask[taskID] = observation.ToTaskSession()
				break
			}
		}
	}
	return primaryByTask
}

func taskDTOsWithStatusSummaries(tasks []*taskmodels.Task, summaries map[string]*statussummary.TaskStatusSummary) []taskdto.TaskDTO {
	result := make([]taskdto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		dto := taskdto.FromTask(task)
		dto.StatusSummary = summaries[task.ID]
		result = append(result, dto)
	}
	return result
}

func bootPrimarySessionIDAndCount(sessions []*taskmodels.TaskSession) (*string, *int) {
	var primarySessionID *string
	for _, session := range sessions {
		if session != nil && session.IsPrimary {
			id := session.ID
			primarySessionID = &id
			break
		}
	}
	var sessionCount *int
	if len(sessions) > 0 {
		count := len(sessions)
		sessionCount = &count
	}
	return primarySessionID, sessionCount
}

func stampBootQueuedPromptCount(dto *taskdto.TaskDTO, queuedByTask map[string]int, queuedErr error) {
	if dto.StatusSummary == nil {
		return
	}
	switch {
	case queuedErr != nil:
		// Counter failures omit the badge without persisting the cleared value.
		dto.StatusSummary.QueuedPromptCount = 0
	case queuedByTask != nil:
		dto.StatusSummary.QueuedPromptCount = queuedByTask[dto.ID]
	}
}

type bootSessionInfoFields struct {
	sessionID         *string
	reviewStatus      taskmodels.ReviewStatus
	sessionState      *string
	executorID        *string
	executorProfileID *string
	executorType      *string
	executorName      *string
	agentName         *string
	agentProfileID    *string
	workingDirectory  *string
}

func bootSessionInfo(session *taskmodels.TaskSession) bootSessionInfoFields {
	var info bootSessionInfoFields
	if session == nil {
		return info
	}
	if session.ID != "" {
		value := session.ID
		info.sessionID = &value
	}
	info.reviewStatus = session.ReviewStatus
	if session.State != "" {
		value := string(session.State)
		info.sessionState = &value
	}
	if session.ExecutorID != "" {
		value := session.ExecutorID
		info.executorID = &value
	}
	if session.ExecutorProfileID != "" {
		value := session.ExecutorProfileID
		info.executorProfileID = &value
	}
	info.executorType, info.executorName, info.agentName, info.workingDirectory = bootSessionSnapshotInfo(session)
	if session.AgentProfileID != "" {
		value := session.AgentProfileID
		info.agentProfileID = &value
	}
	return info
}

func bootSessionSnapshotInfo(session *taskmodels.TaskSession) (executorType, executorName, agentName, workingDirectory *string) {
	if session.ExecutorSnapshot != nil {
		if value, ok := session.ExecutorSnapshot["executor_type"].(string); ok && value != "" {
			executorType = &value
		}
		if value, ok := session.ExecutorSnapshot["executor_name"].(string); ok && value != "" {
			executorName = &value
		}
	}
	if session.AgentProfileSnapshot != nil {
		if value, ok := session.AgentProfileSnapshot["name"].(string); ok && value != "" {
			agentName = &value
		}
	}
	if session.RepositorySnapshot != nil {
		if value, ok := session.RepositorySnapshot["path"].(string); ok && value != "" {
			workingDirectory = &value
		}
	}
	return executorType, executorName, agentName, workingDirectory
}

func (b bootStateBuilder) bootPendingActionsForInputCapableSessions(
	ctx context.Context,
	sessionsByTask map[string][]*taskmodels.TaskSession,
) (map[string]taskmodels.TaskPendingAction, error) {
	sessionIDs := make([]string, 0)
	for _, sessions := range sessionsByTask {
		for _, session := range sessions {
			if bootInputCapableSession(session) {
				sessionIDs = append(sessionIDs, session.ID)
			}
		}
	}
	if len(sessionIDs) == 0 {
		return map[string]taskmodels.TaskPendingAction{}, nil
	}
	return b.p.taskSvc.GetPendingActionsForSessions(ctx, sessionIDs)
}

func bootInputCapableSession(session *taskmodels.TaskSession) bool {
	return session != nil && (session.State == taskmodels.TaskSessionStateRunning || session.State == taskmodels.TaskSessionStateWaitingForInput)
}

func bootTaskPendingActionPtr(sessions []*taskmodels.TaskSession, actions map[string]taskmodels.TaskPendingAction) *string {
	var clarification bool
	for _, session := range sessions {
		if !bootInputCapableSession(session) {
			continue
		}
		switch actions[session.ID] {
		case taskmodels.TaskPendingActionPermission:
			value := string(taskmodels.TaskPendingActionPermission)
			return &value
		case taskmodels.TaskPendingActionClarification:
			clarification = true
		}
	}
	if clarification {
		value := string(taskmodels.TaskPendingActionClarification)
		return &value
	}
	return nil
}

func bootPendingActionPtr(
	sessionID *string,
	pendingActionsBySession map[string]taskmodels.TaskPendingAction,
) *string {
	if sessionID == nil {
		return nil
	}
	action, ok := pendingActionsBySession[*sessionID]
	if !ok {
		return nil
	}
	value := string(action)
	return &value
}
