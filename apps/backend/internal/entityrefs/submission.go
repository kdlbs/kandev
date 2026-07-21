package entityrefs

import (
	"context"
	"errors"
	"strings"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

var ErrUnauthorizedReference = errors.New("entity reference is not authorized")

// ConversationResolver derives trusted workspace scope from a persisted session/task.
type ConversationResolver interface {
	ResolveWorkspace(ctx context.Context, sessionID, assertedTaskID string) (string, error)
}

// WorkspaceAuthorizer dispatches provider-owned authorization in trusted workspace scope.
type WorkspaceAuthorizer interface {
	AuthorizeForWorkspace(ctx context.Context, workspaceID string, reference apiv1.EntityReference) error
}

// SubmissionValidator is the handler-facing reference validation boundary.
type SubmissionValidator interface {
	ValidateForSubmission(
		ctx context.Context,
		sessionID, assertedTaskID string,
		references []apiv1.EntityReference,
	) ([]apiv1.EntityReference, error)
}

// SubmissionService combines structural validation with conversation and provider authorization.
type SubmissionService struct {
	resolver   ConversationResolver
	authorizer WorkspaceAuthorizer
}

func NewSubmissionService(resolver ConversationResolver, authorizer WorkspaceAuthorizer) *SubmissionService {
	return &SubmissionService{resolver: resolver, authorizer: authorizer}
}

func (s *SubmissionService) ValidateForSubmission(
	ctx context.Context,
	sessionID, assertedTaskID string,
	references []apiv1.EntityReference,
) ([]apiv1.EntityReference, error) {
	normalized, err := NormalizeForSubmission(references)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return normalized, nil
	}
	if s == nil || s.resolver == nil || s.authorizer == nil {
		return nil, ErrUnauthorizedReference
	}
	workspaceID, err := s.resolver.ResolveWorkspace(ctx, sessionID, assertedTaskID)
	if err != nil || strings.TrimSpace(workspaceID) == "" {
		return nil, ErrUnauthorizedReference
	}
	for _, reference := range normalized {
		if err := s.authorizer.AuthorizeForWorkspace(ctx, workspaceID, reference); err != nil {
			return nil, ErrUnauthorizedReference
		}
	}
	return normalized, nil
}
