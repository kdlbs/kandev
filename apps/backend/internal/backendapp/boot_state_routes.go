package backendapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/httpcookie"
	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

// tasksPageBootData builds the tasks page boot payload: workspaces, repositories, workflows, steps, tasks, and the user's settings.
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
		return state, map[string]any{"activeWorkspaceId": nil, "workflows": []any{}, "steps": []any{}, "repositories": []any{}, "repositorySets": []any{}, "repositoryBranchPolicies": []any{}, "tasks": []any{}, "total": 0, "tasksListSort": tasksListSort, "tasksListGroup": tasksListGroup}
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
	repositorySets := b.repositorySetsForState(ctx, activeWorkspaceID, state)
	repositoryBranchPolicies := b.repositoryBranchPoliciesForState(ctx, activeWorkspaceID, state)
	steps := b.workflowStepsForWorkspace(ctx, activeWorkspaceID)
	tasks, total := b.tasksForWorkspace(ctx, activeWorkspaceID, activeWorkflowID, settingsRepositoryID, tasksListSort)
	routeData := map[string]any{
		"activeWorkspaceId":        activeWorkspaceID,
		"workflows":                workflowsToDTOs(workflows),
		"steps":                    steps,
		"repositories":             repositories,
		"repositorySets":           repositorySets,
		"repositoryBranchPolicies": repositoryBranchPolicies,
		"tasks":                    tasks,
		"total":                    total,
		"tasksListSort":            tasksListSort,
		"tasksListGroup":           tasksListGroup,
	}
	return state, routeData
}

// routeContextBootData builds the workspace/route context boot payload (workspaces, active workspace, workflows).
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
		return state, map[string]any{"activeWorkspaceId": nil, "workflows": []any{}, "steps": []any{}, "repositories": []any{}, "repositorySets": []any{}, "repositoryBranchPolicies": []any{}}
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
	repositorySets := b.repositorySetsForState(ctx, activeWorkspaceID, state)
	repositoryBranchPolicies := b.repositoryBranchPoliciesForState(ctx, activeWorkspaceID, state)
	steps := b.workflowStepsForWorkspace(ctx, activeWorkspaceID)
	return state, map[string]any{
		"activeWorkspaceId":        activeWorkspaceID,
		"workflows":                workflowsToDTOs(workflows),
		"steps":                    steps,
		"repositories":             repositories,
		"repositorySets":           repositorySets,
		"repositoryBranchPolicies": repositoryBranchPolicies,
	}
}

// workspaceIDSet collects the ids of all workspaces into a set.
func workspaceIDSet(workspaces []*taskmodels.Workspace) map[string]bool {
	result := make(map[string]bool, len(workspaces))
	for _, workspace := range workspaces {
		if workspace != nil {
			result[workspace.ID] = true
		}
	}
	return result
}

// workspaceItemStates maps each workspace to its boot state shape.
func workspaceItemStates(workspaces []*taskmodels.Workspace) []map[string]any {
	items := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace != nil {
			items = append(items, mapWorkspaceItemState(taskdto.FromWorkspace(workspace)))
		}
	}
	return items
}

// resolveHomeWorkflowID picks the home workflow: the query param, else the saved settings workflow, else the first valid one.
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

// validWorkflowOrEmpty returns the workflow id when it exists in the list, otherwise empty.
func validWorkflowOrEmpty(workflows []*taskmodels.Workflow, workflowID string) string {
	for _, workflow := range workflows {
		if workflow != nil && workflow.ID == workflowID {
			return workflowID
		}
	}
	return ""
}

// repositoriesForState loads the workspace repositories for the boot payload, logging and returning empty on failure.
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

// repositorySetsForState loads the workspace's repository sets for the boot
// payload, logging and returning empty on failure. The task-create picker offers
// sets on first paint from this, so the hook does not have to fetch.
func (b bootStateBuilder) repositorySetsForState(
	ctx context.Context,
	workspaceID string,
	state map[string]any,
) []taskdto.RepositorySetDTO {
	items := repositorySetsToDTOs(nil)
	loaded := false
	sets, err := b.p.taskSvc.ListRepositorySets(ctx, workspaceID)
	if err != nil {
		// Non-fatal, like every neighbour here: boot succeeds with an empty list
		// and the client fetches instead. Leaving `loaded` false is what makes
		// that fetch happen; marking a failed load as loaded would hide the
		// workspace's real sets until an explicit refresh.
		b.logBootError("list repository sets", err)
	} else {
		items = repositorySetsToDTOs(sets)
		loaded = true
	}
	state["repositorySets"] = repositorySetsState(workspaceID, items, loaded)
	return items
}

