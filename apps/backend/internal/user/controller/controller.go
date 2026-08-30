package controller

import (
	"context"
	"runtime"

	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/service"
)

// Controller exposes the user settings HTTP handlers backed by the user
// service.
type Controller struct {
	svc *service.Service
}

// NewController builds a user settings controller.
func NewController(svc *service.Service) *Controller {
	return &Controller{svc: svc}
}

// GetCurrentUser returns the authenticated user together with their settings.
func (c *Controller) GetCurrentUser(ctx context.Context) (dto.UserResponse, error) {
	user, err := c.svc.GetCurrentUser(ctx)
	if err != nil {
		return dto.UserResponse{}, err
	}
	settings, err := c.svc.GetUserSettings(ctx)
	if err != nil {
		return dto.UserResponse{}, err
	}
	return dto.UserResponse{
		User:     dto.FromUser(user),
		Settings: dto.FromUserSettings(settings),
	}, nil
}

// GetUserSettings returns the current user's settings and OS shell options.
func (c *Controller) GetUserSettings(ctx context.Context) (dto.UserSettingsResponse, error) {
	settings, err := c.svc.GetUserSettings(ctx)
	if err != nil {
		return dto.UserSettingsResponse{}, err
	}
	return dto.UserSettingsResponse{
		Settings:     dto.FromUserSettings(settings),
		ShellOptions: shellOptionsForOS(),
	}, nil
}

// GetAgentProfileRecentUse returns the current user's independent operational
// profile histories.
func (c *Controller) GetAgentProfileRecentUse(ctx context.Context) ([]dto.AgentProfileRecentUseDTO, error) {
	records, err := c.svc.GetAgentProfileRecentUse(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]dto.AgentProfileRecentUseDTO, 0, len(records))
	for _, record := range records {
		result = append(result, dto.FromAgentProfileRecentUse(record))
	}
	return result, nil
}

// RecordAgentProfileRecentUse records one successful profile launch in the
// requested operational selector context.
func (c *Controller) RecordAgentProfileRecentUse(
	ctx context.Context,
	contextValue string,
	req dto.RecordAgentProfileRecentUseRequest,
) (dto.AgentProfileRecentUseDTO, error) {
	record, err := c.svc.RecordAgentProfileRecentUse(
		ctx,
		models.AgentProfileRecentUseContext(contextValue),
		req.AgentProfileID,
	)
	if err != nil {
		return dto.AgentProfileRecentUseDTO{}, err
	}
	return dto.FromAgentProfileRecentUse(record), nil
}

