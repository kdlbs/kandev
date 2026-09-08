package backendapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	canvasservice "github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/webapp"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// canvasHTTPHandler is the owner-authorized browser API for canvas metadata
// and host operations. Source transfer and publishing stay on the agent MCP
// path; browsers receive only metadata and short-lived runtime capabilities.
type canvasHTTPHandler struct {
	canvases *canvasservice.Service
	plugins  *plugins.Service
	tasks    *taskservice.Service
	editor   *canvasEditService
}

type canvasEditTaskStore interface {
	AuthorizeWorkspaceAccess(context.Context, string) error
	GetWorkspace(context.Context, string) (*models.Workspace, error)
	CreateTask(context.Context, *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error)
	DeleteTask(context.Context, string) error
}

type canvasEditCanvasStore interface {
	Get(context.Context, string) (*canvasservice.Canvas, error)
}

type canvasEditReleaseStore interface {
	GetRelease(context.Context, string) (instances.Release, error)
}

type canvasEditArtifactStore interface {
	ReadFiles(webapp.Artifact) (map[string][]byte, error)
}

type canvasEditSessionStore interface {
	SetSessionMetadataKey(context.Context, string, string, interface{}) error
}

type canvasEditSessionLauncher interface {
	LaunchSession(context.Context, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error)
	PromptTask(context.Context, string, string, string, string, bool, []v1.MessageAttachment, bool) (*orchestrator.PromptResult, error)
}

type canvasEditResult struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	CanvasID         string `json:"canvas_id"`
	AgentExecutionID string `json:"agent_execution_id,omitempty"`
}

type canvasEditError struct {
	status  int
	code    string
	message string
	cause   error
}

