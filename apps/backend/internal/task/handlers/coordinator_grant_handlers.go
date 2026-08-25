package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

// CoordinatorGrantHandlers exposes operator-only HTTP routes for managing
// explicit coordinator task authority grants and their bounded audit trail.
// All routes are under /api/v1 and enforce workspace visibility (404 for
// workspaces the caller cannot see) and workspace ownership (no grants in
// the read-only Improve Kandev workspace).
type CoordinatorGrantHandlers struct {
	repo   coordinatorGrantRepo
	svc    *service.Service
	logger *logger.Logger
}

// coordinatorGrantRepo is the grant and audit persistence surface the handlers
// need — a subset of the full CoordinatorAuthorityRepository.
type coordinatorGrantRepo interface {
	CreateCoordinatorGrant(ctx context.Context, grant *models.CoordinatorGrant) error
	GetCoordinatorGrant(ctx context.Context, id string) (*models.CoordinatorGrant, error)
	ListCoordinatorGrants(ctx context.Context, workspaceID, coordinatorTaskID string, includeRevoked bool) ([]*models.CoordinatorGrant, error)
	RevokeCoordinatorGrant(ctx context.Context, id, revokedByUserID string, revokedAt time.Time) error
	ListCoordinatorAuditEvents(ctx context.Context, workspaceID, taskID string, limit int) ([]*models.CoordinatorAuditEvent, error)
}

func NewCoordinatorGrantHandlers(
	repo coordinatorGrantRepo,
	svc *service.Service,
	log *logger.Logger,
) *CoordinatorGrantHandlers {
	return &CoordinatorGrantHandlers{
		repo:   repo,
		svc:    svc,
		logger: log.WithFields(zap.String("component", "task-coordinator-grant-handlers")),
	}
}

func RegisterCoordinatorGrantRoutes(
	router *gin.Engine,
	repo coordinatorGrantRepo,
	svc *service.Service,
	log *logger.Logger,
) *CoordinatorGrantHandlers {
	h := NewCoordinatorGrantHandlers(repo, svc, log)
	h.registerHTTP(router)
	return h
}

func (h *CoordinatorGrantHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/coordinator-grants", h.httpListWorkspaceCoordinatorGrants)
	api.POST("/workspaces/:id/coordinator-grants", h.httpCreateWorkspaceCoordinatorGrant)
	api.GET("/tasks/:id/coordinator-grants", h.httpListTaskCoordinatorGrants)
	api.DELETE("/coordinator-grants/:id", h.httpRevokeCoordinatorGrant)
	api.GET("/workspaces/:id/coordinator-audit", h.httpListWorkspaceCoordinatorAudit)
}

// coordinatorGrantListResponse is the shared list envelope for both
// workspace-scoped and task-scoped list endpoints.
type coordinatorGrantListResponse struct {
	Grants []grantDTO `json:"grants"`
	Total  int        `json:"total"`
}

type grantDTO struct {
	ID                string  `json:"id"`
	CoordinatorTaskID string  `json:"coordinator_task_id"`
	PrincipalID       string  `json:"principal_id,omitempty"`
	WorkspaceID       string  `json:"workspace_id"`
	ScopeKind         string  `json:"scope_kind"`
	ScopeID           string  `json:"scope_id"`
	Capabilities      string  `json:"capabilities"`
	Note              string  `json:"note"`
	GrantedByUserID   string  `json:"granted_by_user_id"`
	GrantedAt         string  `json:"granted_at"`
	RevokedAt         *string `json:"revoked_at,omitempty"`
	RevokedByUserID   string  `json:"revoked_by_user_id,omitempty"`
}

func toGrantDTO(g *models.CoordinatorGrant) grantDTO {
	dto := grantDTO{
		ID:                g.ID,
		CoordinatorTaskID: g.CoordinatorTaskID,
		PrincipalID:       g.PrincipalID,
		WorkspaceID:       g.WorkspaceID,
		ScopeKind:         g.ScopeKind,
		ScopeID:           g.ScopeID,
		Capabilities:      g.Capabilities,
		Note:              g.Note,
		GrantedByUserID:   g.GrantedByUserID,
		GrantedAt:         g.GrantedAt.Format(time.RFC3339Nano),
	}
	if g.RevokedAt != nil {
		s := g.RevokedAt.Format(time.RFC3339Nano)
		dto.RevokedAt = &s
		dto.RevokedByUserID = g.RevokedByUserID
	}
	return dto
}

