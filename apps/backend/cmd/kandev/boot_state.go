package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
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
	workflowIDs := make(map[string]bool, len(workflows))
	workflowItems := make([]map[string]any, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow == nil {
			continue
		}
		workflowIDs[workflow.ID] = true
		workflowItems = append(workflowItems, mapWorkflowItemState(taskdto.FromWorkflow(workflow)))
	}
	activeWorkflowID := firstValidID(
		workflowIDs,
		queryValue(req, "workflowId"),
		settingsWorkflowID,
		firstWorkflowID(workflows),
	)
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

func (b bootStateBuilder) tasksPageBootData(ctx context.Context, req *http.Request) (map[string]any, map[string]any) {
	if b.p.taskSvc == nil {
		return nil, nil
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		b.logBootError("list tasks page workspaces", err)
		return nil, nil
	}
	settings, hasSettings := b.userSettings(ctx)
	settingsWorkspaceID := ""
	settingsWorkflowID := ""
	settingsRepositoryID := ""
	if hasSettings {
		settingsWorkspaceID = settings.Settings.WorkspaceID
		settingsWorkflowID = settings.Settings.WorkflowFilterID
		if len(settings.Settings.RepositoryIDs) > 0 {
			settingsRepositoryID = settings.Settings.RepositoryIDs[0]
		}
	}
	workspaceIDs := workspaceIDSet(workspaces)
	activeWorkspaceID := firstValidID(
		workspaceIDs,
		queryValue(req, "workspaceId"),
		queryValue(req, "workspace"),
		readActiveWorkspaceCookie(req),
		settingsWorkspaceID,
		firstWorkspaceID(workspaces),
	)
	state := map[string]any{
		"workspaces": map[string]any{
			"items":    workspaceItemStates(workspaces),
			"activeId": nullString(activeWorkspaceID),
		},
	}
	if hasSettings {
		state["userSettings"] = mapUserSettingsState(settings, activeWorkspaceID)
	}
	if activeWorkspaceID == "" {
		return state, map[string]any{"activeWorkspaceId": nil, "workflows": []any{}, "steps": []any{}, "repositories": []any{}, "tasks": []any{}, "total": 0}
	}
	workflows, err := b.p.taskSvc.ListWorkflows(ctx, activeWorkspaceID, false)
	if err != nil {
		b.logBootError("list tasks page workflows", err)
		return state, nil
	}
	activeWorkflowID := validWorkflowOrEmpty(workflows, settingsWorkflowID)
	workflowItems := workflowItemStates(workflows)
	state["workflows"] = map[string]any{"items": workflowItems, "activeId": nullString(activeWorkflowID)}
	if hasSettings {
		state["userSettings"] = mapUserSettingsStateWithWorkflow(settings, activeWorkspaceID, activeWorkflowID)
	}
	repositories := b.repositoriesForState(ctx, activeWorkspaceID, state)
	steps := b.workflowStepsForWorkspace(ctx, activeWorkspaceID)
	tasks, total := b.tasksForWorkspace(ctx, activeWorkspaceID, activeWorkflowID, settingsRepositoryID)
	routeData := map[string]any{
		"activeWorkspaceId": activeWorkspaceID,
		"workflows":         workflowsToDTOs(workflows),
		"steps":             steps,
		"repositories":      repositories,
		"tasks":             tasks,
		"total":             total,
	}
	return state, routeData
}