// repositoryBranchPoliciesForState hydrates the repository-keyed policy slice
// used by repository settings and the task-create picker.
func (b bootStateBuilder) repositoryBranchPoliciesForState(
	ctx context.Context,
	workspaceID string,
	state map[string]any,
) []taskdto.RepositoryBranchPolicyDTO {
	itemsByRepositoryID := map[string]any{}
	loadingByRepositoryID := map[string]any{}
	loadedByRepositoryID := map[string]any{}
	items := make([]taskdto.RepositoryBranchPolicyDTO, 0)
	repositories, err := b.p.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		b.logBootError("list repositories for branch policies", err)
		state["repositoryBranchPolicies"] = map[string]any{
			"itemsByRepositoryId":   itemsByRepositoryID,
			"loadingByRepositoryId": loadingByRepositoryID,
			"loadedByRepositoryId":  loadedByRepositoryID,
		}
		return items
	}
	policies, err := b.p.taskSvc.ListRepositoryBranchPoliciesForWorkspace(ctx, workspaceID)
	if err != nil {
		b.logBootError("list repository branch policies", err)
		for _, repository := range repositories {
			if repository == nil {
				continue
			}
			itemsByRepositoryID[repository.ID] = []taskdto.RepositoryBranchPolicyDTO{}
			loadedByRepositoryID[repository.ID] = false
			loadingByRepositoryID[repository.ID] = false
		}
	} else {
		policiesByRepositoryID := make(map[string][]*taskmodels.RepositoryBranchPolicy)
		for _, policy := range policies {
			if policy != nil {
				policiesByRepositoryID[policy.RepositoryID] = append(policiesByRepositoryID[policy.RepositoryID], policy)
			}
		}
		for _, repository := range repositories {
			if repository == nil {
				continue
			}
			dtos := repositoryBranchPoliciesToDTOs(policiesByRepositoryID[repository.ID])
			itemsByRepositoryID[repository.ID] = dtos
			loadedByRepositoryID[repository.ID] = true
			loadingByRepositoryID[repository.ID] = false
			items = append(items, dtos...)
		}
	}
	state["repositoryBranchPolicies"] = map[string]any{
		"itemsByRepositoryId":   itemsByRepositoryID,
		"loadingByRepositoryId": loadingByRepositoryID,
		"loadedByRepositoryId":  loadedByRepositoryID,
	}
	return items
}

// repositorySetsState is the workspace-keyed slice shape the web store expects.
// An empty workspace still keys an entry: an absent key reads as "not loaded" to
// the client hook, which then refetches on every dialog open.
func repositorySetsState(
	workspaceID string,
	items []taskdto.RepositorySetDTO,
	loaded bool,
) map[string]any {
	return map[string]any{
		"itemsByWorkspaceId":   map[string]any{workspaceID: items},
		"loadingByWorkspaceId": map[string]any{workspaceID: false},
		"loadedByWorkspaceId":  map[string]any{workspaceID: loaded},
	}
}

// repositorySetsToDTOs maps repository set models to DTOs, always as an array.
func repositorySetsToDTOs(sets []*taskmodels.RepositorySet) []taskdto.RepositorySetDTO {
	items := make([]taskdto.RepositorySetDTO, 0, len(sets))
	for _, set := range sets {
		if set != nil {
			items = append(items, taskdto.FromRepositorySet(set))
		}
	}
	return items
}

func repositoryBranchPoliciesToDTOs(policies []*taskmodels.RepositoryBranchPolicy) []taskdto.RepositoryBranchPolicyDTO {
	items := make([]taskdto.RepositoryBranchPolicyDTO, 0, len(policies))
	for _, policy := range policies {
		if policy != nil {
			items = append(items, taskdto.FromRepositoryBranchPolicy(policy))
		}
	}
	return items
}

