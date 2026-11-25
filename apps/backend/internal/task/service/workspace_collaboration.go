package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var ErrWorkspaceCollaborationDisabled = errors.New("workspace collaboration is disabled")
var ErrWorkspaceCollaborationUnavailable = errors.New("workspace collaboration enforcement unavailable")
var ErrWorkspaceCollaborationReportInvalid = errors.New("report content is required")

// WorkspaceCollaborationBoundary is supplied by composition from trusted stream
// identity. The task domain never derives an actor from transport payloads.
type WorkspaceCollaborationBoundary interface {
	Enabled() bool
	IsConversation(context.Context) bool
	CallerTaskID(context.Context) string
	Authorize(context.Context, string, models.WorkspaceCollaborationAction) error
}

type WorkspaceCollaborationStore interface {
	GetWorkspaceCollaborationGrant(context.Context, string, string, string) (*models.WorkspaceCollaborationGrant, error)
	ListWorkspaceCollaborationGrants(context.Context, string, string) ([]*models.WorkspaceCollaborationGrant, error)
	UpsertWorkspaceCollaborationGrant(context.Context, *models.WorkspaceCollaborationGrant) error
}

// SetWorkspaceCollaboration is wired before routes and launch consumers start.
func (s *Service) SetWorkspaceCollaboration(boundary WorkspaceCollaborationBoundary, store WorkspaceCollaborationStore) {
	s.workspaceCollaboration, s.workspaceCollaborationStore = boundary, store
}

func (s *Service) collaborationApplies(ctx context.Context) bool {
	return s.workspaceCollaboration != nil && s.workspaceCollaboration.Enabled() && s.workspaceCollaboration.IsConversation(ctx)
}

func (s *Service) needsWorkspaceAuthorization(ctx context.Context) bool {
	_, scoped := callerScope(ctx)
	return scoped || s.collaborationApplies(ctx)
}

func collaborationAction(scope authz.Scope) models.WorkspaceCollaborationAction {
	switch scope {
	case authz.ScopeWorkspaceRead:
		return models.CollaborationRead
	case authz.ScopeSessionPrompt:
		return models.CollaborationPrompt
	default:
		return models.CollaborationControl
	}
}

func (s *Service) authorizeWorkspaceCollaboration(ctx context.Context, workspaceID string, action models.WorkspaceCollaborationAction) error {
	if !s.collaborationApplies(ctx) {
		return nil
	}
	return s.workspaceCollaboration.Authorize(ctx, workspaceID, action)
}

func (s *Service) filterCollaborationWorkspaces(ctx context.Context, workspaces []*models.Workspace) ([]*models.Workspace, error) {
	if !s.collaborationApplies(ctx) {
		return workspaces, nil
	}
	visible := make([]*models.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		err := s.authorizeWorkspaceCollaboration(ctx, workspace.ID, models.CollaborationRead)
		if errors.Is(err, ErrForbidden) {
			continue
		}
		if err != nil {
			return nil, err
		}
		visible = append(visible, workspace)
	}
	return visible, nil
}

func (s *Service) collaborationManager(ctx context.Context, taskID string) (*models.Task, error) {
	if s.workspaceCollaboration == nil || !s.workspaceCollaboration.Enabled() {
		return nil, ErrWorkspaceCollaborationDisabled
	}
	if _, ok := authn.IdentityFromContext(ctx); !ok || s.workspaceCollaboration.IsConversation(ctx) {
		return nil, ErrForbidden
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil || task.WorkspaceID == "" {
		return nil, repoerrors.ErrTaskNotFound
	}
	if _, err := s.requireWorkspaceScope(ctx, task.WorkspaceID, authz.ScopeWorkspaceManage, repoerrors.ErrTaskNotFound); err != nil {
		return nil, err
	}
	if s.workspaceCollaborationStore == nil {
		return nil, ErrWorkspaceCollaborationUnavailable
	}
	return task, nil
}

func (s *Service) ListWorkspaceCollaborationGrants(ctx context.Context, taskID string) ([]*models.WorkspaceCollaborationGrant, error) {
	task, err := s.collaborationManager(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.workspaceCollaborationStore.ListWorkspaceCollaborationGrants(ctx, taskID, task.WorkspaceID)
}

func (s *Service) PutWorkspaceCollaborationGrant(ctx context.Context, taskID, targetID string, flags models.WorkspaceCollaborationGrant) (*models.WorkspaceCollaborationGrant, error) {
	task, err := s.collaborationManager(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if targetID == "" {
		return nil, ErrWorkspaceCollaborationUnavailable
	}
	if err := s.authorizeWorkspaceID(ctx, targetID); err != nil {
		return nil, err
	}
	target, err := s.workspaces.GetWorkspace(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, repoerrors.ErrWorkspaceNotFound
	}
	identity, _ := authn.IdentityFromContext(ctx)
	grant := &models.WorkspaceCollaborationGrant{
		CallerTaskID: task.ID, SourceWorkspaceID: task.WorkspaceID, TargetWorkspaceID: targetID,
		Read: flags.Read, Report: flags.Report, Prompt: flags.Prompt, Control: flags.Control,
		UpdatedBy: identity.UserID, UpdatedAt: time.Now().UTC(),
	}
	if err := s.workspaceCollaborationStore.UpsertWorkspaceCollaborationGrant(ctx, grant); err != nil {
		return nil, err
	}
	return grant, nil
}

type collaborationReportContextKey struct{}

// CreateWorkspaceCollaborationReport only appends a completed status message.
// It never enters prompt queueing or the orchestrator's launch path.
func (s *Service) CreateWorkspaceCollaborationReport(ctx context.Context, taskID, sessionID, content string) (*models.Message, error) {
	if s.workspaceCollaboration == nil || !s.workspaceCollaboration.Enabled() {
		return nil, ErrWorkspaceCollaborationDisabled
	}
	if !s.workspaceCollaboration.IsConversation(ctx) {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrWorkspaceCollaborationReportInvalid
	}
	if err := s.authorizeTaskScopeAction(ctx, taskID, authz.ScopeTaskWrite, models.CollaborationReport); err != nil {
		return nil, err
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.TaskID != taskID {
		return nil, repoerrors.ErrTaskNotFound
	}
	ctx = context.WithValue(ctx, collaborationReportContextKey{}, true)
	return s.CreateMessage(ctx, &CreateMessageRequest{
		TaskID: taskID, TaskSessionID: sessionID, Content: content,
		Type: string(models.MessageTypeStatus), AuthorType: createdByAgent,
		AuthorID: s.workspaceCollaboration.CallerTaskID(ctx), CompletedTurn: true,
	})
}