func (e *canvasEditError) Error() string {
	if e == nil {
		return "canvas edit failed"
	}
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

func newCanvasEditError(status int, code, message string, cause error) *canvasEditError {
	return &canvasEditError{status: status, code: code, message: message, cause: cause}
}

type canvasEditService struct {
	tasks      canvasEditTaskStore
	canvases   canvasEditCanvasStore
	releases   canvasEditReleaseStore
	artifacts  canvasEditArtifactStore
	launcher   canvasEditSessionLauncher
	sessions   canvasEditSessionStore
	executions canvasEditExecutionResolver
}

func newCanvasEditService(
	tasks canvasEditTaskStore,
	canvases canvasEditCanvasStore,
	releases canvasEditReleaseStore,
	artifacts canvasEditArtifactStore,
	launcher canvasEditSessionLauncher,
	sessions canvasEditSessionStore,
	executions canvasEditExecutionResolver,
) *canvasEditService {
	return &canvasEditService{
		tasks: tasks, canvases: canvases, releases: releases,
		artifacts: artifacts, launcher: launcher, sessions: sessions,
		executions: executions,
	}
}

func (s *canvasEditService) start(ctx context.Context, canvasID, requestedPrompt string) (canvasEditResult, error) {
	if s == nil || s.tasks == nil || s.canvases == nil || s.releases == nil ||
		s.artifacts == nil || s.launcher == nil || s.sessions == nil || s.executions == nil {
		return canvasEditResult{}, newCanvasEditError(http.StatusServiceUnavailable, "canvas_edit_unavailable", "canvas editing is unavailable", nil)
	}
	item, entries, err := s.loadEditableCanvas(ctx, canvasID)
	if err != nil {
		return canvasEditResult{}, err
	}
	agentProfileID, executorID, err := s.editWorkspaceDefaults(ctx, item.WorkspaceID)
	if err != nil {
		return canvasEditResult{}, err
	}
	task, err := s.createEditTask(ctx, item, agentProfileID, executorID, requestedPrompt)
	if err != nil {
		return canvasEditResult{}, err
	}
	launchResponse, err := s.launchEditSession(ctx, task, item, agentProfileID, executorID, entries, requestedPrompt)
	if err != nil {
		return s.rollbackEditTask(task.ID, err)
	}
	return canvasEditResult{
		TaskID: task.ID, SessionID: launchResponse.SessionID,
		CanvasID: item.ID, AgentExecutionID: launchResponse.AgentExecutionID,
	}, nil
}

func (s *canvasEditService) rollbackEditTask(taskID string, err error) (canvasEditResult, error) {
	var editErr *canvasEditError
	if !errors.As(err, &editErr) || editErr == nil {
		editErr = newCanvasEditError(http.StatusInternalServerError, "canvas_edit_failed", "canvas edit session could not be started", err)
	}
	if cleanupErr := s.rollbackTask(taskID); cleanupErr != nil {
		editErr.cause = errors.Join(editErr.cause, fmt.Errorf("rollback canvas edit task: %w", cleanupErr))
	}
	return canvasEditResult{}, editErr
}

func (s *canvasEditService) loadEditableCanvas(ctx context.Context, canvasID string) (*canvasservice.Canvas, []copyfiles.Entry, error) {
	item, err := s.canvases.Get(ctx, strings.TrimSpace(canvasID))
	if err != nil || item == nil {
		return nil, nil, newCanvasEditError(http.StatusNotFound, "canvas_not_found", "canvas was not found", nil)
	}
	if err := s.tasks.AuthorizeWorkspaceAccess(ctx, item.WorkspaceID); err != nil {
		return nil, nil, newCanvasEditError(http.StatusNotFound, "canvas_not_found", "canvas was not found", nil)
	}
	if item.ScopeKind != canvasservice.ScopeWorkspace || item.TaskID != "" {
		return nil, nil, newCanvasEditError(http.StatusNotFound, "canvas_not_found", "canvas was not found", nil)
	}
	if item.Status != instances.StatusActive || item.ActiveReleaseID == "" {
		return nil, nil, newCanvasEditError(http.StatusConflict, "pending_first_release", "canvas has no active release to edit", nil)
	}
	if item.ActiveReleaseStatus != instances.ValidationValid {
		return nil, nil, newCanvasEditError(http.StatusConflict, "canvas_not_editable", "canvas active release is not valid", nil)
	}
	release, err := s.releases.GetRelease(ctx, item.ActiveReleaseID)
	if err != nil || release.InstanceID != item.PluginInstanceID || release.ValidationStatus != instances.ValidationValid {
		return nil, nil, newCanvasEditError(http.StatusConflict, "canvas_not_editable", "canvas active release is unavailable", err)
	}
	files, err := s.artifacts.ReadFiles(webapp.Artifact{Digest: release.PackageDigest, RelativePath: release.ArtifactPath, Bytes: release.ArtifactBytes, Available: true})
	if err != nil {
		return nil, nil, newCanvasEditError(http.StatusConflict, "source_unavailable", "canvas active release source is unavailable", err)
	}
	entries := canvasEditSourceEntries(item.ID, files)
	if len(entries) == 0 {
		return nil, nil, newCanvasEditError(http.StatusConflict, "source_unavailable", "canvas active release source is empty", nil)
	}
	return item, entries, nil
}

func (s *canvasEditService) editWorkspaceDefaults(ctx context.Context, workspaceID string) (string, string, error) {
	workspace, err := s.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return "", "", newCanvasEditError(http.StatusNotFound, "canvas_not_found", "canvas workspace was not found", nil)
	}
	agentProfileID := workspaceDefaultID(workspace.DefaultAgentProfileID)
	if agentProfileID == "" {
		return "", "", newCanvasEditError(http.StatusBadRequest, "agent_profile_required", "workspace has no default agent profile configured", nil)
	}
	return agentProfileID, workspaceDefaultID(workspace.DefaultExecutorID), nil
}