func toGrantDTOs(grants []*models.CoordinatorGrant) []grantDTO {
	if grants == nil {
		return nil
	}
	dtos := make([]grantDTO, 0, len(grants))
	for _, g := range grants {
		if g != nil {
			dtos = append(dtos, toGrantDTO(g))
		}
	}
	return dtos
}

type auditDTO struct {
	ID             string `json:"id"`
	OccurredAt     string `json:"occurred_at"`
	PrincipalID    string `json:"principal_id"`
	ActorTaskID    string `json:"actor_task_id"`
	ActorSessionID string `json:"actor_session_id"`
	TargetTaskID   string `json:"target_task_id"`
	WorkspaceID    string `json:"workspace_id"`
	Action         string `json:"action"`
	Capability     string `json:"capability"`
	Decision       string `json:"decision"`
	GrantID        string `json:"grant_id"`
	Result         string `json:"result"`
	Detail         string `json:"detail"`
}

func toAuditDTO(e *models.CoordinatorAuditEvent) auditDTO {
	return auditDTO{
		ID:             e.ID,
		OccurredAt:     e.OccurredAt.Format(time.RFC3339Nano),
		PrincipalID:    e.PrincipalID,
		ActorTaskID:    e.ActorTaskID,
		ActorSessionID: e.ActorSessionID,
		TargetTaskID:   e.TargetTaskID,
		WorkspaceID:    e.WorkspaceID,
		Action:         e.Action,
		Capability:     e.Capability,
		Decision:       e.Decision,
		GrantID:        e.GrantID,
		Result:         e.Result,
		Detail:         e.Detail,
	}
}

type coordinatorAuditListResponse struct {
	Events []auditDTO `json:"events"`
	Total  int        `json:"total"`
}

// resolveWorkspace ensures the workspace exists and is not the read-only
// Improve Kandev workspace. Returns false if the caller should be short-circuited
// (a response has already been written). Workspace ID is read from the :id param.
func (h *CoordinatorGrantHandlers) resolveWorkspace(c *gin.Context) bool {
	workspaceID := c.Param("id")
	workspace, err := h.svc.GetWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return false
	}
	if workspace.IsImproveKandev() {
		c.JSON(http.StatusConflict, gin.H{"error": workspaceReadOnlyMsg})
		return false
	}
	return true
}

// createGrantRequest is the POST body for creating a new coordinator grant.
type createGrantRequest struct {
	CoordinatorTaskID string `json:"coordinator_task_id"`
	ScopeKind         string `json:"scope_kind"` // "workspace" or "workflow"
	ScopeID           string `json:"scope_id"`
	Capabilities      string `json:"capabilities"` // comma-separated, e.g. "inspect,orchestrate"
	Note              string `json:"note"`
}