func (b bootStateBuilder) routeContextBootData(ctx context.Context, req *http.Request) (map[string]any, map[string]any) {
	if b.p.taskSvc == nil {
		return nil, nil
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		b.logBootError("list route context workspaces", err)
		return nil, nil
	}
	settings, hasSettings := b.userSettings(ctx)
	settingsWorkspaceID := ""
	settingsWorkflowID := ""
	if hasSettings {
		settingsWorkspaceID = settings.Settings.WorkspaceID
		settingsWorkflowID = settings.Settings.WorkflowFilterID
	}
	activeWorkspaceID := firstValidID(
		workspaceIDSet(workspaces),
		queryValue(req, "workspaceId"),
		queryValue(req, "workspace"),
		readActiveWorkspaceCookie(req),
		settingsWorkspaceID,
		firstWorkspaceID(workspaces),
	)
	state := map[string]any{
		"workspaces": map[string]any{
			"items":    workspaceItemStates(workspaces),
			"activeId": nullString(activeWorkspaceID),
		},
	}
	if hasSettings {
		state["userSettings"] = mapUserSettingsState(settings, activeWorkspaceID)
	}
	if activeWorkspaceID == "" {
		return state, map[string]any{"activeWorkspaceId": nil, "workflows": []any{}, "steps": []any{}, "repositories": []any{}}
	}
	workflows, err := b.p.taskSvc.ListWorkflows(ctx, activeWorkspaceID, false)
	if err != nil {
		b.logBootError("list route context workflows", err)
		return state, nil
	}
	activeWorkflowID := validWorkflowOrEmpty(workflows, settingsWorkflowID)
	state["workflows"] = map[string]any{
		"items":    workflowItemStates(workflows),
		"activeId": nullString(activeWorkflowID),
	}
	if hasSettings {
		state["userSettings"] = mapUserSettingsStateWithWorkflow(settings, activeWorkspaceID, activeWorkflowID)
	}
	repositories := b.repositoriesForState(ctx, activeWorkspaceID, state)
	steps := b.workflowStepsForWorkspace(ctx, activeWorkspaceID)
	return state, map[string]any{
		"activeWorkspaceId": activeWorkspaceID,
		"workflows":         workflowsToDTOs(workflows),
		"steps":             steps,
		"repositories":      repositories,
	}
}

func workspaceIDSet(workspaces []*taskmodels.Workspace) map[string]bool {
	result := make(map[string]bool, len(workspaces))
	for _, workspace := range workspaces {
		if workspace != nil {
			result[workspace.ID] = true
		}
	}
	return result
}

func workspaceItemStates(workspaces []*taskmodels.Workspace) []map[string]any {
	items := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace != nil {
			items = append(items, mapWorkspaceItemState(taskdto.FromWorkspace(workspace)))
		}
	}
	return items
}

func validWorkflowOrEmpty(workflows []*taskmodels.Workflow, workflowID string) string {
	for _, workflow := range workflows {
		if workflow != nil && workflow.ID == workflowID {
			return workflowID
		}
	}
	return ""
}

func (b bootStateBuilder) repositoriesForState(ctx context.Context, workspaceID string, state map[string]any) []taskdto.RepositoryDTO {
	repositories, err := b.p.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		b.logBootError("list tasks page repositories", err)
		return []taskdto.RepositoryDTO{}
	}
	items := repositoriesToDTOs(repositories)
	state["repositories"] = map[string]any{
		"itemsByWorkspaceId":   map[string]any{workspaceID: items},
		"loadingByWorkspaceId": map[string]any{workspaceID: false},
		"loadedByWorkspaceId":  map[string]any{workspaceID: true},
	}
	return items
}

func repositoriesToDTOs(repositories []*taskmodels.Repository) []taskdto.RepositoryDTO {
	items := make([]taskdto.RepositoryDTO, 0, len(repositories))
	for _, repository := range repositories {
		if repository != nil {
			items = append(items, taskdto.FromRepository(repository))
		}
	}
	return items
}

func workflowsToDTOs(workflows []*taskmodels.Workflow) []taskdto.WorkflowDTO {
	items := make([]taskdto.WorkflowDTO, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow != nil {
			items = append(items, taskdto.FromWorkflow(workflow))
		}
	}
	return items
}

func (b bootStateBuilder) workflowStepsForWorkspace(ctx context.Context, workspaceID string) []taskdto.WorkflowStepDTO {
	if b.p.services == nil || b.p.services.Workflow == nil {
		return []taskdto.WorkflowStepDTO{}
	}
	steps, err := b.p.services.Workflow.ListStepsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		b.logBootError("list tasks page workflow steps", err)
		return []taskdto.WorkflowStepDTO{}
	}
	items := make([]taskdto.WorkflowStepDTO, 0, len(steps))
	for _, step := range steps {
		if step != nil {
			items = append(items, taskdto.FromWorkflowStepWithTimestamps(step))
		}
	}
	return items
}

