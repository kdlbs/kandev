package main

import (
	"context"
	"net/http"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userdto "github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/webapp"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	activeWorkspaceCookie       = "kandev-active-workspace"
	legacyOfficeWorkspaceCookie = "office-active-workspace"
)

func bootInitialState(
	ctx context.Context,
	req *http.Request,
	p routeParams,
	route webapp.RouteClassification,
) map[string]any {
	builder := bootStateBuilder{p: p}
	state := map[string]any{
		"features": p.features,
	}

	if route.Route == webapp.RouteSettings {
		builder.addWorkspaceState(ctx, state, nil)
		builder.addUserSettingsState(ctx, state, "")
		builder.addSettingsRouteState(ctx, state, route.Path)
	}
	if route.Route == webapp.RouteHome {
		builder.addHomeKanbanRouteState(ctx, req, state)
	}
	if route.Route == webapp.RouteTasks {
		tasksState, _ := builder.tasksPageBootData(ctx, req)
		mergeBootState(state, tasksState)
	}
	if isLocalContextRoute(route.Route) {
		contextState, _ := builder.routeContextBootData(ctx, req)
		mergeBootState(state, contextState)
	}
	if route.Route == webapp.RouteOffice {
		builder.addOfficeRouteState(ctx, req, state)
	}
	return state
}

func bootRouteData(
	ctx context.Context,
	req *http.Request,
	p routeParams,
	route webapp.RouteClassification,
) map[string]any {
	builder := bootStateBuilder{p: p}
	switch route.Route {
	case webapp.RouteTaskDetail:
		return builder.taskDetailRouteData(ctx, route.Params["taskId"])
	case webapp.RouteTasks:
		_, routeData := builder.tasksPageBootData(ctx, req)
		if routeData == nil {
			return nil
		}
		return map[string]any{"tasksPage": routeData}
	case webapp.RouteGitHub, webapp.RouteGitLab, webapp.RouteJira, webapp.RouteLinear, webapp.RouteStats:
		_, routeData := builder.routeContextBootData(ctx, req)
		if routeData == nil {
			return nil
		}
		return map[string]any{"routeContext": routeData}
	default:
		return nil
	}
}

func isLocalContextRoute(route webapp.RouteName) bool {
	switch route {
	case webapp.RouteGitHub, webapp.RouteGitLab, webapp.RouteJira, webapp.RouteLinear, webapp.RouteStats:
		return true
	default:
		return false
	}
}

type bootStateBuilder struct {
	p routeParams
}

func (b bootStateBuilder) addWorkspaceState(ctx context.Context, state map[string]any, activeID *string) {
	if b.p.taskSvc == nil {
		return
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		b.logBootError("list workspaces", err)
		return
	}
	items := make([]taskdto.WorkspaceDTO, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		items = append(items, taskdto.FromWorkspace(workspace))
	}
	var active any
	if activeID != nil {
		active = *activeID
	}
	state["workspaces"] = map[string]any{
		"items":    items,
		"activeId": active,
	}
}

func (b bootStateBuilder) addUserSettingsState(ctx context.Context, state map[string]any, workspaceID string) {
	if b.p.userCtrl == nil {
		return
	}
	response, err := b.p.userCtrl.GetUserSettings(ctx)
	if err != nil {
		b.logBootError("get user settings", err)
		return
	}
	state["userSettings"] = mapUserSettingsState(response, workspaceID)
}

func (b bootStateBuilder) addSettingsRouteState(ctx context.Context, state map[string]any, path string) {
	switch path {
	case "/settings/prompts":
		b.addPromptsState(ctx, state)
	case "/settings/general/editors":
		b.addEditorsState(ctx, state)
	}
}