// coordinatorGrantStatus maps partial failures onto HTTP status codes.
func coordinatorGrantStatus(err error) int {
	switch {
	case errors.Is(err, repoerrors.ErrCoordinatorGrantNotFound):
		return http.StatusNotFound
	case errors.Is(err, repoerrors.ErrCoordinatorGrantConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// abortWithGrantError responds with the mapped status. A 500 is logged with a
// generic body; other codes carry the service error message.
func (h *CoordinatorGrantHandlers) abortWithGrantError(c *gin.Context, action string, err error) {
	status := coordinatorGrantStatus(err)
	if status == http.StatusInternalServerError {
		h.logger.Error("coordinator grant request failed", zap.String("action", action), zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to " + action})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

// ---- HTTP handlers ----

// httpListWorkspaceCoordinatorGrants lists coordinator grants by workspace.
// Supports an optional ?task_id= query parameter to filter by coordinator task,
// and ?include_revoked=true to include revoked grants.
func (h *CoordinatorGrantHandlers) httpListWorkspaceCoordinatorGrants(c *gin.Context) {
	if !h.resolveWorkspace(c) {
		return
	}
	workspaceID := c.Param("id")
	taskID := c.Query("task_id")
	includeRevoked := c.Query("include_revoked") == "true"

	grants, err := h.repo.ListCoordinatorGrants(c.Request.Context(), workspaceID, taskID, includeRevoked)
	if err != nil {
		h.abortWithGrantError(c, "list coordinator grants", err)
		return
	}
	c.JSON(http.StatusOK, coordinatorGrantListResponse{
		Grants: toGrantDTOs(grants),
		Total:  len(grants),
	})
}

// validateCreateGrantRequest validates the incoming grant creation payload and
// returns the parsed capabilities and resolved scope ID, or writes an error
// response and returns false.
func (h *CoordinatorGrantHandlers) validateCreateGrantRequest(c *gin.Context, workspaceID string, req *createGrantRequest) (capabilities []string, scopeID string, ok bool) {
	if req.CoordinatorTaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "coordinator_task_id is required"})
		return nil, "", false
	}
	if req.ScopeKind != "workspace" && req.ScopeKind != "workflow" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope_kind must be 'workspace' or 'workflow'"})
		return nil, "", false
	}
	if req.Capabilities == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "capabilities is required"})
		return nil, "", false
	}
	task, err := h.svc.GetTask(c.Request.Context(), req.CoordinatorTaskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "coordinator task not found"})
		return nil, "", false
	}
	if task.WorkspaceID != workspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "coordinator task is not in this workspace"})
		return nil, "", false
	}
	if req.ScopeKind == "workflow" {
		if req.ScopeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope_id is required for workflow scope"})
			return nil, "", false
		}
		wf, err := h.svc.GetWorkflow(c.Request.Context(), req.ScopeID)
		if err != nil || wf.WorkspaceID != workspaceID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workflow not found"})
			return nil, "", false
		}
	}
	caps := parseCapabilities(req.Capabilities)
	if len(caps) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "capabilities must include at least one of: inspect, orchestrate"})
		return nil, "", false
	}
	sid := req.ScopeID
	if req.ScopeKind == "workspace" && sid == "" {
		sid = workspaceID
	}
	return caps, sid, true
}

// httpCreateWorkspaceCoordinatorGrant creates a new coordinator grant in the
// workspace. Validates the task exists in the workspace, the scope is valid,
// and the capabilities are recognized.
func (h *CoordinatorGrantHandlers) httpCreateWorkspaceCoordinatorGrant(c *gin.Context) {
	if !h.resolveWorkspace(c) {
		return
	}
	workspaceID := c.Param("id")

	var req createGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	caps, scopeID, ok := h.validateCreateGrantRequest(c, workspaceID, &req)
	if !ok {
		return
	}

	now := time.Now()
	grant := &models.CoordinatorGrant{
		CoordinatorTaskID: req.CoordinatorTaskID,
		WorkspaceID:       workspaceID,
		ScopeKind:         req.ScopeKind,
		ScopeID:           scopeID,
		Capabilities:      joinCapabilities(caps),
		Note:              req.Note,
		GrantedByUserID:   resolveUserID(c),
		GrantedAt:         now,
	}
	if err := h.repo.CreateCoordinatorGrant(c.Request.Context(), grant); err != nil {
		h.abortWithGrantError(c, "create coordinator grant", err)
		return
	}

	created, err := h.repo.GetCoordinatorGrant(c.Request.Context(), grant.ID)
	if err != nil {
		h.logger.Warn("coordinator grant created but read-back failed", zap.String("grant_id", grant.ID), zap.Error(err))
		c.JSON(http.StatusCreated, gin.H{"grant": toGrantDTO(grant)})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"grant": toGrantDTO(created)})
}

// httpListTaskCoordinatorGrants lists grants by coordinator task ID.
func (h *CoordinatorGrantHandlers) httpListTaskCoordinatorGrants(c *gin.Context) {
	taskID := c.Param("id")

	// Verify the task exists and the caller can see it.
	task, err := h.svc.GetTask(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}

	includeRevoked := c.Query("include_revoked") == "true"
	grants, err := h.repo.ListCoordinatorGrants(c.Request.Context(), task.WorkspaceID, taskID, includeRevoked)
	if err != nil {
		h.abortWithGrantError(c, "list coordinator grants", err)
		return
	}
	c.JSON(http.StatusOK, coordinatorGrantListResponse{
		Grants: toGrantDTOs(grants),
		Total:  len(grants),
	})
}