func (b bootStateBuilder) tasksForWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID string) ([]taskdto.TaskDTO, int) {
	tasks, total, err := b.p.taskSvc.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, "", 1, 25, false, false, false, false)
	if err != nil {
		b.logBootError("list tasks page tasks", err)
		return []taskdto.TaskDTO{}, 0
	}
	return b.taskDTOsWithSessionInfo(ctx, tasks), total
}

func mergeBootState(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}

func (b bootStateBuilder) addPromptsState(ctx context.Context, state map[string]any) {
	if b.p.promptCtrl == nil {
		return
	}
	response, err := b.p.promptCtrl.ListPrompts(ctx)
	if err != nil {
		b.logBootError("list prompts", err)
		return
	}
	state["prompts"] = map[string]any{
		"items":   response.Prompts,
		"loaded":  true,
		"loading": false,
	}
}

func (b bootStateBuilder) addEditorsState(ctx context.Context, state map[string]any) {
	if b.p.editorCtrl == nil {
		return
	}
	response, err := b.p.editorCtrl.ListEditors(ctx)
	if err != nil {
		b.logBootError("list editors", err)
		return
	}
	state["editors"] = map[string]any{
		"items":   response.Editors,
		"loaded":  true,
		"loading": false,
	}
}

func (b bootStateBuilder) addOfficeRouteState(ctx context.Context, req *http.Request, state map[string]any) {
	if !b.p.features.Office || b.p.services == nil || b.p.services.OfficeSvcs == nil {
		return
	}
	officeSvcs := b.p.services.OfficeSvcs
	if officeSvcs.Onboarding != nil {
		onboarding, err := officeSvcs.Onboarding.GetOnboardingState(ctx)
		if err != nil {
			b.logBootError("get office onboarding", err)
			return
		}
		if onboarding != nil && !onboarding.Completed {
			return
		}
	}

	workspaces, activeID, err := b.officeWorkspaces(ctx, req)
	if err != nil {
		b.logBootError("list office workspaces", err)
		return
	}
	state["workspaces"] = map[string]any{
		"items":    workspaces,
		"activeId": activeID,
	}
	b.addUserSettingsState(ctx, state, activeID)
	state["office"] = b.officeState(ctx, activeID)
}

func (b bootStateBuilder) officeWorkspaces(ctx context.Context, req *http.Request) ([]taskdto.WorkspaceDTO, string, error) {
	if b.p.taskSvc == nil {
		return nil, "", nil
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		return nil, "", err
	}
	items := make([]taskdto.WorkspaceDTO, 0, len(workspaces))
	officeItems := make([]taskdto.WorkspaceDTO, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		item := taskdto.FromWorkspace(workspace)
		items = append(items, item)
		if item.OfficeWorkflowID != "" {
			officeItems = append(officeItems, item)
		}
	}
	return items, resolveActiveOfficeWorkspaceID(officeItems, readActiveWorkspaceCookie(req)), nil
}

