package backendapp

import (
	"context"
	"encoding/json"
	"net/http"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/i18n"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (b bootStateBuilder) taskDetailRouteData(ctx context.Context, req *http.Request, taskID, requestedSessionID string) map[string]any {
	if b.p.taskSvc == nil || taskID == "" {
		return nil
	}
	task, err := b.p.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		b.logBootError("get task detail task", err)
		return nil
	}
	sessionObservations, err := b.p.taskSvc.ListTaskSessionSummaryObservations(ctx, task.ID)
	if err != nil {
		b.logBootError("list task detail session summaries", err)
		sessionObservations = nil
	}
	sessions := make([]*taskmodels.TaskSession, 0, len(sessionObservations))
	for _, observation := range sessionObservations {
		if observation != nil {
			sessions = append(sessions, observation.ToTaskSession())
		}
	}
	activeSessionID := resolveTaskDetailSessionID(task, sessions, requestedSessionID)
	var activeSession *taskmodels.TaskSession
	if activeSessionID != "" {
		activeSession, err = b.p.taskSvc.GetTaskSession(ctx, activeSessionID)
		if err != nil {
			b.logBootError("get task detail active session", err)
			activeSession = nil
		} else if activeSession.TaskID != task.ID {
			activeSession = nil
		}
	}
	taskDTO := b.taskDTOWithSessionInfo(ctx, task)
	settings, hasSettings := b.userSettings(ctx)
	initialState := b.taskDetailInitialState(ctx, task, taskDTO, sessions, activeSession, activeSessionID, settings, hasSettings)
	sidebarTaskPage := b.taskDetailSidebarTaskPage(ctx, req, task, settings, hasSettings)
	return map[string]any{
		"taskDetail": map[string]any{
			"task":             taskDTO,
			"sessionId":        nullString(activeSessionID),
			"initialState":     initialState,
			"initialTerminals": []any{},
			"sidebarTaskPage":  sidebarTaskPage,
		},
	}
}

func (b bootStateBuilder) taskDetailSidebarTaskPage(
	ctx context.Context,
	req *http.Request,
	task *taskmodels.Task,
	settings userdto.UserSettingsResponse,
	hasSettings bool,
) any {
	if task == nil || b.p.taskSvc == nil {
		return nil
	}
	var settingsDTO *userdto.UserSettingsDTO
	if hasSettings {
		settingsDTO = &settings.Settings
	}
	query, prefs := taskDetailSidebarQuery(settingsDTO, task.WorkspaceID, i18n.FromRequest(req))
	page, err := b.p.taskSvc.QuerySidebarTaskPage(ctx, task.WorkspaceID, query, prefs)
	if err != nil {
		b.logBootError("query task detail sidebar page", err)
		return nil
	}
	dtos := b.taskDTOsWithSessionInfo(ctx, page.Tasks)
	tasksByID := make(map[string]*taskdto.TaskDTO, len(dtos))
	for index := range dtos {
		tasksByID[dtos[index].ID] = &dtos[index]
	}
	type sidebarPageEntry struct {
		Kind             string           `json:"kind"`
		TaskID           string           `json:"task_id,omitempty"`
		Task             *taskdto.TaskDTO `json:"task,omitempty"`
		GroupKey         string           `json:"group_key,omitempty"`
		GroupLabel       string           `json:"group_label,omitempty"`
		WorkflowName     string           `json:"workflow_name,omitempty"`
		StepName         string           `json:"workflow_step_name,omitempty"`
		StepColor        string           `json:"workflow_step_color,omitempty"`
		Depth            int              `json:"depth,omitempty"`
		ParentID         string           `json:"parent_id,omitempty"`
		ParentTitle      string           `json:"parent_title,omitempty"`
		Continuation     bool             `json:"continuation,omitempty"`
		MatchingCount    int              `json:"matching_count,omitempty"`
		WIPQueuePosition int              `json:"wip_queue_position,omitempty"`
		WIPQueueTotal    int              `json:"wip_queue_total,omitempty"`
		SubtaskCount     int              `json:"subtask_count,omitempty"`
	}
	type sidebarPageResponse struct {
		QueryKey          string             `json:"query_key"`
		Page              int                `json:"page"`
		PageSize          int                `json:"page_size"`
		TotalEntries      int                `json:"total_entries"`
		TotalTasks        int                `json:"total_tasks"`
		TotalVisibleTasks int                `json:"total_visible_tasks"`
		HasPrevious       bool               `json:"has_previous"`
		HasNext           bool               `json:"has_next"`
		Entries           []sidebarPageEntry `json:"entries"`
	}
	entries := make([]sidebarPageEntry, 0, len(page.Entries))
	for _, entry := range page.Entries {
		entries = append(entries, sidebarPageEntry{
			Kind: entry.Kind, TaskID: entry.TaskID, Task: tasksByID[entry.TaskID],
			GroupKey: entry.GroupKey, GroupLabel: entry.GroupLabel, Depth: entry.Depth,
			WorkflowName: entry.WorkflowName, StepName: entry.StepName, StepColor: entry.StepColor,
			ParentID: entry.ParentID, ParentTitle: entry.ParentTitle,
			Continuation: entry.Continuation, MatchingCount: entry.MatchingCount,
			WIPQueuePosition: entry.WIPQueuePosition, WIPQueueTotal: entry.WIPQueueTotal,
			SubtaskCount: entry.SubtaskCount,
		})
	}
	return sidebarPageResponse{
		QueryKey: page.QueryKey, Page: page.Page, PageSize: page.PageSize,
		TotalEntries: page.TotalEntries, TotalTasks: page.TotalTasks,
		TotalVisibleTasks: page.TotalVisibleTasks, HasPrevious: page.HasPrevious,
		HasNext: page.HasNext, Entries: entries,
	}
}

