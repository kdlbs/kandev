package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
)

// SavedReviewSource prefers task metadata, with session metadata as a fallback.
func (c reviewTaskContext) SavedReviewSource(ctx context.Context, taskID, sessionID string) (*review.Source, error) {
	if c.tasks == nil {
		return nil, nil
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%w: read task review source: %v", review.ErrWorkspaceUnavailable, err)
	}
	if task != nil && task.Metadata[review.SourceMetadataKey] != nil {
		return c.admittedReviewSource(ctx, taskID, task.Metadata, true)
	}
	sessions, ok := c.tasks.(reviewSessionGetter)
	if !ok || sessionID == "" {
		return nil, nil
	}
	session, err := sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: read session review source: %v", review.ErrWorkspaceUnavailable, err)
	}
	if session != nil && session.TaskID == taskID {
		return c.admittedReviewSource(ctx, taskID, session.Metadata, false)
	}
	return nil, nil
}

type reviewSourceTaskAccess interface {
	review.TaskSourceLookup
	AuthorizeTaskScope(context.Context, string, authz.Scope) error
	AuthorizeReviewSourceOperator(context.Context, string) error
	ReadReviewSourceApproval(context.Context, string) (*models.ReviewSourceApproval, error)
}

func (c reviewTaskContext) admittedReviewSource(
	ctx context.Context, taskID string, metadata map[string]any, taskSource bool,
) (*review.Source, error) {
	source, err := review.SourceFromMetadata(metadata)
	if err != nil || source == nil {
		return nil, err
	}
	tasks, ok := c.tasks.(reviewSourceTaskAccess)
	if !ok || tasks.AuthorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead) != nil {
		return nil, review.ErrWorkspaceUnavailable
	}
	resolved, err := review.ResolveSourcePath(*source)
	if err != nil {
		return nil, err
	}
	admitted, err := review.TaskReviewSource(ctx, tasks, taskID, resolved)
	if err != nil {
		return nil, err
	}
	if admitted != nil {
		return admitted, nil
	}
	if taskSource {
		approval, err := tasks.ReadReviewSourceApproval(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if c.reviewOperatorStillAuthorized(ctx, tasks, taskID, approval) {
			return review.ApprovedSource(ctx, resolved, approval)
		}
	}
	return nil, fmt.Errorf("%w: saved review source is no longer authorized for this task", review.ErrWorkspaceUnavailable)
}

func (c reviewTaskContext) ExplicitReviewSource(ctx context.Context, taskID string, source review.Source) (*review.Source, error) {
	tasks, ok := c.tasks.(reviewSourceTaskAccess)
	if !ok || tasks.AuthorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite) != nil {
		return nil, review.ErrWorkspaceUnavailable
	}
	resolved, err := review.ResolveSourcePath(source)
	if err != nil {
		return nil, err
	}
	admitted, err := review.TaskReviewSource(ctx, tasks, taskID, resolved)
	if err != nil {
		return nil, err
	}
	if admitted != nil {
		return admitted, nil
	}
	identity, ok := authn.IdentityFromContext(ctx)
	_, agent := mcpscope.PrincipalFromContext(ctx)
	if !agent && !mcporigin.IsTrustedExternalTransport(ctx) && ok &&
		!identity.Synthetic && identity.UserID != "" && identity.SessionID != "" &&
		tasks.AuthorizeReviewSourceOperator(ctx, taskID) == nil {
		return review.PinSource(ctx, resolved)
	}
	return nil, review.ErrWorkspaceUnavailable
}

func (c reviewTaskContext) reviewOperatorStillAuthorized(
	ctx context.Context, tasks reviewSourceTaskAccess, taskID string, approval *models.ReviewSourceApproval,
) bool {
	if approval == nil || c.operatorIdentity == nil {
		return false
	}
	identity, ok := c.operatorIdentity(ctx, approval.UserID)
	if !ok || identity.Synthetic || identity.UserID != approval.UserID {
		return false
	}
	operatorCtx := authn.WithIdentity(ctx, identity)
	return tasks.AuthorizeReviewSourceOperator(operatorCtx, taskID) == nil
}
