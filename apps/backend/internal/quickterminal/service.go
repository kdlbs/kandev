package quickterminal

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agent/loginpty"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/quickterminal/models"
	"github.com/kandev/kandev/internal/quickterminal/repository"
	"github.com/kandev/kandev/internal/user/store"
)

var (
	ErrInvalidTabID   = errors.New("quick terminal tab id must be a UUID")
	ErrInvalidStatus  = errors.New("invalid quick terminal status")
	ErrSessionBinding = errors.New("quick terminal session is not owned by this tab")
)

// WorkspaceAuthorizer is the narrow task-service capability needed to keep
// descriptors scoped to a workspace the caller can see.
type WorkspaceAuthorizer interface {
	AuthorizeWorkspaceAccess(context.Context, string) error
}

type Service struct {
	repo       *repository.Repository
	manager    *loginpty.Manager
	authorizer WorkspaceAuthorizer
}

func NewService(
	repo *repository.Repository,
	manager *loginpty.Manager,
	authorizer WorkspaceAuthorizer,
) *Service {
	return &Service{repo: repo, manager: manager, authorizer: authorizer}
}

func (s *Service) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/quick-terminal-tabs", s.httpList)
	api.POST("/quick-terminal-tabs", s.httpCreate)
	api.PATCH("/quick-terminal-tabs/:tabID", s.httpUpdate)
	api.DELETE("/quick-terminal-tabs/:tabID", s.httpDelete)
}

func (s *Service) List(ctx context.Context, workspaceID string) ([]*models.Tab, error) {
	if err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	}
	userID := callerID(ctx)
	tabs, err := s.repo.List(ctx, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, tab := range tabs {
		if err := s.reconcileTab(ctx, tab); err != nil {
			return nil, err
		}
	}
	return tabs, nil
}

func (s *Service) Create(ctx context.Context, workspaceID, tabID string) (*models.Tab, error) {
	if err := validateTabID(tabID); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	}
	tab, err := s.repo.Create(ctx, callerID(ctx), workspaceID, tabID)
	if err != nil {
		return nil, err
	}
	if tab.WorkspaceID != workspaceID {
		return nil, repository.ErrTabIDConflict
	}
	return tab, nil
}

func (s *Service) UpdateLifecycle(
	ctx context.Context,
	tabID, sessionID, status, errorText string,
	exitCode *int,
) (*models.Tab, error) {
	if err := validateTabID(tabID); err != nil {
		return nil, err
	}
	if !validStatus(status) {
		return nil, ErrInvalidStatus
	}
	userID := callerID(ctx)
	tab, err := s.repo.Get(ctx, userID, tabID)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, tab.WorkspaceID); err != nil {
		return nil, err
	}

	sessionID = strings.TrimSpace(sessionID)
	if status == models.StatusExited {
		sessionID = ""
	}
	if status == models.StatusRunning && sessionID == "" {
		return nil, ErrSessionBinding
	}
	if sessionID != "" && !s.ownsSession(tabID, sessionID) {
		return nil, ErrSessionBinding
	}
	if err := s.repo.UpdateLifecycle(ctx, userID, tabID, sessionID, status, exitCode, errorText); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, userID, tabID)
}

func (s *Service) Delete(ctx context.Context, tabID string) error {
	if err := validateTabID(tabID); err != nil {
		return err
	}
	userID := callerID(ctx)
	tab, err := s.repo.Get(ctx, userID, tabID)
	if err != nil {
		return err
	}
	if err := s.authorize(ctx, tab.WorkspaceID); err != nil {
		return err
	}
	if tab.SessionID != nil && s.ownsSession(tabID, *tab.SessionID) {
		if err := s.manager.StopSession(*tab.SessionID); err != nil && !errors.Is(err, loginpty.ErrSessionNotFound) {
			return err
		}
	}
	return s.repo.Delete(ctx, userID, tabID)
}

// HandleSessionExit is registered as loginpty's owner-aware callback. It is
// intentionally context-free because the manager outlives any one request.
func (s *Service) HandleSessionExit(managerKey, sessionID, _ string, exitCode int, exitErr error) {
	const prefix = loginpty.HostShellAgentID + ":"
	if !strings.HasPrefix(managerKey, prefix) {
		return
	}
	tabID := strings.TrimPrefix(managerKey, prefix)
	if validateTabID(tabID) != nil {
		return
	}
	errorText := ""
	if exitErr != nil {
		errorText = exitErr.Error()
	}
	if err := s.repo.UpdateLifecycleByTabID(
		context.Background(), tabID, sessionID, models.StatusExited, &exitCode, errorText,
	); err != nil && !errors.Is(err, repository.ErrNotFound) {
		// The descriptor may have been explicitly deleted while the PTY was
		// unwinding. There is no request to report that failure to.
		return
	}
}