func (s *canvasEditService) createEditTask(ctx context.Context, item *canvasservice.Canvas, agentProfileID, executorID, requestedPrompt string) (*models.Task, error) {
	metadata := map[string]interface{}{
		"origin":                     canvasEditOrigin,
		"canvas_id":                  item.ID,
		"canvas_release_id":          item.ActiveReleaseID,
		models.MetaKeyAgentProfileID: agentProfileID,
	}
	if executorID != "" {
		metadata[models.MetaKeyExecutorID] = executorID
	}
	result, err := s.tasks.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: item.WorkspaceID,
		Title:       "Edit canvas: " + item.Title,
		Description: strings.TrimSpace(requestedPrompt),
		IsEphemeral: true,
		Origin:      canvasEditOrigin,
		Metadata:    metadata,
	})
	if err == nil && result.Task != nil && result.Outcome == taskservice.CreateTaskOutcomeCreated {
		return result.Task, nil
	}
	if err == nil {
		err = errors.New("canvas edit task was not created")
	}
	return nil, newCanvasEditError(http.StatusInternalServerError, "task_create_failed", "canvas edit task could not be created", err)
}

func (s *canvasEditService) launchEditSession(ctx context.Context, task *models.Task, item *canvasservice.Canvas, agentProfileID, executorID string, entries []copyfiles.Entry, requestedPrompt string) (*orchestrator.LaunchSessionResponse, error) {
	launchResponse, err := s.launcher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID: task.ID, Intent: orchestrator.IntentStart,
		AgentProfileID: agentProfileID, ExecutorID: executorID,
	})
	if err != nil || launchResponse == nil || launchResponse.SessionID == "" || launchResponse.AgentExecutionID == "" {
		if err == nil {
			err = errors.New("canvas edit session launch returned no execution")
		}
		return nil, newCanvasEditError(http.StatusInternalServerError, "session_start_failed", "canvas edit session could not be started", err)
	}
	target := canvasEditSessionTarget{Origin: canvasEditOrigin, TaskID: task.ID, CanvasID: item.ID, ReleaseID: item.ActiveReleaseID}
	if err := s.sessions.SetSessionMetadataKey(ctx, launchResponse.SessionID, canvasEditSessionTargetMetadataKey, target); err != nil {
		return nil, newCanvasEditError(http.StatusInternalServerError, "session_target_failed", "canvas edit session target could not be recorded", err)
	}
	if err := s.materializeEditSource(ctx, launchResponse.AgentExecutionID, entries); err != nil {
		return nil, newCanvasEditError(http.StatusInternalServerError, "source_materialization_failed", "canvas source could not be materialized", err)
	}
	prompt := canvasEditPrompt(*item, promptSourceRoot(item.ID), requestedPrompt)
	if _, err := s.launcher.PromptTask(ctx, task.ID, launchResponse.SessionID, prompt, "", false, nil, false); err != nil {
		return nil, newCanvasEditError(http.StatusInternalServerError, "prompt_dispatch_failed", "canvas edit instructions could not be sent", err)
	}
	return launchResponse, nil
}

func (s *canvasEditService) materializeEditSource(ctx context.Context, executionID string, entries []copyfiles.Entry) error {
	copier, err := s.executions.ResolveAgentCtl(executionID)
	if err != nil || copier == nil {
		if err == nil {
			err = errors.New("agentctl client is unavailable")
		}
		return err
	}
	response, err := copier.CopyFiles(ctx, "", entries)
	if err != nil {
		return err
	}
	if !response.Present {
		return errors.New("copy-files returned no response")
	}
	if len(response.Warnings) > 0 {
		return fmt.Errorf("copy-files returned warnings: %s", strings.Join(response.Warnings, "; "))
	}
	return nil
}

func (s *canvasEditService) rollbackTask(taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), constants.TaskDeleteTimeout)
	defer cancel()
	return s.tasks.DeleteTask(ctx, taskID)
}

