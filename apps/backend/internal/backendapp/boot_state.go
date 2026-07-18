package backendapp

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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
	bootStateKeySessionID       = "sessionId"
	bootStateKeyWorkspaceID     = "workspaceId"
	quickChatSessionKindChat    = "chat"
	quickChatSessionKindConfig  = "config"
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
	// Home and unknown SPA routes both render the full app shell (nav,
	// workspace picker) without a route-specific data payload. Unknown
	// covers plugin-owned routes (e.g. /github-plugin) registered at
	// runtime, which the backend classifier can't enumerate — they still
	// need the base workspace/workflow/kanban context so native plugin UI
	// (like host.ui.TaskCreateDialog) has workspaces and workflows to work
	// with, not an empty store.
	if route.Route == webapp.RouteHome || route.Route == webapp.RouteUnknown {
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
	builder.addQuickChatState(ctx, req, state, route)
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

func (b bootStateBuilder) addQuickChatState(
	ctx context.Context,
	req *http.Request,
	state map[string]any,
	route webapp.RouteClassification,
) {
	workspaceID := b.resolveQuickChatWorkspaceID(ctx, req, state, route)
	if workspaceID == "" {
		return
	}
	quickChat, err := b.quickChatSessions(ctx, workspaceID)
	if err != nil {
		b.logBootError("list quick-chat sessions", err)
		return
	}
	state["quickChat"] = map[string]any{
		"isOpen":          false,
		"sessions":        quickChat.sessions,
		"activeSessionId": nil,
	}
	mergeBootTaskSessionItems(state, quickChat.taskSessions)
}

func (b bootStateBuilder) resolveQuickChatWorkspaceID(
	ctx context.Context,
	req *http.Request,
	state map[string]any,
	route webapp.RouteClassification,
) string {
	if workspaceID := b.quickChatTaskRouteWorkspaceID(ctx, route); workspaceID != "" {
		return workspaceID
	}
	if active := activeWorkspaceIDFromState(state); active != "" {
		return active
	}
	if b.p.taskSvc == nil {
		return ""
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		b.logBootError("list quick-chat workspaces", err)
		return ""
	}
	settingsWorkspaceID := ""
	if settings, ok := b.userSettings(ctx); ok {
		settingsWorkspaceID = settings.Settings.WorkspaceID
	}
	return firstValidID(
		workspaceIDSet(workspaces),
		queryValue(req, "workspaceId"),
		queryValue(req, "workspace"),
		readActiveWorkspaceCookie(req),
		settingsWorkspaceID,
		firstWorkspaceID(workspaces),
	)
}

func (b bootStateBuilder) quickChatTaskRouteWorkspaceID(
	ctx context.Context,
	route webapp.RouteClassification,
) string {
	if b.p.taskSvc == nil || (route.Route != webapp.RouteTaskDetail && route.Route != webapp.RouteOffice) {
		return ""
	}
	taskID := route.Params["taskId"]
	if taskID == "" {
		return ""
	}
	task, err := b.p.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		b.logBootError("get quick-chat task route workspace", err)
		return ""
	}
	if task == nil {
		return ""
	}
	return task.WorkspaceID
}

func activeWorkspaceIDFromState(state map[string]any) string {
	workspaces, ok := state["workspaces"].(map[string]any)
	if !ok {
		return ""
	}
	active, _ := workspaces["activeId"].(string)
	return active
}

func (b bootStateBuilder) quickChatSessions(ctx context.Context, workspaceID string) (quickChatBootState, error) {
	tasks, err := b.listQuickChatTasks(ctx, workspaceID)
	if err != nil {
		return quickChatBootState{}, err
	}
	if len(tasks) == 0 {
		return quickChatBootState{sessions: []map[string]any{}}, nil
	}
	taskIDs := taskIDs(tasks)
	primaryByTask, err := b.p.taskSvc.GetPrimarySessionInfoForTasks(ctx, taskIDs)
	if err != nil {
		return quickChatBootState{}, err
	}

	items := make([]quickChatBootSession, 0, len(tasks))
	taskSessions := make(map[string]taskdto.TaskSessionDTO, len(tasks))
	for _, task := range tasks {
		if !isRestorableQuickChatTask(task) {
			continue
		}
		primary := primaryByTask[task.ID]
		if primary == nil || primary.ID == "" {
			continue
		}
		items = append(items, quickChatBootSession{
			state:          mapQuickChatSessionState(task, primary),
			taskID:         task.ID,
			createdAt:      task.CreatedAt,
			lastActivityAt: quickChatLastActivityAt(task, primary),
		})
		taskSessions[primary.ID] = taskdto.FromTaskSession(primary)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].lastActivityAt.Equal(items[j].lastActivityAt) {
			return items[i].lastActivityAt.After(items[j].lastActivityAt)
		}
		if !items[i].createdAt.Equal(items[j].createdAt) {
			return items[i].createdAt.Before(items[j].createdAt)
		}
		return items[i].taskID < items[j].taskID
	})
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, item.state)
	}
	return quickChatBootState{sessions: result, taskSessions: taskSessions}, nil
}