// repositoriesToDTOs maps repository models to DTOs.
func repositoriesToDTOs(repositories []*taskmodels.Repository) []taskdto.RepositoryDTO {
	items := make([]taskdto.RepositoryDTO, 0, len(repositories))
	for _, repository := range repositories {
		if repository != nil {
			items = append(items, taskdto.FromRepository(repository))
		}
	}
	return items
}

// workflowsToDTOs maps workflow models to DTOs.
func workflowsToDTOs(workflows []*taskmodels.Workflow) []taskdto.WorkflowDTO {
	items := make([]taskdto.WorkflowDTO, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow != nil {
			items = append(items, taskdto.FromWorkflow(workflow))
		}
	}
	return items
}

// workflowStepsForWorkspace loads the workflow steps for a workspace, returning empty when the service is unavailable.
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

// tasksForWorkspace loads the task page's task list for a workspace/workflow, returning empty on failure.
func (b bootStateBuilder) tasksForWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, sort string) ([]taskdto.TaskDTO, int) {
	tasks, total, err := b.p.taskSvc.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, "", 1, 25, sort, false, false, false, false)
	if err != nil {
		b.logBootError("list tasks page tasks", err)
		return []taskdto.TaskDTO{}, 0
	}
	return b.taskDTOsWithSessionInfo(ctx, tasks), total
}

// mergeBootState overlays src key/value pairs onto dst.
func mergeBootState(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}

// addPromptsState merges the prompts listing into the boot state when available.
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

// addEditorsState merges the editors listing into the boot state when available.
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

// addOfficeRouteState merges office route state into the boot state when the office feature is enabled.
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

// officeWorkspaces resolves the office workspaces and the active workspace id
// from the request cookies and user settings. Candidates, each validated
// against the office workspace set: general cookie (when it names an office
// workspace) → office cookie → settings → first office workspace.
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
	settingsWorkspaceID := ""
	if settings, ok := b.userSettings(ctx); ok {
		settingsWorkspaceID = settings.Settings.WorkspaceID
	}
	return items, resolveActiveOfficeWorkspaceID(
		officeItems,
		readActiveWorkspaceCookie(req),
		readOfficeWorkspaceCookie(req),
		settingsWorkspaceID,
	), nil
}