func taskDetailSidebarQuery(
	settings *userdto.UserSettingsDTO,
	workspaceID string,
	locale string,
) (taskmodels.SidebarTaskViewQuery, taskmodels.SidebarTaskViewPreferences) {
	query := taskmodels.SidebarTaskViewQuery{
		Filters: []taskmodels.SidebarTaskViewClause{},
		Sort:    taskmodels.SidebarTaskViewSort{Key: "state", Direction: "asc"},
		Group:   bootRepositoryGroup, Page: 1, PageSize: taskmodels.MaxSidebarTaskPageSize, Locale: locale,
	}
	if query.Locale == "" {
		query.Locale = i18n.DefaultLocale
	}
	prefs := taskmodels.SidebarTaskViewPreferences{}
	if settings == nil {
		return query, prefs
	}
	settingsState := *settings
	views := settingsState.SidebarViews
	activeViewID := settingsState.SidebarActiveViewID
	draft := settingsState.SidebarDraft
	if scoped, ok := settingsState.SidebarViewsByWorkspace[workspaceID]; ok && len(scoped.Views) > 0 {
		views = scoped.Views
		activeViewID = scoped.ActiveViewID
		draft = scoped.Draft
	}
	var activeView *usermodels.SidebarView
	for index := range views {
		if views[index].ID == activeViewID {
			activeView = &views[index]
			break
		}
	}
	if activeView == nil && len(views) > 0 {
		activeView = &views[0]
	}
	if activeView != nil {
		query.Filters = sidebarTaskViewClauses(activeView.Filters)
		query.Sort = sidebarTaskViewSort(activeView.Sort)
		query.Group = activeView.Group
		query.CollapsedGroupKeys = append([]string{}, activeView.CollapsedGroups...)
		if draft != nil && draft.BaseViewID == activeView.ID {
			query.Filters = sidebarTaskViewClauses(draft.Filters)
			query.Sort = sidebarTaskViewSort(draft.Sort)
			query.Group = draft.Group
		}
	}
	prefs.PinnedTaskIDs = settingsState.SidebarTaskPrefs.PinnedTaskIDs
	prefs.OrderedTaskIDs = settingsState.SidebarTaskPrefs.OrderedTaskIDs
	prefs.SubtaskOrderByParentID = settingsState.SidebarTaskPrefs.SubtaskOrderByParentID
	automation, err := json.Marshal(settingsState.SidebarTaskColorAutomation)
	if err == nil {
		prefs.ColorSettings = &taskmodels.SidebarTaskColorSettings{
			ManualColors: settingsState.SidebarTaskColors,
			Automation:   automation,
		}
	}
	if query.Validate() != nil {
		query.Filters = []taskmodels.SidebarTaskViewClause{}
		query.Sort = taskmodels.SidebarTaskViewSort{Key: "state", Direction: "asc"}
		query.Group = bootRepositoryGroup
		query.CollapsedGroupKeys = []string{}
	}
	return query, prefs
}