func (b bootStateBuilder) listQuickChatTasks(ctx context.Context, workspaceID string) ([]*taskmodels.Task, error) {
	const pageSize = 1000
	var all []*taskmodels.Task
	for page := 1; ; page++ {
		tasks, total, err := b.p.taskSvc.ListTasksByWorkspace(ctx, workspaceID, "", "", "", page, pageSize, "", false, false, true, false)
		if err != nil {
			return nil, err
		}
		all = append(all, tasks...)
		if len(tasks) == 0 || len(all) >= total {
			return all, nil
		}
	}
}

type quickChatBootSession struct {
	state          map[string]any
	taskID         string
	createdAt      time.Time
	lastActivityAt time.Time
}

func quickChatLastActivityAt(task *taskmodels.Task, primary *taskmodels.TaskSession) time.Time {
	if primary.UpdatedAt.After(task.UpdatedAt) {
		return primary.UpdatedAt
	}
	return task.UpdatedAt
}

type quickChatBootState struct {
	sessions     []map[string]any
	taskSessions map[string]taskdto.TaskSessionDTO
}

func mergeBootTaskSessionItems(state map[string]any, items map[string]taskdto.TaskSessionDTO) {
	if len(items) == 0 {
		return
	}
	taskSessions, ok := state["taskSessions"].(map[string]any)
	if !ok {
		state["taskSessions"] = map[string]any{"items": items}
		return
	}
	merged := make(map[string]any, len(items))
	switch existing := taskSessions["items"].(type) {
	case map[string]taskdto.TaskSessionDTO:
		for id, session := range existing {
			merged[id] = session
		}
	case map[string]any:
		for id, session := range existing {
			merged[id] = session
		}
	}
	for id, session := range items {
		merged[id] = session
	}
	taskSessions["items"] = merged
}