// officeState assembles the office boot state (agents, projects, inbox, dashboard) for the active workspace.
func (b bootStateBuilder) officeState(ctx context.Context, activeID string) map[string]any {
	agents := b.officeAgents(ctx, activeID)
	projects := b.officeProjects(ctx, activeID)
	inboxItems, inboxCount := b.officeInbox(ctx, activeID)
	dashboard := b.officeDashboard(ctx, activeID)
	// Workspace-scoped collections ship keyed by workspace id, matching the
	// store shape in lib/state/slices/office/types.ts. An empty activeID means
	// no office workspace resolved, so every map stays empty rather than
	// keying data under "".
	byActive := func(value any) map[string]any {
		if activeID == "" {
			return map[string]any{}
		}
		return map[string]any{activeID: value}
	}
	return map[string]any{
		"agentProfilesByWorkspaceId": byActive(agents),
		"skills":                     []any{},
		"projectsByWorkspaceId":      byActive(projects),
		"approvals":                  []any{},
		"activity":                   []any{},
		"costSummary":                nil,
		"budgetPolicies":             []any{},
		"routines":                   []any{},
		"inboxItemsByWorkspaceId":    byActive(inboxItems),
		"inboxCountByWorkspaceId":    byActive(inboxCount),
		"runs":                       []any{},
		"dashboardByWorkspaceId":     byActive(dashboard),
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

// officeAgents loads the office agent list for a workspace, returning empty when unavailable.
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

// officeProjects loads the office project list with counts, returning empty when unavailable.
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

// officeInbox loads the office inbox items and count, returning empty when unavailable.
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

// officeDashboard loads the office dashboard data, returning nil when unavailable.
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

// mapUserSettingsState converts a user settings response to the SPA boot shape, preferring the given workspace id.
func mapUserSettingsState(response userdto.UserSettingsResponse, workspaceID string) map[string]any {
	settings := response.Settings
	effectiveWorkspaceID := nullString(settings.WorkspaceID)
	if workspaceID != "" {
		effectiveWorkspaceID = workspaceID
	}
	return map[string]any{
		"revision":                        settings.Revision,
		"workspaceId":                     effectiveWorkspaceID,
		"kanbanViewMode":                  nullString(settings.KanbanViewMode),
		"startupPage":                     usermodels.NormalizeStartupPage(settings.StartupPage),
		"workflowId":                      nullString(settings.WorkflowFilterID),
		"repositoryIds":                   stringSlice(settings.RepositoryIDs),
		"tasksListSort":                   usermodels.NormalizeTasksListSort(settings.TasksListSort),
		"tasksListGroup":                  usermodels.NormalizeTasksListGroup(settings.TasksListGroup),
		"tasksListShowDetails":            settings.TasksListShowDetails,
		"preferredShell":                  nullString(settings.PreferredShell),
		"shellOptions":                    response.ShellOptions,
		"defaultEditorId":                 nullString(settings.DefaultEditorID),
		"enablePreviewOnClick":            settings.EnablePreviewOnClick,
		"chatSubmitKey":                   defaultString(settings.ChatSubmitKey, "cmd_enter"),
		"reviewAutoMarkOnScroll":          settings.ReviewAutoMarkOnScroll,
		"confirmTaskArchive":              settings.ConfirmTaskArchive,
		"preventAutoStartAgentOnOpen":     settings.PreventAutoStartAgentOnOpen,
		"unreadDivider":                   settings.UnreadDivider,
		"agentGeneratedTaskTitles":        settings.AgentGeneratedTaskTitles,
		"mcpTaskAgentProfileDefault":      usermodels.NormalizeMCPTaskAgentProfileDefault(settings.MCPTaskAgentProfileDefault),
		"showAnchoredPromptBar":           settings.ShowAnchoredPromptBar,
		"showScrollToLastPrompt":          settings.ShowScrollToLastPrompt,
		"showScrollToStart":               settings.ShowScrollToStart,
		"showTranscriptAutoScrollControl": settings.ShowTranscriptAutoScrollControl,
		"showTodoListPanel":               settings.ShowTodoListPanel,

		// Sub-option: only auto-pin when the agent's todo list is not empty.
		"showTodoListPanelOnlyWhenNotEmpty": settings.ShowTodoListPanelOnlyWhenNotEmpty,
		"showReleaseNotification":           settings.ShowReleaseNotification,
		"releaseNotesLastSeenVersion":       nullString(settings.ReleaseNotesLastSeenVersion),
		"lspAutoStartLanguages":             stringSlice(settings.LspAutoStartLanguages),
		"lspAutoInstallLanguages":           stringSlice(settings.LspAutoInstallLanguages),
		"lspStatusHiddenLanguages":          stringSlice(settings.LspStatusHiddenLanguages),
		"lspServerConfigs":                  mapStringMap(settings.LspServerConfigs),
		"lspStatusLocation":                 usermodels.NormalizeLspStatusLocation(settings.LspStatusLocation),
		"savedLayouts":                      settings.SavedLayouts,
		"sidebarViews":                      mapSidebarViews(settings.SidebarViews),
		"sidebarActiveViewId":               nullString(settings.SidebarActiveViewID),
		"sidebarDraft":                      mapSidebarDraft(settings.SidebarDraft),
		"sidebarTaskPrefs":                  mapSidebarTaskPrefs(settings.SidebarTaskPrefs),
		"taskCreateLastUsed":                mapTaskCreateLastUsed(settings.TaskCreateLastUsed),
		"defaultUtilityAgentId":             nullString(settings.DefaultUtilityAgentID),
		"keyboardShortcuts":                 mapStringAny(settings.KeyboardShortcuts),
		"terminalLinkBehavior":              terminalLinkBehavior(settings.TerminalLinkBehavior),
		"terminalFontFamily":                nullString(settings.TerminalFontFamily),
		"terminalFontSize":                  nullInt(settings.TerminalFontSize),
		"changesPanelLayout":                changesPanelLayout(settings.ChangesPanelLayout),
		"lastSeenDisplay":                   lastSeenDisplay(settings.LastSeenDisplay),
		"azureDevOpsBrowsePreferences":      settings.AzureDevOpsBrowsePreferences,
		"systemMetricsDisplay": map[string]any{
			"showInTopbar": settings.SystemMetricsDisplay.ShowInTopbar,
			"simplified":   settings.SystemMetricsDisplay.Simplified,
		},
		"appStatusBarEnabled":               settings.AppStatusBarEnabled,
		"appStatusBarOrder":                 mapAppStatusBarOrder(settings.AppStatusBarOrder),
		"quickChatTabOrderByWorkspace":      settings.QuickChatTabOrderByWorkspace,
		"hiddenWorkflowStepIds":             stringSliceMap(settings.KanbanHiddenStepIDs),
		"workflowIdsWithAutoHideEmptySteps": stringSlice(settings.WorkflowIDsWithAutoHideEmptySteps),
		"loaded":                            true,
	}
}

// mapUserSettingsStateWithWorkflow adds the resolved workflow id onto the settings boot shape.
func mapUserSettingsStateWithWorkflow(response userdto.UserSettingsResponse, workspaceID, workflowID string) map[string]any {
	state := mapUserSettingsState(response, workspaceID)
	state["workflowId"] = nullString(workflowID)
	return state
}

// mapWorkspaceItemState maps a workspace DTO to its SPA boot shape.
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

// mapWorkflowItemState maps a workflow DTO to its SPA boot shape.
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

// mapKanbanStepState maps a workflow step DTO to the kanban step boot shape.
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
		"wip_limit":             step.WIPLimit,
		"pull_from_step_id":     nullString(step.PullFromStepID),
	}
}