func (b bootStateBuilder) officeState(ctx context.Context, activeID string) map[string]any {
	agents := b.officeAgents(ctx, activeID)
	projects := b.officeProjects(ctx, activeID)
	inboxItems, inboxCount := b.officeInbox(ctx, activeID)
	dashboard := b.officeDashboard(ctx, activeID)
	return map[string]any{
		"agentProfiles":  agents,
		"skills":         []any{},
		"projects":       projects,
		"approvals":      []any{},
		"activity":       []any{},
		"costSummary":    nil,
		"budgetPolicies": []any{},
		"routines":       []any{},
		"inboxItems":     inboxItems,
		"inboxCount":     inboxCount,
		"runs":           []any{},
		"dashboard":      dashboard,
		"tasks": map[string]any{
			"items":          []any{},
			"filters":        map[string]any{"statuses": []any{}, "priorities": []any{}, "assigneeIds": []any{}, "projectIds": []any{}, "search": ""},
			"viewMode":       "list",
			"sortField":      "updated",
			"sortDir":        "desc",
			"groupBy":        "none",
			"nestingEnabled": true,
			"isLoading":      false,
		},
		"meta":           officedashboard.BuildMetaResponse(),
		"isLoading":      false,
		"refetchTrigger": nil,
		"routing":        map[string]any{"byWorkspace": map[string]any{}, "knownProviders": []any{}, "preview": map[string]any{"byWorkspace": map[string]any{}}},
		"providerHealth": map[string]any{"byWorkspace": map[string]any{}},
		"runAttempts":    map[string]any{"byRunId": map[string]any{}},
		"agentRouting":   map[string]any{"byAgentId": map[string]any{}},
	}
}

func (b bootStateBuilder) officeAgents(ctx context.Context, activeID string) any {
	if activeID == "" || b.p.services.OfficeSvcs.Agents == nil {
		return []any{}
	}
	result, err := b.p.services.OfficeSvcs.Agents.ListAgentsFromConfig(ctx, activeID)
	if err != nil {
		b.logBootError("list office agents", err)
		return []any{}
	}
	return result
}

func (b bootStateBuilder) officeProjects(ctx context.Context, activeID string) any {
	if activeID == "" || b.p.services.OfficeSvcs.Projects == nil {
		return []any{}
	}
	result, err := b.p.services.OfficeSvcs.Projects.ListProjectsWithCountsFromConfig(ctx, activeID)
	if err != nil {
		b.logBootError("list office projects", err)
		return []any{}
	}
	return result
}

func (b bootStateBuilder) officeInbox(ctx context.Context, activeID string) (any, int) {
	if activeID == "" || b.p.services.OfficeSvcs.Dashboard == nil {
		return []any{}, 0
	}
	result, err := b.p.services.OfficeSvcs.Dashboard.GetInboxItems(ctx, activeID)
	if err != nil {
		b.logBootError("get office inbox", err)
		return []any{}, 0
	}
	return result, len(result)
}

func (b bootStateBuilder) officeDashboard(ctx context.Context, activeID string) any {
	if activeID == "" || b.p.services.OfficeSvcs.Dashboard == nil {
		return nil
	}
	data, err := b.p.services.OfficeSvcs.Dashboard.GetDashboardData(ctx, activeID)
	if err != nil {
		b.logBootError("get office dashboard", err)
		return nil
	}
	summaries, err := b.p.services.OfficeSvcs.Dashboard.GetAgentSummaries(ctx, activeID)
	if err != nil {
		b.logBootError("get office agent summaries", err)
		summaries = []officedashboard.AgentSummary{}
	}
	return officedashboard.NewDashboardResponse(data, summaries)
}