func sidebarTaskViewClauses(clauses []usermodels.SidebarViewClause) []taskmodels.SidebarTaskViewClause {
	result := make([]taskmodels.SidebarTaskViewClause, 0, len(clauses))
	for _, clause := range clauses {
		result = append(result, taskmodels.SidebarTaskViewClause{
			Dimension: clause.Dimension, Op: clause.Op, Value: clause.Value,
		})
	}
	return result
}

func sidebarTaskViewSort(sort usermodels.SidebarViewSort) taskmodels.SidebarTaskViewSort {
	result := taskmodels.SidebarTaskViewSort{
		Key: sort.Key, Direction: sort.Direction, Color: sort.Color,
		ThenBy: make([]taskmodels.SidebarTaskViewSortCriterion, 0, len(sort.ThenBy)),
	}
	for _, criterion := range sort.ThenBy {
		result.ThenBy = append(result.ThenBy, taskmodels.SidebarTaskViewSortCriterion{
			Key: criterion.Key, Direction: criterion.Direction, Color: criterion.Color,
		})
	}
	return result
}

func resolveTaskDetailSessionID(task *taskmodels.Task, sessions []*taskmodels.TaskSession, requestedSessionID string) string {
	if task == nil {
		return ""
	}
	for _, session := range sessions {
		if session != nil && session.TaskID == task.ID && session.ID == requestedSessionID && requestedSessionID != "" {
			return session.ID
		}
	}
	for _, session := range sessions {
		if session != nil && session.TaskID == task.ID && session.IsPrimary {
			return session.ID
		}
	}
	for _, session := range sessions {
		if session != nil && session.TaskID == task.ID && session.ID != "" {
			return session.ID
		}
	}
	return ""
}

func (b bootStateBuilder) taskDetailInitialState(
	ctx context.Context,
	task *taskmodels.Task,
	taskDTO taskdto.TaskDTO,
	sessions []*taskmodels.TaskSession,
	activeSession *taskmodels.TaskSession,
	activeSessionID string,
	settings userdto.UserSettingsResponse,
	hasSettings bool,
) map[string]any {
	state := map[string]any{}
	b.addTaskDetailResourceState(ctx, state, task, settings, hasSettings)
	b.addTaskDetailKanbanState(ctx, state, task, taskDTO)
	b.addTaskDetailActiveTaskState(ctx, state, taskDTO, activeSessionID)
	var projections bootSessionRuntimeProvider
	if b.p.orchestratorSvc != nil {
		projections = b.p.orchestratorSvc
	}
	b.addTaskDetailSessionsState(ctx, state, task.ID, sessions, activeSession, activeSessionID, projections)
	b.addTaskDetailAgentsState(ctx, state)
	return state
}