// mapKanbanTaskState maps a task DTO to the kanban task boot shape.
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
		"id":                          task.ID,
		"workflowStepId":              task.WorkflowStepID,
		"title":                       task.Title,
		"description":                 task.Description,
		"position":                    task.Position,
		"state":                       task.State,
		"repositoryId":                primaryRepositoryID,
		"repositories":                repositories,
		"primarySessionId":            task.PrimarySessionID,
		"primarySessionState":         task.PrimarySessionState,
		"primarySessionPendingAction": task.PrimarySessionPendingAction,
		"taskPendingAction":           task.TaskPendingAction,
		"wipAdmitted":                 task.WIPAdmitted,
		"queuedForStepId":             nullString(task.QueuedForStepID),
		"queuedAt":                    task.QueuedAt,
		"interrupted":                 task.Interrupted,
		"autoStartFailed":             task.AutoStartFailed,
		"statusSummary":               task.StatusSummary,
		"sessionCount":                task.SessionCount,
		"reviewStatus":                nullString(string(task.ReviewStatus)),
		"parentTaskId":                nullString(task.ParentID),
		"updatedAt":                   task.UpdatedAt,
		"createdAt":                   task.CreatedAt,
		// Dependency projection. This mapper is a camelCase whitelist writing
		// straight into the frontend store shape, so a new DTO field is invisible
		// to the boot payload until it is listed here — the board badge and the
		// dependency chip both read these on first paint.
		"blocked":            task.Blocked,
		"blockedReason":      nullString(task.BlockedReason),
		"dependsOn":          dependencyRefStates(task.DependsOn),
		"blocks":             dependencyRefStates(task.Blocks),
		"startWhenUnblocked": task.StartWhenUnblocked,
	}
}

// dependencyRefStates maps dependency edge entries into the store shape. Returns
// an empty slice rather than nil so the client reads "no edges" instead of
// "unknown" and does not keep a stale badge.
func dependencyRefStates(refs []taskdto.TaskDependencyRefDTO) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, map[string]any{
			"id":      ref.ID,
			"title":   ref.Title,
			"state":   ref.State,
			statusKey: ref.Status,
		})
	}
	return out
}

// mapSidebarViews maps sidebar views to the SPA boot shape.
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
			"taskRow":         mapSidebarTaskRow(view.TaskRow),
		})
	}
	return result
}