// httpRevokeCoordinatorGrant revokes a coordinator grant by ID (soft delete —
// sets revoked_at, never deletes the row).
func (h *CoordinatorGrantHandlers) httpRevokeCoordinatorGrant(c *gin.Context) {
	grantID := c.Param("id")

	grant, err := h.repo.GetCoordinatorGrant(c.Request.Context(), grantID)
	if err != nil {
		h.abortWithGrantError(c, "get coordinator grant", err)
		return
	}
	if grant == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "coordinator grant not found"})
		return
	}

	// Verify the workspace is writable.
	workspace, err := h.svc.GetWorkspace(c.Request.Context(), grant.WorkspaceID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return
	}
	if workspace.IsImproveKandev() {
		c.JSON(http.StatusConflict, gin.H{"error": workspaceReadOnlyMsg})
		return
	}

	userID := resolveUserID(c)
	now := time.Now()
	if err := h.repo.RevokeCoordinatorGrant(c.Request.Context(), grantID, userID, now); err != nil {
		h.abortWithGrantError(c, "revoke coordinator grant", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": grantID, "revoked": true})
}

// httpListWorkspaceCoordinatorAudit lists audit events for a workspace.
// Supports optional ?task_id= and ?limit= (default 50, capped at 200) query parameters.
func (h *CoordinatorGrantHandlers) httpListWorkspaceCoordinatorAudit(c *gin.Context) {
	if !h.resolveWorkspace(c) {
		return
	}
	workspaceID := c.Param("id")
	taskID := c.Query("task_id")
	limit := parseLimitParam(c.Query("limit"), 50, 200)

	events, err := h.repo.ListCoordinatorAuditEvents(c.Request.Context(), workspaceID, taskID, limit)
	if err != nil {
		h.logger.Error("failed to list coordinator audit events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list coordinator audit events"})
		return
	}

	dtos := make([]auditDTO, 0, len(events))
	for _, e := range events {
		if e != nil {
			dtos = append(dtos, toAuditDTO(e))
		}
	}
	c.JSON(http.StatusOK, coordinatorAuditListResponse{
		Events: dtos,
		Total:  len(dtos),
	})
}

// ---- helpers ----

// parseCapabilities normalizes a comma-separated capabilities string into a
// sorted, deduplicated list of recognized values. Unknown values are silently
// dropped.
func parseCapabilities(raw string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range splitAndTrim(raw) {
		switch s {
		case "inspect":
			if !seen["inspect"] {
				result = append(result, "inspect")
				seen["inspect"] = true
			}
		case "orchestrate":
			if !seen["orchestrate"] {
				result = append(result, "orchestrate")
				seen["orchestrate"] = true
			}
		}
	}
	return result
}

// joinCapabilities joins capabilities into a comma-separated string.
func joinCapabilities(caps []string) string {
	result := ""
	for i, c := range caps {
		if i > 0 {
			result += ","
		}
		result += c
	}
	return result
}

// splitAndTrim splits a comma-separated string and trims whitespace from each part.
func splitAndTrim(s string) []string {
	var result []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			trimmed := trimSpace(current)
			if trimmed != "" {
				result = append(result, trimmed)
			}
			current = ""
		} else {
			current += string(ch)
		}
	}
	trimmed := trimSpace(current)
	if trimmed != "" {
		result = append(result, trimmed)
	}
	return result
}

// trimSpace returns s with leading and trailing whitespace removed.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// parseLimitParam parses a limit query parameter with a default and maximum.
func parseLimitParam(raw string, defaultLimit, maxLimit int) int {
	if raw == "" {
		return defaultLimit
	}
	limit := 0
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			limit = limit*10 + int(ch-'0')
		} else {
			return defaultLimit
		}
	}
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// resolveUserID extracts the authenticated user ID from the gin context.
// Falls back to an empty string for unauthenticated requests.
func resolveUserID(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		if s, ok := userID.(string); ok {
			return s
		}
	}
	return ""
}