func workspaceDefaultID(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func canvasEditSourceEntries(canvasID string, files map[string][]byte) []copyfiles.Entry {
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	entries := make([]copyfiles.Entry, 0, len(paths))
	for _, name := range paths {
		entries = append(entries, copyfiles.Entry{
			RelPath: filepath.ToSlash(filepath.Join(canvasSourceRoot(canvasID), filepath.FromSlash(name))),
			Mode:    os.FileMode(0o600),
			Content: files[name],
		})
	}
	return entries
}

func promptSourceRoot(canvasID string) string {
	return canvasSourceRoot(canvasID)
}

func canvasEditPrompt(item canvasservice.Canvas, sourceRoot, requestedPrompt string) string {
	requestedPrompt = strings.TrimSpace(requestedPrompt)
	if requestedPrompt == "" {
		requestedPrompt = "Review the current canvas and make a useful improvement."
	}
	grants := "none"
	if len(item.EffectiveGrants) > 0 {
		values := make([]string, 0, len(item.EffectiveGrants))
		for _, grant := range item.EffectiveGrants {
			resource := grant.Resource
			if grant.PermissionKind == "network" {
				resource = grant.NetworkOrigin
			}
			values = append(values, grant.PermissionKind+":"+resource+" (scope "+grant.ScopeCeiling+")")
		}
		grants = strings.Join(values, ", ")
	}
	return fmt.Sprintf(
		"Edit canvas %q. If the core authoring bundle is not already available, read it once with read_canvas_authoring_skill_kandev without a path. Inspect the current source in %s, use only these effective grants: %s, apply the requested change, validate the result, and publish it with publish_canvas_kandev using canvas_id %s and source_path %s. Requested change: %s",
		item.Title, sourceRoot, grants, item.ID, sourceRoot, requestedPrompt,
	)
}

// registerCanvasRoutes mounts the feature-gated canvas host API. This helper
// is called only from registerRoutes when features.canvases is enabled.
func registerCanvasRoutes(p routeParams) {
	if p.router == nil || p.services == nil || p.services.Canvas == nil || p.services.Plugins == nil {
		return
	}
	h := &canvasHTTPHandler{
		canvases: p.services.Canvas,
		plugins:  p.services.Plugins,
		tasks:    p.taskSvc,
	}
	if p.taskSvc != nil && p.taskRepo != nil && p.orchestratorSvc != nil && p.lifecycleMgr != nil {
		instanceStore := p.services.Plugins.Instances()
		artifactStore := p.services.Plugins.WebArtifacts()
		if instanceStore != nil && artifactStore != nil {
			h.editor = newCanvasEditService(
				p.taskSvc, p.services.Canvas, instanceStore, artifactStore,
				p.orchestratorSvc, p.taskRepo,
				lifecycleCanvasExecutionResolver{manager: p.lifecycleMgr},
			)
		}
	}
	// Keep the canonical resource-shaped paths alongside the short aliases.
	// The aliases make the API convenient for direct hosts while preserving a
	// predictable path for the web client.
	p.router.GET("/api/v1/tasks/:id/canvases", h.listTask)
	p.router.GET("/api/v1/canvases/task/:taskID", h.listTask)
	p.router.GET("/api/v1/canvases/tasks/:taskID", h.listTask)
	p.router.GET("/api/v1/workspaces/:id/canvases", h.listWorkspace)
	p.router.GET("/api/v1/canvases/workspace/:workspaceID", h.listWorkspace)
	p.router.GET("/api/v1/canvases/workspaces/:workspaceID", h.listWorkspace)
	p.router.GET("/api/v1/canvases/:canvasID", h.get)
	p.router.GET("/api/v1/canvases/:canvasID/releases", h.releases)
	p.router.GET("/api/v1/canvases/:canvasID/promotion-preview", h.promotionPreview)
	p.router.POST("/api/v1/canvases/:canvasID/promotion", h.promote)
	p.router.POST("/api/v1/canvases/:canvasID/releases/:releaseID/approve", h.approveRelease)
	p.router.POST("/api/v1/canvases/:canvasID/releases/:releaseID/reject", h.rejectRelease)
	p.router.POST("/api/v1/canvases/:canvasID/rollback", h.rollback)
	p.router.POST("/api/v1/canvases/:canvasID/edit", h.edit)
	p.router.POST("/api/v1/canvases/:canvasID/runtime", h.runtime)
	p.router.GET("/api/v1/canvases/:canvasID/runtime", h.runtime)
	p.router.POST("/api/v1/canvases/:canvasID/archive", h.archive)
	p.router.POST("/api/v1/canvases/:canvasID/restore", h.restore)
	p.router.POST("/api/v1/canvases/:canvasID/remove", h.remove)
	p.router.DELETE("/api/v1/canvases/:canvasID", h.remove)
}

type canvasHTTPResponse struct {
	ID                  string                          `json:"id"`
	PluginInstanceID    string                          `json:"plugin_instance_id"`
	PluginID            string                          `json:"plugin_id"`
	WorkspaceID         string                          `json:"workspace_id"`
	TaskID              string                          `json:"task_id,omitempty"`
	OriginTaskID        string                          `json:"origin_task_id,omitempty"`
	ScopeKind           string                          `json:"scope_kind"`
	Title               string                          `json:"title"`
	CreatedBySessionID  string                          `json:"created_by_session_id,omitempty"`
	PromotedByUserID    string                          `json:"promoted_by_user_id,omitempty"`
	PromotedAt          *time.Time                      `json:"promoted_at,omitempty"`
	Status              string                          `json:"status"`
	ActiveReleaseID     string                          `json:"active_release_id,omitempty"`
	ActiveReleaseStatus string                          `json:"active_release_status,omitempty"`
	ActiveReleaseError  string                          `json:"active_release_error,omitempty"`
	GrantGeneration     int64                           `json:"grant_generation,omitempty"`
	EffectiveGrants     []canvasservice.GrantProjection `json:"effective_grants,omitempty"`
	ActiveRelease       *canvasReleaseResponse          `json:"active_release,omitempty"`
	PendingRelease      *canvasReleaseResponse          `json:"pending_release,omitempty"`
	CreatedAt           time.Time                       `json:"created_at"`
	UpdatedAt           time.Time                       `json:"updated_at"`
}

type canvasReleaseResponse struct {
	ID                 string                           `json:"id"`
	PackageDigest      string                           `json:"package_digest,omitempty"`
	ValidationStatus   string                           `json:"validation_status"`
	ValidationError    string                           `json:"validation_error,omitempty"`
	Permissions        *canvasservice.PermissionSummary `json:"permissions,omitempty"`
	MissingPermissions []string                         `json:"missing_permissions,omitempty"`
	PermissionDigest   string                           `json:"permission_digest,omitempty"`
	SourceActorKind    string                           `json:"source_actor_kind,omitempty"`
	SourceUserID       string                           `json:"source_user_id,omitempty"`
	SourceTaskID       string                           `json:"source_task_id,omitempty"`
	SourceSessionID    string                           `json:"source_session_id,omitempty"`
	ProtocolVersion    int                              `json:"protocol_version,omitempty"`
	CreatedAt          time.Time                        `json:"created_at"`
}

func canvasResponse(value canvasservice.Canvas) canvasHTTPResponse {
	result := canvasHTTPResponse{
		ID:                  value.ID,
		PluginInstanceID:    value.PluginInstanceID,
		PluginID:            value.PluginID,
		WorkspaceID:         value.WorkspaceID,
		TaskID:              value.TaskID,
		OriginTaskID:        value.OriginTaskID,
		ScopeKind:           value.ScopeKind,
		Title:               value.Title,
		CreatedBySessionID:  value.CreatedBySessionID,
		PromotedByUserID:    value.PromotedByUserID,
		PromotedAt:          value.PromotedAt,
		Status:              value.Status,
		ActiveReleaseID:     value.ActiveReleaseID,
		ActiveReleaseStatus: value.ActiveReleaseStatus,
		ActiveReleaseError:  value.ActiveReleaseError,
		GrantGeneration:     value.GrantGeneration,
		EffectiveGrants:     value.EffectiveGrants,
		CreatedAt:           value.CreatedAt,
		UpdatedAt:           value.UpdatedAt,
	}
	if value.ActiveRelease != nil {
		result.ActiveRelease = releaseResponseFromMetadata(value.ActiveRelease)
	}
	if value.PendingRelease != nil {
		result.PendingRelease = releaseResponseFromMetadata(value.PendingRelease)
	}
	return result
}

func (h *canvasHTTPHandler) listTask(c *gin.Context) {
	taskID := strings.TrimSpace(canvasRouteParam(c, "id", "taskID"))
	taskSvc := h.tasks
	if taskSvc == nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return
	}
	task, err := taskSvc.GetTask(c.Request.Context(), taskID)
	if err != nil || task == nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return
	}
	if err := taskSvc.AuthorizeWorkspaceAccess(c.Request.Context(), task.WorkspaceID); err != nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return
	}
	includeArchived, ok := canvasIncludeArchived(c)
	if !ok {
		return
	}
	canvases, err := h.canvases.ListForTask(c.Request.Context(), task.WorkspaceID, taskID, includeArchived)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, map[string]interface{}{"canvases": mapCanvasResponses(canvases)})
}