// mapSidebarDraft maps the sidebar draft (nil-safe) to the boot shape.
func mapSidebarDraft(draft *usermodels.SidebarViewDraft) map[string]any {
	if draft == nil {
		return nil
	}
	return map[string]any{
		"baseViewId": draft.BaseViewID,
		"filters":    draft.Filters,
		"sort":       draft.Sort,
		"group":      draft.Group,
		"taskRow":    mapSidebarTaskRow(draft.TaskRow),
	}
}

func mapSidebarTaskRow(value *usermodels.SidebarTaskRowPresentation) map[string]any {
	if value == nil {
		return nil
	}
	return map[string]any{
		"detailsEnabled": value.DetailsEnabled,
		"detailOrder":    stringSlice(value.DetailOrder),
		"visibleDetails": stringSlice(value.VisibleDetails),
		"trailing":       value.Trailing,
	}
}

// mapSidebarTaskPrefs maps sidebar task prefs to the boot shape.
func mapSidebarTaskPrefs(prefs usermodels.SidebarTaskPrefs) map[string]any {
	return map[string]any{
		"pinnedTaskIds":          stringSlice(prefs.PinnedTaskIDs),
		"orderedTaskIds":         stringSlice(prefs.OrderedTaskIDs),
		"subtaskOrderByParentId": stringSliceMap(prefs.SubtaskOrderByParentID),
	}
}

// mapAppStatusBarOrder maps the status bar order to the boot shape.
func mapAppStatusBarOrder(order usermodels.AppStatusBarOrder) map[string]any {
	return map[string]any{
		"leftItemIds":  stringSlice(order.LeftItemIDs),
		"rightItemIds": stringSlice(order.RightItemIDs),
	}
}

// mapTaskCreateLastUsed maps the last task-creation choices to the boot shape.
func mapTaskCreateLastUsed(value usermodels.TaskCreateLastUsed) map[string]any {
	workflowIDsByWorkspace := value.WorkflowIDsByWorkspace
	if workflowIDsByWorkspace == nil {
		workflowIDsByWorkspace = map[string]string{}
	}
	return map[string]any{
		"repositoryId":           nullString(value.RepositoryID),
		branchFieldKey:           nullString(value.Branch),
		"agentProfileId":         nullString(value.AgentProfileID),
		"executorProfileId":      nullString(value.ExecutorProfileID),
		"workflowIdsByWorkspace": workflowIDsByWorkspace,
		"synced": value.RepositoryID != "" || value.Branch != "" || value.AgentProfileID != "" ||
			value.ExecutorProfileID != "" || len(workflowIDsByWorkspace) > 0,
	}
}

// resolveActiveOfficeWorkspaceID returns the first candidate — general cookie,
// office cookie, settings — that names a valid office workspace, otherwise the
// first office workspace id.
func resolveActiveOfficeWorkspaceID(
	workspaces []taskdto.WorkspaceDTO,
	generalCookieID string,
	officeCookieID string,
	settingsID string,
) string {
	for _, candidate := range []string{generalCookieID, officeCookieID, settingsID} {
		for _, workspace := range workspaces {
			if workspace.ID == candidate {
				return workspace.ID
			}
		}
	}
	if len(workspaces) > 0 {
		return workspaces[0].ID
	}
	return ""
}

// firstValidID returns the first non-empty candidate present in the valid set.
func firstValidID(valid map[string]bool, candidates ...string) string {
	for _, candidate := range candidates {
		value := strings.TrimSpace(candidate)
		if value != "" && valid[value] {
			return value
		}
	}
	return ""
}

// firstWorkspaceID returns the first workspace id in the list.
func firstWorkspaceID(workspaces []*taskmodels.Workspace) string {
	for _, workspace := range workspaces {
		if workspace != nil && workspace.ID != "" {
			return workspace.ID
		}
	}
	return ""
}

// firstWorkflowID returns the first workflow id in the list.
func firstWorkflowID(workflows []*taskmodels.Workflow) string {
	for _, workflow := range workflows {
		if workflow != nil && workflow.ID != "" {
			return workflow.ID
		}
	}
	return ""
}

// queryValue reads a trimmed query parameter, returning empty when absent.
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