func mapUserSettingsState(response userdto.UserSettingsResponse, workspaceID string) map[string]any {
	settings := response.Settings
	effectiveWorkspaceID := nullString(settings.WorkspaceID)
	if workspaceID != "" {
		effectiveWorkspaceID = workspaceID
	}
	return map[string]any{
		"workspaceId":                 effectiveWorkspaceID,
		"kanbanViewMode":              nullString(settings.KanbanViewMode),
		"workflowId":                  nullString(settings.WorkflowFilterID),
		"repositoryIds":               stringSlice(settings.RepositoryIDs),
		"preferredShell":              nullString(settings.PreferredShell),
		"shellOptions":                response.ShellOptions,
		"defaultEditorId":             nullString(settings.DefaultEditorID),
		"enablePreviewOnClick":        settings.EnablePreviewOnClick,
		"chatSubmitKey":               defaultString(settings.ChatSubmitKey, "cmd_enter"),
		"reviewAutoMarkOnScroll":      settings.ReviewAutoMarkOnScroll,
		"showReleaseNotification":     settings.ShowReleaseNotification,
		"releaseNotesLastSeenVersion": nullString(settings.ReleaseNotesLastSeenVersion),
		"lspAutoStartLanguages":       stringSlice(settings.LspAutoStartLanguages),
		"lspAutoInstallLanguages":     stringSlice(settings.LspAutoInstallLanguages),
		"lspServerConfigs":            mapStringMap(settings.LspServerConfigs),
		"savedLayouts":                settings.SavedLayouts,
		"sidebarViews":                mapSidebarViews(settings.SidebarViews),
		"defaultUtilityAgentId":       nullString(settings.DefaultUtilityAgentID),
		"keyboardShortcuts":           mapStringAny(settings.KeyboardShortcuts),
		"terminalLinkBehavior":        terminalLinkBehavior(settings.TerminalLinkBehavior),
		"terminalFontFamily":          nullString(settings.TerminalFontFamily),
		"terminalFontSize":            nullInt(settings.TerminalFontSize),
		"changesPanelLayout":          changesPanelLayout(settings.ChangesPanelLayout),
		"systemMetricsDisplay":        map[string]any{"showInTopbar": settings.SystemMetricsDisplay.ShowInTopbar},
		"voiceMode":                   mapVoiceMode(settings.VoiceMode),
		"loaded":                      true,
	}
}

func mapUserSettingsStateWithWorkflow(response userdto.UserSettingsResponse, workspaceID, workflowID string) map[string]any {
	state := mapUserSettingsState(response, workspaceID)
	state["workflowId"] = nullString(workflowID)
	return state
}

func mapWorkspaceItemState(workspace taskdto.WorkspaceDTO) map[string]any {
	return map[string]any{
		"id":                              workspace.ID,
		"name":                            workspace.Name,
		"description":                     workspace.Description,
		"owner_id":                        workspace.OwnerID,
		"default_executor_id":             workspace.DefaultExecutorID,
		"default_environment_id":          workspace.DefaultEnvironmentID,
		"default_agent_profile_id":        workspace.DefaultAgentProfileID,
		"default_config_agent_profile_id": workspace.DefaultConfigAgentProfileID,
		"office_workflow_id":              nullString(workspace.OfficeWorkflowID),
		"created_at":                      workspace.CreatedAt,
		"updated_at":                      workspace.UpdatedAt,
	}
}

func mapWorkflowItemState(workflow taskdto.WorkflowDTO) map[string]any {
	return map[string]any{
		"id":               workflow.ID,
		"workspaceId":      workflow.WorkspaceID,
		"name":             workflow.Name,
		"description":      workflow.Description,
		"sortOrder":        workflow.SortOrder,
		"agent_profile_id": nullString(workflow.AgentProfileID),
		"hidden":           workflow.Hidden,
		"style":            workflow.Style,
	}
}

func mapKanbanStepState(step taskdto.WorkflowStepDTO) map[string]any {
	return map[string]any{
		"id":                    step.ID,
		"title":                 step.Name,
		"color":                 defaultString(step.Color, "bg-neutral-400"),
		"position":              step.Position,
		"events":                step.Events,
		"allow_manual_move":     step.AllowManualMove,
		"prompt":                step.Prompt,
		"is_start_step":         step.IsStartStep,
		"show_in_command_panel": step.ShowInCommandPanel,
		"agent_profile_id":      nullString(step.AgentProfileID),
		"stage_type":            nullString(step.StageType),
	}
}