func (h *canvasHTTPHandler) listWorkspace(c *gin.Context) {
	workspaceID := strings.TrimSpace(canvasRouteParam(c, "id", "workspaceID"))
	taskSvc := h.tasks
	if taskSvc == nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return
	}
	if err := taskSvc.AuthorizeWorkspaceAccess(c.Request.Context(), workspaceID); err != nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return
	}
	includeArchived, ok := canvasIncludeArchived(c)
	if !ok {
		return
	}
	canvases, err := h.canvases.ListWorkspaceCanvases(c.Request.Context(), workspaceID, includeArchived)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, map[string]interface{}{"canvases": mapCanvasResponses(canvases)})
}

func (h *canvasHTTPHandler) get(c *gin.Context) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	writeCanvasJSON(c, http.StatusOK, canvasResponse(*canvas))
}

func (h *canvasHTTPHandler) releases(c *gin.Context) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	instanceStore := h.plugins.Instances()
	instance, err := instanceStore.Get(c.Request.Context(), canvas.PluginInstanceID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	grants, err := instanceStore.ListGrants(c.Request.Context(), canvas.PluginInstanceID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	list, err := instanceStore.ListReleases(c.Request.Context(), canvas.PluginInstanceID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result := make([]canvasReleaseResponse, 0, len(list))
	for _, release := range list {
		result = append(result, releaseResponse(release, instance.ScopeKind, grants))
	}
	writeCanvasJSON(c, http.StatusOK, map[string]interface{}{"releases": result})
}

func releaseResponse(release instances.Release, scope string, grants []instances.Grant) canvasReleaseResponse {
	permissions := canvasservice.ReleasePermissionSummary(release)
	return canvasReleaseResponse{
		ID: release.ID, PackageDigest: release.PackageDigest,
		ValidationStatus: release.ValidationStatus, ValidationError: release.ValidationError,
		Permissions: &permissions, MissingPermissions: canvasservice.MissingPermissionKeys(permissions, scope, grants),
		PermissionDigest: canvasservice.PermissionDigest(release), SourceActorKind: release.SourceActorKind,
		SourceUserID: release.SourceUserID, SourceTaskID: release.SourceTaskID,
		SourceSessionID: release.SourceSessionID, ProtocolVersion: release.ProtocolVersion,
		CreatedAt: release.CreatedAt,
	}
}

func releaseResponseFromMetadata(value *canvasservice.ReleaseMetadata) *canvasReleaseResponse {
	if value == nil {
		return nil
	}
	return &canvasReleaseResponse{
		ID: value.ID, PackageDigest: value.PackageDigest, ValidationStatus: value.ValidationStatus,
		ValidationError: value.ValidationError, Permissions: value.Permissions,
		MissingPermissions: value.MissingPermissions, PermissionDigest: value.PermissionDigest,
		SourceActorKind: value.SourceActorKind, SourceUserID: value.SourceUserID,
		SourceTaskID: value.SourceTaskID, SourceSessionID: value.SourceSessionID,
		ProtocolVersion: value.ProtocolVersion, CreatedAt: value.CreatedAt,
	}
}

func (h *canvasHTTPHandler) promotionPreview(c *gin.Context) {
	item, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if item == nil || !h.authorizeWorkspace(c, item.WorkspaceID) {
		return
	}
	preview, err := h.canvases.PromotionPreview(c.Request.Context(), item.ID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if preview == nil || preview.Canvas == nil {
		return
	}
	writeCanvasJSON(c, http.StatusOK, map[string]interface{}{
		"canvas_id":         preview.Canvas.ID,
		"title":             preview.Canvas.Title,
		"origin_task_id":    preview.Canvas.OriginTaskID,
		"active_release":    releaseResponseFromCanvas(preview.Canvas),
		"source_actor_kind": preview.SourceActorKind,
		"source_user_id":    preview.SourceUserID,
		"source_task_id":    preview.SourceTaskID,
		"source_session_id": preview.SourceSessionID,
		"permissions":       preview.Permissions,
		"active_release_id": preview.ActiveReleaseID,
		"permission_digest": preview.PermissionDigest,
		"grant_generation":  preview.GrantGeneration,
		"current_scope":     preview.CurrentScope,
		"target_scope":      preview.TargetScope,
		"placement":         preview.Placement,
	})
}

func releaseResponseFromCanvas(value *canvasservice.Canvas) *canvasReleaseResponse {
	if value == nil || value.ActiveRelease == nil {
		return nil
	}
	return releaseResponseFromMetadata(value.ActiveRelease)
}

type canvasPromotionRequest struct {
	ExpectedReleaseID        string `json:"expected_release_id"`
	ExpectedPermissionDigest string `json:"expected_permission_digest"`
	ExpectedGrantGeneration  *int64 `json:"expected_grant_generation"`
}

func (h *canvasHTTPHandler) promote(c *gin.Context) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	var request canvasPromotionRequest
	if c.Request.Body == nil || c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.ExpectedReleaseID) == "" || strings.TrimSpace(request.ExpectedPermissionDigest) == "" || request.ExpectedGrantGeneration == nil || *request.ExpectedGrantGeneration < 0 {
		writeCanvasError(c, http.StatusBadRequest, "promotion_review_required", nil)
		return
	}
	updated, err := h.canvases.PromoteCanvasReviewed(c.Request.Context(), canvas.ID, canvasRuntimeUser(c), request.ExpectedReleaseID, request.ExpectedPermissionDigest, *request.ExpectedGrantGeneration)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, canvasResponse(*updated))
}

