package backendapp

import (
	"context"
	"net/http"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userdto "github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/webapp"
)

const (
	activeWorkspaceCookie       = "kandev-active-workspace"
	legacyOfficeWorkspaceCookie = "office-active-workspace"
	bootStateKeySessionID       = "sessionId"
	bootStateKeyWorkspaceID     = "workspaceId"
	bootRepositoryGroup         = "repository"
)

// ssoProvidersForBoot lists the plugin-contributed external-login options for
// the pre-auth login screen. Empty unless auth is enabled and at least one
// active auth-capable plugin declares auth_providers.
func ssoProvidersForBoot(p routeParams) []auth.SSOProvider {
	if p.authSvc == nil || p.authSvc.Mode() == auth.ModeDisabled {
		return nil
	}
	if p.services == nil || p.services.Plugins == nil {
		return nil
	}
	providers := p.services.Plugins.SSOProviders()
	if len(providers) == 0 {
		return nil
	}
	out := make([]auth.SSOProvider, 0, len(providers))
	for _, prov := range providers {
		out = append(out, auth.SSOProvider{
			ID:          prov.ID,
			DisplayName: prov.DisplayName,
			InitiateURL: prov.InitiateURL,
		})
	}
	return out
}

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
	// The auth block is always present so the SPA knows whether to render the
	// app, the login page, or the setup wizard. For unauthenticated visitors
	// on an auth-enabled instance, NO data loaders run — the payload carries
	// only features + auth.
	if p.authSvc != nil {
		var identityPtr *authn.Identity
		if identity, ok := authn.IdentityFromContext(req.Context()); ok {
			identityPtr = &identity
		}
		authState := p.authSvc.StateFor(ctx, identityPtr)
		authState.SSOProviders = ssoProvidersForBoot(p)
		state["auth"] = authState
		if p.authSvc.Mode() != auth.ModeDisabled && identityPtr == nil {
			return state
		}
	}
	if p.agentRuntimeAvailability != nil {
		if snapshot, ok := p.agentRuntimeAvailability.Snapshot(); ok {
			state["agentRuntime"] = snapshot
		}
	}
	builder.addAgentProfileRecentUseState(ctx, state)

	if route.Route == webapp.RouteSettings {
		// Resolve an active workspace rather than emitting null. The SPA derives
		// Office-vs-kanban chrome from the active workspace record, so a settings
		// boot that names no workspace leaves the sidebar unable to tell which
		// mode it is in until the client's own fetch lands.
		// One workspace snapshot for both selection and serialisation: listing
		// twice could name an activeId that a concurrent deletion has already
		// removed from items.
		workspaces, ok := builder.listBootWorkspaces(ctx)
		activeID := ""
		if ok {
			activeID = builder.settingsWorkspaceID(ctx, req, workspaces)
			builder.addWorkspaceStateFrom(ctx, workspaces, state, &activeID)
		}
		builder.addUserSettingsState(ctx, state, activeID)
		builder.addSettingsRouteState(ctx, state, route.Path)
	}
	// Home, Threads, and unknown SPA routes all render the full app shell (nav,
	// workspace picker) without a route-specific data payload. Unknown
	// covers plugin-owned routes (e.g. /github-plugin) registered at
	// runtime, which the backend classifier can't enumerate — they still
	// need the base workspace/workflow/kanban context so native plugin UI
	// (like host.ui.TaskCreateDialog) has workspaces and workflows to work
	// with, not an empty store.
	if route.Route == webapp.RouteHome || route.Route == webapp.RouteThreads || route.Route == webapp.RouteUnknown {
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
	builder.addNeedsYouInboxState(ctx, req, state, route)
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
		return builder.taskDetailRouteData(ctx, req, route.Params["taskId"], queryValue(req, "sessionId"))
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

// bootItemsKey and bootActiveIDKey name the fields of a boot slice block.
const (
	bootItemsKey    = "items"
	bootActiveIDKey = "activeId"
)

type bootStateBuilder struct {
	p routeParams
}

func (b bootStateBuilder) addWorkspaceState(ctx context.Context, state map[string]any, activeID *string) {
	workspaces, ok := b.listBootWorkspaces(ctx)
	if !ok {
		return
	}
	b.addWorkspaceStateFrom(ctx, workspaces, state, activeID)
}

// listBootWorkspaces returns the workspace snapshot a boot payload is built
// from; ok is false when the service is unavailable or the listing fails.
func (b bootStateBuilder) listBootWorkspaces(ctx context.Context) ([]*taskmodels.Workspace, bool) {
	if b.p.taskSvc == nil {
		return nil, false
	}
	workspaces, err := b.p.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		b.logBootError("list workspaces", err)
		return nil, false
	}
	return workspaces, true
}