func (b bootStateBuilder) addHomeKanbanRouteState(ctx context.Context, req *http.Request, state map[string]any) {
	if b.p.taskSvc == nil {
		return
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		b.logBootError("list home workspaces", err)
		return
	}
	workspaceItems := make([]map[string]any, 0, len(workspaces))
	workspaceIDs := make(map[string]bool, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		workspaceIDs[workspace.ID] = true
		workspaceItems = append(workspaceItems, mapWorkspaceItemState(taskdto.FromWorkspace(workspace)))
	}

	settings, hasSettings := b.userSettings(ctx)
	settingsWorkspaceID := ""
	settingsWorkflowID := ""
	if hasSettings {
		settingsWorkspaceID = settings.Settings.WorkspaceID
		settingsWorkflowID = settings.Settings.WorkflowFilterID
	}
	activeWorkspaceID := firstValidID(
		workspaceIDs,
		queryValue(req, "workspaceId"),
		readActiveWorkspaceCookie(req),
		settingsWorkspaceID,
		firstWorkspaceID(workspaces),
	)
	state["workspaces"] = map[string]any{
		"items":    workspaceItems,
		"activeId": nullString(activeWorkspaceID),
	}
	if hasSettings {
		state["userSettings"] = mapUserSettingsState(settings, activeWorkspaceID)
	}
	if activeWorkspaceID == "" {
		return
	}

	workflows, err := b.homeWorkflows(ctx, activeWorkspaceID)
	if err != nil {
		b.logBootError("list home workflows", err)
		return
	}
	workflowItems := make([]map[string]any, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow == nil {
			continue
		}
		workflowItems = append(workflowItems, mapWorkflowItemState(taskdto.FromWorkflow(workflow)))
	}
	activeWorkflowID := resolveHomeWorkflowID(workflows, queryValue(req, "workflowId"), settingsWorkflowID, hasSettings)
	state["workflows"] = map[string]any{
		"items":    workflowItems,
		"activeId": nullString(activeWorkflowID),
	}
	if hasSettings {
		state["userSettings"] = mapUserSettingsStateWithWorkflow(settings, activeWorkspaceID, activeWorkflowID)
	}
	b.addRepositoriesState(ctx, state, activeWorkspaceID)
	b.addKanbanSnapshotsState(ctx, state, workflows, activeWorkflowID)
}

func (b bootStateBuilder) userSettings(ctx context.Context) (userdto.UserSettingsResponse, bool) {
	if b.p.userCtrl == nil {
		return userdto.UserSettingsResponse{}, false
	}
	response, err := b.p.userCtrl.GetUserSettings(ctx)
	if err != nil {
		b.logBootError("get user settings", err)
		return userdto.UserSettingsResponse{}, false
	}
	return response, true
}

func (b bootStateBuilder) homeWorkflows(ctx context.Context, workspaceID string) ([]*taskmodels.Workflow, error) {
	workflows, err := b.p.taskSvc.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	officeIDs := b.p.taskSvc.GetOfficeWorkflowIDs(ctx)
	filtered := make([]*taskmodels.Workflow, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow == nil {
			continue
		}
		if _, isOffice := officeIDs[workflow.ID]; isOffice {
			continue
		}
		filtered = append(filtered, workflow)
	}
	return filtered, nil
}

func (b bootStateBuilder) addRepositoriesState(ctx context.Context, state map[string]any, workspaceID string) {
	repositories, err := b.p.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		b.logBootError("list home repositories", err)
		return
	}
	items := make([]taskdto.RepositoryDTO, 0, len(repositories))
	for _, repository := range repositories {
		if repository == nil {
			continue
		}
		items = append(items, taskdto.FromRepository(repository))
	}
	state["repositories"] = map[string]any{
		"itemsByWorkspaceId": map[string]any{workspaceID: items},
		"loadingByWorkspaceId": map[string]any{
			workspaceID: false,
		},
		"loadedByWorkspaceId": map[string]any{
			workspaceID: true,
		},
	}
}

func (b bootStateBuilder) addKanbanSnapshotsState(
	ctx context.Context,
	state map[string]any,
	workflows []*taskmodels.Workflow,
	activeWorkflowID string,
) {
	snapshots := make(map[string]any, len(workflows))
	var active map[string]any
	for _, workflow := range workflows {
		if workflow == nil {
			continue
		}
		snapshot, ok := b.workflowSnapshotState(ctx, workflow)
		if !ok {
			continue
		}
		snapshots[workflow.ID] = snapshot
		if workflow.ID == activeWorkflowID {
			active = snapshot
		}
	}
	state["kanbanMulti"] = map[string]any{
		"snapshots": snapshots,
		"isLoading": false,
	}
	if active != nil {
		state["kanban"] = map[string]any{
			"workflowId": active["workflowId"],
			"steps":      active["steps"],
			"tasks":      active["tasks"],
			"isLoading":  false,
		}
	}
}