func (h *canvasHTTPHandler) approveRelease(c *gin.Context) {
	h.releaseMutation(c, func(ctx context.Context, canvasID, releaseID, userID string) (*canvasservice.Canvas, error) {
		return h.canvases.ApproveRelease(ctx, canvasID, releaseID, userID)
	})
}

func (h *canvasHTTPHandler) rejectRelease(c *gin.Context) {
	h.releaseMutation(c, func(ctx context.Context, canvasID, releaseID, _ string) (*canvasservice.Canvas, error) {
		return h.canvases.RejectRelease(ctx, canvasID, releaseID)
	})
}

func (h *canvasHTTPHandler) releaseMutation(c *gin.Context, mutation func(context.Context, string, string, string) (*canvasservice.Canvas, error)) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	updated, err := mutation(c.Request.Context(), canvas.ID, c.Param("releaseID"), canvasRuntimeUser(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, canvasResponse(*updated))
}

func (h *canvasHTTPHandler) rollback(c *gin.Context) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	var body struct {
		ReleaseID string `json:"release_id"`
	}
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
			return
		}
	}
	updated, err := h.canvases.RollbackRelease(c.Request.Context(), canvas.ID, body.ReleaseID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, canvasResponse(*updated))
}

func (h *canvasHTTPHandler) edit(c *gin.Context) {
	if h.editor == nil {
		writeCanvasError(c, http.StatusServiceUnavailable, "canvas_edit_unavailable", nil)
		return
	}
	var request struct {
		Prompt string `json:"prompt,omitempty"`
	}
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
			writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
			return
		}
	}
	result, err := h.editor.start(c.Request.Context(), c.Param("canvasID"), request.Prompt)
	if err != nil {
		var editErr *canvasEditError
		if errors.As(err, &editErr) && editErr != nil {
			writeCanvasError(c, editErr.status, editErr.code, nil)
			return
		}
		writeCanvasError(c, http.StatusInternalServerError, "canvas_edit_failed", nil)
		return
	}
	writeCanvasJSON(c, http.StatusOK, result)
}