func taskIDs(tasks []*taskmodels.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func isRestorableQuickChatTask(task *taskmodels.Task) bool {
	return task != nil &&
		task.IsEphemeral &&
		task.WorkflowID == "" &&
		task.Origin != taskmodels.TaskOriginAutomationRun
}

func mapQuickChatSessionState(task *taskmodels.Task, primary *taskmodels.TaskSession) map[string]any {
	state := map[string]any{
		bootStateKeySessionID:   primary.ID,
		bootStateKeyWorkspaceID: task.WorkspaceID,
		"kind":                  quickChatSessionKind(task),
	}
	if task.Title != "" && task.Title != "Quick Chat" {
		state["name"] = task.Title
	}
	if agentProfileID := quickChatAgentProfileID(task, primary); agentProfileID != "" {
		state["agentProfileId"] = agentProfileID
	}
	return state
}

func quickChatSessionKind(task *taskmodels.Task) string {
	if task != nil {
		if configMode, ok := task.Metadata["config_mode"].(bool); ok && configMode {
			return quickChatSessionKindConfig
		}
	}
	return quickChatSessionKindChat
}

func quickChatAgentProfileID(task *taskmodels.Task, primary *taskmodels.TaskSession) string {
	if task != nil {
		if value, ok := task.Metadata[taskmodels.MetaKeyAgentProfileID].(string); ok {
			return value
		}
	}
	if primary != nil {
		return primary.AgentProfileID
	}
	return ""
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
	visibleTasks := make([]*taskmodels.Task, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.IsEphemeral || task.WorkflowStepID == "" {
			continue
		}
		visibleTasks = append(visibleTasks, task)
	}
	taskStates := make([]map[string]any, 0, len(visibleTasks))
	for _, task := range b.taskDTOsWithSessionInfo(ctx, visibleTasks) {
		taskStates = append(taskStates, mapKanbanTaskState(task))
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
	pendingActionsBySession, err := b.bootPendingActionsForWaitingPrimarySessions(ctx, primaryInfoByTask)
	if err != nil {
		b.logBootError("get task detail pending actions", err)
		pendingActionsBySession = map[string]taskmodels.TaskPendingAction{}
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
		dto := taskdto.FromTaskWithSessionInfo(
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
			bootPendingActionPtr(info.sessionID, pendingActionsBySession),
		)
		// Stamp the task-level MOST-ACTIVE-WINS activity aggregate so the board
		// card and task list show the background-running affordance on first paint
		// / in a second tab, without holding the task's full session set client-side
		// (§spec:task-level-indicator). No-op when no session is running.
		if b.p.orchestratorSvc != nil {
			taskdto.EnrichTaskForegroundActivity(&dto, sessions, b.p.orchestratorSvc)
		}
		result = append(result, dto)
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
	sessionID        *string
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

func (b bootStateBuilder) bootPendingActionsForWaitingPrimarySessions(
	ctx context.Context,
	primaryInfoByTask map[string]*taskmodels.TaskSession,
) (map[string]taskmodels.TaskPendingAction, error) {
	sessionIDs := make([]string, 0, len(primaryInfoByTask))
	for _, info := range primaryInfoByTask {
		if info != nil && info.State == taskmodels.TaskSessionStateWaitingForInput {
			sessionIDs = append(sessionIDs, info.ID)
		}
	}
	if len(sessionIDs) == 0 {
		return map[string]taskmodels.TaskPendingAction{}, nil
	}
	return b.p.taskSvc.GetPendingActionsForSessions(ctx, sessionIDs)
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
	b.addTaskDetailSessionsState(ctx, state, task.ID, sessions, activeSessionID)
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
	ctx context.Context,
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
	sessionModelsByID := make(map[string]any)
	for _, session := range sessions {
		if session == nil {
			continue
		}
		dto := taskdto.FromTaskSession(session)
		// Mirror the in-memory fine-grained busy substate onto a RUNNING session
		// so a fresh page-load / second tab sees the accept-input +
		// working-in-background affordance without waiting for a WS flip
		// (ADR-0038). No-op for non-RUNNING sessions.
		if b.p.orchestratorSvc != nil {
			taskdto.EnrichForegroundActivity(&dto, b.p.orchestratorSvc)
		}
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
		if snapshot, ok := lifecycle.LoadSessionModelsSnapshot(
			session.Metadata[taskmodels.SessionMetaKeyACPModelState],
		); ok {
			sessionModelsByID[session.ID] = taskSessionModelsBootState(
				snapshot, sessionACPConfigBaseline(session),
			)
		}
	}
	state["taskSessions"] = map[string]any{"items": sessionItems}
	state["taskSessionsByTask"] = map[string]any{
		"itemsByTaskId":   map[string]any{taskID: sessionList},
		"loadingByTaskId": map[string]any{taskID: false},
		"loadedByTaskId":  map[string]any{taskID: true},
	}
	state["turns"] = b.taskDetailTurnsState(ctx, activeSessionID)
	state["environmentIdBySessionId"] = environmentBySession
	state["worktrees"] = map[string]any{"items": worktrees}
	state["sessionWorktreesBySessionId"] = map[string]any{"itemsBySessionId": worktreesBySession}
	if len(sessionModelsByID) > 0 {
		state["sessionModels"] = map[string]any{"bySessionId": sessionModelsByID}
	}
}

func (b bootStateBuilder) taskDetailTurnsState(ctx context.Context, sessionID string) map[string]any {
	bySession := map[string]any{}
	activeBySession := activeTurnBySessionState(sessionID)
	if sessionID == "" {
		return map[string]any{"bySession": bySession, "activeBySession": activeBySession}
	}
	turns, err := b.p.taskSvc.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		b.logBootError("list task detail turns", err)
		return map[string]any{"bySession": bySession, "activeBySession": activeBySession}
	}
	items := make([]taskdto.TurnDTO, 0, len(turns))
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		items = append(items, taskdto.FromTurn(turn))
		if turn.CompletedAt == nil {
			activeBySession[sessionID] = turn.ID
		}
	}
	bySession[sessionID] = items
	return map[string]any{"bySession": bySession, "activeBySession": activeBySession}
}

func taskSessionModelsBootState(
	snapshot lifecycle.SessionModelsSnapshot,
	baseline map[string]string,
) map[string]any {
	models := make([]map[string]any, 0, len(snapshot.Models))
	for _, model := range snapshot.Models {
		models = append(models, map[string]any{
			"modelId":         model.ModelID,
			"name":            model.Name,
			"description":     model.Description,
			"usageMultiplier": model.UsageMultiplier,
		})
	}
	options := make([]map[string]any, 0, len(snapshot.ConfigOptions))
	for _, option := range snapshot.ConfigOptions {
		options = append(options, map[string]any{
			"type":         option.Type,
			"id":           option.ID,
			"name":         option.Name,
			"description":  option.Description,
			"currentValue": option.CurrentValue,
			"category":     option.Category,
			"options":      option.Options,
		})
	}
	state := map[string]any{
		"currentModelId": snapshot.CurrentModelID,
		"models":         models,
		"configOptions":  options,
	}
	if len(baseline) > 0 {
		state["configBaseline"] = baseline
	}
	return state
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