func (b bootStateBuilder) workflowSnapshotState(ctx context.Context, workflow *taskmodels.Workflow) (map[string]any, bool) {
	steps, err := b.workflowStepStates(ctx, workflow.ID)
	if err != nil {
		b.logBootError("list home workflow steps", err)
		return nil, false
	}
	tasks, err := b.p.taskSvc.ListTasks(ctx, workflow.ID)
	if err != nil {
		b.logBootError("list home workflow tasks", err)
		return nil, false
	}
	taskStates := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.IsEphemeral || task.WorkflowStepID == "" {
			continue
		}
		taskStates = append(taskStates, mapKanbanTaskState(taskdto.FromTask(task)))
	}
	return map[string]any{
		"workflowId":   workflow.ID,
		"workflowName": workflow.Name,
		"steps":        steps,
		"tasks":        taskStates,
	}, true
}

func (b bootStateBuilder) workflowStepStates(ctx context.Context, workflowID string) ([]map[string]any, error) {
	if b.p.services == nil || b.p.services.Workflow == nil {
		return []map[string]any{}, nil
	}
	steps, err := b.p.services.Workflow.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		result = append(result, mapKanbanStepState(taskdto.FromWorkflowStepWithTimestamps(step)))
	}
	return result, nil
}

func (b bootStateBuilder) taskDetailRouteData(ctx context.Context, taskID string) map[string]any {
	if b.p.taskSvc == nil || taskID == "" {
		return nil
	}
	task, err := b.p.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		b.logBootError("get task detail task", err)
		return nil
	}
	sessions, err := b.p.taskSvc.ListTaskSessions(ctx, task.ID)
	if err != nil {
		b.logBootError("list task detail sessions", err)
		sessions = nil
	}
	activeSessionID := resolveTaskDetailSessionID(task, sessions)
	taskDTO := b.taskDTOWithSessionInfo(ctx, task)
	initialState := b.taskDetailInitialState(ctx, task, taskDTO, sessions, activeSessionID)
	return map[string]any{
		"taskDetail": map[string]any{
			"task":             taskDTO,
			"sessionId":        nullString(activeSessionID),
			"initialState":     initialState,
			"initialTerminals": []any{},
		},
	}
}

func resolveTaskDetailSessionID(task *taskmodels.Task, sessions []*taskmodels.TaskSession) string {
	if task != nil {
		for _, session := range sessions {
			if session != nil && session.IsPrimary {
				return session.ID
			}
		}
	}
	for _, session := range sessions {
		if session != nil && session.ID != "" {
			return session.ID
		}
	}
	return ""
}

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
	sessionsByTask, err := b.p.taskSvc.BatchGetSessionsForTasks(ctx, taskIDs)
	if err != nil {
		b.logBootError("batch task detail sessions", err)
		return taskDTOs(tasks)
	}
	primaryInfoByTask, err := b.p.taskSvc.GetPrimarySessionInfoForTasks(ctx, taskIDs)
	if err != nil {
		b.logBootError("get task detail primary session info", err)
		return taskDTOs(tasks)
	}
	result := make([]taskdto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		sessions := sessionsByTask[task.ID]
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
		info := bootSessionInfo(primaryInfoByTask[task.ID])
		result = append(result, taskdto.FromTaskWithSessionInfo(
			task,
			primarySessionID,
			sessionCount,
			info.reviewStatus,
			info.executorID,
			info.executorType,
			info.executorName,
			info.agentName,
			info.workingDirectory,
			info.sessionState,
		))
	}
	return result
}

func taskDTOs(tasks []*taskmodels.Task) []taskdto.TaskDTO {
	result := make([]taskdto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			result = append(result, taskdto.FromTask(task))
		}
	}
	return result
}

type bootSessionInfoFields struct {
	reviewStatus     taskmodels.ReviewStatus
	sessionState     *string
	executorID       *string
	executorType     *string
	executorName     *string
	agentName        *string
	workingDirectory *string
}