// UpdateUserSettings applies a partial settings patch and returns the
// resulting settings with OS shell options.
func (c *Controller) UpdateUserSettings(ctx context.Context, req dto.UpdateUserSettingsRequest) (dto.UserSettingsResponse, error) {
	settings, err := c.svc.UpdateUserSettings(ctx, &service.UpdateUserSettingsRequest{
		WorkspaceID:                       req.WorkspaceID,
		KanbanViewMode:                    req.KanbanViewMode,
		StartupPage:                       req.StartupPage,
		WorkflowFilterID:                  req.WorkflowFilterID,
		RepositoryIDs:                     req.RepositoryIDs,
		TasksListSort:                     req.TasksListSort,
		TasksListGroup:                    req.TasksListGroup,
		TasksListShowDetails:              req.TasksListShowDetails,
		InitialSetupComplete:              req.InitialSetupComplete,
		PreferredShell:                    req.PreferredShell,
		DefaultEditorID:                   req.DefaultEditorID,
		EnablePreviewOnClick:              req.EnablePreviewOnClick,
		ChatSubmitKey:                     req.ChatSubmitKey,
		ReviewAutoMarkOnScroll:            req.ReviewAutoMarkOnScroll,
		ConfirmTaskArchive:                req.ConfirmTaskArchive,
		PreventAutoStartAgentOnOpen:       req.PreventAutoStartAgentOnOpen,
		UnreadDivider:                     req.UnreadDivider,
		AgentGeneratedTaskTitles:          req.AgentGeneratedTaskTitles,
		MCPTaskAgentProfileDefault:        req.MCPTaskAgentProfileDefault,
		ShowAnchoredPromptBar:             req.ShowAnchoredPromptBar,
		ShowScrollToLastPrompt:            req.ShowScrollToLastPrompt,
		ShowScrollToStart:                 req.ShowScrollToStart,
		ShowTranscriptAutoScrollControl:   req.ShowTranscriptAutoScrollControl,
		ShowTodoListPanel:                 req.ShowTodoListPanel,
		ShowTodoListPanelOnlyWhenNotEmpty: req.ShowTodoListPanelOnlyWhenNotEmpty,
		ShowReleaseNotification:           req.ShowReleaseNotification,
		ReleaseNotesLastSeenVersion:       req.ReleaseNotesLastSeenVersion,
		LspAutoStartLanguages:             req.LspAutoStartLanguages,
		LspAutoInstallLanguages:           req.LspAutoInstallLanguages,
		LspServerConfigs:                  req.LspServerConfigs,
		LspStatusLocation:                 req.LspStatusLocation,
		SavedLayouts:                      req.SavedLayouts,
		SidebarViews:                      req.SidebarViews,
		SidebarActiveViewID:               req.SidebarActiveViewID,
		SidebarDraft:                      req.SidebarDraft.ServiceValue(),
		SidebarTaskPrefs:                  req.SidebarTaskPrefs,
		TaskCreateLastUsed:                req.TaskCreateLastUsed,
		JiraSavedViews:                    req.JiraSavedViews.ServiceValue(),
		JiraTaskPresets:                   req.JiraTaskPresets.ServiceValue(),
		GitHubSavedPresets:                req.GitHubSavedPresets.ServiceValue(),
		GitHubDefaultQueryPresets:         req.GitHubDefaultQueryPresets.ServiceValue(),
		GitLabSavedPresets:                req.GitLabSavedPresets.ServiceValue(),
		AzureDevOpsBrowsePreferences:      req.AzureDevOpsBrowsePreferences.ServiceValue(),
		DefaultUtilityAgentID:             req.DefaultUtilityAgentID,
		DefaultUtilityModel:               req.DefaultUtilityModel,
		DefaultUtilityAgentProfileID:      req.DefaultUtilityAgentProfileID,
		KeyboardShortcuts:                 req.KeyboardShortcuts,
		TerminalLinkBehavior:              req.TerminalLinkBehavior,
		TerminalFontFamily:                req.TerminalFontFamily,
		TerminalFontSize:                  req.TerminalFontSize,
		ChangesPanelLayout:                req.ChangesPanelLayout,
		LastSeenDisplay:                   req.LastSeenDisplay,
		SystemMetricsDisplay:              systemMetricsDisplayPatch(req.SystemMetricsDisplay),
		AppStatusBarEnabled:               req.AppStatusBarEnabled,
		AppStatusBarOrder:                 req.AppStatusBarOrder,
		QuickChatTabOrderByWorkspace:      req.QuickChatTabOrderByWorkspace,
		KanbanHiddenStepIDs:               req.KanbanHiddenStepIDs,
		WorkflowIDsWithAutoHideEmptySteps: req.WorkflowIDsWithAutoHideEmptySteps,
	})
	if err != nil {
		return dto.UserSettingsResponse{}, err
	}
	return dto.UserSettingsResponse{
		Settings:     dto.FromUserSettings(settings),
		ShellOptions: shellOptionsForOS(),
	}, nil
}

// systemMetricsDisplayPatch maps the API patch shape to the service layer,
// returning nil when the patch is omitted.
func systemMetricsDisplayPatch(patch *dto.SystemMetricsDisplaySettingsPatch) *service.SystemMetricsDisplaySettingsPatch {
	if patch == nil {
		return nil
	}
	return &service.SystemMetricsDisplaySettingsPatch{
		ShowInTopbar: patch.ShowInTopbar,
		Simplified:   patch.Simplified,
	}
}

// shellOptionsForOS returns the shell choices offered in settings for the
// current operating system.
func shellOptionsForOS() []dto.ShellOption {
	switch runtime.GOOS {
	case "windows":
		return []dto.ShellOption{
			{Value: "auto", Label: "System default"},
			{Value: "pwsh.exe", Label: "PowerShell (pwsh)"},
			{Value: "powershell.exe", Label: "Windows PowerShell"},
			{Value: "cmd.exe", Label: "Command Prompt"},
			{Value: "custom", Label: "Custom"},
		}
	default:
		return []dto.ShellOption{
			{Value: "auto", Label: "System default"},
			{Value: "/bin/zsh", Label: "zsh"},
			{Value: "/bin/bash", Label: "bash"},
			{Value: "/bin/sh", Label: "sh"},
			{Value: "custom", Label: "Custom"},
		}
	}
}