type canvasRuntimeRequest struct {
	WebAppKey string `json:"web_app_key"`
	Placement string `json:"placement"`
}

func (h *canvasHTTPHandler) runtime(c *gin.Context) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	if canvas.Status != instances.StatusActive || canvas.ActiveReleaseID == "" {
		writeCanvasError(c, http.StatusConflict, "pending_first_release", map[string]interface{}{"status": canvas.Status})
		return
	}
	if canvas.ActiveReleaseStatus != instances.ValidationValid {
		writeCanvasError(c, http.StatusConflict, canvasHostError(canvas.ActiveReleaseStatus), map[string]interface{}{"status": canvas.ActiveReleaseStatus, "error": canvas.ActiveReleaseError})
		return
	}
	var request canvasRuntimeRequest
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, http.ErrNotSupported) && !errors.Is(err, io.EOF) {
			writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
			return
		}
	}
	path, release, app, binding, err := h.runtimeBinding(c, *canvas, request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, map[string]interface{}{
		"runtime_url":        path,
		"release_id":         release.ID,
		"web_app_key":        app.Key,
		"placement":          appPlacement(app, request.Placement, canvas.ScopeKind),
		"expires_in_seconds": int(webapp.RuntimeTokenTTL / time.Second),
		"canvas":             canvasResponse(*canvas),
		"binding":            map[string]interface{}{"scope_kind": binding.ScopeKind, "grant_generation": binding.GrantGeneration},
	})
}