func bootSessionInfo(session *taskmodels.TaskSession) bootSessionInfoFields {
	var info bootSessionInfoFields
	if session == nil {
		return info
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
	if session.ExecutorSnapshot != nil {
		if value, ok := session.ExecutorSnapshot["executor_type"].(string); ok && value != "" {
			info.executorType = &value
		}
		if value, ok := session.ExecutorSnapshot["executor_name"].(string); ok && value != "" {
			info.executorName = &value
		}
	}
	if session.AgentProfileSnapshot != nil {
		if value, ok := session.AgentProfileSnapshot["name"].(string); ok && value != "" {
			info.agentName = &value
		}
	}
	if session.RepositorySnapshot != nil {
		if value, ok := session.RepositorySnapshot["path"].(string); ok && value != "" {
			info.workingDirectory = &value
		}
	}
	return info
}

func (b bootStateBuilder) taskDetailInitialState(
	ctx context.Context,
	task *taskmodels.Task,
	taskDTO taskdto.TaskDTO,
	sessions []*taskmodels.TaskSession,
	activeSessionID string,
) map[string]any {
	state := map[string]any{}
	b.addTaskDetailResourceState(ctx, state, task)
	b.addTaskDetailKanbanState(ctx, state, task)
	b.addTaskDetailActiveTaskState(ctx, state, taskDTO, activeSessionID)
	b.addTaskDetailSessionsState(state, task.ID, sessions, activeSessionID)
	b.addTaskDetailAgentsState(ctx, state)
	return state
}

func (b bootStateBuilder) addTaskDetailResourceState(ctx context.Context, state map[string]any, task *taskmodels.Task) {
	b.addWorkspaceState(ctx, state, &task.WorkspaceID)
	b.addUserSettingsState(ctx, state, task.WorkspaceID)
	workflows, err := b.p.taskSvc.ListWorkflows(ctx, task.WorkspaceID, true)
	if err != nil {
		b.logBootError("list task detail workflows", err)
	} else {
		state["workflows"] = map[string]any{
			"items":    workflowItemStates(workflows),
			"activeId": nil,
		}
	}
	b.addRepositoriesState(ctx, state, task.WorkspaceID)
}

func workflowItemStates(workflows []*taskmodels.Workflow) []map[string]any {
	items := make([]map[string]any, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow != nil {
			items = append(items, mapWorkflowItemState(taskdto.FromWorkflow(workflow)))
		}
	}
	return items
}

func (b bootStateBuilder) addTaskDetailKanbanState(ctx context.Context, state map[string]any, task *taskmodels.Task) {
	if task.WorkflowID == "" {
		state["kanban"] = map[string]any{"workflowId": "", "steps": []any{}, "tasks": []any{}, "isLoading": false}
		return
	}
	workflows, err := b.p.taskSvc.ListWorkflows(ctx, task.WorkspaceID, true)
	if err != nil {
		b.logBootError("list task detail kanban workflows", err)
		return
	}
	for _, workflow := range workflows {
		if workflow == nil || workflow.ID != task.WorkflowID {
			continue
		}
		snapshot, ok := b.workflowSnapshotState(ctx, workflow)
		if !ok {
			return
		}
		state["kanban"] = map[string]any{
			"workflowId": snapshot["workflowId"],
			"steps":      snapshot["steps"],
			"tasks":      snapshot["tasks"],
			"isLoading":  false,
		}
		state["kanbanMulti"] = map[string]any{
			"snapshots": map[string]any{workflow.ID: snapshot},
			"isLoading": false,
		}
		return
	}
}

func (b bootStateBuilder) addTaskDetailActiveTaskState(
	ctx context.Context,
	state map[string]any,
	task taskdto.TaskDTO,
	activeSessionID string,
) {
	state["tasks"] = map[string]any{
		"activeTaskId":        task.ID,
		"activeSessionId":     nullString(activeSessionID),
		"pinnedSessionId":     nil,
		"lastSessionByTaskId": lastSessionByTaskState(task.ID, activeSessionID),
	}
	if activeSessionID == "" {
		return
	}
	messages, hasMore, err := b.p.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{
		TaskSessionID: activeSessionID,
		Limit:         50,
		Sort:          "desc",
	})
	if err != nil {
		b.logBootError("list task detail messages", err)
		return
	}
	apiMessages := make([]*v1.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil {
			apiMessages = append(apiMessages, messages[i].ToAPI())
		}
	}
	var oldest any
	if len(apiMessages) > 0 {
		oldest = apiMessages[0].ID
	}
	state["messages"] = map[string]any{
		"bySession": map[string]any{activeSessionID: apiMessages},
		"metaBySession": map[string]any{
			activeSessionID: map[string]any{
				"isLoading":    false,
				"hasMore":      hasMore,
				"oldestCursor": oldest,
			},
		},
	}
}

