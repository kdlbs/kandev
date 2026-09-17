package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type MemoryScopeValidator interface {
	ValidateAssistantMemoryEnvironment(context.Context, string, string) error
}

func validMemoryScopeName(scope string) bool {
	switch scope {
	case authorTypeUser, scopeWorkspace, scopeProject, scopeEnvironment, scopeTask:
		return true
	default:
		return false
	}
}

const (
	scopeProject      = "project"
	scopeTask         = "task"
	scopeEnvironment  = "environment"
	memoryResponseKey = "memory"
)

func (s *Service) validateMemoryScope(ctx context.Context, b *models.AssistantBinding, scope, id string) (string, error) {
	if len(id) > 200 || strings.ContainsAny(id, "\r\n") {
		return "", fmt.Errorf("invalid scope")
	}
	switch scope {
	case authorTypeUser:
		return canonicalMemoryScope(id, b.OwnerUserID)
	case scopeWorkspace:
		return canonicalMemoryScope(id, b.WorkspaceID)
	default:
		if id == "" {
			return "", fmt.Errorf("scope required")
		}
		ctx = authn.WithIdentity(ctx, authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
		return id, s.validateResourceMemoryScope(ctx, b.WorkspaceID, scope, id)
	}
}

func canonicalMemoryScope(id, expected string) (string, error) {
	if id != "" && id != expected {
		return "", fmt.Errorf("scope unavailable")
	}
	return expected, nil
}

func (s *Service) validateResourceMemoryScope(ctx context.Context, workspace, scope, id string) error {
	switch scope {
	case scopeTask:
		return s.validateContextScope(ctx, workspace, models.ContextScope{TaskID: id})
	case scopeProject:
		return s.validateContextScope(ctx, workspace, models.ContextScope{ProjectID: id})
	case scopeEnvironment:
		if validator, ok := s.Manager.(MemoryScopeValidator); ok {
			return validator.ValidateAssistantMemoryEnvironment(ctx, workspace, id)
		}
	}
	return fmt.Errorf("scope unavailable")
}

func (s *Service) memoryVisible(ctx context.Context, b *models.AssistantBinding, row *models.AgentMemory) bool {
	if row == nil || row.ForgottenAt != nil || (row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now())) {
		return false
	}
	if row.OwnerUserID != b.OwnerUserID && (row.OwnerUserID != "" || row.Scope != scopeWorkspace) {
		return false
	}
	_, err := s.validateMemoryScope(ctx, b, row.Scope, row.ScopeID)
	return err == nil
}