func (h *canvasHTTPHandler) archive(c *gin.Context) {
	h.mutate(c, func(ctx context.Context, id string) (*canvasservice.Canvas, error) {
		return h.canvases.Archive(ctx, id)
	})
}

func (h *canvasHTTPHandler) restore(c *gin.Context) {
	h.mutate(c, func(ctx context.Context, id string) (*canvasservice.Canvas, error) {
		return h.canvases.Restore(ctx, id)
	})
}

func (h *canvasHTTPHandler) mutate(c *gin.Context, mutation func(context.Context, string) (*canvasservice.Canvas, error)) {
	// Keeping mutation routing in one place prevents archive and restore from
	// accidentally gaining different authorization behavior.
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	updated, err := mutation(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, canvasResponse(*updated))
}

func (h *canvasHTTPHandler) remove(c *gin.Context) {
	canvas, err := h.canvases.Get(c.Request.Context(), c.Param("canvasID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if !h.authorizeWorkspace(c, canvas.WorkspaceID) {
		return
	}
	if err := h.canvases.Remove(c.Request.Context(), c.Param("canvasID")); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *canvasHTTPHandler) authorizeWorkspace(c *gin.Context, workspaceID string) bool {
	taskSvc := h.tasks
	if taskSvc == nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return false
	}
	if err := taskSvc.AuthorizeWorkspaceAccess(c.Request.Context(), workspaceID); err != nil {
		writeCanvasError(c, http.StatusNotFound, "canvas_not_found", nil)
		return false
	}
	return true
}

func canvasRouteParam(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := c.Param(name); value != "" {
			return value
		}
	}
	return ""
}