func mapKanbanTaskState(task taskdto.TaskDTO) map[string]any {
	repositories := make([]map[string]any, 0, len(task.Repositories))
	var primaryRepositoryID any
	for i, repo := range task.Repositories {
		if i == 0 {
			primaryRepositoryID = repo.RepositoryID
		}
		repositories = append(repositories, map[string]any{
			"id":              repo.ID,
			"repository_id":   repo.RepositoryID,
			"base_branch":     repo.BaseBranch,
			"checkout_branch": repo.CheckoutBranch,
			"position":        repo.Position,
		})
	}
	return map[string]any{
		"id":                  task.ID,
		"workflowStepId":      task.WorkflowStepID,
		"title":               task.Title,
		"description":         task.Description,
		"position":            task.Position,
		"state":               task.State,
		"repositoryId":        primaryRepositoryID,
		"repositories":        repositories,
		"primarySessionId":    task.PrimarySessionID,
		"primarySessionState": task.PrimarySessionState,
		"sessionCount":        task.SessionCount,
		"reviewStatus":        nullString(string(task.ReviewStatus)),
		"parentTaskId":        nullString(task.ParentID),
		"updatedAt":           task.UpdatedAt,
		"createdAt":           task.CreatedAt,
	}
}

func mapSidebarViews(views []usermodels.SidebarView) []map[string]any {
	if len(views) == 0 {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(views))
	for _, view := range views {
		result = append(result, map[string]any{
			"id":              view.ID,
			"name":            view.Name,
			"filters":         view.Filters,
			"sort":            view.Sort,
			"group":           view.Group,
			"collapsedGroups": stringSlice(view.CollapsedGroups),
		})
	}
	return result
}

func mapVoiceMode(value usermodels.VoiceModeSettings) map[string]any {
	return map[string]any{
		"enabled":         value.Enabled,
		"engine":          defaultString(value.Engine, "auto"),
		"language":        defaultString(value.Language, "auto"),
		"mode":            defaultString(value.Mode, "toggle"),
		"autoSend":        value.AutoSend,
		"whisperWebModel": defaultString(value.WhisperWebModel, "base"),
	}
}

func resolveActiveOfficeWorkspaceID(workspaces []taskdto.WorkspaceDTO, cookieWorkspaceID string) string {
	for _, workspace := range workspaces {
		if workspace.ID == cookieWorkspaceID {
			return workspace.ID
		}
	}
	if len(workspaces) > 0 {
		return workspaces[0].ID
	}
	return ""
}

func firstValidID(valid map[string]bool, candidates ...string) string {
	for _, candidate := range candidates {
		value := strings.TrimSpace(candidate)
		if value != "" && valid[value] {
			return value
		}
	}
	return ""
}

func firstWorkspaceID(workspaces []*taskmodels.Workspace) string {
	for _, workspace := range workspaces {
		if workspace != nil && workspace.ID != "" {
			return workspace.ID
		}
	}
	return ""
}

func firstWorkflowID(workflows []*taskmodels.Workflow) string {
	for _, workflow := range workflows {
		if workflow != nil && workflow.ID != "" {
			return workflow.ID
		}
	}
	return ""
}

func queryValue(req *http.Request, name string) string {
	if req == nil || req.URL == nil {
		return ""
	}
	if value := strings.TrimSpace(req.URL.Query().Get(name)); value != "" {
		return value
	}
	routePath := strings.TrimSpace(req.URL.Query().Get("path"))
	if routePath == "" {
		return ""
	}
	parsed, err := url.Parse(routePath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get(name))
}

func readActiveWorkspaceCookie(req *http.Request) string {
	if req == nil {
		return ""
	}
	for _, name := range []string{activeWorkspaceCookie, legacyOfficeWorkspaceCookie} {
		cookie, err := req.Cookie(name)
		if err == nil {
			if value := strings.TrimSpace(cookie.Value); value != "" {
				return value
			}
		}
	}
	return ""
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func stringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func mapStringAny(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func mapStringMap(value map[string]map[string]any) map[string]map[string]any {
	if value == nil {
		return map[string]map[string]any{}
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func terminalLinkBehavior(value string) string {
	if value == "browser_panel" {
		return "browser_panel"
	}
	return "new_tab"
}

func changesPanelLayout(value string) string {
	if value == "flat" {
		return "flat"
	}
	return "tree"
}

func (b bootStateBuilder) logBootError(operation string, err error) {
	if err == nil || b.p.log == nil {
		return
	}
	b.p.log.Debug("SPA boot state skipped optional data", zap.String("operation", operation), zap.Error(err))
}