func (s *Service) BindHostShellSession(ctx context.Context, tabID, sessionID string) error {
	_, err := s.UpdateLifecycle(ctx, tabID, sessionID, models.StatusRunning, "", nil)
	return err
}

func (s *Service) reconcileTab(ctx context.Context, tab *models.Tab) error {
	if tab == nil || tab.SessionID == nil {
		if tab != nil && tab.Status == models.StatusConnecting {
			tab.Status = models.StatusExited
			tab.Error = "terminal session unavailable"
			if err := s.repo.UpdateLifecycle(ctx, tab.UserID, tab.TabID, "", tab.Status, nil, tab.Error); err != nil {
				return err
			}
		}
		return nil
	}
	if !s.ownsSession(tab.TabID, *tab.SessionID) {
		return s.markUnavailable(ctx, tab)
	}
	session := s.manager.GetByID(*tab.SessionID)
	if session == nil {
		return s.markUnavailable(ctx, tab)
	}
	status := session.Status()
	if !status.Running {
		return s.markUnavailable(ctx, tab)
	}
	tab.Status = models.StatusRunning
	tab.ExitCode = nil
	tab.Error = ""
	return s.repo.UpdateLifecycle(ctx, tab.UserID, tab.TabID, *tab.SessionID, tab.Status, nil, "")
}

func (s *Service) markUnavailable(ctx context.Context, tab *models.Tab) error {
	tab.SessionID = nil
	tab.Status = models.StatusExited
	tab.ExitCode = nil
	tab.Error = "terminal session unavailable"
	return s.repo.UpdateLifecycle(ctx, tab.UserID, tab.TabID, "", tab.Status, nil, tab.Error)
}

func (s *Service) ownsSession(tabID, sessionID string) bool {
	if s.manager == nil {
		return false
	}
	session := s.manager.GetByID(sessionID)
	return session != nil &&
		session.AgentID == loginpty.HostShellAgentID &&
		session.ManagerKey() == loginpty.HostShellAgentID+":"+tabID
}

func (s *Service) authorize(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return errors.New("workspace id is required")
	}
	if s.authorizer == nil {
		return nil
	}
	return s.authorizer.AuthorizeWorkspaceAccess(ctx, workspaceID)
}

func (s *Service) httpList(c *gin.Context) {
	tabs, err := s.List(c.Request.Context(), c.Query("workspace_id"))
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(200, gin.H{"tabs": tabs})
}

type createRequest struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (s *Service) httpCreate(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}
	tab, err := s.Create(c.Request.Context(), req.WorkspaceID, req.TabID)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(200, tab)
}

type updateRequest struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	ExitCode  *int   `json:"exit_code"`
	Error     string `json:"error"`
}

func (s *Service) httpUpdate(c *gin.Context) {
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}
	tab, err := s.UpdateLifecycle(c.Request.Context(), c.Param("tabID"), req.SessionID, req.Status, req.Error, req.ExitCode)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(200, tab)
}

func (s *Service) httpDelete(c *gin.Context) {
	if err := s.Delete(c.Request.Context(), c.Param("tabID")); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Service) writeError(c *gin.Context, err error) {
	status := 500
	switch {
	case errors.Is(err, repository.ErrNotFound):
		status = 404
	case errors.Is(err, repository.ErrTabIDConflict), errors.Is(err, ErrSessionBinding):
		status = 409
	case errors.Is(err, ErrInvalidTabID), errors.Is(err, ErrInvalidStatus):
		status = 400
	case strings.Contains(err.Error(), "workspace id is required"):
		status = 400
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func callerID(ctx context.Context) string {
	if identity, ok := authn.IdentityFromContext(ctx); ok && identity.UserID != "" {
		return identity.UserID
	}
	return store.DefaultUserID
}

func validateTabID(tabID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(tabID)); err != nil {
		return ErrInvalidTabID
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case models.StatusConnecting, models.StatusRunning, models.StatusExited, models.StatusError:
		return true
	default:
		return false
	}
}
