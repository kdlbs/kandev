package main

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/webapp"
)

const officeActiveWorkspaceCookie = "office-active-workspace"

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

	if route.Route == webapp.RouteSettings || route.Route == webapp.RouteOffice {
		builder.addWorkspaceState(ctx, state, nil)
		builder.addUserSettingsState(ctx, state, "")
	}

	if route.Route == webapp.RouteSettings {
		builder.addSettingsRouteState(ctx, state, route.Path)
	}
	if route.Route == webapp.RouteOffice {
		builder.addOfficeRouteState(ctx, req, state)
	}
	return state
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
	return items, resolveActiveOfficeWorkspaceID(officeItems, readOfficeWorkspaceCookie(req)), nil
}

func (b bootStateBuilder) officeState(ctx context.Context, activeID string) map[string]any {
	agents := b.officeAgents(ctx, activeID)
	projects := b.officeProjects(ctx, activeID)
	inboxItems, inboxCount := b.officeInbox(ctx, activeID)
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
		"dashboard":      nil,
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

func readOfficeWorkspaceCookie(req *http.Request) string {
	if req == nil {
		return ""
	}
	cookie, err := req.Cookie(officeActiveWorkspaceCookie)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
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
