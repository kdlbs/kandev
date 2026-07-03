package backendapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

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
	settingsTasksListSort := ""
	settingsTasksListGroup := ""
	if hasSettings {
		settingsWorkspaceID = settings.Settings.WorkspaceID
		settingsWorkflowID = settings.Settings.WorkflowFilterID
		settingsTasksListSort = settings.Settings.TasksListSort
		settingsTasksListGroup = settings.Settings.TasksListGroup
		if len(settings.Settings.RepositoryIDs) > 0 {
			settingsRepositoryID = settings.Settings.RepositoryIDs[0]
		}
	}
	tasksListSort := tasksListSortForRoute(queryValue(req, "sort"), settingsTasksListSort)
	tasksListGroup := tasksListGroupForRoute(queryValue(req, "group"), settingsTasksListGroup)
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
		return state, map[string]any{"activeWorkspaceId": nil, "workflows": []any{}, "steps": []any{}, "repositories": []any{}, "tasks": []any{}, "total": 0, "tasksListSort": tasksListSort, "tasksListGroup": tasksListGroup}
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
	tasks, total := b.tasksForWorkspace(ctx, activeWorkspaceID, activeWorkflowID, settingsRepositoryID, tasksListSort)
	routeData := map[string]any{
		"activeWorkspaceId": activeWorkspaceID,
		"workflows":         workflowsToDTOs(workflows),
		"steps":             steps,
		"repositories":      repositories,
		"tasks":             tasks,
		"total":             total,
		"tasksListSort":     tasksListSort,
		"tasksListGroup":    tasksListGroup,
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

func resolveHomeWorkflowID(
	workflows []*taskmodels.Workflow,
	queryWorkflowID string,
	settingsWorkflowID string,
	hasSettings bool,
) string {
	if activeWorkflowID := validWorkflowOrEmpty(workflows, queryWorkflowID); activeWorkflowID != "" {
		return activeWorkflowID
	}
	if hasSettings {
		return validWorkflowOrEmpty(workflows, settingsWorkflowID)
	}
	return firstWorkflowID(workflows)
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

func (b bootStateBuilder) tasksForWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, sort string) ([]taskdto.TaskDTO, int) {
	tasks, total, err := b.p.taskSvc.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, "", 1, 25, sort, false, false, false, false)
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
		"tasksListSort":               usermodels.NormalizeTasksListSort(settings.TasksListSort),
		"tasksListGroup":              usermodels.NormalizeTasksListGroup(settings.TasksListGroup),
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

func tasksListSortForRoute(queryValue, settingsValue string) string {
	if usermodels.IsValidTasksListSort(queryValue) {
		return strings.TrimSpace(queryValue)
	}
	return usermodels.NormalizeTasksListSort(settingsValue)
}

func tasksListGroupForRoute(queryValue, settingsValue string) string {
	if usermodels.IsValidTasksListGroup(queryValue) {
		return strings.TrimSpace(queryValue)
	}
	return usermodels.NormalizeTasksListGroup(settingsValue)
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