func (b bootStateBuilder) addWorkspaceStateFrom(
	ctx context.Context,
	workspaces []*taskmodels.Workspace,
	state map[string]any,
	activeID *string,
) {
	var active any
	if activeID != nil {
		active = *activeID
	}
	state["workspaces"] = map[string]any{
		bootItemsKey:    b.workspaceItemStates(ctx, workspaces),
		bootActiveIDKey: active,
	}
}

// settingsWorkspaceID resolves the active workspace for a /settings boot:
// whatever the user last had active, then their stored preference, then the
// first workspace that exists.
//
// Deliberately not filtered to kanban workspaces. Settings is shared chrome —
// reachable from either mode — so preferring a kanban workspace here would
// silently switch an Office user's active workspace just by opening Settings.
func (b bootStateBuilder) settingsWorkspaceID(
	ctx context.Context,
	req *http.Request,
	workspaces []*taskmodels.Workspace,
) string {
	settingsWorkspaceID := ""
	if settings, ok := b.userSettings(ctx); ok {
		settingsWorkspaceID = settings.Settings.WorkspaceID
	}
	return firstValidID(
		workspaceIDSet(workspaces),
		readActiveWorkspaceCookie(req),
		settingsWorkspaceID,
		firstWorkspaceID(workspaces),
	)
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

func (b bootStateBuilder) addAgentProfileRecentUseState(ctx context.Context, state map[string]any) {
	if b.p.userCtrl == nil {
		return
	}
	records, err := b.p.userCtrl.GetAgentProfileRecentUse(ctx)
	if err != nil {
		b.logBootError("get agent profile recent use", err)
		return
	}
	state["agentProfileRecentUse"] = mapAgentProfileRecentUseState(records)
}

func mapAgentProfileRecentUseState(records []userdto.AgentProfileRecentUseDTO) map[string]any {
	byContext := make(map[string]any, len(records))
	for _, record := range records {
		byContext[string(record.Context)] = map[string]any{
			"profileIds": append([]string{}, record.ProfileIDs...),
			"revision":   record.Revision,
			"updatedAt":  record.UpdatedAt,
		}
	}
	return map[string]any{"records": byContext, "loaded": true}
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
	workspaceItems := b.workspaceItemStates(ctx, workspaces)
	workspaceIDs := workspaceIDSet(workspaces)

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
		"items":                workflowItems,
		"activeId":             nullString(activeWorkflowID),
		"taskWorkflowCoverage": b.taskWorkflowCoverage(ctx, activeWorkspaceID),
	}
	if hasSettings {
		state["userSettings"] = mapUserSettingsStateWithWorkflow(settings, activeWorkspaceID, activeWorkflowID)
	}
	b.addRepositoriesState(ctx, state, activeWorkspaceID)
	b.addRepositorySetsState(ctx, state, activeWorkspaceID)
	b.addRepositoryBranchPoliciesState(ctx, state, activeWorkspaceID)
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

// addRepositorySetsState hydrates the home/kanban route with the workspace's
// repository sets, so the create dialog can offer them without a fetch.
func (b bootStateBuilder) addRepositorySetsState(ctx context.Context, state map[string]any, workspaceID string) {
	items := repositorySetsToDTOs(nil)
	loaded := false
	sets, err := b.p.taskSvc.ListRepositorySets(ctx, workspaceID)
	if err != nil {
		// Not loaded, so the client's hook still fetches; see
		// repositorySetsForState.
		b.logBootError("list home repository sets", err)
	} else {
		items = repositorySetsToDTOs(sets)
		loaded = true
	}
	state["repositorySets"] = repositorySetsState(workspaceID, items, loaded)
}

func (b bootStateBuilder) addRepositoryBranchPoliciesState(ctx context.Context, state map[string]any, workspaceID string) {
	b.repositoryBranchPoliciesForState(ctx, workspaceID, state)
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
	terminalTabs := []any{}
	if b.p.quickTerminalSvc != nil {
		if tabs, terminalErr := b.p.quickTerminalSvc.List(ctx, workspaceID); terminalErr != nil {
			b.logBootError("list quick-terminal tabs", terminalErr)
		} else {
			terminalTabs = make([]any, 0, len(tabs))
			for _, tab := range tabs {
				terminalTabs = append(terminalTabs, tab)
			}
		}
	}
	state["quickChat"] = map[string]any{
		"isOpen":          false,
		"sessions":        quickChat.sessions,
		"terminalTabs":    terminalTabs,
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
	items, err := b.p.taskSvc.ListQuickChatSessions(ctx, workspaceID)
	if err != nil {
		return quickChatBootState{}, err
	}
	sessions := make([]map[string]any, 0, len(items))
	taskSessions := make(map[string]taskdto.TaskSessionDTO, len(items))
	for _, item := range items {
		sessions = append(sessions, mapQuickChatSessionState(item))
		sessionDTO := taskdto.FromTaskSession(item.Session)
		if item.Session != nil && item.Session.TaskEnvironmentID != "" {
			operation, runnerLive, recoveryErr := b.p.taskSvc.WorkspaceRecoveryProjection(ctx, item.Session.TaskEnvironmentID)
			if recoveryErr != nil {
				b.logBootError("get quick chat workspace recovery projection", recoveryErr)
			} else {
				taskdto.EnrichWorkspaceRecovery(&sessionDTO, operation, runnerLive)
			}
		}
		if b.p.orchestratorSvc != nil {
			taskdto.EnrichCancellationPending(&sessionDTO, b.p.orchestratorSvc)
			taskdto.EnrichParkedProjection(&sessionDTO, b.p.orchestratorSvc)
		}
		taskSessions[item.SessionID] = sessionDTO
	}
	return quickChatBootState{sessions: sessions, taskSessions: taskSessions}, nil
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

func mapQuickChatSessionState(item taskservice.QuickChatSession) map[string]any {
	state := map[string]any{
		bootStateKeySessionID:   item.SessionID,
		bootStateKeyWorkspaceID: item.WorkspaceID,
		"taskId":                item.TaskID,
		"kind":                  item.Kind,
	}
	if item.Name != "" {
		state["name"] = item.Name
	}
	if item.AgentProfileID != "" {
		state["agentProfileId"] = item.AgentProfileID
	}
	return state
}

func (b bootStateBuilder) addKanbanSnapshotsState(
	ctx context.Context,
	state map[string]any,
	workflows []*taskmodels.Workflow,
	activeWorkflowID string,
) {
	if activeWorkflowID == "" {
		activeWorkflowID = firstBootBoardWorkflowID(workflows, state)
	}
	snapshots := make(map[string]any, len(workflows))
	var active map[string]any
	for _, workflow := range workflows {
		if workflow == nil || activeWorkflowID == "" || workflow.ID != activeWorkflowID {
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

func firstBootBoardWorkflowID(workflows []*taskmodels.Workflow, state map[string]any) string {
	demanded := bootBoardDemandIDs(state)
	first := ""
	for _, workflow := range workflows {
		if workflow == nil || workflow.Hidden {
			continue
		}
		if first == "" {
			first = workflow.ID
		}
		if demanded == nil || demanded[workflow.ID] {
			return workflow.ID
		}
	}
	return first
}

func bootBoardDemandIDs(state map[string]any) map[string]bool {
	workflowState, _ := state["workflows"].(map[string]any)
	coverage, _ := workflowState["taskWorkflowCoverage"].(*taskmodels.TaskWorkflowCoverage)
	if coverage == nil || !coverage.Complete {
		return nil
	}
	demanded := make(map[string]bool, len(coverage.WorkflowIDs))
	for _, id := range coverage.WorkflowIDs {
		demanded[id] = true
	}
	settings, _ := state["userSettings"].(map[string]any)
	hiddenSteps, _ := settings["hiddenWorkflowStepIds"].(map[string][]string)
	for id, steps := range hiddenSteps {
		if len(steps) > 0 {
			demanded[id] = true
		}
	}
	autoHideIDs, _ := settings["workflowIdsWithAutoHideEmptySteps"].([]string)
	for _, id := range autoHideIDs {
		demanded[id] = true
	}
	return demanded
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
		"taskCoverage": b.p.taskSvc.WorkflowTaskCoverage(workflow, len(tasks), len(taskStates)),
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

func addTaskDetailSessionMetadata(
	session *taskmodels.TaskSession,
	sessionModelsByID, sessionModeByID, sessionMCPStatusByID map[string]any,
) {
	if snapshot, ok := lifecycle.LoadSessionModelsSnapshot(
		session.Metadata[taskmodels.SessionMetaKeyACPModelState],
	); ok {
		sessionModelsByID[session.ID] = taskSessionModelsBootState(
			snapshot, sessionACPConfigBaseline(session),
		)
		if snapshot.SettingsPolicy == streams.SessionSettingsPolicyProviderRestored {
			sessionModeByID[session.ID] = taskSessionModeBootState(snapshot)
		}
	}
	if history, ok := lifecycle.LoadMCPAttachmentHistory(
		session.Metadata[taskmodels.SessionMetaKeyMCPAttachmentState],
	); ok {
		sessionMCPStatusByID[session.ID] = history
	}
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
	if snapshot.ConfigOptionsSettled {
		state["configOptionsSettled"] = true
	}
	if snapshot.SettingsPolicy == streams.SessionSettingsPolicyProviderRestored {
		state["settingsPolicy"] = string(snapshot.SettingsPolicy)
	}
	if len(baseline) > 0 {
		state["configBaseline"] = baseline
	}
	return state
}

func taskSessionModeBootState(snapshot lifecycle.SessionModelsSnapshot) map[string]any {
	return map[string]any{
		"currentModeId":  snapshot.CurrentModeID,
		"availableModes": []any{},
		"settingsPolicy": string(snapshot.SettingsPolicy),
	}
}