// tasksListSortForRoute prefers a valid query sort, falling back to the saved settings value.
func tasksListSortForRoute(queryValue, settingsValue string) string {
	if usermodels.IsValidTasksListSort(queryValue) {
		return strings.TrimSpace(queryValue)
	}
	return usermodels.NormalizeTasksListSort(settingsValue)
}

// tasksListGroupForRoute prefers a valid query group, falling back to the saved settings value.
func tasksListGroupForRoute(queryValue, settingsValue string) string {
	if usermodels.IsValidTasksListGroup(queryValue) {
		return strings.TrimSpace(queryValue)
	}
	return usermodels.NormalizeTasksListGroup(settingsValue)
}

// readActiveWorkspaceCookie reads the active workspace id from the GENERAL
// cookie family only: the port-scoped name first, then the legacy unprefixed
// name as a read-only fallback (a pre-upgrade selection survives). The
// office-family cookies are deliberately not consulted — generic boot paths
// resolve settings/first when only an office cookie exists (parity with the
// frontend generic reader). The suffix derives from the request host
// (X-Forwarded-Host precedence), the same port the SPA derives from the API
// base URL.
func readActiveWorkspaceCookie(req *http.Request) string {
	if req == nil {
		return ""
	}
	// On a default-port host the scoped name IS the legacy name; reading the
	// same cookie twice is a no-op, so only probe the legacy name when the
	// port actually scopes it.
	names := []string{httpcookie.ScopedName(req, activeWorkspaceCookie)}
	if scoped := names[0]; scoped != activeWorkspaceCookie {
		names = append(names, activeWorkspaceCookie)
	}
	for _, name := range names {
		cookie, err := req.Cookie(name)
		if err == nil {
			if value := strings.TrimSpace(cookie.Value); value != "" {
				return value
			}
		}
	}
	return ""
}

// readOfficeWorkspaceCookie reads the active workspace id from the OFFICE
// cookie family only: the port-scoped name first, then the legacy unprefixed
// name as a read-only fallback. The general cookie is never consulted here.
func readOfficeWorkspaceCookie(req *http.Request) string {
	if req == nil {
		return ""
	}
	// Same default-port dedupe as readActiveWorkspaceCookie: the scoped name
	// equals the legacy name when the Host carries no (non-default) port.
	names := []string{httpcookie.ScopedName(req, legacyOfficeWorkspaceCookie)}
	if scoped := names[0]; scoped != legacyOfficeWorkspaceCookie {
		names = append(names, legacyOfficeWorkspaceCookie)
	}
	for _, name := range names {
		cookie, err := req.Cookie(name)
		if err == nil {
			if value := strings.TrimSpace(cookie.Value); value != "" {
				return value
			}
		}
	}
	return ""
}

// nullString returns nil for an empty string (JSON null).
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// nullInt returns nil for a zero int (JSON null).
func nullInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

// stringSlice returns an empty slice instead of nil.
func stringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

// stringSliceMap returns an empty map instead of nil.
func stringSliceMap(value map[string][]string) map[string][]string {
	if value == nil {
		return map[string][]string{}
	}
	return value
}

// mapStringAny returns an empty map instead of nil.
func mapStringAny(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

// mapStringMap returns an empty map instead of nil.
func mapStringMap(value map[string]map[string]any) map[string]map[string]any {
	if value == nil {
		return map[string]map[string]any{}
	}
	return value
}

// defaultString returns the fallback when the value is empty.
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// terminalLinkBehavior normalizes the terminal link behavior to new_tab or browser_panel.
func terminalLinkBehavior(value string) string {
	if value == "browser_panel" {
		return "browser_panel"
	}
	return "new_tab"
}

// changesPanelLayout normalizes the changes panel layout to flat or tree.
func changesPanelLayout(value string) string {
	if value == "flat" {
		return "flat"
	}
	return "tree"
}

// lastSeenDisplay normalizes the last-seen display to absolute or relative.
func lastSeenDisplay(value string) string {
	return usermodels.NormalizeLastSeenDisplay(value)
}

// logBootError logs a debug entry when optional boot data failed to load.
func (b bootStateBuilder) logBootError(operation string, err error) {
	if err == nil || b.p.log == nil {
		return
	}
	b.p.log.Debug("SPA boot state skipped optional data", zap.String("operation", operation), zap.Error(err))
}