func (b bootStateBuilder) addTaskDetailResourceState(
	ctx context.Context,
	state map[string]any,
	task *taskmodels.Task,
	settings userdto.UserSettingsResponse,
	hasSettings bool,
) {
	b.addWorkspaceState(ctx, state, &task.WorkspaceID)
	if hasSettings {
		state["userSettings"] = mapUserSettingsState(settings, task.WorkspaceID)
	}
	workflows, err := b.p.taskSvc.ListWorkflows(ctx, task.WorkspaceID, true)
	if err != nil {
		b.logBootError("list task detail workflows", err)
	} else {
		state["workflows"] = map[string]any{
			"items":                workflowItemStates(workflows),
			"activeId":             nil,
			"taskWorkflowCoverage": b.taskWorkflowCoverage(ctx, task.WorkspaceID),
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

func (b bootStateBuilder) addTaskDetailKanbanState(
	ctx context.Context,
	state map[string]any,
	task *taskmodels.Task,
	taskDTO taskdto.TaskDTO,
) {
	if task.WorkflowID == "" {
		state["kanban"] = map[string]any{"workflowId": "", "steps": []any{}, "tasks": []any{}, "isLoading": false}
		return
	}
	steps, err := b.workflowStepStates(ctx, task.WorkflowID)
	if err != nil {
		b.logBootError("list task detail workflow steps", err)
		return
	}
	selectedTask := mapKanbanTaskState(taskDTO)
	workflowName := ""
	if workflows, ok := state["workflows"].(map[string]any); ok {
		if items, ok := workflows["items"].([]map[string]any); ok {
			for _, workflow := range items {
				if workflow["id"] == task.WorkflowID {
					workflowName, _ = workflow["name"].(string)
					break
				}
			}
		}
	}
	snapshot := map[string]any{
		"workflowId":    task.WorkflowID,
		"workflowName":  workflowName,
		"steps":         steps,
		"tasks":         []map[string]any{selectedTask},
		"isPlaceholder": true,
	}
	state["kanban"] = map[string]any{
		"workflowId": task.WorkflowID,
		"steps":      steps,
		"tasks":      []map[string]any{selectedTask},
		"isLoading":  false,
	}
	state["kanbanMulti"] = map[string]any{
		"snapshots": map[string]any{task.WorkflowID: snapshot},
		"isLoading": false,
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
	state["turns"] = taskDetailTurnWindowState(activeSessionID, taskmodels.MessageTurnWindow{})
	if activeSessionID == "" {
		return
	}
	window, err := b.p.taskSvc.ListMessagesWithTurnWindow(ctx, taskservice.ListMessagesRequest{
		TaskSessionID: activeSessionID,
		Limit:         50,
		Sort:          "desc",
		IncludeTurns:  true,
	})
	if err != nil {
		b.logBootError("list task detail message turn window", err)
		return
	}
	apiMessages := make([]*v1.Message, 0, len(window.Messages))
	for i := len(window.Messages) - 1; i >= 0; i-- {
		if window.Messages[i] != nil {
			apiMessages = append(apiMessages, window.Messages[i].ToAPI())
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
				"hasMore":      window.HasMore,
				"oldestCursor": oldest,
			},
		},
	}
	state["turns"] = taskDetailTurnWindowState(activeSessionID, window)
}

func taskDetailTurnWindowState(sessionID string, window taskmodels.MessageTurnWindow) map[string]any {
	bySession := map[string]any{}
	activeBySession := activeTurnBySessionState(sessionID)
	windowCoverageBySession := map[string]any{}
	if sessionID == "" {
		return map[string]any{
			"bySession":               bySession,
			"activeBySession":         activeBySession,
			"loadedBySession":         map[string]bool{},
			"windowCoverageBySession": windowCoverageBySession,
		}
	}
	items := make([]taskdto.TurnDTO, 0, len(window.Turns))
	for _, turn := range window.Turns {
		if turn != nil {
			items = append(items, taskdto.FromTurn(turn))
		}
	}
	bySession[sessionID] = items
	if window.Coverage != nil {
		if window.Coverage.ActiveTurnID != "" {
			activeBySession[sessionID] = window.Coverage.ActiveTurnID
		}
		windowCoverageBySession[sessionID] = map[string]any{
			"messageIds":         window.Coverage.MessageIDs,
			"activeTurnObserved": true,
		}
	}
	return map[string]any{
		"bySession":               bySession,
		"activeBySession":         activeBySession,
		"loadedBySession":         map[string]bool{},
		"windowCoverageBySession": windowCoverageBySession,
	}
}

func lastSessionByTaskState(taskID, sessionID string) map[string]string {
	if taskID == "" || sessionID == "" {
		return map[string]string{}
	}
	return map[string]string{taskID: sessionID}
}

type bootSessionRuntimeProvider interface {
	taskdto.ForegroundActivityProvider
	taskdto.CancellationPendingProvider
	taskdto.ParkedProvider
}

func (b bootStateBuilder) addTaskDetailSessionsState(
	ctx context.Context,
	state map[string]any,
	taskID string,
	sessions []*taskmodels.TaskSession,
	activeSession *taskmodels.TaskSession,
	activeSessionID string,
	projections bootSessionRuntimeProvider,
) {
	sessionItems := make(map[string]any, len(sessions))
	sessionList := make([]any, 0, len(sessions))
	environmentBySession := make(map[string]string, len(sessions))
	worktrees := make(map[string]any)
	worktreesBySession := make(map[string]any)
	sessionModelsByID := make(map[string]any)
	sessionModeByID := make(map[string]any)
	sessionMCPStatusByID := make(map[string]any)
	pendingActionsBySession, err := b.bootPendingActionsForInputCapableSessions(
		ctx,
		map[string][]*taskmodels.TaskSession{taskID: sessions},
	)
	if err != nil {
		b.logBootError("get task detail session pending actions", err)
		pendingActionsBySession = map[string]taskmodels.TaskPendingAction{}
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		isActive := session.ID == activeSessionID
		if isActive && activeSession != nil {
			session = activeSession
		}
		summaryDTO := taskdto.FromTaskSessionSummary(session)
		var item any
		if isActive {
			item = b.taskDetailActiveSessionDTO(ctx, session, pendingActionsBySession, projections)
		} else {
			enrichBootSessionRuntime(&summaryDTO, projections)
			summaryDTO.PendingAction = bootPendingActionPtr(&session.ID, pendingActionsBySession)
			item = summaryDTO
		}
		sessionItems[session.ID] = item
		sessionList = append(sessionList, item)
		if session.TaskEnvironmentID != "" {
			environmentBySession[session.ID] = session.TaskEnvironmentID
		}
		if summaryDTO.WorktreeID != "" {
			worktrees[summaryDTO.WorktreeID] = map[string]any{
				"id":           summaryDTO.WorktreeID,
				"sessionId":    session.ID,
				"repositoryId": nullString(summaryDTO.RepositoryID),
				"path":         nullString(summaryDTO.WorktreePath),
				branchFieldKey: nullString(summaryDTO.WorktreeBranch),
			}
			worktreesBySession[session.ID] = []string{summaryDTO.WorktreeID}
		}
		if !isActive {
			continue
		}
		addTaskDetailSessionMetadata(session, sessionModelsByID, sessionModeByID, sessionMCPStatusByID)
	}
	state["taskSessions"] = map[string]any{"items": sessionItems}
	state["taskSessionsByTask"] = map[string]any{
		"itemsByTaskId":   map[string]any{taskID: sessionList},
		"loadingByTaskId": map[string]any{taskID: false},
		"loadedByTaskId":  map[string]any{taskID: true},
	}
	state["environmentIdBySessionId"] = environmentBySession
	state["worktrees"] = map[string]any{"items": worktrees}
	state["sessionWorktreesBySessionId"] = map[string]any{"itemsBySessionId": worktreesBySession}
	if len(sessionModelsByID) > 0 {
		state["sessionModels"] = map[string]any{"bySessionId": sessionModelsByID}
	}
	if len(sessionModeByID) > 0 {
		state["sessionMode"] = map[string]any{"bySessionId": sessionModeByID}
	}
	if len(sessionMCPStatusByID) > 0 {
		state["sessionMcpStatus"] = map[string]any{"bySessionId": sessionMCPStatusByID}
	}
}

func enrichBootSessionRuntime(summary *taskdto.TaskSessionSummaryDTO, projections bootSessionRuntimeProvider) {
	taskdto.EnrichForegroundActivitySummary(summary, projections)
	taskdto.EnrichCancellationPendingSummary(summary, projections)
	taskdto.EnrichParkedProjectionSummary(summary, projections)
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
		"items":         agentProfileOptionStates(response.Agents),
		versionFieldKey: 0,
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

func (b bootStateBuilder) taskDetailActiveSessionDTO(ctx context.Context, session *taskmodels.TaskSession, pending map[string]taskmodels.TaskPendingAction, projections bootSessionRuntimeProvider) taskdto.TaskSessionDTO {
	fullDTO := taskdto.FromTaskSession(session)
	if session.TaskEnvironmentID != "" {
		operation, runnerLive, err := b.p.taskSvc.WorkspaceRecoveryProjection(ctx, session.TaskEnvironmentID)
		if err != nil {
			b.logBootError("get task detail workspace recovery projection", err)
		} else {
			taskdto.EnrichWorkspaceRecovery(&fullDTO, operation, runnerLive)
		}
	}
	taskdto.EnrichForegroundActivity(&fullDTO, projections)
	taskdto.EnrichCancellationPending(&fullDTO, projections)
	taskdto.EnrichParkedProjection(&fullDTO, projections)
	fullDTO.PendingAction = bootPendingActionPtr(&session.ID, pending)
	return fullDTO
}