func lastSessionByTaskState(taskID, sessionID string) map[string]string {
	if taskID == "" || sessionID == "" {
		return map[string]string{}
	}
	return map[string]string{taskID: sessionID}
}

func (b bootStateBuilder) addTaskDetailSessionsState(
	state map[string]any,
	taskID string,
	sessions []*taskmodels.TaskSession,
	activeSessionID string,
) {
	sessionItems := make(map[string]taskdto.TaskSessionDTO, len(sessions))
	sessionList := make([]taskdto.TaskSessionDTO, 0, len(sessions))
	environmentBySession := make(map[string]string, len(sessions))
	worktrees := make(map[string]any)
	worktreesBySession := make(map[string]any)
	for _, session := range sessions {
		if session == nil {
			continue
		}
		dto := taskdto.FromTaskSession(session)
		sessionItems[session.ID] = dto
		sessionList = append(sessionList, dto)
		if session.TaskEnvironmentID != "" {
			environmentBySession[session.ID] = session.TaskEnvironmentID
		}
		if dto.WorktreeID != "" {
			worktrees[dto.WorktreeID] = map[string]any{
				"id":           dto.WorktreeID,
				"sessionId":    session.ID,
				"repositoryId": nullString(dto.RepositoryID),
				"path":         nullString(dto.WorktreePath),
				"branch":       nullString(dto.WorktreeBranch),
			}
			worktreesBySession[session.ID] = []string{dto.WorktreeID}
		}
	}
	state["taskSessions"] = map[string]any{"items": sessionItems}
	state["taskSessionsByTask"] = map[string]any{
		"itemsByTaskId":   map[string]any{taskID: sessionList},
		"loadingByTaskId": map[string]any{taskID: false},
		"loadedByTaskId":  map[string]any{taskID: true},
	}
	state["turns"] = map[string]any{
		"bySession":       map[string]any{},
		"activeBySession": activeTurnBySessionState(activeSessionID),
	}
	state["environmentIdBySessionId"] = environmentBySession
	state["worktrees"] = map[string]any{"items": worktrees}
	state["sessionWorktreesBySessionId"] = map[string]any{"itemsBySessionId": worktreesBySession}
}

func activeTurnBySessionState(sessionID string) map[string]any {
	if sessionID == "" {
		return map[string]any{}
	}
	return map[string]any{sessionID: nil}
}

func (b bootStateBuilder) addTaskDetailAgentsState(ctx context.Context, state map[string]any) {
	if b.p.agentSettingsController == nil {
		return
	}
	response, err := b.p.agentSettingsController.ListAgents(ctx)
	if err != nil {
		b.logBootError("list task detail agents", err)
		return
	}
	state["settingsAgents"] = map[string]any{"items": response.Agents}
	state["settingsData"] = map[string]any{"agentsLoaded": true, "executorsLoaded": false}
	state["agentProfiles"] = map[string]any{
		"items":   agentProfileOptionStates(response.Agents),
		"version": 0,
	}
}

func agentProfileOptionStates(agents []agentsettingsdto.AgentDTO) []map[string]any {
	items := []map[string]any{}
	for _, agent := range agents {
		for _, profile := range agent.Profiles {
			items = append(items, map[string]any{
				"id":                profile.ID,
				"label":             profile.AgentDisplayName + " - " + profile.Name,
				"agent_id":          agent.ID,
				"agent_name":        agent.Name,
				"cli_passthrough":   profile.CLIPassthrough,
				"capability_status": nullString(agent.CapabilityStatus),
				"capability_error":  nullString(agent.CapabilityError),
			})
		}
	}
	return items
}
